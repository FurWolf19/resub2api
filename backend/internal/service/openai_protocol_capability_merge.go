package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
)

// OpenAIProtocolCapabilityPatch 描述一次只针对单个协议的结论或探测时间更新。
// Supported 为 nil 表示 touch：只刷新 probed_at，不改变已知的 supported。
type OpenAIProtocolCapabilityPatch struct {
	Protocol  string
	Supported *bool
	ProbedAt  time.Time
}

// OpenAIProtocolCapabilityRepository 是协议能力合并写回的可选仓储扩展。
// 生产仓储必须实现；未实现时调用方跳过写入，不得退回整包 UpdateExtra。
type OpenAIProtocolCapabilityRepository interface {
	MergeOpenAIProtocolCapabilities(ctx context.Context, accountID int64, patches []OpenAIProtocolCapabilityPatch) error
}

// MergeOpenAIProtocolCapabilities 按 probed_at 把 patch 合并到当前快照。
// 返回合并后的快照、需同步的旧键 openai_responses_supported（Responses 未知时为
// nil，表示不删除旧键）以及快照是否发生变化。
func MergeOpenAIProtocolCapabilities(
	current openai_compat.ProtocolCapabilities,
	patches []OpenAIProtocolCapabilityPatch,
) (merged openai_compat.ProtocolCapabilities, legacyResponsesSupported *bool, changed bool, err error) {
	merged = current
	for _, patch := range patches {
		switch patch.Protocol {
		case openai_compat.ProtocolResponses:
			next, updated := mergeProtocolCapability(merged.Responses, patch)
			if updated {
				merged.Responses = next
				changed = true
			}
		case openai_compat.ProtocolChatCompletions:
			next, updated := mergeProtocolCapability(merged.ChatCompletions, patch)
			if updated {
				merged.ChatCompletions = next
				changed = true
			}
		default:
			return openai_compat.ProtocolCapabilities{}, nil, false, fmt.Errorf("unknown OpenAI protocol capability %q", patch.Protocol)
		}
	}
	if merged.Responses.Known {
		supported := merged.Responses.Supported
		legacyResponsesSupported = &supported
	}
	return merged, legacyResponsesSupported, changed, nil
}

func mergeProtocolCapability(current openai_compat.ProtocolCapability, patch OpenAIProtocolCapabilityPatch) (openai_compat.ProtocolCapability, bool) {
	if patch.Supported != nil {
		if current.Known && current.ProbedAt.After(patch.ProbedAt) {
			return current, false
		}
		next := openai_compat.ProtocolCapability{
			Known:     true,
			Supported: *patch.Supported,
			ProbedAt:  patch.ProbedAt,
		}
		return next, next != current
	}
	// touch 不能凭空创建结论，也不能倒退 probed_at。
	if !current.Known || current.ProbedAt.After(patch.ProbedAt) {
		return current, false
	}
	if current.ProbedAt.Equal(patch.ProbedAt) {
		return current, false
	}
	current.ProbedAt = patch.ProbedAt
	return current, true
}
