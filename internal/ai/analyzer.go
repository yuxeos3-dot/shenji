package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/db"
)

type AnalysisResult struct {
	SEOIssues      json.RawMessage `json:"seo_issues"`
	CodeIssues     json.RawMessage `json:"code_issues"`
	SecurityIssues json.RawMessage `json:"security_issues"`
	PerfIssues     json.RawMessage `json:"perf_issues"`
	Suggestions    string          `json:"suggestions"`
	Summary        string          `json:"summary"`
}

type GSCDataForPrompt struct {
	Clicks      int    `json:"clicks"`
	Impressions int    `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
	Keywords    string  `json:"keywords_json"`
	Pages       string  `json:"pages_json"`
}

type GADataForPrompt struct {
	Sessions  int    `json:"sessions"`
	Users     int    `json:"users"`
	Pageviews int    `json:"pageviews"`
	Channels  string `json:"channels_json"`
}

type YandexDataForPrompt struct {
	IndexCount int    `json:"index_count"`
	Errors     string `json:"errors_json"`
}

func BuildAnalysisPrompt(domain string, pages []crawler.PageResult, gsc *GSCDataForPrompt, ga *GADataForPrompt, yandex *YandexDataForPrompt) string {
	prompt := fmt.Sprintf(`你是一位资深SEO技术顾问，请对以下网站进行全面分析。

## 站点信息
域名: %s
分析时间: %s

## 各页面抓取结果
`, domain, time.Now().Format("2006-01-02 15:04:05"))

	for _, p := range pages {
		html := p.HTML
		if len(html) > 8000 {
			html = html[:8000]
		}
		errInfo := ""
		if p.Error != "" {
			errInfo = fmt.Sprintf(" 错误: %s", p.Error)
		}
		prompt += fmt.Sprintf("\n### %s (%s) - 状态码%d TTFB:%dms HTML:%dKB%s\n```html\n%s\n```\n",
			p.PageType, p.URL, p.StatusCode, p.TTFB, p.HTMLSize/1024, errInfo, html)
	}

	if gsc != nil {
		prompt += fmt.Sprintf(`
## Google Search Console数据（最近30天）
总点击: %d 总展示: %d 平均CTR: %.2f%% 平均排名: %.1f
Top关键词: %s
Top页面: %s
`, gsc.Clicks, gsc.Impressions, gsc.CTR*100, gsc.Position, gsc.Keywords, gsc.Pages)
	}

	if ga != nil {
		prompt += fmt.Sprintf(`
## Google Analytics数据（最近30天）
会话: %d 用户: %d 页面浏览: %d
渠道分布: %s
`, ga.Sessions, ga.Users, ga.Pageviews, ga.Channels)
	}

	if yandex != nil {
		prompt += fmt.Sprintf(`
## Yandex Webmaster数据
收录数: %d 诊断错误: %s
`, yandex.IndexCount, yandex.Errors)
	}

	prompt += `
## 请按以下JSON格式输出分析结果（只返回JSON，不要其他内容）：
{
  "seo_issues": [{"level":"P0/P1/P2","title":"问题标题","desc":"详细说明","fix":"修复建议"}],
  "code_issues": [{"level":"P0/P1/P2","title":"","desc":"","fix":""}],
  "security_issues": [{"level":"P0/P1/P2","title":"","desc":"","fix":""}],
  "perf_issues": [{"level":"P0/P1/P2","title":"","desc":"","fix":""}],
  "suggestions": "整体整改建议（markdown格式）",
  "summary": "一句话总结站点健康状况"
}`

	return prompt
}

func CallAI(prompt string) (*AnalysisResult, string, error) {
	apiURL := db.GetSetting("ai_api_url")
	apiKey := db.GetSetting("ai_api_key")
	model := db.GetSetting("ai_model")

	if apiURL == "" {
		apiURL = "https://ai.router.team/v1"
	}
	if model == "" {
		model = "claude-sonnet-4-6"
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  4096,
		"temperature": 0.3,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, model, fmt.Errorf("构建请求失败: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, model, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, model, fmt.Errorf("AI请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, model, fmt.Errorf("读取AI响应失败: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, model, fmt.Errorf("AI返回错误 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, model, fmt.Errorf("解析AI响应失败: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, model, fmt.Errorf("AI未返回结果")
	}

	content := apiResp.Choices[0].Message.Content

	// Try to extract JSON from the response
	content = extractJSON(content)

	var result AnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// If parsing fails, store raw content as summary
		result = AnalysisResult{
			Summary:    content,
			SEOIssues:  json.RawMessage("[]"),
			CodeIssues: json.RawMessage("[]"),
			SecurityIssues: json.RawMessage("[]"),
			PerfIssues: json.RawMessage("[]"),
		}
	}

	return &result, model, nil
}

func extractJSON(s string) string {
	// Find first { and last }
	start := -1
	end := -1
	depth := 0
	for i, c := range s {
		if c == '{' {
			if start == -1 {
				start = i
			}
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if start >= 0 && end > start {
		return s[start:end]
	}
	return s
}

func TestAIConnection() error {
	apiURL := db.GetSetting("ai_api_url")
	apiKey := db.GetSetting("ai_api_key")
	model := db.GetSetting("ai_model")

	if apiURL == "" || apiKey == "" {
		return fmt.Errorf("AI API未配置")
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "Say hello in one word."},
		},
		"max_tokens": 10,
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", apiURL+"/chat/completions", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("AI返回错误 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
