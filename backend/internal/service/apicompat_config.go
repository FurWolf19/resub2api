package service

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

func configuredMinOutputTokens(cfg *config.Config) int {
	if cfg != nil && cfg.Gateway.MinOutputTokens >= 16 {
		return cfg.Gateway.MinOutputTokens
	}
	return apicompat.DefaultMinMaxOutputTokens
}

func chatCompletionsToResponses(cfg *config.Config, req *apicompat.ChatCompletionsRequest) (*apicompat.ResponsesRequest, error) {
	return apicompat.ChatCompletionsToResponsesWithMinOutputTokens(req, configuredMinOutputTokens(cfg))
}

// chatCompletionsToResponsesForAccount converts a Chat Completions request for
// the given account. DeepSeek's native Responses API and OpenCode Zen / Go
// forwards of DeepSeek models take historical reasoning as reasoning_text;
// other upstreams cannot consume plaintext reasoning, so it is dropped instead
// of leaking into visible content.
func chatCompletionsToResponsesForAccount(cfg *config.Config, account *Account, req *apicompat.ChatCompletionsRequest) (*apicompat.ResponsesRequest, error) {
	replay := apicompat.ChatCompletionsReasoningReplayDrop
	if requiresDeepSeekChatReasoningForModel(account, req.Model) {
		replay = apicompat.ChatCompletionsReasoningReplayText
	}
	return apicompat.ChatCompletionsToResponsesWithReasoningReplay(req, configuredMinOutputTokens(cfg), replay)
}

func settingServiceConfig(settingService *SettingService) *config.Config {
	if settingService == nil {
		return nil
	}
	return settingService.cfg
}
