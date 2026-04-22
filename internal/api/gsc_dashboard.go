package api

// GSC Dashboard handlers — used by the new "overview → detail" data
// management page.
//
// Design choices:
//   - All 3 handlers take start_date and end_date (ISO 8601 YYYY-MM-DD).
//     The UI offers presets (yesterday, last 7/28/90 days, custom range).
//   - No DB write-through. We call Google directly and forward the
//     result. Data-management is "live view", not a sync. The old
//     SyncGSCData stays for the site-detail page's per-site history.
//   - Site list comes from Google's Search Console API
//     (webmasters/v3/sites) — not from our local sites table. That's
//     the whole point of the refactor: the account-level view shows
//     everything GSC can see, so URL mismatches don't blind us.
//   - Overview aggregates client-side (front-end) from the per-site
//     results. This keeps the backend dumb and the logic visible.

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Date helpers ───────────────────────────────────────────────────────────

// parseDateRange reads start_date/end_date query params and validates them.
// GSC API only returns data from the last 16 months; we enforce that bound
// server-side so a silly date picker can't produce empty results without
// explanation.
func parseDateRange(c *gin.Context, defaultDays int) (start, end string, err error) {
	end = c.Query("end_date")
	start = c.Query("start_date")

	// Default: last `defaultDays` days ending yesterday (GSC data lags ~2 days).
	if end == "" {
		end = time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	}
	if start == "" {
		start = time.Now().AddDate(0, 0, -defaultDays-1).Format("2006-01-02")
	}

	sd, e1 := time.Parse("2006-01-02", start)
	ed, e2 := time.Parse("2006-01-02", end)
	if e1 != nil || e2 != nil {
		return "", "", fmt.Errorf("日期格式错误，需为 YYYY-MM-DD")
	}
	if sd.After(ed) {
		return "", "", fmt.Errorf("开始日期不能晚于结束日期")
	}
	// GSC stores the last ~16 months; 17 gives a bit of slack.
	earliest := time.Now().AddDate(0, -17, 0)
	if sd.Before(earliest) {
		return "", "", fmt.Errorf("GSC 最多支持查询近 16 个月数据")
	}
	return start, end, nil
}

// ─── Handler: list all GSC sites under the authorized Google account ───────

// GSCListSites GET /api/v1/gsc/sites
//
// Returns whatever sites the OAuth account has access to. Includes both
// domain properties ("sc-domain:example.com") and URL-prefix properties
// ("https://example.com/"). permissionLevel is passed through so the
// frontend can filter siteFullUser vs siteRestrictedUser if needed —
// users with only "restrictedUser" can't query searchAnalytics.
func GSCListSites(c *gin.Context) {
	token, err := getGoogleAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	raw, err := gscFetch(token, gscAPI+"/sites", "GET", nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "GSC API 调用失败: " + err.Error()})
		return
	}
	// Pass-through: {"siteEntry":[{"siteUrl":"...","permissionLevel":"..."}, ...]}
	var resp struct {
		SiteEntry []struct {
			SiteURL         string `json:"siteUrl"`
			PermissionLevel string `json:"permissionLevel"`
		} `json:"siteEntry"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解析 GSC 响应失败"})
		return
	}
	sites := make([]gin.H, 0, len(resp.SiteEntry))
	for _, s := range resp.SiteEntry {
		sites = append(sites, gin.H{
			"site_url":         s.SiteURL,
			"permission_level": s.PermissionLevel,
			// Friendly display: strip scheme/trailing slash for URL-prefix
			// properties, strip "sc-domain:" for domain properties.
			"display_name": displayNameForGSCSite(s.SiteURL),
			"is_domain":    strings.HasPrefix(s.SiteURL, "sc-domain:"),
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": sites})
}

// displayNameForGSCSite returns a compact human-friendly label for a GSC
// property identifier. Used by the frontend card titles.
func displayNameForGSCSite(siteURL string) string {
	if strings.HasPrefix(siteURL, "sc-domain:") {
		return strings.TrimPrefix(siteURL, "sc-domain:")
	}
	d := strings.TrimPrefix(siteURL, "https://")
	d = strings.TrimPrefix(d, "http://")
	return strings.TrimRight(d, "/")
}

// ─── Handler: overview across all sites (parallel fan-out) ──────────────────

// GSCOverview GET /api/v1/gsc/overview?start_date=&end_date=
//
// Fans out a /searchAnalytics/query (no dimensions) request to every site
// the account has access to. Sums the KPIs for the top strip and returns
// the per-site rows as cards.
//
// Failures on individual sites are surfaced in `errors[]`. We never hard-
// fail the whole overview on one bad site — partial data is much more
// useful to the admin than a red error banner.
func GSCOverview(c *gin.Context) {
	sd, ed, err := parseDateRange(c, 28)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	token, err := getGoogleAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}

	// Step 1: list sites
	rawSites, err := gscFetch(token, gscAPI+"/sites", "GET", nil)
	if err != nil {
		// Log server-side too so ops can grep journalctl for the real
		// Google error body when a user reports a 502. err.Error()
		// carries the HTTP status + the Google JSON payload (scope
		// mismatch / invalid_grant / quota exhausted etc.).
		log.Printf("[gsc-overview] /sites list failed user=%d: %v", c.GetInt64("user_id"), err)
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "获取 GSC 站点列表失败：" + err.Error()})
		return
	}
	var sitesResp struct {
		SiteEntry []struct {
			SiteURL         string `json:"siteUrl"`
			PermissionLevel string `json:"permissionLevel"`
		} `json:"siteEntry"`
	}
	if err := json.Unmarshal(rawSites, &sitesResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解析站点列表失败"})
		return
	}

	// Step 2: fan out overview queries. Cap concurrency so a 100-site
	// account doesn't spray Google with 100 parallel requests — 8 at a
	// time is plenty for response time and stays polite.
	type siteCard struct {
		SiteURL     string  `json:"site_url"`
		DisplayName string  `json:"display_name"`
		Clicks      float64 `json:"clicks"`
		Impressions float64 `json:"impressions"`
		CTR         float64 `json:"ctr"`
		Position    float64 `json:"position"`
		HasData     bool    `json:"has_data"`
		Error       string  `json:"error,omitempty"`
	}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	cards := make([]siteCard, len(sitesResp.SiteEntry))

	for i, site := range sitesResp.SiteEntry {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, siteURL string) {
			defer wg.Done()
			defer func() { <-sem }()

			card := siteCard{SiteURL: siteURL, DisplayName: displayNameForGSCSite(siteURL)}
			encoded := url.QueryEscape(siteURL)
			q := map[string]interface{}{"startDate": sd, "endDate": ed}
			raw, err := gscFetch(token, gscAPI+"/sites/"+encoded+"/searchAnalytics/query", "POST", q)
			if err != nil {
				card.Error = err.Error()
				cards[idx] = card
				return
			}
			var r struct {
				Rows []struct {
					Clicks      float64 `json:"clicks"`
					Impressions float64 `json:"impressions"`
					CTR         float64 `json:"ctr"`
					Position    float64 `json:"position"`
				} `json:"rows"`
			}
			if err := json.Unmarshal(raw, &r); err != nil {
				card.Error = "解析失败"
				cards[idx] = card
				return
			}
			if len(r.Rows) > 0 {
				card.Clicks = r.Rows[0].Clicks
				card.Impressions = r.Rows[0].Impressions
				card.CTR = r.Rows[0].CTR
				card.Position = r.Rows[0].Position
				card.HasData = card.Clicks > 0 || card.Impressions > 0
			}
			cards[idx] = card
		}(i, site.SiteURL)
	}
	wg.Wait()

	// Filter out sites the current user has hidden via × on the overview
	// page. Defense in depth — the frontend also filters client-side.
	hidden := loadPlatformHides(c.GetInt64("user_id"), "gsc")
	filtered := make([]siteCard, 0, len(cards))
	for _, cd := range cards {
		if hidden[cd.SiteURL] {
			continue
		}
		filtered = append(filtered, cd)
	}
	cards = filtered

	// Step 3: return per-site cards only. Per user decision: no
	// aggregate total strip — each site's numbers speak for themselves.

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"start_date": sd,
		"end_date":   ed,
		"site_count": len(cards),
		"cards":      cards,
	}})
}

// ─── Handler: detailed data for a single GSC site ──────────────────────────

// GSCSiteDetail GET /api/v1/gsc/site-detail?site_url=...&start_date=&end_date=
//
// This is the account-level counterpart to SyncGSCData. The difference:
//   - SyncGSCData is keyed by our sites.id and writes to gsc_data table;
//     used by the per-site detail page.
//   - GSCSiteDetail is keyed by site_url (the GSC property identifier);
//     writes nothing. Used by the data-management card-click detail view.
func GSCSiteDetail(c *gin.Context) {
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
	token, err := getGoogleAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}

	encoded := url.QueryEscape(siteURL)
	base := gscAPI + "/sites/" + encoded + "/searchAnalytics/query"

	// Parallel fetch all 6 slices. Reuses the pattern from SyncGSCData but
	// doesn't persist anything. Using a struct slice rather than a map so
	// iteration order stays deterministic (helps with debugging).
	type slice struct {
		name string
		body map[string]interface{}
	}
	slices := []slice{
		{"overview", map[string]interface{}{"startDate": sd, "endDate": ed}},
		{"trend", map[string]interface{}{"startDate": sd, "endDate": ed, "dimensions": []string{"date"}, "rowLimit": 500}},
		{"keywords", map[string]interface{}{"startDate": sd, "endDate": ed, "dimensions": []string{"query"}, "rowLimit": 500}},
		{"pages", map[string]interface{}{"startDate": sd, "endDate": ed, "dimensions": []string{"page"}, "rowLimit": 100}},
		{"devices", map[string]interface{}{"startDate": sd, "endDate": ed, "dimensions": []string{"device"}, "rowLimit": 10}},
		{"countries", map[string]interface{}{"startDate": sd, "endDate": ed, "dimensions": []string{"country"}, "rowLimit": 10}},
	}

	result := make(map[string]json.RawMessage)
	errs := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, s := range slices {
		wg.Add(1)
		go func(name string, body map[string]interface{}) {
			defer wg.Done()
			raw, err := gscFetch(token, base, "POST", body)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[name] = err.Error()
				return
			}
			result[name] = raw
		}(s.name, s.body)
	}
	wg.Wait()

	// If overview itself failed, the site probably isn't queryable (e.g.
	// permissionLevel=siteRestrictedUser). Return the error front-and-
	// center so the UI can show "no access" instead of "no data".
	if errs["overview"] != "" {
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "GSC API: " + errs["overview"]})
		return
	}

	// Also record this as a best-effort sync for the local gsc_data table
	// if we can match the GSC property to one of our sites. This keeps the
	// per-site detail page's sync history populated even when admins use
	// the overview path. Best-effort = silent on mismatch.
	go saveOverviewAsSync(siteURL, sd, ed, result["overview"])

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"site_url":   siteURL,
		"start_date": sd,
		"end_date":   ed,
		"overview":   result["overview"],
		"trend":      result["trend"],
		"keywords":   result["keywords"],
		"pages":      result["pages"],
		"devices":    result["devices"],
		"countries":  result["countries"],
		"errors":     errs,
	}})
}

// saveOverviewAsSync opportunistically writes the overview numbers into
// gsc_data if this GSC property maps to one of our local sites. Runs in
// a goroutine so it never blocks the HTTP response. Failures are
// swallowed — this table is a nice-to-have historical series, not the
// source of truth.
func saveOverviewAsSync(gscSiteURL, sd, ed string, overviewRaw json.RawMessage) {
	if len(overviewRaw) == 0 {
		return
	}
	var resp struct {
		Rows []struct {
			Clicks      float64 `json:"clicks"`
			Impressions float64 `json:"impressions"`
			CTR         float64 `json:"ctr"`
			Position    float64 `json:"position"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(overviewRaw, &resp); err != nil || len(resp.Rows) == 0 {
		return
	}
	// Match to local site by gsc_site_url or by domain fragment.
	var siteID int64
	db.DB.QueryRow("SELECT id FROM sites WHERE gsc_site_url = $1 LIMIT 1", gscSiteURL).Scan(&siteID)
	if siteID == 0 {
		return
	}
	row := resp.Rows[0]
	db.DB.Exec(`
		INSERT INTO gsc_data (site_id, date, clicks, impressions, ctr, position) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (site_id, date) DO UPDATE SET clicks = EXCLUDED.clicks, impressions = EXCLUDED.impressions, ctr = EXCLUDED.ctr, position = EXCLUDED.position`,
		siteID, ed, int64(row.Clicks), int64(row.Impressions), row.CTR, row.Position)
}
