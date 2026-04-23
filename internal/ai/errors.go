package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ExplainError translates raw AI-provider error strings into an actionable
// Chinese message so audit task owners see "余额不足，请充值" instead of
// a raw "HTTP 402: {...}". Shared by the manual audit path (api/audit.go)
// and the scheduled-audit path (scheduler/scheduler.go) so both produce
// identical error_msg content.
//
// Strategy: try to extract the provider's own error.message / error.code
// fields from any embedded JSON payload first (those are always more
// specific than our heuristic matches), then fall through to keyword
// matching on HTTP status lines. Always appends the original raw string
// so ops can see the exact upstream payload for deep debugging.
func ExplainError(raw string) string {
	detail := extractProviderError(raw)
	low := strings.ToLower(raw)

	// Collapse HTML dump into a short readable snippet (Cloudflare /
	// WAF block pages are 4-8 KB of markup that tell the user nothing).
	trimmed := stripHTMLBoilerplate(raw)
	if len(trimmed) > 800 {
		trimmed = trimmed[:800] + "…"
	}

	// Build the message: headline + optional provider detail + suggested
	// action + raw snippet for debugging.
	headline, action := classifyError(raw, low)
	var sb strings.Builder
	sb.WriteString(headline)
	if detail != "" {
		sb.WriteString("\n供应商原文：")
		sb.WriteString(detail)
	}
	if action != "" {
		sb.WriteString("\n建议处理：")
		sb.WriteString(action)
	}
	sb.WriteString("\n\n原始响应片段：")
	sb.WriteString(trimmed)
	return sb.String()
}

// classifyError returns (headline, action) — the core human-readable
// explanation + a one-line suggested fix. Classification is by HTTP
// status code first (most reliable), then by keyword fallback.
func classifyError(raw, low string) (headline, action string) {
	switch {
	case strings.Contains(raw, "返回空内容"), strings.Contains(raw, "AI 无返回"),
		strings.Contains(raw, "空响应"):
		return "❌ AI 调用失败：模型返回空内容",
			"你当前配置的模型在此网关可能未正确配置（常见于自建网关 + 推理型模型如 gpt-5 系列）。请到「个人 AI 设置」切换到稳定模型（推荐 qwen3.6-plus / claude-sonnet-4-5 / gpt-4o / deepseek-chat）"
	case strings.Contains(raw, "HTTP 401"), strings.Contains(low, "unauthorized"),
		strings.Contains(low, "invalid api key"), strings.Contains(low, "invalid_api_key"),
		strings.Contains(low, "authentication"):
		return "❌ AI 调用失败：API Key 无效 / 已过期 / 未授权（HTTP 401）",
			"让对应用户到「个人 AI 设置」重新填入有效的 API Key，并点「测试 AI 接口」确认"
	case strings.Contains(raw, "HTTP 402"), strings.Contains(low, "insufficient_quota"),
		strings.Contains(low, "insufficient balance"), strings.Contains(low, "insufficient credit"),
		strings.Contains(low, "payment required"):
		return "❌ AI 调用失败：账户余额不足 / 付费额度耗尽（HTTP 402）",
			"让对应用户到 AI 供应商后台充值，或在「个人 AI 设置」换一把还有余额的 Key"
	case strings.Contains(raw, "HTTP 429"), strings.Contains(low, "rate limit"),
		strings.Contains(low, "too many requests"), strings.Contains(low, "requests per min"),
		strings.Contains(low, "tokens per min"):
		return "❌ AI 调用失败：触发速率限制（HTTP 429）",
			"等 3-5 分钟让限流窗口过去；长期解决方法是让用户升级 AI 套餐、或避免多个站点同时触发巡检"
	case strings.Contains(raw, "HTTP 403"):
		// 403 + HTML 响应 = 几乎可以确定是 Cloudflare / WAF 拦截
		if strings.Contains(low, "<!doctype") || strings.Contains(low, "<html") ||
			strings.Contains(low, "cloudflare") || strings.Contains(low, "attention required") {
			return "❌ AI 调用失败：被 Cloudflare / WAF 拦截（HTTP 403 + HTML 页面）",
				"API Key 可能被 AI 供应商封禁，或服务器 IP 被 Cloudflare 拉黑；让用户去 AI 后台检查 Key 状态、必要时重新生成"
		}
		return "❌ AI 调用失败：权限被拒绝（HTTP 403）",
			"检查 API Key 对应的账户是否开通了 /chat/completions 接口 & 所选模型的权限"
	case strings.Contains(raw, "HTTP 404"),
		strings.Contains(low, "model") && strings.Contains(low, "not found"),
		strings.Contains(low, "model does not exist"),
		strings.Contains(low, "the model"):
		return "❌ AI 调用失败：所选模型在当前 API 端点找不到（HTTP 404）",
			"到「个人 AI 设置」换一个模型名，推荐 claude-sonnet-4-6 / gpt-4o / deepseek-chat"
	case strings.Contains(raw, "HTTP 413"),
		strings.Contains(low, "context length exceeded"), strings.Contains(low, "context_length_exceeded"),
		strings.Contains(low, "maximum context"),
		(strings.Contains(low, "token") && (strings.Contains(low, "exceed") || strings.Contains(low, "limit"))):
		return "❌ AI 调用失败：prompt 超过模型最大上下文长度（HTTP 413 / context_length_exceeded）",
			"减少站点自定义页面数量（目前可能有太多页面被抓取），或切换到支持更大上下文的模型（claude-sonnet-4-6 / gpt-4o）"
	case strings.Contains(raw, "HTTP 500"):
		return "❌ AI 调用失败：上游服务内部错误（HTTP 500）",
			"通常几分钟后恢复，可稍后重新触发巡检"
	case strings.Contains(raw, "HTTP 502"), strings.Contains(raw, "HTTP 503"),
		strings.Contains(raw, "HTTP 504"), strings.Contains(low, "bad gateway"),
		strings.Contains(low, "service unavailable"), strings.Contains(low, "gateway timeout"):
		return "❌ AI 调用失败：AI 网关不可用（HTTP 502/503/504）",
			"AI 供应商或中间代理（如 ai.router.team）临时抽风，等 5-10 分钟重试；连续多次同错联系供应商"
	case strings.Contains(low, "connection refused"), strings.Contains(low, "dial tcp"),
		strings.Contains(low, "no such host"), strings.Contains(low, "dns"):
		return "❌ AI 调用失败：网络连接失败（DNS / TCP 层面）",
			"检查 API URL 是否写对（常见错误：多 / 少 /v1）、服务器到该域名的网络是否通畅"
	case strings.Contains(low, "timeout"), strings.Contains(low, "deadline exceeded"),
		strings.Contains(low, "context deadline"):
		return "❌ AI 调用失败：请求超时",
			"模型响应过慢或网络不稳；稍后重试；如果频繁超时考虑换更快的模型（如 claude-sonnet-4-6）"
	case strings.Contains(low, "json 解析失败"), strings.Contains(raw, "JSON 解析失败"),
		strings.Contains(low, "ai chapters json"):
		return "❌ AI 调用失败：模型返回的内容不是合法 JSON（可能被截断，或加了太多解释性文字）",
			"在「个人 AI 设置」换一个输出更稳定的模型：claude-sonnet-4-6 / gpt-4o / deepseek-chat；或把 ai_max_tokens 调高"
	case strings.Contains(low, "ai 无返回"), strings.Contains(raw, "AI 无返回"):
		return "❌ AI 调用失败：模型返回了空响应",
			"通常是内容被供应商安全策略过滤；换一个模型重试"
	}
	// HTML returned where JSON expected — a very common failure mode where
	// the gateway URL is wrong or returns error pages
	if strings.Contains(raw, "invalid character '<'") {
		return "❌ AI 调用失败：网关返回HTML页面（不是JSON）",
			"可能原因：1) API URL配置错误（应含/v1路径）; 2) 模型名不存在于网关; 3) Cloudflare/WAF拦截。建议到「个人 AI 设置」检查URL和模型"
	}
	return "❌ AI 调用失败：未识别的错误类型", "对照下方原始响应片段判断，必要时登录服务器 journalctl -u seo-system 查详细日志"
}

// extractProviderError best-effort parses the most common AI-provider
// JSON error envelope shapes and returns just the human-readable
// "message" / "detail" field. Returns "" when nothing parseable is
// found, in which case classifyError's headline stands alone.
//
// Handles OpenAI / Anthropic / DeepSeek / Qwen-compatible shapes:
//
//	{"error": {"message": "...", "code": "...", "type": "..."}}
//	{"error": "string"}
//	{"message": "..."}
//	{"detail": "..."}
//	{"err_msg": "..."} (Baidu Qianfan)
func extractProviderError(raw string) string {
	// Try to locate a JSON object inside the raw string — errors often
	// look like "HTTP 402: {...}" so we slice from the first `{` on.
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return ""
	}
	blob := raw[start : end+1]

	// Try structured decode into a generic map so we can reach into
	// any of the known envelope shapes.
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(blob), &envelope); err != nil {
		return ""
	}

	// Shape 1: {"error": {"message": "..."}}
	if e, ok := envelope["error"].(map[string]interface{}); ok {
		parts := []string{}
		if m, _ := e["message"].(string); m != "" {
			parts = append(parts, m)
		}
		if c, _ := e["code"].(string); c != "" {
			parts = append(parts, fmt.Sprintf("code=%s", c))
		}
		if t, _ := e["type"].(string); t != "" {
			parts = append(parts, fmt.Sprintf("type=%s", t))
		}
		if len(parts) > 0 {
			return strings.Join(parts, " · ")
		}
	}
	// Shape 2: {"error": "string"}
	if s, ok := envelope["error"].(string); ok && s != "" {
		return s
	}
	// Shape 3 / 4: top-level "message" / "detail" / "err_msg"
	for _, key := range []string{"message", "detail", "err_msg", "error_message", "msg"} {
		if v, _ := envelope[key].(string); v != "" {
			return v
		}
	}
	return ""
}

// stripHTMLBoilerplate truncates really long HTML error bodies (typical
// of Cloudflare / WAF block pages) to just a short "HTML page returned"
// hint — the full HTML is useless noise in error_msg.
var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func stripHTMLBoilerplate(raw string) string {
	low := strings.ToLower(raw)
	if !strings.Contains(low, "<!doctype") && !strings.Contains(low, "<html") {
		return raw
	}
	// Strip HTML tags, collapse whitespace, keep first 200 chars of text.
	text := htmlTagRe.ReplaceAllString(raw, " ")
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 200 {
		text = text[:200] + "…"
	}
	return "[HTML 拦截页] " + text
}
