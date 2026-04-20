package frw

// FRW (aiaiartist) AI generation client.
//
// The upstream uses a two-layer auth:
//   - apikey     (account-level, set per-user in frw_config; falls back
//                  to the global "frw_apikey" setting when empty)
//   - Bearer tok (session-level, obtained via /api/auth/login, which is
//                  captcha-gated — so for now users copy-paste it from
//                  their browser devtools and we persist it here.)
//
// Submit is one request, response returns a task_id. Progress comes from
// polling /api/ai-generations/query until status reaches 2 (done) / -1
// (failed) / 4 (timeout).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"seo-system/internal/httpx"
)

const (
	BaseURL     = "https://frw-dreamaiai-api2.aiaiartist.com"
	SubmitPath  = "/api/ai-generations/submit"
	QueryPath   = "/api/ai-generations/query"
	ReqTimeout  = 30 * time.Second
	PollTimeout = 15 * time.Minute // hard cap per-task
)

// ─── Mode presets ────────────────────────────────────────────────────────
//
// Pulled verbatim from the original openskills config.py. We don't let
// the caller override template_id / tool_id because those are tied to
// account entitlements — if FRW rotates them we'll update here.

type ModePreset struct {
	TemplateID    string                 `json:"template_id"`
	ToolID        string                 `json:"tool_id"`
	Title         string                 `json:"title"`
	ModelName     string                 `json:"model_name"`
	ModelShowName string                 `json:"model_show_name"`
	Model         string                 `json:"model"`
	ExtraParams   map[string]interface{} `json:"extra_params"`
}

var Presets = map[string]ModePreset{
	"text2image": {
		TemplateID:    "3469316249878233419",
		ToolID:        "3469326281949057024",
		Title:         "文生图",
		ModelName:     "aicomic_generate_images",
		ModelShowName: "基础算法",
		Model:         "qwen",
		ExtraParams: map[string]interface{}{
			"model_style_type": "qwen",
			"cfg_scale":        3.5,
			"sampling_steps":   30,
			"sampler_method":   "simple",
			"width":            768,
			"height":           1024,
			"batch_size":       1,
			"noise_seed":       0,
			"loras":            []interface{}{},
		},
	},
	"img2img": {
		TemplateID:    "3469316249878532288",
		ToolID:        "3469326281949057024",
		Title:         "图生图",
		ModelName:     "generate_image_style_transfer",
		ModelShowName: "基础算法XL + IPAdapter",
		Model:         "qwen",
		ExtraParams: map[string]interface{}{
			"loras":      []interface{}{},
			"width":      768,
			"height":     1024,
			"batch_size": 1,
		},
	},
	"text2video": {
		TemplateID:    "3469316249878532099",
		ToolID:        "3469326281949057024",
		Title:         "文生视频",
		ModelName:     "text_to_video",
		ModelShowName: "文生视频",
		Model:         "wan2.1",
		ExtraParams: map[string]interface{}{
			"width":  704,
			"height": 960,
			"loras":  []interface{}{},
		},
	},
	"img2video": {
		TemplateID:    "3469316249878532098",
		ToolID:        "3469326281949057024",
		Title:         "图生视频",
		ModelName:     "image_to_video",
		ModelShowName: "图生视频",
		Model:         "wan2.2",
		ExtraParams: map[string]interface{}{
			"width":  704,
			"height": 960,
		},
	},
}

// ─── Client ──────────────────────────────────────────────────────────────

type Client struct {
	APIKey string // account-level FRW apikey
	Token  string // Bearer session token
	UserID string // FRW user id (from login response)
	HTTP   *http.Client
}

func New(apikey, token, userID string) *Client {
	return &Client{
		APIKey: apikey,
		Token:  token,
		UserID: userID,
		HTTP:   httpx.Outbound(ReqTimeout),
	}
}

// SubmitResult is the unmarshalled "data" from a successful submit call.
// FRW returns a handful of fields; we only need task_id downstream.
type SubmitResult struct {
	TaskID        string  `json:"task_id"`
	Status        int     `json:"status"`
	Progress      float64 `json:"progress"`
	EstimatedTime int     `json:"estimated_time"`
}

// submitEnvelope holds the outer {success, data, ...} wrapper. data is
// decoded as RawMessage so we can apply lenient field-name matching
// ourselves — FRW has returned variants like `taskID` (camelCase) and
// `task_id` (snake_case) in the same API across deploys, and the
// status / progress fields occasionally come back as strings instead
// of numbers. A strict struct unmarshal blows up on the first
// type mismatch; a map walk skips problem fields per-row instead.
type submitEnvelope struct {
	Success bool            `json:"success"`
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// parseSubmitData walks the RawMessage data object as a generic map,
// pulling task_id / taskID / id (in that preference order) and
// numeric fields via asFloat-style coercion. Anything missing just
// stays zero-valued on the result.
func parseSubmitData(raw json.RawMessage) (*SubmitResult, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}
	r := &SubmitResult{}
	// task_id preferred names, in order: task_id, taskID, id.
	for _, k := range []string{"task_id", "taskID", "id"} {
		if v, ok := m[k].(string); ok && v != "" {
			r.TaskID = v
			break
		}
	}
	// Numeric fields — tolerate number-as-string ("0" instead of 0).
	if v, ok := numericAny(m["status"]); ok {
		r.Status = int(v)
	}
	if v, ok := numericAny(m["progress"]); ok {
		r.Progress = v
	}
	if v, ok := numericAny(m["estimated_time"]); ok {
		r.EstimatedTime = int(v)
	} else if v, ok := numericAny(m["estimatedTime"]); ok {
		r.EstimatedTime = int(v)
	}
	return r, r.TaskID != ""
}

// numericAny accepts float64 / string / json.Number / int and returns
// a float64. Returns (0, false) for anything else.
func numericAny(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	case string:
		if x == "" {
			return 0, false
		}
		var f float64
		if _, err := fmt.Sscanf(x, "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// Submit sends a generation task. mode must be one of Presets keys.
// sourceURL is only used by img2img / img2video.
func (c *Client) Submit(mode, prompt, sourceURL string) (*SubmitResult, error) {
	preset, ok := Presets[mode]
	if !ok {
		return nil, fmt.Errorf("不支持的生成模式: %s", mode)
	}
	if c.APIKey == "" {
		return nil, fmt.Errorf("FRW 尚未配置, 请到个人中心粘贴 FRW API Key")
	}
	if c.Token == "" || c.UserID == "" {
		// With the single-field onboarding flow, Token / UserID are
		// derived from APIKey (JWT). Reaching here means APIKey was
		// saved but decoding its payload couldn't yield a user_id —
		// typically because it isn't a standard JWT.
		return nil, fmt.Errorf("FRW API Key 格式不是标准 JWT, 无法自动解析 user_id — 请确认密钥是 FRW 平台发放的对接 KEY; 如格式特殊, 联系管理员")
	}

	gen := map[string]interface{}{
		"text":            prompt,
		"extra_params":    preset.ExtraParams,
		"model_name":      preset.ModelName,
		"model_show_name": preset.ModelShowName,
		"title":           preset.Title,
		"negative_prompt": "",
		"model":           preset.Model,
	}
	// img2img/img2video reference image goes on source_path.
	if sourceURL != "" && (mode == "img2img" || mode == "img2video") {
		gen["source_path"] = sourceURL
	}

	payload := map[string]interface{}{
		"templateID":           preset.TemplateID,
		"toolID":               preset.ToolID,
		"generationParameters": gen,
		"userID":               c.UserID,
	}
	body, _ := json.Marshal(payload)

	// apikey goes in BOTH query string and header per the reference client.
	url := BaseURL + SubmitPath + "?apikey=" + c.APIKey
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("apikey", c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FRW submit 请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("FRW 返回 401, Bearer token 已失效, 请重新从浏览器复制")
	}
	var env submitEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("FRW 响应解析失败 (HTTP %d): %s", resp.StatusCode, truncate(string(raw), 200))
	}
	if !env.Success {
		msg := env.Message
		if env.Error != nil && env.Error.Message != "" {
			msg = env.Error.Message
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("FRW submit 失败: %s", msg)
	}
	result, ok := parseSubmitData(env.Data)
	if !ok {
		return nil, fmt.Errorf("FRW submit 未返回 task_id, 原始响应: %s", truncate(string(raw), 200))
	}
	return result, nil
}

// QueryResult mirrors what we care about from /query. The upstream
// returns a list keyed by task_id; we collapse to the single entry.
type QueryResult struct {
	TaskID   string  `json:"task_id"`
	Status   int     `json:"status"`   // 0/1/2/-1/4
	Progress float64 `json:"progress"` // 0..1
	OutData  string  `json:"out_data"` // final URL when status==2
	Message  string  `json:"message"`  // failure reason when status==-1
}

// Query a single task's status. Returns (nil, nil) if the upstream says
// success=true but returns no entry for our task_id (shouldn't happen,
// but handled gracefully).
func (c *Client) Query(taskID string) (*QueryResult, error) {
	if taskID == "" {
		return nil, fmt.Errorf("task_id 为空")
	}
	url := fmt.Sprintf("%s%s?task_ids=%s&apikey=%s", BaseURL, QueryPath, taskID, c.APIKey)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("apikey", c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("FRW 401, token 失效")
	}

	// FRW's /query has been seen to return data as either a map keyed
	// by task_id or as a flat list. Parse defensively.
	var envMap struct {
		Success bool                    `json:"success"`
		Data    map[string]*QueryResult `json:"data"`
	}
	if err := json.Unmarshal(raw, &envMap); err == nil && envMap.Success && envMap.Data != nil {
		if e, ok := envMap.Data[taskID]; ok && e != nil {
			e.TaskID = taskID
			return e, nil
		}
		// empty but success — treat as still pending.
		return &QueryResult{TaskID: taskID, Status: 0}, nil
	}
	var envList struct {
		Success bool           `json:"success"`
		Data    []*QueryResult `json:"data"`
	}
	if err := json.Unmarshal(raw, &envList); err == nil && envList.Success {
		for _, e := range envList.Data {
			if e != nil && e.TaskID == taskID {
				return e, nil
			}
		}
		return &QueryResult{TaskID: taskID, Status: 0}, nil
	}

	// Third fallback: neither strict-struct shape matched. Parse data
	// as generic and lift whatever fields exist under any name variant.
	// Guards against FRW shipping yet another field naming change that
	// used to black-hole Query into "格式异常".
	var envGeneric struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envGeneric); err == nil && envGeneric.Success && len(envGeneric.Data) > 0 {
		if r, ok := parseQueryDataGeneric(envGeneric.Data, taskID); ok {
			return r, nil
		}
		return &QueryResult{TaskID: taskID, Status: 0}, nil
	}

	return nil, fmt.Errorf("FRW query 响应格式异常: %s", truncate(string(raw), 200))
}

// parseQueryDataGeneric reads a Query /data/ payload regardless of
// whether it's shipped as a map (keyed by task id) or a list. Each
// entry is walked as a generic map so camelCase (taskID) / snake_case
// (task_id) / id field-name drift doesn't matter, and numeric fields
// accept both numbers and numeric strings.
func parseQueryDataGeneric(raw json.RawMessage, taskID string) (*QueryResult, bool) {
	// Try map shape first: { "task_id": { ... } }
	var asMap map[string]interface{}
	if err := json.Unmarshal(raw, &asMap); err == nil {
		if row, ok := asMap[taskID].(map[string]interface{}); ok {
			return pickQueryFields(row, taskID), true
		}
	}
	// Fall through to list shape: [ { "task_id": ..., ... }, ... ]
	var asList []interface{}
	if err := json.Unmarshal(raw, &asList); err == nil {
		for _, item := range asList {
			row, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			id, _ := row["task_id"].(string)
			if id == "" {
				id, _ = row["taskID"].(string)
			}
			if id == "" {
				id, _ = row["id"].(string)
			}
			if id == taskID {
				return pickQueryFields(row, taskID), true
			}
		}
	}
	return nil, false
}

func pickQueryFields(row map[string]interface{}, taskID string) *QueryResult {
	r := &QueryResult{TaskID: taskID}
	if v, ok := numericAny(row["status"]); ok {
		r.Status = int(v)
	}
	if v, ok := numericAny(row["progress"]); ok {
		r.Progress = v
	}
	for _, k := range []string{"out_data", "outData", "output", "url"} {
		if v, ok := row[k].(string); ok && v != "" {
			r.OutData = v
			break
		}
	}
	if v, ok := row["message"].(string); ok {
		r.Message = v
	}
	return r
}

// Download fetches the final output URL (image/video) into memory.
// Caller is responsible for writing it to disk. We limit to 200 MB to
// avoid eating RAM on a misbehaving response.
// allowedFRWHosts is the whitelist of domains we permit downloads from.
var allowedFRWHosts = []string{
	"aiaiartist.com",
	"frw-dreamaiai-api2.aiaiartist.com",
}

func isAllowedFRWHost(host string) bool {
	host = strings.ToLower(host)
	for _, h := range allowedFRWHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

func Download(rawURL string) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("URL 解析失败: %w", err)
	}
	if !isAllowedFRWHost(u.Hostname()) {
		return nil, "", fmt.Errorf("下载域名不在白名单内: %s", u.Hostname())
	}
	resp, err := httpx.Outbound(60 * time.Second).Get(rawURL)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("下载失败 HTTP %d", resp.StatusCode)
	}
	const maxBytes = 200 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxBytes {
		return nil, "", fmt.Errorf("文件过大 (>200MB)")
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
