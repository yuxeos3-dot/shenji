package ai

import "strings"

// ExplainError translates raw AI-provider error strings into an actionable
// Chinese message so audit task owners see "余额不足，请充值" instead of
// a raw "HTTP 402: {...}". Shared by the manual audit path (api/audit.go)
// and the scheduled-audit path (scheduler/scheduler.go) so both produce
// identical error_msg content.
func ExplainError(raw string) string {
	low := strings.ToLower(raw)
	// Trim absurdly long upstream JSON payloads — the 审计详情 page shows
	// error_msg in a small banner, anything longer than a couple hundred
	// chars would be unreadable.
	trimmed := raw
	if len(trimmed) > 400 {
		trimmed = trimmed[:400] + "…"
	}
	switch {
	case strings.Contains(raw, "HTTP 401"), strings.Contains(low, "unauthorized"),
		strings.Contains(low, "invalid api key"), strings.Contains(low, "invalid_api_key"):
		return "AI 调用失败：API Key 无效或已过期。请到「个人 AI 设置」检查 API Key。\n\n原始错误：" + trimmed
	case strings.Contains(raw, "HTTP 402"), strings.Contains(low, "insufficient"),
		strings.Contains(low, "balance"), strings.Contains(low, "payment"),
		strings.Contains(low, "quota"), strings.Contains(low, "credit"):
		return "AI 调用失败：账户余额不足或已超出配额。请到 AI 平台充值 / 增加配额后重试。\n\n原始错误：" + trimmed
	case strings.Contains(raw, "HTTP 429"), strings.Contains(low, "rate limit"), strings.Contains(low, "too many requests"):
		return "AI 调用失败：触发限流（请求过于频繁）。请稍等几分钟后重新触发巡检。\n\n原始错误：" + trimmed
	case strings.Contains(raw, "HTTP 404"), (strings.Contains(low, "model") && strings.Contains(low, "not found")),
		strings.Contains(low, "unknown model"), strings.Contains(low, "does not exist"):
		return "AI 调用失败：所选模型在当前 API 端点不可用。请到「个人 AI 设置」换一个模型（例如 claude-sonnet-4-6 / gpt-4o / deepseek-chat）。\n\n原始错误：" + trimmed
	case strings.Contains(raw, "HTTP 413"), strings.Contains(low, "context length"), strings.Contains(low, "context_length_exceeded"),
		(strings.Contains(low, "token") && strings.Contains(low, "exceed")):
		return "AI 调用失败：prompt 超过模型上下文长度。请减少站点自定义页面数量或换一个支持更大上下文的模型（如 claude-sonnet-4-6）。\n\n原始错误：" + trimmed
	case strings.Contains(raw, "HTTP 5"), strings.Contains(low, "502"), strings.Contains(low, "503"), strings.Contains(low, "504"):
		return "AI 调用失败：上游服务异常（502/503/504）。通常几分钟后恢复，可稍后重试。\n\n原始错误：" + trimmed
	case strings.Contains(low, "timeout"), strings.Contains(low, "deadline"), strings.Contains(low, "context deadline"):
		return "AI 调用失败：请求超时。可能是网络不稳定或模型响应过慢，稍后重试即可。\n\n原始错误：" + trimmed
	case strings.Contains(low, "json 解析失败"), strings.Contains(raw, "JSON 解析失败"):
		return "AI 调用失败：模型返回不是合法 JSON（可能是模型输出被截断或说了太多解释性文字）。请换成支持 JSON 模式的强模型（claude-sonnet-4-6 / gpt-4o），或重新触发。\n\n原始错误：" + trimmed
	}
	return "AI 调用失败：" + trimmed
}
