package api

import (
	"bytes"
	"strconv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/httpx"

	"github.com/gin-gonic/gin"
)

type RewriteRequest struct {
	SiteID   int64  `json:"site_id"`
	Mode     string `json:"mode"` // "tdk" or "content"
	PageType string `json:"page_type"`
	Keywords string `json:"keywords"`
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	Extra    string `json:"extra"`
	Type     string `json:"type"`
	Style    string `json:"style"`
	Length   string `json:"length"`
	Original string `json:"original"`
}

func RewriteContent(c *gin.Context) {
	var req RewriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	// Input validation — prevent excessively large payloads from reaching the AI
	if req.SiteID > 0 {
		if !canAccessSite(c, strconv.FormatInt(req.SiteID, 10)) {
			return
		}
	}

	if len(req.Keywords) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "关键词过长（最多1000字符）"})
		return
	}
	if len(req.Original) > 50000 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "原始内容过长（最多50000字符）"})
		return
	}
	if len(req.Title) > 500 || len(req.Desc) > 2000 || len(req.Extra) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "输入字段过长"})
		return
	}

	var prompt string
	if req.Mode == "tdk" {
		prompt = buildTDKPrompt(req)
	} else {
		prompt = buildContentPrompt(req)
	}

	result, err := callRewriteAI(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "AI 调用失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func buildTDKPrompt(req RewriteRequest) string {
	return fmt.Sprintf(`你是一位资深 SEO 专家，请对以下页面的 Title 和 Description 进行 SEO 优化改写。

页面类型：%s
目标关键词：%s
当前 Title：%s
当前 Description：%s
补充说明：%s

要求：
1. 生成 3 个不同版本，每个版本各有侧重（版本1强调关键词，版本2强调用户点击率，版本3均衡）
2. Title 控制在 30-60 字符，必须包含核心关键词，品牌词放末尾
3. Description 控制在 120-160 字符，包含核心关键词，有明确的用户价值点
4. 每个版本附上一句"优化说明"解释改写思路

只返回 JSON，格式如下（不要任何其他内容）：
[
  {"title":"...", "desc":"...", "reason":"优化说明..."},
  {"title":"...", "desc":"...", "reason":"优化说明..."},
  {"title":"...", "desc":"...", "reason":"优化说明..."}
]`, req.PageType, req.Keywords, req.Title, req.Desc, req.Extra)
}

func buildContentPrompt(req RewriteRequest) string {
	return fmt.Sprintf(`你是一位专业的 SEO 内容编辑，请对以下内容进行二次创作改写，使其独特且 SEO 友好。

内容类型：%s
改写风格：%s
目标字数：约 %s 字
核心关键词：%s
原始内容：
%s

要求：
1. 生成 2 个风格不同的改写版本
2. 改写后内容必须与原文有明显差异，避免重复内容惩罚
3. 自然融入核心关键词（不堆砌）
4. 内容对用户有吸引力，有可读性
5. 每个版本附简短说明

只返回 JSON，格式如下：
[
  {"style":"风格名", "content":"改写内容...", "note":"说明"},
  {"style":"风格名", "content":"改写内容...", "note":"说明"}
]`, req.Type, req.Style, req.Length, req.Keywords, req.Original)
}

func callRewriteAI(prompt string) (interface{}, error) {
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
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  3000,
		"temperature": 0.7,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("rewrite 请求体序列化失败: %w", err)
	}
	req, err := http.NewRequest("POST", strings.TrimRight(apiURL, "/")+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("rewrite 构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := httpx.Outbound(120 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil || len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("AI响应解析失败")
	}

	raw := apiResp.Choices[0].Message.Content
	// Strip markdown code blocks
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 2 {
			inner := strings.Join(lines[1:], "\n")
			if idx := strings.LastIndex(inner, "```"); idx >= 0 {
				inner = inner[:idx]
			}
			raw = strings.TrimSpace(inner)
		}
	}
	// Find JSON array or object
	content := raw
	if start := strings.IndexAny(raw, "[{"); start >= 0 {
		content = raw[start:]
	}
	var result interface{}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %v", err)
	}
	return result, nil
}
