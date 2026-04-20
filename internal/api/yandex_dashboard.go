package api

// Yandex Dashboard handlers.
//
// Yandex API is fundamentally different from GSC/GA:
//   - Every call needs a user_id prefix: /user/{user_id}/hosts/...
//   - Host list is flat under the user — no "account" grouping.
//   - Metrics come back in "indicators" arrays keyed by date, so the
//     overview requires summing a history range on our side rather
//     than querying an aggregate endpoint.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Handler: list all Yandex hosts ────────────────────────────────────────

// YandexListHosts GET /api/v1/yandex/hosts
func YandexListHosts(c *gin.Context) {
	token, err := getYandexAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	userID, err := getYandexUserID(token)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "获取 Yandex 用户 ID 失败: " + err.Error()})
		return
	}
	raw, err := yandexFetch(token, "/user/"+userID+"/hosts")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "Yandex Webmaster API: " + err.Error()})
		return
	}
	var resp struct {
		Hosts []struct {
			HostID            string `json:"host_id"`
			UnicodeHostURL    string `json:"unicode_host_url"`
			MainMirrorURL     string `json:"main_mirror_url"`
			VerificationState struct {
				State string `json:"state"`
			} `json:"verification"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解析 Yandex 响应失败"})
		return
	}
	hosts := make([]gin.H, 0, len(resp.Hosts))
	for _, h := range resp.Hosts {
		name := h.UnicodeHostURL
		if name == "" {
			name = h.MainMirrorURL
		}
		hosts = append(hosts, gin.H{
			"host_id":      h.HostID,
			"display_name": name,
			"verified":     h.VerificationState.State == "VERIFIED",
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": hosts})
}

// ─── Handler: overview across all Yandex hosts ─────────────────────────────

// YandexOverview GET /api/v1/yandex/overview?start_date=&end_date=
func YandexOverview(c *gin.Context) {
	sd, ed, err := parseDateRange(c, 28)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	token, err := getYandexAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	userID, err := getYandexUserID(token)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "获取 Yandex 用户 ID 失败: " + err.Error()})
		return
	}
	raw, err := yandexFetch(token, "/user/"+userID+"/hosts")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "Yandex hosts: " + err.Error()})
		return
	}
	var hostsResp struct {
		Hosts []struct {
			HostID         string `json:"host_id"`
			UnicodeHostURL string `json:"unicode_host_url"`
			MainMirrorURL  string `json:"main_mirror_url"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(raw, &hostsResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解析 Yandex 响应失败"})
		return
	}

	type card struct {
		HostID      string  `json:"host_id"`
		DisplayName string  `json:"display_name"`
		Clicks      float64 `json:"clicks"`
		Shows       float64 `json:"shows"`
		CTR         float64 `json:"ctr"`
		AvgPosition float64 `json:"avg_position"`
		HasData     bool    `json:"has_data"`
		Error       string  `json:"error,omitempty"`
	}
	cards := make([]card, len(hostsResp.Hosts))
	sem := make(chan struct{}, 6) // Yandex rate limits are tighter than Google's
	var wg sync.WaitGroup

	for i, h := range hostsResp.Hosts {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, host struct {
			HostID         string `json:"host_id"`
			UnicodeHostURL string `json:"unicode_host_url"`
			MainMirrorURL  string `json:"main_mirror_url"`
		}) {
			defer wg.Done()
			defer func() { <-sem }()
			name := host.UnicodeHostURL
			if name == "" {
				name = host.MainMirrorURL
			}
			cd := card{HostID: host.HostID, DisplayName: name}
			encHost := url.QueryEscape(host.HostID)
			// search-queries/all/history returns per-date indicator arrays;
			// sum them client-side for a single KPI card.
			path := fmt.Sprintf(
				"/user/%s/hosts/%s/search-queries/all/history?date_from=%s&date_to=%s"+
					"&query_indicator=TOTAL_CLICKS&query_indicator=TOTAL_SHOWS"+
					"&query_indicator=AVG_SHOW_POSITION&query_indicator=AVG_CLICK_POSITION",
				userID, encHost, sd, ed)
			raw, err := yandexFetch(token, path)
			if err != nil {
				cd.Error = err.Error()
				cards[idx] = cd
				return
			}
			cd.Clicks, cd.Shows, cd.AvgPosition = sumYandexIndicators(raw)
			if cd.Shows > 0 {
				cd.CTR = cd.Clicks / cd.Shows
			}
			cd.HasData = cd.Clicks > 0 || cd.Shows > 0
			cards[idx] = cd
		}(i, h)
	}
	wg.Wait()

	// Translate Yandex API error payloads to friendlier Chinese messages
	// before filtering — the user mostly sees HOST_NOT_LOADED for a newly
	// verified host that Webmaster hasn't indexed yet.
	for i := range cards {
		if cards[i].Error != "" {
			cards[i].Error = friendlyYandexError(cards[i].Error)
		}
	}

	// Filter out hosts the user has hidden via × on the overview.
	hidden := loadPlatformHides(c.GetInt64("user_id"), "yandex")
	filtered := make([]card, 0, len(cards))
	for _, cd := range cards {
		if hidden[cd.HostID] {
			continue
		}
		filtered = append(filtered, cd)
	}
	cards = filtered

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"start_date": sd,
		"end_date":   ed,
		"host_count": len(cards),
		"cards":      cards,
	}})
}

// friendlyYandexError maps common Yandex Webmaster error codes to a
// short Chinese explanation. Falls back to the raw error string when we
// don't recognise the code.
//
//	HOST_NOT_LOADED — host verified but Webmaster still indexing (1-3 天)
//	HOST_NOT_VERIFIED — host exists but verification step incomplete
//	ACCESS_DENIED — token expired / lacking scope
//	NOT_FOUND — host/path no longer exists
func friendlyYandexError(raw string) string {
	// Raw is usually "HTTP 404: {...json...}" — look for the code substring
	// rather than parsing JSON, because Yandex occasionally returns HTML.
	switch {
	case strings.Contains(raw, "HOST_NOT_LOADED"):
		return "Yandex Webmaster 正在加载该站点数据，通常验证后 1-3 天内完成。可稍后再查"
	case strings.Contains(raw, "HOST_NOT_VERIFIED"):
		return "该站点尚未在 Yandex Webmaster 完成验证，请到 Yandex 后台验证后再查"
	case strings.Contains(raw, "ACCESS_DENIED"):
		return "Yandex 访问凭证已失效，请到「系统管理 → 数据平台」重新授权"
	case strings.Contains(raw, "NOT_FOUND"):
		return "该站点在 Yandex Webmaster 中已不存在"
	case strings.Contains(raw, "quota") || strings.Contains(raw, "RATE_LIMIT"):
		return "Yandex API 配额已用完，请稍后再查"
	}
	return raw
}

// sumYandexIndicators walks the history payload and totals the TOTAL_CLICKS
// + TOTAL_SHOWS series, plus computes a weighted-by-shows average position.
// Yandex returns data in the shape:
//
//	{ "indicators": {
//	   "TOTAL_CLICKS":       [{"date":"2024-01-01","value":12}, ...],
//	   "TOTAL_SHOWS":        [...],
//	   "AVG_SHOW_POSITION":  [...]
//	}}
func sumYandexIndicators(raw json.RawMessage) (clicks, shows, avgPos float64) {
	var r struct {
		Indicators map[string][]struct {
			Value float64 `json:"value"`
		} `json:"indicators"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return
	}
	for _, p := range r.Indicators["TOTAL_CLICKS"] {
		clicks += p.Value
	}
	for _, p := range r.Indicators["TOTAL_SHOWS"] {
		shows += p.Value
	}
	// Position is per-date average; weight by shows for the period average.
	posPoints := r.Indicators["AVG_SHOW_POSITION"]
	showsPoints := r.Indicators["TOTAL_SHOWS"]
	if len(posPoints) == len(showsPoints) && shows > 0 {
		var weighted float64
		for i := range posPoints {
			weighted += posPoints[i].Value * showsPoints[i].Value
		}
		avgPos = weighted / shows
	}
	return
}

// ─── Handler: Yandex host detail ───────────────────────────────────────────

// YandexHostDetail GET /api/v1/yandex/host-detail?host_id=&start_date=&end_date=
func YandexHostDetail(c *gin.Context) {
	hostID := c.Query("host_id")
	if hostID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "host_id 必填"})
		return
	}
	sd, ed, err := parseDateRange(c, 28)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	token, err := getYandexAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	userID, err := getYandexUserID(token)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "获取 Yandex 用户 ID 失败: " + err.Error()})
		return
	}
	encHost := url.QueryEscape(hostID)

	// Same pattern as SyncYandexData but date-range driven.
	historyPath := fmt.Sprintf(
		"/user/%s/hosts/%s/search-queries/all/history?date_from=%s&date_to=%s"+
			"&query_indicator=TOTAL_CLICKS&query_indicator=TOTAL_SHOWS"+
			"&query_indicator=AVG_SHOW_POSITION&query_indicator=AVG_CLICK_POSITION",
		userID, encHost, sd, ed)
	popularPath := fmt.Sprintf(
		"/user/%s/hosts/%s/search-queries/popular?date_from=%s&date_to=%s"+
			"&order_by=TOTAL_CLICKS&limit=20"+
			"&query_indicator=TOTAL_CLICKS&query_indicator=TOTAL_SHOWS"+
			"&query_indicator=AVG_SHOW_POSITION&query_indicator=AVG_CLICK_POSITION",
		userID, encHost, sd, ed)
	diagPath := fmt.Sprintf("/user/%s/hosts/%s/diagnostics", userID, encHost)
	indexPath := fmt.Sprintf("/user/%s/hosts/%s/indexing/history", userID, encHost)

	type slice struct {
		name, path string
	}
	slices := []slice{
		{"history", historyPath},
		{"popular", popularPath},
		{"diagnostics", diagPath},
		{"indexing", indexPath},
	}

	result := make(map[string]json.RawMessage)
	errs := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, s := range slices {
		wg.Add(1)
		go func(name, path string) {
			defer wg.Done()
			raw, err := yandexFetch(token, path)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[name] = err.Error()
				return
			}
			result[name] = raw
		}(s.name, s.path)
	}
	wg.Wait()

	if errs["history"] != "" {
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "Yandex API: " + errs["history"]})
		return
	}

	go saveYandexOverviewAsSync(hostID, ed, result["history"])

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"host_id":     hostID,
		"start_date":  sd,
		"end_date":    ed,
		"history":     result["history"],
		"popular":     result["popular"],
		"diagnostics": result["diagnostics"],
		"indexing":    result["indexing"],
		"errors":      errs,
	}})
}

func saveYandexOverviewAsSync(hostID, date string, historyRaw json.RawMessage) {
	if len(historyRaw) == 0 {
		return
	}
	clicks, shows, _ := sumYandexIndicators(historyRaw)
	if clicks == 0 && shows == 0 {
		return
	}
	var siteID int64
	db.DB.QueryRow("SELECT id FROM sites WHERE yandex_host_id = $1 LIMIT 1", hostID).Scan(&siteID)
	if siteID == 0 {
		// Try fuzzy match by host URL fragment.
		cleanHost := strings.ReplaceAll(hostID, "https:", "")
		cleanHost = strings.ReplaceAll(cleanHost, "http:", "")
		cleanHost = strings.ReplaceAll(cleanHost, "/", "")
		cleanHost = strings.TrimSpace(cleanHost)
		if cleanHost == "" {
			return
		}
		db.DB.QueryRow("SELECT id FROM sites WHERE REPLACE(REPLACE(domain, 'https://', ''), 'http://', '') = $1 LIMIT 1", cleanHost).Scan(&siteID)
		if siteID == 0 {
			return
		}
	}
	db.DB.Exec(`
		INSERT INTO yandex_data (site_id, date, clicks, impressions) VALUES ($1, $2, $3, $4) ON CONFLICT (site_id, date) DO UPDATE SET clicks = EXCLUDED.clicks, impressions = EXCLUDED.impressions`,
		siteID, date,
		int64(clicks),
		int64(shows),
	)
}
