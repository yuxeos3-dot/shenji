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
	"log"
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

	log.Printf("[frw-client] Query task=%s status=%d body=%s", taskID, resp.StatusCode, truncate(string(raw), 1200))

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("FRW 401, token 失效")
	}

	// FRW's /query response shape has drifted over time — use the generic
	// parser (which walks map / list / nested "generation" object) as the
	// primary path. Falls back to "pending" only when we truly can't find
	// any matching row.
	var envGeneric struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envGeneric); err == nil && envGeneric.Success && len(envGeneric.Data) > 0 {
		if r, ok := parseQueryDataGeneric(envGeneric.Data, taskID); ok {
			return r, nil
		}
		log.Printf("[frw-client] WARN task %s: parseQueryDataGeneric returned no match, treating as pending", taskID)
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
	// Try list shape first (FRW current default):
	// data: [{"taskId": "X", "generation": {...}}]
	var asList []map[string]interface{}
	if err := json.Unmarshal(raw, &asList); err == nil && len(asList) > 0 {
		// First try: find a row whose taskId / task_id / taskID matches
		for _, row := range asList {
			if rowMatchesTaskID(row, taskID) {
				return pickQueryFields(row, taskID), true
			}
		}
		// Fallback: if only one row, use it regardless of id matching
		// (some upstreams echo a different id after normalization)
		if len(asList) == 1 {
			return pickQueryFields(asList[0], taskID), true
		}
	}

	// Try map shape: { "task_id": { ... } }
	var asMap map[string]interface{}
	if err := json.Unmarshal(raw, &asMap); err == nil {
		if row, ok := asMap[taskID].(map[string]interface{}); ok {
			return pickQueryFields(row, taskID), true
		}
	}
	return nil, false
}


// rowMatchesTaskID checks if a row's taskId / task_id / taskID / id field
// matches our task identifier. Handles camelCase / snake_case variants
// across FRW response format changes.
func rowMatchesTaskID(row map[string]interface{}, taskID string) bool {
	for _, k := range []string{"taskId", "task_id", "taskID", "id"} {
		if v, ok := row[k].(string); ok && v == taskID {
			return true
		}
	}
	// Also check nested generation object
	if gen, ok := row["generation"].(map[string]interface{}); ok {
		for _, k := range []string{"taskId", "task_id", "taskID", "id"} {
			if v, ok := gen[k].(string); ok && v == taskID {
				return true
			}
		}
	}
	return false
}

func pickQueryFields(row map[string]interface{}, taskID string) *QueryResult {
	r := &QueryResult{TaskID: taskID}

	// Merge top-level + nested "generation" object if present.
	// FRW's current /query response wraps actual fields inside a
	// "generation" child object: {"taskId": "X", "generation": {...}}.
	// Older shape had them flat at the top. Walk both.
	sources := []map[string]interface{}{row}
	if gen, ok := row["generation"].(map[string]interface{}); ok {
		sources = append(sources, gen)
	}

	for _, src := range sources {
		if r.Status == 0 {
			// status may be numeric (0/1/2/-1) or string ("queued"/"running"/
			// "processing"/"completed"/"failed"). Handle both.
			if v, ok := numericAny(src["status"]); ok {
				r.Status = int(v)
			} else if s, ok := src["status"].(string); ok && s != "" {
				r.Status = stringStatusToInt(s)
			}
		}
		if r.Progress == 0 {
			if v, ok := numericAny(src["progress"]); ok {
				r.Progress = v
			}
		}
		if r.OutData == "" {
			for _, k := range []string{"out_data", "outData", "output", "outputUrl", "output_url", "resultUrl", "result_url", "url"} {
				if v, ok := src[k].(string); ok && v != "" {
					r.OutData = v
					break
				}
			}
			// Also check array-of-URLs fields. FRW's current response
			// uses "resultUrls" (plural, camelCase).
			if r.OutData == "" {
				for _, k := range []string{"resultUrls", "result_urls", "outputs", "output_urls", "results"} {
					if arr, ok := src[k].([]interface{}); ok && len(arr) > 0 {
						if s, ok := arr[0].(string); ok && s != "" {
							r.OutData = s
							break
						}
						// array of objects with url field
						if m, ok := arr[0].(map[string]interface{}); ok {
							for _, uk := range []string{"url", "output_url", "resultUrl"} {
								if s, ok := m[uk].(string); ok && s != "" {
									r.OutData = s
									break
								}
							}
							if r.OutData != "" {
								break
							}
						}
					}
				}
			}
		}
		if r.Message == "" {
			for _, k := range []string{"message", "error_msg", "errorMsg", "msg"} {
				if v, ok := src[k].(string); ok && v != "" {
					r.Message = v
					break
				}
			}
		}
	}

	// If we have a result URL, always treat as completed regardless of
	// what the status field says (safety net: FRW sometimes sets URL
	// before updating status, or uses unknown status strings).
	if r.OutData != "" && r.Status != 2 && r.Status != -1 {
		r.Status = 2
	}

	return r
}

// stringStatusToInt maps FRW string status values to our internal
// numeric codes (0=queued, 1=running, 2=completed, -1=failed, 4=timeout).
func stringStatusToInt(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "completed", "complete", "success", "succeeded", "done", "finish", "finished":
		return 2
	case "failed", "fail", "error", "rejected":
		return -1
	case "timeout", "timedout", "expired":
		return 4
	case "running", "processing", "generating", "in_progress":
		return 1
	case "queued", "pending", "waiting":
		return 0
	}
	// Unknown string status — assume queued so poll keeps going
	return 0
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
