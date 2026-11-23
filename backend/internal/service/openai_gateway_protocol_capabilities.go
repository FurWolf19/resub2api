package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAIProtocolFallbackTriedKey = "openai_auto_protocol_fallback_tried"

// openAIProtocolRequiresResponses 汇总优先于“跟随入站”的硬约束：
// 生图意图、/responses/compact、remote compaction v2，以及 gpt-6.1-sol/terra
// 带工具调用（这些模型只有 Responses 能承载工具调用）。
func openAIProtocolRequiresResponses(c *gin.Context, account *Account, requestedModel string, body []byte, compactPath bool) bool {
	if compactPath || isOpenAINativeCompactionV2(c) {
		return true
	}
	if IsImageGenerationIntent(openAIResponsesEndpoint, requestedModel, body) {
		return true
	}
	hasTools := len(gjson.GetBytes(body, "tools").Array()) > 0 || len(gjson.GetBytes(body, "functions").Array()) > 0
	if !hasTools {
		return false
	}
	upstreamModel := normalizeOpenAIModelForUpstream(account, resolveOpenAIForwardModel(account, requestedModel, ""))
	if openai.RequiresResponsesForToolCalls(upstreamModel) {
		return true
	}
	// gpt-6-sol / gpt-6-luna 带 reasoning 的工具调用同样只有 Responses 能承载，
	// 与 raw CC 路径的原有限制保持一致。
	if openai.IsGPT6SolOrLunaModelSpelling(upstreamModel) {
		effort := strings.TrimSpace(gjson.GetBytes(body, "reasoning_effort").String())
		if effort == "" {
			effort = strings.TrimSpace(gjson.GetBytes(body, "reasoning.effort").String())
		}
		return effort != "none"
	}
	return false
}

// openAIResponsesAutoMode 报告 OpenAI APIKey 账号是否处于 auto 协议模式。
// 强制模式（force_responses / force_chat_completions）不走新的跟随入站逻辑。
func openAIResponsesAutoMode(account *Account) bool {
	if account == nil || !account.IsOpenAIApiKey() || account.routesByModel() {
		return false
	}
	mode, _ := account.Extra[openai_compat.ExtraKeyResponsesMode].(string)
	return openai_compat.NormalizeResponsesSupportMode(mode) == openai_compat.ResponsesSupportModeAuto
}

func (s *OpenAIGatewayService) openAIProtocolProbeDeps() openAIProtocolProbeDeps {
	return openAIProtocolProbeDeps{
		accountRepo:     s.accountRepo,
		httpUpstream:    s.httpUpstream,
		cfg:             s.cfg,
		validateBaseURL: s.validateUpstreamBaseURL,
	}
}

// ensureOpenAIProtocolCapabilities 在真实请求前确保 auto 账号的能力已加载：
//   - 从未探测过：阻塞探测一次（约 2 秒预算），随后转发；
//   - 已过期/部分未知：先用旧值转发，同时后台异步重探。
//
// 返回的账号可能是带最新 Extra 的副本，避免修改调度快照共享的 map。
func (s *OpenAIGatewayService) ensureOpenAIProtocolCapabilities(ctx context.Context, account *Account) *Account {
	if !s.openAIProtocolCapabilityProbeEnabled {
		return account
	}
	if !openAIResponsesAutoMode(account) || s.accountRepo == nil || s.httpUpstream == nil {
		return account
	}
	if ctx == nil {
		ctx = context.Background()
	}
	caps := openai_compat.ResolveProtocolCapabilities(account.Extra)
	if !caps.HasKnown() {
		probeCtx, cancel := context.WithTimeout(ctx, openAIProtocolBlockingProbeTimeout)
		result := probeOpenAIProtocolCapabilities(probeCtx, s.openAIProtocolProbeDeps(), account, openAIProtocolBlockingProbeTimeout)
		cancel()
		account = applyOpenAIProtocolProbeResultToAccount(account, result)
		caps = openai_compat.ResolveProtocolCapabilities(account.Extra)
	}
	if !caps.HasKnown() || !caps.NeedsRefresh(time.Now()) {
		return account
	}
	s.startOpenAIProtocolCapabilityReprobe(account.ID)
	return account
}

func (s *OpenAIGatewayService) startOpenAIProtocolCapabilityReprobe(accountID int64) {
	if s.accountRepo == nil || s.httpUpstream == nil {
		return
	}
	now := time.Now()
	if openAIProtocolProbeInBackoff(accountID, now) {
		return
	}
	if last, ok := openAIProtocolProbeLastAttempt.Load(accountID); ok {
		if attemptedAt, ok := last.(time.Time); ok && now.Sub(attemptedAt) < openAIProtocolReprobeCooldown {
			return
		}
	}
	if _, loaded := openAIProtocolProbeInFlight.LoadOrStore(accountID, struct{}{}); loaded {
		return
	}
	openAIProtocolProbeLastAttempt.Store(accountID, now)
	go func() {
		defer openAIProtocolProbeInFlight.Delete(accountID)
		ctx, cancel := context.WithTimeout(context.Background(), openAIProtocolAsyncProbeTimeout)
		defer cancel()
		account, err := s.accountRepo.GetByID(ctx, accountID)
		if err != nil || account == nil {
			return
		}
		probeOpenAIProtocolCapabilities(ctx, s.openAIProtocolProbeDeps(), account, openAIProtocolAsyncProbeTimeout)
	}()
}

type openAIProtocolUnsupportedError struct {
	Protocol   string
	StatusCode int
}

func (e *openAIProtocolUnsupportedError) Error() string {
	return "upstream does not support " + e.Protocol
}

func asOpenAIProtocolUnsupportedError(err error) (*openAIProtocolUnsupportedError, bool) {
	if err == nil {
		return nil, false
	}
	var unsupported *openAIProtocolUnsupportedError
	if errors.As(err, &unsupported) {
		return unsupported, true
	}
	return nil, false
}

func openAIProtocolFallbackTried(c *gin.Context) bool {
	return c != nil && c.GetBool(openAIProtocolFallbackTriedKey)
}

func markOpenAIProtocolFallbackTried(c *gin.Context) {
	if c != nil {
		c.Set(openAIProtocolFallbackTriedKey, true)
	}
}

// shouldUseOpenAIProtocolFallback 判断 auto 模式下本次端点不支持错误是否应触发
// 另一种协议重试。404/405 才算端点不存在；模型不存在或其它状态码不重试。
func (s *OpenAIGatewayService) shouldUseOpenAIProtocolFallback(c *gin.Context, account *Account, attempted string, status int, body []byte) bool {
	if !openAIResponsesAutoMode(account) || openAIProtocolFallbackTried(c) {
		return false
	}
	if status != 404 && status != 405 {
		return false
	}
	if isResponsesProbeModelUnavailable(status, body) {
		return false
	}
	return true
}

// recordOpenAIProtocolCapability 更新内存中的能力，并异步持久化到 accounts.extra。
func (s *OpenAIGatewayService) recordOpenAIProtocolCapability(ctx context.Context, account *Account, protocol string, supported bool) *Account {
	if account == nil {
		return account
	}
	caps := openai_compat.ResolveProtocolCapabilities(account.Extra)
	entry := openai_compat.ProtocolCapability{Known: true, Supported: supported, ProbedAt: time.Now()}
	switch protocol {
	case openai_compat.ProtocolResponses:
		caps.Responses = entry
	case openai_compat.ProtocolChatCompletions:
		caps.ChatCompletions = entry
	default:
		return account
	}
	updated := accountWithOpenAIProtocolCapabilities(account, caps)
	if s.accountRepo == nil {
		return updated
	}
	updates := map[string]any{
		openai_compat.ExtraKeyProtocolCapabilities: caps.ToExtra(),
	}
	if protocol == openai_compat.ProtocolResponses {
		updates[openai_compat.ExtraKeyResponsesSupported] = supported
	}
	if ctx == nil {
		ctx = context.Background()
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	go func() {
		defer cancel()
		_ = s.accountRepo.UpdateExtra(persistCtx, account.ID, updates)
	}()
	return updated
}
