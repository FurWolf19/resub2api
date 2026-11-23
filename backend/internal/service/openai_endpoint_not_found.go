package service

import (
	"bytes"
	"strings"

	"github.com/tidwall/gjson"
)

// openAIEndpointNotFoundJSON 判断上游 404/405 响应是否是可识别的
// “endpoint 不存在”JSON 错误。只有这类响应才能作为协议不支持的证据落库；
// HTML、空体、纯文本和未知 JSON 都可能来自反向代理或临时故障，保持未知。
func openAIEndpointNotFoundJSON(status int, body []byte) bool {
	if status != 404 && status != 405 {
		return false
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	if !gjson.ParseBytes(body).IsObject() {
		return false
	}

	// 只有 endpoint 专用 code 可以跳过模型文案排除；泛化的 not_found 可能来自
	// 模型不可用，必须先走下面的模型排除。
	for _, path := range []string{"error.code", "error.type", "code", "type"} {
		switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, path).String())) {
		case "endpoint_not_found", "route_not_found", "unknown_endpoint",
			"unsupported_endpoint", "method_not_allowed", "invalid_url", "unknown_url", "no_route":
			return true
		}
	}

	// 模型不可用（model_not_found / 「Model <name> not found」等）说的是模型，
	// 不是端点不存在；必须先排除，否则会被后面的 "not found" 文案误判。
	if isResponsesProbeModelUnavailable(status, body) {
		return false
	}
	for _, path := range []string{"error.message", "message", "detail"} {
		if looksLikeOpenAIModelUnavailableMessage(gjson.GetBytes(body, path).String()) {
			return false
		}
	}

	for _, path := range []string{"error.code", "error.type", "code", "type"} {
		if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(body, path).String()), "not_found") {
			return true
		}
	}

	for _, path := range []string{"error.message", "message", "detail"} {
		message := strings.ToLower(gjson.GetBytes(body, path).String())
		for _, marker := range []string{
			"not found", "no route", "unknown endpoint", "unsupported endpoint",
			"endpoint does not exist", "cannot post", "method not allowed", "not implemented",
			"invalid url", "unknown url", "unknown request url",
		} {
			if strings.Contains(message, marker) {
				return true
			}
		}
	}

	return false
}

func looksLikeOpenAIModelUnavailableMessage(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	if message == "" {
		return false
	}
	if isExplicitOpenAIModelAvailabilityMessage(message) {
		return true
	}
	if !strings.Contains(message, "model") {
		return false
	}
	for _, marker := range []string{
		"not found", "does not exist", "not exist", "unavailable", "not available",
		"unsupported", "not supported",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
