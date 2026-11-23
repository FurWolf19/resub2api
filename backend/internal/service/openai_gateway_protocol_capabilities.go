package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAIProtocolFallbackTriedKey = "openai_auto_protocol_fallback_tried"

// openAIProtocolPersistQueue 串行化同一账号 capabilities 的异步按协议写回。
//
// 请求路径只把本次变化的协议 patch 投递到队列，不等待 DB 写；每个账号最多一个
// 后台 writer goroutine，且只在有待写任务时运行。多实例正确性由仓储方法的行锁
// 合并保证，队列只负责单实例内的合并与背压。
type openAIProtocolPersistQueue struct {
	mu sync.Mutex
	// pending 是最新的待写 patch 集合。相邻记录会合并，后写覆盖先写，但旧快照不会
	// 覆盖新快照（按 probed_at 单调校验）。
	pending  *openAIProtocolPersistJob
	running  bool
	lastSeen time.Time
}

type openAIProtocolPersistJob struct {
	parent   context.Context
	patches  []OpenAIProtocolCapabilityPatch
	probedAt time.Time
}

var (
	openAIProtocolPersistQueuesMu sync.Mutex
	openAIProtocolPersistQueues   = make(map[int64]*openAIProtocolPersistQueue)
)

// enqueueOpenAIProtocolPersistJob 在全局锁内完成“取/建队列 + 入队”，与
// writer 排空后的删除互斥。否则删除后仍可能向已脱离 map 的旧队列投递任务，
// 造成同一账号出现两个 writer 或丢失合并语义。
func enqueueOpenAIProtocolPersistJob(accountID int64, job openAIProtocolPersistJob) (*openAIProtocolPersistQueue, bool) {
	openAIProtocolPersistQueuesMu.Lock()
	defer openAIProtocolPersistQueuesMu.Unlock()
	queue, ok := openAIProtocolPersistQueues[accountID]
	if !ok {
		queue = &openAIProtocolPersistQueue{}
		openAIProtocolPersistQueues[accountID] = queue
	}
	return queue, queue.enqueue(job)
}

// openAIProtocolPersistQueueRelease 在 writer 排空退出时清理账号条目，避免
// 已删除账号的队列常驻内存。清理前在全局锁内复查队列空闲状态：入队路径在
// 同一把锁内完成入队，因此有任务待写或 writer 仍存活时不会误删。
func openAIProtocolPersistQueueRelease(accountID int64, queue *openAIProtocolPersistQueue) {
	openAIProtocolPersistQueuesMu.Lock()
	defer openAIProtocolPersistQueuesMu.Unlock()
	queue.mu.Lock()
	idle := queue.pending == nil && !queue.running
	queue.mu.Unlock()
	if !idle {
		return
	}
	if openAIProtocolPersistQueues[accountID] == queue {
		delete(openAIProtocolPersistQueues, accountID)
	}
}

// enqueue 返回是否需要启动 writer。按协议合并新旧 patch：不同协议都保留；
// 同一协议分别保留“结论 patch”和“touch patch”，flush 时先结论后 touch。
// touch 只刷新 probed_at，绝不能用 touch 时间提升旧结论，否则会覆盖其它实例
// 在结论之后写入的更新值。旧时间戳的 patch 不能整包丢弃，否则会连带丢掉它
// 携带的另一种协议结论；最终新旧判定交给仓储按协议 probed_at 合并。
func (q *openAIProtocolPersistQueue) enqueue(job openAIProtocolPersistJob) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !job.probedAt.IsZero() && job.probedAt.After(q.lastSeen) {
		q.lastSeen = job.probedAt
	}
	if q.pending != nil && len(q.pending.patches) > 0 {
		job.patches = mergeQueuedProtocolPatches(q.pending.patches, job.patches)
	}
	q.pending = &job
	if q.running {
		return false
	}
	q.running = true
	return true
}

func mergeQueuedProtocolPatches(existing, incoming []OpenAIProtocolCapabilityPatch) []OpenAIProtocolCapabilityPatch {
	type protocolPatchSet struct {
		conclusive *OpenAIProtocolCapabilityPatch
		touch      *OpenAIProtocolCapabilityPatch
	}
	sets := make(map[string]*protocolPatchSet, len(existing)+len(incoming))
	order := make([]string, 0, len(existing)+len(incoming))
	for _, patch := range append(append([]OpenAIProtocolCapabilityPatch{}, existing...), incoming...) {
		set, seen := sets[patch.Protocol]
		if !seen {
			set = &protocolPatchSet{}
			sets[patch.Protocol] = set
			order = append(order, patch.Protocol)
		}
		if patch.Supported != nil {
			// 结论保留自己的 probed_at；不能用 touch 时间把它“变新”，否则会
			// 覆盖其它实例在结论之后写入的更新结论。
			if set.conclusive == nil || !patch.ProbedAt.Before(set.conclusive.ProbedAt) {
				copied := patch
				set.conclusive = &copied
			}
			continue
		}
		if set.touch == nil || !patch.ProbedAt.Before(set.touch.ProbedAt) {
			copied := patch
			set.touch = &copied
		}
	}
	merged := make([]OpenAIProtocolCapabilityPatch, 0, len(order)*2)
	for _, protocol := range order {
		set := sets[protocol]
		// 先应用结论，再应用 touch：touch 只刷新 probed_at，不能抹掉结论，
		// 也不会把旧结论的时间提升到 touch 的时间。
		if set.conclusive != nil {
			merged = append(merged, *set.conclusive)
		}
		if set.touch != nil {
			merged = append(merged, *set.touch)
		}
	}
	return merged
}

func (q *openAIProtocolPersistQueue) take() *openAIProtocolPersistJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	job := q.pending
	q.pending = nil
	if job == nil {
		q.running = false
	}
	return job
}

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

// openAIProtocolProbeEligible 报告账号是否应执行双协议能力探测。
//
// 只有“OpenAI APIKey + auto”且不按模型分流、不走 adaptive、不按入站协议
// 分流（国产多协议供应商）的账号才需要探测。其余账号的协议选择有显式权威
// 来源，探测结论既不会被使用，还会让 NeedsRefresh 恒真并造成重复上游调用。
func openAIProtocolProbeEligible(account *Account) bool {
	if !openAIResponsesAutoMode(account) {
		return false
	}
	if account.routesByModel() || account.IsAdaptiveAPIProtocol() || account.RoutesProtocolByInbound() || account.IsCNProvider() {
		return false
	}
	return true
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
	if !openAIProtocolProbeEligible(account) || s.accountRepo == nil || s.httpUpstream == nil {
		return account
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	before := openai_compat.ResolveProtocolCapabilities(account.Extra)
	caps := before
	// 旧键只有 openai_responses_supported、没有 probed_at。把旧值视为刚探测过
	// 并落一次时间戳，避免 NeedsRefresh 恒真、每分钟对活跃账号重复探测。
	if stamped, changed := caps.StampMissingProbedAt(now); changed {
		account = accountWithOpenAIProtocolCapabilities(account, stamped)
		caps = stamped
		if shouldPersistOpenAIProtocolLegacyStamp(account.ID, now) {
			s.stampOpenAIProtocolCapabilities(ctx, account.ID, before, stamped)
		}
	}
	if !caps.HasKnown() {
		// 上一次探测无结论（401/模型不可用等）时直接跟随入站协议，不再阻塞。
		if openAIProtocolProbeInBackoff(account.ID, now) {
			return account
		}
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

// stampOpenAIProtocolCapabilities 只提交 probed_at touch，不重写 supported。
func (s *OpenAIGatewayService) stampOpenAIProtocolCapabilities(ctx context.Context, accountID int64, before, stamped openai_compat.ProtocolCapabilities) {
	patches := make([]OpenAIProtocolCapabilityPatch, 0, 2)
	if before.Responses.Known && before.Responses.ProbedAt.IsZero() {
		patches = append(patches, OpenAIProtocolCapabilityPatch{
			Protocol: openai_compat.ProtocolResponses,
			ProbedAt: stamped.Responses.ProbedAt,
		})
	}
	if before.ChatCompletions.Known && before.ChatCompletions.ProbedAt.IsZero() {
		patches = append(patches, OpenAIProtocolCapabilityPatch{
			Protocol: openai_compat.ProtocolChatCompletions,
			ProbedAt: stamped.ChatCompletions.ProbedAt,
		})
	}
	if len(patches) == 0 {
		return
	}
	s.persistOpenAIProtocolCapabilityPatches(ctx, accountID, patches, patches[len(patches)-1].ProbedAt)
}

func (s *OpenAIGatewayService) startOpenAIProtocolCapabilityReprobe(accountID int64) {
	if s.accountRepo == nil || s.httpUpstream == nil {
		return
	}
	now := time.Now()
	pruneOpenAIProtocolProbeState(now)
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
		if !openAIProtocolProbeEligible(account) {
			return
		}
		probeOpenAIProtocolCapabilities(ctx, s.openAIProtocolProbeDeps(), account, openAIProtocolAsyncProbeTimeout)
	}()
}

type openAIProtocolUnsupportedError struct {
	Protocol   string
	StatusCode int
	// CapabilityEvidence 表示 404/405 响应是可识别的 JSON endpoint-not-found，
	// 只有它为 true 时才允许记录 supported=false。
	CapabilityEvidence bool
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
// 注意：这里只决定是否重试；是否把 supported=false 落库由 openAIEndpointNotFoundJSON 决定。
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

// openAIProtocolFromUpstreamEndpoint 把实际命中的上游路径映射为能力协议。
func openAIProtocolFromUpstreamEndpoint(endpoint string) string {
	endpoint = strings.ToLower(strings.TrimSpace(endpoint))
	switch {
	case strings.HasSuffix(endpoint, "/v1/responses") || strings.Contains(endpoint, "/v1/responses/"):
		return openai_compat.ProtocolResponses
	case strings.HasSuffix(endpoint, "/v1/chat/completions"):
		return openai_compat.ProtocolChatCompletions
	default:
		return ""
	}
}

func openAIForwardResultEndpoint(result *OpenAIForwardResult) string {
	if result == nil {
		return ""
	}
	return result.UpstreamEndpoint
}

// recordOpenAIProtocolCapability 更新内存中的能力，并异步按协议持久化到 accounts.extra。
func (s *OpenAIGatewayService) recordOpenAIProtocolCapability(ctx context.Context, account *Account, protocol string, supported bool) *Account {
	if account == nil {
		return account
	}
	now := time.Now()
	caps := openai_compat.ResolveProtocolCapabilities(account.Extra)
	entry := openai_compat.ProtocolCapability{Known: true, Supported: supported, ProbedAt: now}
	switch protocol {
	case openai_compat.ProtocolResponses:
		caps.Responses = entry
	case openai_compat.ProtocolChatCompletions:
		caps.ChatCompletions = entry
	default:
		return account
	}
	updated := accountWithOpenAIProtocolCapabilities(account, caps)
	s.persistOpenAIProtocolCapabilityPatches(ctx, account.ID, []OpenAIProtocolCapabilityPatch{{
		Protocol:  protocol,
		Supported: &supported,
		ProbedAt:  now,
	}}, now)
	return updated
}

// persistOpenAIProtocolCapabilityPatches 异步按协议写回能力变更。写入按账号串行化，
// 请求路径只投递 patch，不等待 DB 写，也不在请求 goroutine 内持有锁。
func (s *OpenAIGatewayService) persistOpenAIProtocolCapabilityPatches(ctx context.Context, accountID int64, patches []OpenAIProtocolCapabilityPatch, probedAt time.Time) {
	if s == nil || s.accountRepo == nil || accountID <= 0 || len(patches) == 0 {
		return
	}
	if _, ok := s.accountRepo.(OpenAIProtocolCapabilityRepository); !ok {
		logger.LegacyPrintf("service.openai_probe", "protocol_capability_persist_unsupported: account_id=%d", accountID)
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	queue, start := enqueueOpenAIProtocolPersistJob(accountID, openAIProtocolPersistJob{
		parent:   context.WithoutCancel(ctx),
		patches:  patches,
		probedAt: probedAt,
	})
	if start {
		go s.runOpenAIProtocolPersistQueue(accountID, queue)
	}
}

func (s *OpenAIGatewayService) runOpenAIProtocolPersistQueue(accountID int64, queue *openAIProtocolPersistQueue) {
	for {
		job := queue.take()
		if job == nil {
			openAIProtocolPersistQueueRelease(accountID, queue)
			return
		}
		parent := job.parent
		if parent == nil {
			parent = context.Background()
		}
		persistCtx, cancel := context.WithTimeout(parent, 5*time.Second)
		repo, ok := s.accountRepo.(OpenAIProtocolCapabilityRepository)
		if ok {
			if err := repo.MergeOpenAIProtocolCapabilities(persistCtx, accountID, job.patches); err != nil {
				logger.LegacyPrintf("service.openai_probe", "protocol_capability_persist_failed: account_id=%d err=%v", accountID, err)
			}
		}
		cancel()
	}
}
