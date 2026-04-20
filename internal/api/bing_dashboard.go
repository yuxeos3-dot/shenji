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
	"net/http"
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
				cd.Error = err.Error()
				cards[idx] = cd
				return
			}
			// Bing returns {"d":[{"Clicks":N,"Impressions":M,"AvgImpressionPosition":P,...}, ...]}
			// Each row is per-keyword; sum for site totals.
			var r struct {
				D []struct {
					Clicks                float64 `json:"Clicks"`
					Impressions           float64 `json:"Impressions"`
					AvgImpressionPosition float64 `json:"AvgImpressionPosition"`
				} `json:"d"`
			}
			if err := json.Unmarshal(raw, &r); err != nil {
				cd.Error = "解析失败"
				cards[idx] = cd
				return
			}
			var weightedPos float64
			for _, row := range r.D {
				cd.Clicks += row.Clicks
				cd.Impressions += row.Impressions
				weightedPos += row.AvgImpressionPosition * row.Impressions
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

	if errs["queries"] != "" {
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "Bing API: " + errs["queries"]})
		return
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
		"api_errors": errs,
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
