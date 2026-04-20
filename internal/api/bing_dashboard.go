package api

// Bing Dashboard handlers.
//
// Unlike GSC/GA/Yandex, Bing uses an API key (no OAuth) — the same key
// works for every site the user added to Bing Webmaster Tools under
// that account. We use the GetUserSites endpoint to enumerate them.
//
// Date format quirk: the Bing JSON API wants "YYYY-MM-DDTHH:MM:SS"
// (ISO 8601 with time). We append T00:00:00 to whatever the user
// picked in the date range.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Handler: list Bing sites ──────────────────────────────────────────────

// BingListSites GET /api/v1/bing/sites
//
// Bing returns sites under the API key via GetUserSites. Each site has
// a URL and a verification status; we pass both through so the UI can
// gray out unverified ones.
func BingListSites(c *gin.Context) {
	apiKey, _ := LoadUserBingKey(c.GetInt64("user_id"))
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请先在「个人中心 → Bing」填写 API Key"})
		return
	}
	raw, err := bingFetch(apiKey, "GetUserSites", nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "Bing API: " + err.Error()})
		return
	}
	// Response shape: {"d":[{"Url":"https://...","IsVerified":true,...}, ...]}
	var resp struct {
		D []struct {
			URL        string `json:"Url"`
			IsVerified bool   `json:"IsVerified"`
		} `json:"d"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解析 Bing 响应失败"})
		return
	}
	sites := make([]gin.H, 0, len(resp.D))
	for _, s := range resp.D {
		sites = append(sites, gin.H{
			"site_url":     s.URL,
			"display_name": bingDisplayName(s.URL),
			"verified":     s.IsVerified,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": sites})
}

// bingDisplayName strips scheme/trailing slash for compact card titles.
func bingDisplayName(u string) string {
	out := u
	for _, p := range []string{"https://", "http://"} {
		if len(out) >= len(p) && out[:len(p)] == p {
			out = out[len(p):]
		}
	}
	// drop trailing slash if present
	for len(out) > 0 && out[len(out)-1] == '/' {
		out = out[:len(out)-1]
	}
	return out
}

// ─── Handler: overview across all Bing sites ───────────────────────────────

// BingOverview GET /api/v1/bing/overview?start_date=&end_date=
//
// Bing's "overview" isn't a single call — we have to query GetQueryStats
// for each site and sum the rows within the range. Keeps the card shape
// consistent with GSC/GA/Yandex.
func BingOverview(c *gin.Context) {
	sd, ed, err := parseDateRange(c, 28)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	apiKey, _ := LoadUserBingKey(c.GetInt64("user_id"))
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请先在「个人中心 → Bing」填写 API Key"})
		return
	}
	// List sites first.
	rawSites, err := bingFetch(apiKey, "GetUserSites", nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "Bing API: " + err.Error()})
		return
	}
	var sitesResp struct {
		D []struct {
			URL        string `json:"Url"`
			IsVerified bool   `json:"IsVerified"`
		} `json:"d"`
	}
	if err := json.Unmarshal(rawSites, &sitesResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解析 Bing 响应失败"})
		return
	}

	type card struct {
		SiteURL     string  `json:"site_url"`
		DisplayName string  `json:"display_name"`
		Verified    bool    `json:"verified"`
		Clicks      float64 `json:"clicks"`
		Impressions float64 `json:"impressions"`
		CTR         float64 `json:"ctr"`
		AvgPosition float64 `json:"avg_position"`
		HasData     bool    `json:"has_data"`
		Error       string  `json:"error,omitempty"`
	}
	cards := make([]card, len(sitesResp.D))
	sem := make(chan struct{}, 4) // Bing rate-limits harder than Google; stay conservative
	var wg sync.WaitGroup

	startDate := sd + "T00:00:00"
	endDate := ed + "T00:00:00"

	for i, s := range sitesResp.D {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, siteURL string, verified bool) {
			defer wg.Done()
			defer func() { <-sem }()
			cd := card{SiteURL: siteURL, DisplayName: bingDisplayName(siteURL), Verified: verified}
			if !verified {
				// Unverified sites can't be queried; skip fast.
				cards[idx] = cd
				return
			}
			raw, err := bingFetch(apiKey, "GetQueryStats", map[string]string{
				"siteUrl":   siteURL,
				"startDate": startDate,
				"endDate":   endDate,
			})
			if err != nil {
				cd.Error = friendlyBingError(err.Error(), nil)
				cards[idx] = cd
				return
			}
			// Bing returns {"d":[{"Clicks":N,"Impressions":M,"AvgImpressionPosition"|"AvgClickPosition":P,
			// "Date":"/Date(...)/", "__type":"...", "Query":"..."}, ...]}
			//
			// Two reasons we can't use a tight struct Unmarshal:
			//   1. Which position field exists varies — some deployments
			//      return AvgImpressionPosition, others only AvgClickPosition.
			//   2. Dates come as Microsoft JSON date strings ("/Date(...)/");
			//      if we bound them to a numeric field by mistake the whole
			//      payload fails to parse.
			// Solution: decode the envelope as RawMessage, then walk rows as
			// generic maps — any per-field type mismatch is an ignored
			// skip, not a full-payload abort.
			var envelope struct {
				D json.RawMessage `json:"d"`
			}
			if err := json.Unmarshal(raw, &envelope); err != nil {
				cd.Error = friendlyBingError(string(raw), err)
				cards[idx] = cd
				return
			}
			if len(envelope.D) == 0 || string(envelope.D) == "null" {
				// {"d":null} or {"d":[]} — site registered but no data for
				// the date range; not an error, just blank numbers.
				cards[idx] = cd
				return
			}
			var rows []map[string]interface{}
			if err := json.Unmarshal(envelope.D, &rows); err != nil {
				cd.Error = friendlyBingError(string(raw), err)
				cards[idx] = cd
				return
			}
			var weightedPos float64
			for _, row := range rows {
				clicks := asFloat(row["Clicks"])
				imp := asFloat(row["Impressions"])
				cd.Clicks += clicks
				cd.Impressions += imp
				// Position field name varies by Bing deployment. Prefer
				// AvgImpressionPosition if present, else AvgClickPosition.
				pos := asFloat(row["AvgImpressionPosition"])
				if pos == 0 {
					pos = asFloat(row["AvgClickPosition"])
				}
				weightedPos += pos * imp
			}
			if cd.Impressions > 0 {
				cd.CTR = cd.Clicks / cd.Impressions
				cd.AvgPosition = weightedPos / cd.Impressions
			}
			cd.HasData = cd.Clicks > 0 || cd.Impressions > 0
			cards[idx] = cd
		}(i, s.URL, s.IsVerified)
	}
	wg.Wait()

	// Filter out sites the user has hidden via × on the overview.
	hidden := loadPlatformHides(c.GetInt64("user_id"), "bing")
	filtered := make([]card, 0, len(cards))
	for _, cd := range cards {
		if hidden[cd.SiteURL] {
			continue
		}
		filtered = append(filtered, cd)
	}
	cards = filtered

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"start_date": sd,
		"end_date":   ed,
		"site_count": len(cards),
		"cards":      cards,
	}})
}

// asFloat coerces a generic interface{} (from a map parsed via
// json.Unmarshal into map[string]interface{}) into a float64, accepting
// either a numeric JSON value or a numeric string like "82". Anything
// else (bool / nil / struct / malformed string) returns 0. Used by the
// Bing overview parser so a single weird row doesn't nuke the whole
// site card.
func asFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		if x == "" {
			return 0
		}
		var f float64
		// strconv would be overkill; Fprintf-round-trip via fmt.Sscanf.
		if _, err := fmt.Sscanf(x, "%f", &f); err == nil {
			return f
		}
	}
	return 0
}

// friendlyBingError maps a Bing Webmaster API error (either a fetch
// error string or a raw response body that failed to parse) into a
// short Chinese explanation for the overview card.
//
// Bing returns a mix of formats depending on what went wrong:
//   - Valid JSON but empty/unverified sites → {"d": null} or {"d": []}
//     (not actually an error, just handled upstream)
//   - 401/403 auth issues → XML/HTML error page
//   - Invalid site URL or no permission → {"Message": "...",
//     "ExceptionType": "...", "ExceptionMessage": "..."}
//   - Transient 5xx → HTML gateway page
//
// jsonErr is non-nil when the caller already tried to Unmarshal the
// raw body and it failed; we fall through to pattern matching in that
// case to pull out whatever human-readable bits we can.
func friendlyBingError(raw string, jsonErr error) string {
	low := strings.ToLower(raw)
	// Trim noise but keep enough for ops to grep.
	trimmed := raw
	if len(trimmed) > 500 {
		trimmed = trimmed[:500] + "…"
	}

	// Try structured decode — Bing's .NET-style error envelope.
	var asErr struct {
		Message          string `json:"Message"`
		ExceptionMessage string `json:"ExceptionMessage"`
		ExceptionType    string `json:"ExceptionType"`
		ErrorCode        string `json:"ErrorCode"`
	}
	if err := json.Unmarshal([]byte(raw), &asErr); err == nil {
		parts := []string{}
		if asErr.Message != "" {
			parts = append(parts, asErr.Message)
		}
		if asErr.ExceptionMessage != "" && asErr.ExceptionMessage != asErr.Message {
			parts = append(parts, asErr.ExceptionMessage)
		}
		if asErr.ErrorCode != "" {
			parts = append(parts, "code="+asErr.ErrorCode)
		}
		if len(parts) > 0 {
			return "❌ Bing 接口错误：" + strings.Join(parts, " · ")
		}
	}

	switch {
	case strings.Contains(raw, "HTTP 401"), strings.Contains(low, "unauthorized"),
		strings.Contains(low, "access denied"), strings.Contains(low, "invalid api key"):
		return "❌ Bing API Key 无效或未授权，请到「个人中心 → Bing」检查 / 重新填入"
	case strings.Contains(raw, "HTTP 403"), strings.Contains(low, "forbidden"):
		return "❌ 无权访问该站点的 Bing 数据（通常是站点未在当前 API Key 对应的账号下完成验证）"
	case strings.Contains(raw, "HTTP 404"), strings.Contains(low, "not found"):
		return "❌ Bing 上找不到该站点（可能已被删除，或 siteUrl 拼写不一致）"
	case strings.Contains(raw, "HTTP 429"), strings.Contains(low, "rate limit"):
		return "❌ Bing API 触发限流（请求过多），请等几分钟后刷新"
	case strings.Contains(raw, "HTTP 5"), strings.Contains(low, "internal server"),
		strings.Contains(low, "bad gateway"), strings.Contains(low, "service unavailable"):
		return "❌ Bing 后端临时不可用（5xx），稍后再试"
	case strings.Contains(low, "<!doctype"), strings.Contains(low, "<html"):
		// HTML page usually means WAF / auth redirect; strip tags.
		return "❌ Bing 返回了 HTML 页面（可能是认证跳转 / 拦截），请核对 API Key 是否正确"
	case strings.Contains(low, "timeout"), strings.Contains(low, "deadline"):
		return "❌ 调用 Bing API 超时，稍后刷新"
	}

	// Fallback: include whatever the raw body says so the user can at
	// least see Bing's literal text rather than a bare "解析失败".
	if jsonErr != nil && trimmed != "" {
		return "❌ Bing 返回格式异常（无法解析）。响应片段：" + trimmed
	}
	if trimmed != "" {
		return "❌ Bing 接口错误：" + trimmed
	}
	return "❌ Bing 接口错误：未知错误"
}

// ─── Handler: Bing site detail ─────────────────────────────────────────────

// BingSiteDetail GET /api/v1/bing/site-detail?site_url=&start_date=&end_date=
func BingSiteDetail(c *gin.Context) {
	siteURL := c.Query("site_url")
	if siteURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "site_url 必填"})
		return
	}
	sd, ed, err := parseDateRange(c, 28)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	apiKey, _ := LoadUserBingKey(c.GetInt64("user_id"))
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请先在「个人中心 → Bing」填写 API Key"})
		return
	}
	startDate := sd + "T00:00:00"
	endDate := ed + "T00:00:00"

	// Parallel fan-out: queries, pages, crawl, index, errors.
	type slice struct {
		name     string
		endpoint string
		params   map[string]string
	}
	slices := []slice{
		{"queries", "GetQueryStats", map[string]string{"siteUrl": siteURL, "startDate": startDate, "endDate": endDate}},
		{"pages", "GetPageStats", map[string]string{"siteUrl": siteURL, "startDate": startDate, "endDate": endDate}},
		{"crawl", "GetCrawlStats", map[string]string{"siteUrl": siteURL}},
		{"index", "GetIndexStats", map[string]string{"siteUrl": siteURL}},
		{"errors", "GetCrawlIssues", map[string]string{"siteUrl": siteURL}},
	}

	result := make(map[string]json.RawMessage)
	errs := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, s := range slices {
		wg.Add(1)
		go func(name, endpoint string, params map[string]string) {
			defer wg.Done()
			raw, err := bingFetch(apiKey, endpoint, params)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[name] = err.Error()
				return
			}
			result[name] = raw
		}(s.name, s.endpoint, s.params)
	}
	wg.Wait()

	// Don't 502 on a single sub-endpoint failure — return whatever we
	// got plus a per-endpoint error map so the frontend can render
	// partial data ("the queries succeeded but pages failed because
	// rate-limit") instead of just blanking the whole page.
	// `friendlyBingError` runs over each error string so the frontend
	// gets the same actionable text the overview cards use.
	friendlyErrs := make(map[string]string, len(errs))
	for k, v := range errs {
		friendlyErrs[k] = friendlyBingError(v, nil)
	}

	go saveBingOverviewAsSync(siteURL, ed, result["queries"])

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"site_url":   siteURL,
		"start_date": sd,
		"end_date":   ed,
		"queries":    result["queries"],
		"pages":      result["pages"],
		"crawl":      result["crawl"],
		"index":      result["index"],
		"errors":     result["errors"],
		"api_errors": friendlyErrs,
	}})
}

// saveBingOverviewAsSync: best-effort mirror to bing_data table. Same
// keyword-sum approach as the overview card.
func saveBingOverviewAsSync(siteURL, date string, queriesRaw json.RawMessage) {
	if len(queriesRaw) == 0 {
		return
	}
	var r struct {
		D []struct {
			Clicks                float64 `json:"Clicks"`
			Impressions           float64 `json:"Impressions"`
			AvgImpressionPosition float64 `json:"AvgImpressionPosition"`
		} `json:"d"`
	}
	if err := json.Unmarshal(queriesRaw, &r); err != nil {
		return
	}
	var clicks, impr, weightedPos float64
	for _, row := range r.D {
		clicks += row.Clicks
		impr += row.Impressions
		weightedPos += row.AvgImpressionPosition * row.Impressions
	}
	if clicks == 0 && impr == 0 {
		return
	}
	ctr := 0.0
	avgPos := 0.0
	if impr > 0 {
		ctr = clicks / impr
		avgPos = weightedPos / impr
	}

	var siteID int64
	db.DB.QueryRow("SELECT id FROM sites WHERE bing_site_url = $1 LIMIT 1", siteURL).Scan(&siteID)
	if siteID == 0 {
		return
	}
	db.DB.Exec(`
		INSERT INTO bing_data (site_id, date, clicks, impressions, ctr, avg_position) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (site_id, date) DO UPDATE SET clicks = EXCLUDED.clicks, impressions = EXCLUDED.impressions, ctr = EXCLUDED.ctr, avg_position = EXCLUDED.avg_position`,
		siteID, date, int64(clicks), int64(impr), ctr, avgPos)
}
