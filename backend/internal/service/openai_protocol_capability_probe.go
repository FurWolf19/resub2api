package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/tidwall/gjson"
)

const (
	// openAIProtocolBlockingProbeTimeout 是首次真实请求前阻塞探测的总预算。
	openAIProtocolBlockingProbeTimeout = 2 * time.Second
	// openAIProtocolAsyncProbeTimeout 是后台异步重探的总预算。
	openAIProtocolAsyncProbeTimeout = 15 * time.Second
	// openAIProtocolProbeFailureBackoff 是网络层探测失败后的短退避，避免打爆上游。
	openAIProtocolProbeFailureBackoff = 30 * time.Second
	// openAIProtocolProbeInconclusiveBackoff 是探测有 HTTP 响应但无法下结论
	// （401、模型不可用等）时的负缓存，期间直接跟随入站协议，不再阻塞探测。
	openAIProtocolProbeInconclusiveBackoff = 45 * time.Second
	// openAIProtocolReprobeCooldown 避免过期账号在每个请求上都重复触发后台重探。
	openAIProtocolReprobeCooldown = time.Minute
	// openAIProtocolProbeLastAttemptTTL 用于清理长时间未活跃账号的探测状态。
	openAIProtocolProbeLastAttemptTTL = 24 * time.Hour

	openAIProtocolProbeChatMaxTokens = 8
)

type openAIProtocolProbeDeps struct {
	accountRepo         AccountRepository
	httpUpstream        HTTPUpstream
	cfg                 *config.Config
	tlsFPProfileService *TLSFingerprintProfileService
	validateBaseURL     func(string) (string, error)
}

type openAIProtocolProbeOutcome struct {
	Conclusive bool
	Supported  bool
}

type openAIProtocolProbeResult struct {
	Responses openAIProtocolProbeOutcome
	Chat      openAIProtocolProbeOutcome
	// Responded 表示至少完成了一次 HTTP 往返，可用于判断是否需要写入缓存。
	Responded bool
	// TransportFailed 表示探测请求在网络层失败，需要短退避。
	TransportFailed bool
}

func (r openAIProtocolProbeResult) anyConclusive() bool {
	return r.Responses.Conclusive || r.Chat.Conclusive
}

var (
	openAIProtocolProbeGroup       singleflight.Group
	openAIProtocolProbeBackoffMu   sync.Mutex
	openAIProtocolProbeBackoffTill = map[int64]time.Time{}
	openAIProtocolProbeInFlight    sync.Map // accountID -> struct{}
	openAIProtocolProbeLastAttempt sync.Map // accountID -> time.Time
	openAIProtocolLegacyStampAt    sync.Map // accountID -> time.Time
	openAIProtocolProbeLastPrune   atomic.Int64
)

// openAIProtocolLegacyStampCooldown 避免同一账号的 legacy probed_at 迁移在
// 首次异步写回完成前被每个并发请求重复触发。
const openAIProtocolLegacyStampCooldown = time.Minute

func shouldPersistOpenAIProtocolLegacyStamp(accountID int64, now time.Time) bool {
	if last, ok := openAIProtocolLegacyStampAt.Load(accountID); ok {
		if attemptedAt, ok := last.(time.Time); ok && now.Sub(attemptedAt) < openAIProtocolLegacyStampCooldown {
			return false
		}
	}
	openAIProtocolLegacyStampAt.Store(accountID, now)
	return true
}

func openAIProtocolProbeInBackoff(accountID int64, now time.Time) bool {
	openAIProtocolProbeBackoffMu.Lock()
	defer openAIProtocolProbeBackoffMu.Unlock()
	until, ok := openAIProtocolProbeBackoffTill[accountID]
	if !ok {
		return false
	}
	if now.Before(until) {
		return true
	}
	delete(openAIProtocolProbeBackoffTill, accountID)
	return false
}

func setOpenAIProtocolProbeBackoff(accountID int64, until time.Time) {
	openAIProtocolProbeBackoffMu.Lock()
	defer openAIProtocolProbeBackoffMu.Unlock()
	if existing, ok := openAIProtocolProbeBackoffTill[accountID]; ok && existing.After(until) {
		return
	}
	openAIProtocolProbeBackoffTill[accountID] = until
}

// clearOpenAIProtocolProbeBackoff 供显式“测试账号”探测绕过负缓存：
// 管理员主动触发的探测应当真实请求上游，而不是被后台自动探测的 45s 退避吞掉。
func clearOpenAIProtocolProbeBackoff(accountID int64) {
	openAIProtocolProbeBackoffMu.Lock()
	defer openAIProtocolProbeBackoffMu.Unlock()
	delete(openAIProtocolProbeBackoffTill, accountID)
}

func markOpenAIProtocolProbeFailure(accountID int64, now time.Time) {
	setOpenAIProtocolProbeBackoff(accountID, now.Add(openAIProtocolProbeFailureBackoff))
}

// markOpenAIProtocolProbeInconclusive 缓存“有响应但无法下结论”的探测，
// 只影响无结论结果；明确支持/不支持不会走这里。
func markOpenAIProtocolProbeInconclusive(accountID int64, now time.Time) {
	setOpenAIProtocolProbeBackoff(accountID, now.Add(openAIProtocolProbeInconclusiveBackoff))
}

// pruneOpenAIProtocolProbeState 低频清理探测状态，避免已删除/长期不活跃账号的
// sync.Map 条目无限增长。每 10 分钟最多执行一次，代价可忽略。
func pruneOpenAIProtocolProbeState(now time.Time) {
	last := openAIProtocolProbeLastPrune.Load()
	if now.Unix()-last < int64((10 * time.Minute).Seconds()) {
		return
	}
	if !openAIProtocolProbeLastPrune.CompareAndSwap(last, now.Unix()) {
		return
	}
	openAIProtocolProbeLastAttempt.Range(func(key, value any) bool {
		if attemptedAt, ok := value.(time.Time); ok && now.Sub(attemptedAt) > openAIProtocolProbeLastAttemptTTL {
			openAIProtocolProbeLastAttempt.Delete(key)
		}
		return true
	})
	openAIProtocolLegacyStampAt.Range(func(key, value any) bool {
		if attemptedAt, ok := value.(time.Time); ok && now.Sub(attemptedAt) > openAIProtocolProbeLastAttemptTTL {
			openAIProtocolLegacyStampAt.Delete(key)
		}
		return true
	})
	openAIProtocolProbeBackoffMu.Lock()
	for accountID, until := range openAIProtocolProbeBackoffTill {
		if !now.Before(until) {
			delete(openAIProtocolProbeBackoffTill, accountID)
		}
	}
	openAIProtocolProbeBackoffMu.Unlock()
}

// openAIChatCompletionsProbePayload 构造最小 Chat Completions 探测请求。
func openAIChatCompletionsProbePayload(modelID string) []byte {
	if modelID == "" {
		modelID = openai.DefaultTestModel
	}
	body, _ := json.Marshal(map[string]any{
		"model": modelID,
		"messages": []map[string]any{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": openAIProtocolProbeChatMaxTokens,
		"stream":     false,
	})
	return body
}

// decideChatCompletionsProbeSupport 判定 Chat Completions 端点探测结果。
// 2xx=true；可识别的 JSON 404/405=false；其它 404/405（HTML/空体/纯文本/未知 JSON）
// 保持 unknown；401/网络/模型不存在=unknown；其余非 2xx 保守 true。
func decideChatCompletionsProbeSupport(status int, body []byte) (supported bool, conclusive bool) {
	if status == http.StatusUnauthorized {
		return false, false
	}
	if isResponsesProbeModelUnavailable(status, body) {
		return false, false
	}
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
		if openAIEndpointNotFoundJSON(status, body) {
			return false, true
		}
		return false, false
	}
	if status < 200 || status >= 300 {
		return true, true
	}
	return true, true
}

// probeOpenAIProtocolCapabilities 执行双协议探测（Chat Completions 先、Responses 后），
// 并将结论写回 accounts.extra。按 account ID singleflight 合并并发探测。
func probeOpenAIProtocolCapabilities(
	ctx context.Context,
	deps openAIProtocolProbeDeps,
	account *Account,
	timeout time.Duration,
) openAIProtocolProbeResult {
	if account == nil || deps.httpUpstream == nil || account.Type != AccountTypeAPIKey || account.Platform != PlatformOpenAI {
		return openAIProtocolProbeResult{}
	}
	accountID := account.ID
	now := time.Now()
	if openAIProtocolProbeInBackoff(accountID, now) {
		return openAIProtocolProbeResult{}
	}

	key := strconv.FormatInt(accountID, 10)
	ch := openAIProtocolProbeGroup.DoChan(key, func() (result any, err error) {
		// 探测是旁路优化，任何实现细节上的 panic 都不能影响真实请求。
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.LegacyPrintf("service.openai_probe", "protocol_probe_panic_recovered: account_id=%d panic=%v", accountID, recovered)
				result = openAIProtocolProbeResult{}
				err = nil
			}
		}()
		if openAIProtocolProbeInBackoff(accountID, time.Now()) {
			return openAIProtocolProbeResult{}, nil
		}
		probeCtx := ctx
		cancel := func() {}
		if timeout > 0 {
			probeCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		defer cancel()
		probeResult := executeOpenAIProtocolProbe(probeCtx, deps, account)
		switch {
		case probeResult.TransportFailed:
			markOpenAIProtocolProbeFailure(accountID, time.Now())
		case probeResult.Responded && !(probeResult.Responses.Conclusive && probeResult.Chat.Conclusive):
			// 401/模型不可用等“有响应但无结论”同样需要负缓存，否则首次阻塞
			// 探测会在每个请求上重复执行。只有一个协议有结论、另一个无结论时
			// NeedsRefresh 会恒真，后台会每隔约一分钟重探；这里同样退避，避免
			// 有结论的账号被另一个协议的不确定性拖入持续探测。
			markOpenAIProtocolProbeInconclusive(accountID, time.Now())
		}
		if probeResult.anyConclusive() && deps.accountRepo != nil {
			persistOpenAIProtocolProbeResult(probeCtx, deps, account, probeResult)
		}
		return probeResult, nil
	})

	select {
	case item := <-ch:
		if item.Val == nil {
			return openAIProtocolProbeResult{}
		}
		result, _ := item.Val.(openAIProtocolProbeResult)
		return result
	case <-ctx.Done():
		return openAIProtocolProbeResult{}
	}
}

func executeOpenAIProtocolProbe(ctx context.Context, deps openAIProtocolProbeDeps, account *Account) openAIProtocolProbeResult {
	var result openAIProtocolProbeResult
	apiKey := account.GetOpenAIApiKey()
	if apiKey == "" {
		return result
	}
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	normalizedBaseURL, err := deps.validateBaseURL(baseURL)
	if err != nil {
		logger.LegacyPrintf("service.openai_probe", "protocol_probe_invalid_baseurl: account_id=%d base_url=%q err=%v", account.ID, baseURL, err)
		return result
	}
	probeModel := selectResponsesProbeModel(account)

	// Chat Completions 探测：最小 messages + 很小 max_tokens + stream=false。
	result.Chat, result.Responded, result.TransportFailed = probeOpenAIProtocolEndpoint(
		ctx, deps, account, apiKey,
		buildOpenAIChatCompletionsURL(normalizedBaseURL),
		openAIChatCompletionsProbePayload(probeModel),
		decideChatCompletionsProbeSupport,
	)
	if ctx.Err() != nil {
		return result
	}

	// Responses 探测：复用原有 payload 与判定，保留工具调用能力语义。
	responses, responded, transportFailed := probeOpenAIProtocolEndpoint(
		ctx, deps, account, apiKey,
		buildOpenAIResponsesURL(normalizedBaseURL),
		openaiResponsesProbePayload(probeModel),
		func(status int, body []byte) (bool, bool) {
			if !responsesProbeVerdictIsConclusive(status, body) {
				return false, false
			}
			return decideResponsesProbeSupport(status, body), true
		},
	)
	result.Responses = responses
	result.Responded = result.Responded || responded
	result.TransportFailed = result.TransportFailed || transportFailed
	return result
}

func probeOpenAIProtocolEndpoint(
	ctx context.Context,
	deps openAIProtocolProbeDeps,
	account *Account,
	apiKey, probeURL string,
	payload []byte,
	decide func(int, []byte) (supported bool, conclusive bool),
) (outcome openAIProtocolProbeOutcome, responded bool, transportFailed bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, probeURL, bytes.NewReader(payload))
	if err != nil {
		return outcome, false, false
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	applyOpenAICodexProbeHeaders(req.Header)
	account.ApplyHeaderOverrides(ctx, req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	var resp *http.Response
	if deps.tlsFPProfileService != nil {
		resp, err = deps.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, deps.tlsFPProfileService.ResolveTLSProfile(account))
	} else {
		resp, err = deps.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	}
	if err != nil {
		logger.LegacyPrintf("service.openai_probe", "protocol_probe_request_failed: account_id=%d url=%s err=%v", account.ID, probeURL, err)
		return outcome, false, true
	}
	defer func() { _ = resp.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, responsesProbeMaxBodyBytes))
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, responsesProbeMaxBodyBytes))
	if readErr != nil {
		logger.LegacyPrintf("service.openai_probe", "protocol_probe_read_body_failed: account_id=%d url=%s err=%v", account.ID, probeURL, readErr)
		return outcome, false, true
	}
	supported, conclusive := decide(resp.StatusCode, body)
	outcome = openAIProtocolProbeOutcome{Conclusive: conclusive, Supported: supported}
	logger.LegacyPrintf("service.openai_probe",
		"protocol_probe_endpoint: account_id=%d url=%s model=%s status=%d supported=%v conclusive=%v response_status=%s",
		account.ID, probeURL, gjson.GetBytes(payload, "model").String(), resp.StatusCode, supported, conclusive,
		gjson.GetBytes(body, "status").String(),
	)
	return outcome, true, false
}

func persistOpenAIProtocolProbeResult(ctx context.Context, deps openAIProtocolProbeDeps, account *Account, result openAIProtocolProbeResult) {
	if deps.accountRepo == nil || account == nil {
		return
	}
	now := time.Now()
	prior := openai_compat.ResolveProtocolCapabilities(account.Extra)
	patches := make([]OpenAIProtocolCapabilityPatch, 0, 2)
	if result.Chat.Conclusive {
		supported := result.Chat.Supported
		patches = append(patches, OpenAIProtocolCapabilityPatch{
			Protocol: openai_compat.ProtocolChatCompletions, Supported: &supported, ProbedAt: now,
		})
	} else if prior.ChatCompletions.Known {
		// 无法下结论时保留旧值，只刷新探测时间，避免该协议在 TTL 内被
		// 每个请求反复重探（旧值语义不变）。
		patches = append(patches, OpenAIProtocolCapabilityPatch{
			Protocol: openai_compat.ProtocolChatCompletions, ProbedAt: now,
		})
	}
	if result.Responses.Conclusive {
		supported := result.Responses.Supported
		patches = append(patches, OpenAIProtocolCapabilityPatch{
			Protocol: openai_compat.ProtocolResponses, Supported: &supported, ProbedAt: now,
		})
	} else if prior.Responses.Known {
		patches = append(patches, OpenAIProtocolCapabilityPatch{
			Protocol: openai_compat.ProtocolResponses, ProbedAt: now,
		})
	}
	if len(patches) == 0 {
		return
	}
	repo, ok := deps.accountRepo.(OpenAIProtocolCapabilityRepository)
	if !ok {
		logger.LegacyPrintf("service.openai_probe", "protocol_probe_persist_unsupported: account_id=%d", account.ID)
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := repo.MergeOpenAIProtocolCapabilities(ctx, account.ID, patches); err != nil {
		logger.LegacyPrintf("service.openai_probe", "protocol_probe_persist_failed: account_id=%d err=%v", account.ID, err)
	}
}

// applyOpenAIProtocolProbeResultToAccount 返回带最新能力快照的账号副本，避免并发请求
// 共享同一个 Extra map。
func applyOpenAIProtocolProbeResultToAccount(account *Account, result openAIProtocolProbeResult) *Account {
	if account == nil || !result.anyConclusive() {
		return account
	}
	caps := openai_compat.ResolveProtocolCapabilities(account.Extra)
	now := time.Now()
	if result.Chat.Conclusive {
		caps.ChatCompletions = openai_compat.ProtocolCapability{Known: true, Supported: result.Chat.Supported, ProbedAt: now}
	}
	if result.Responses.Conclusive {
		caps.Responses = openai_compat.ProtocolCapability{Known: true, Supported: result.Responses.Supported, ProbedAt: now}
	}
	return accountWithOpenAIProtocolCapabilities(account, caps)
}

func accountWithOpenAIProtocolCapabilities(account *Account, caps openai_compat.ProtocolCapabilities) *Account {
	if account == nil {
		return nil
	}
	clone := *account
	clone.Extra = make(map[string]any, len(account.Extra)+2)
	for key, value := range account.Extra {
		clone.Extra[key] = value
	}
	clone.Extra[openai_compat.ExtraKeyProtocolCapabilities] = caps.ToExtra()
	if caps.Responses.Known {
		clone.Extra[openai_compat.ExtraKeyResponsesSupported] = caps.Responses.Supported
	}
	return &clone
}
