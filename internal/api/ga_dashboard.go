package api

// GA4 Dashboard handlers — parallel to gsc_dashboard.go.
//
// GA4 property discovery: we use the Admin API
//   https://analyticsadmin.googleapis.com/v1beta/accountSummaries
// which returns every account the authorized user can see + each
// account's property list in a single call. This means "list all
// properties under the Google account" is cheap (1 HTTP call) even
// when the user has dozens of properties across many accounts.
//
// Like GSC, we don't write-through to ga_data from these handlers.
// The site-detail endpoint does opportunistically populate ga_data
// for the matching local site so the per-site history stays current.

import (
	"encoding/json"
	"log"
	"strings"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/httpx"

	"github.com/gin-gonic/gin"
)

// parseFloatSafe converts a GA4 metric string value to float64. GA4
// returns everything as strings in metricValues; malformed values
// default to 0 rather than bubbling errors.
func parseFloatSafe(s string) float64 {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// ─── Handler: list GA4 properties ──────────────────────────────────────────

// GAListProperties GET /api/v1/ga4/properties
//
// Returns the flattened list of every GA4 property visible to the OAuth
// account, with its account name for grouping. A user typically has
// 1-5 properties so the UI shows them as a flat sortable list.
func GAListProperties(c *gin.Context) {
	token, err := getGAAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	raw, err := gaAdminGET(token, "https://analyticsadmin.googleapis.com/v1beta/accountSummaries?pageSize=200")
	if err != nil {
		log.Printf("[ga4] Admin API error uid=%d: %v", c.GetInt64("user_id"), err)
		msg := translateGAError(err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": msg})
		return
	}
	var resp struct {
		AccountSummaries []struct {
			Account           string `json:"account"`     // "accounts/123"
			DisplayName       string `json:"displayName"` // account friendly name
			PropertySummaries []struct {
				Property     string `json:"property"`    // "properties/456"
				DisplayName  string `json:"displayName"` // property friendly name
				PropertyType string `json:"propertyType"`
			} `json:"propertySummaries"`
		} `json:"accountSummaries"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解析 GA4 响应失败"})
		return
	}
	type prop struct {
		PropertyID   string `json:"property_id"` // bare id, e.g. "456"
		DisplayName  string `json:"display_name"`
		AccountName  string `json:"account_name"`
		PropertyType string `json:"property_type"`
	}
	out := []prop{}
	for _, a := range resp.AccountSummaries {
		for _, p := range a.PropertySummaries {
			// Extract bare id — Admin API returns "properties/XYZ" but
			// the Data API's runReport expects just "XYZ" in the path.
			id := p.Property
			if len(id) > len("properties/") && id[:len("properties/")] == "properties/" {
				id = id[len("properties/"):]
			}
			out = append(out, prop{
				PropertyID:   id,
				DisplayName:  p.DisplayName,
				AccountName:  a.DisplayName,
				PropertyType: p.PropertyType,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

// gaAdminGET is a small GET helper for the GA4 Admin API. The public
// ga4Fetch is POST-only for runReport; this is its read-only sibling.
func gaAdminGET(token, url string) (json.RawMessage, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := httpx.Outbound(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxOAuthResponseBytes))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// ─── Handler: overview across all GA4 properties ───────────────────────────

// GAOverview GET /api/v1/ga4/overview?start_date=&end_date=
//
// Fans out one runReport per property, returns one card per property.
// Mirrors GSCOverview structure so the frontend can reuse its grid.
func GAOverview(c *gin.Context) {
	sd, ed, err := parseDateRange(c, 28)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	effectiveUserID := c.GetInt64("user_id")
	if v := c.Query("user_id"); v != "" {
		if role, _ := c.Get("role"); role == "admin" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				effectiveUserID = n
			}
		}
	}
	token, err := getGAAccessToken(effectiveUserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}

	// Reuse the Admin API once to get the property list.
	rawProps, err := gaAdminGET(token, "https://analyticsadmin.googleapis.com/v1beta/accountSummaries?pageSize=200")
	if err != nil {
		log.Printf("[ga4] Admin API error uid=%d: %v", effectiveUserID, err)
		msg := translateGAError(err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": msg})
		return
	}
	var propsResp struct {
		AccountSummaries []struct {
			DisplayName       string `json:"displayName"`
			PropertySummaries []struct {
				Property    string `json:"property"`
				DisplayName string `json:"displayName"`
			} `json:"propertySummaries"`
		} `json:"accountSummaries"`
	}
	if err := json.Unmarshal(rawProps, &propsResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解析 GA4 响应失败"})
		return
	}

	type card struct {
		PropertyID      string  `json:"property_id"`
		DisplayName     string  `json:"display_name"`
		AccountName     string  `json:"account_name"`
		Sessions        float64 `json:"sessions"`
		TotalUsers      float64 `json:"total_users"`
		NewUsers        float64 `json:"new_users"`
		ScreenPageViews float64 `json:"screen_page_views"`
		BounceRate      float64 `json:"bounce_rate"`
		EngagementRate  float64 `json:"engagement_rate"`
		HasData         bool    `json:"has_data"`
		Error           string  `json:"error,omitempty"`
	}
	// Collect flat list first so the slice size is known up front.
	type propRef struct{ id, disp, acct string }
	var propList []propRef
	for _, a := range propsResp.AccountSummaries {
		for _, p := range a.PropertySummaries {
			id := p.Property
			if len(id) > len("properties/") && id[:len("properties/")] == "properties/" {
				id = id[len("properties/"):]
			}
			propList = append(propList, propRef{id: id, disp: p.DisplayName, acct: a.DisplayName})
		}
	}

	cards := make([]card, len(propList))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	body := map[string]interface{}{
		"dateRanges": []map[string]string{{"startDate": sd, "endDate": ed}},
		"metrics": []map[string]string{
			{"name": "sessions"}, {"name": "totalUsers"}, {"name": "newUsers"},
			{"name": "screenPageViews"}, {"name": "bounceRate"}, {"name": "engagementRate"},
		},
	}
	for i, p := range propList {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, pr propRef) {
			defer wg.Done()
			defer func() { <-sem }()
			cd := card{PropertyID: pr.id, DisplayName: pr.disp, AccountName: pr.acct}
			raw, err := ga4Fetch(token, pr.id, body)
			if err != nil {
				cd.Error = err.Error()
				cards[idx] = cd
				return
			}
			var r struct {
				Rows []struct {
					MetricValues []struct {
						Value string `json:"value"`
					} `json:"metricValues"`
				} `json:"rows"`
			}
			if err := json.Unmarshal(raw, &r); err != nil {
				cd.Error = "解析失败"
				cards[idx] = cd
				return
			}
			if len(r.Rows) > 0 && len(r.Rows[0].MetricValues) >= 6 {
				// Order matches the metrics array above:
				// 0=sessions 1=totalUsers 2=newUsers 3=pageViews 4=bounceRate 5=engagementRate
				cd.Sessions = parseFloatSafe(r.Rows[0].MetricValues[0].Value)
				cd.TotalUsers = parseFloatSafe(r.Rows[0].MetricValues[1].Value)
				cd.NewUsers = parseFloatSafe(r.Rows[0].MetricValues[2].Value)
				cd.ScreenPageViews = parseFloatSafe(r.Rows[0].MetricValues[3].Value)
				cd.BounceRate = parseFloatSafe(r.Rows[0].MetricValues[4].Value)
				cd.EngagementRate = parseFloatSafe(r.Rows[0].MetricValues[5].Value)
				cd.HasData = cd.Sessions > 0 || cd.TotalUsers > 0
			}
			cards[idx] = cd
		}(i, p)
	}
	wg.Wait()

	// Filter out properties the user has hidden via × on the overview.
	hidden := loadPlatformHides(c.GetInt64("user_id"), "ga4")
	filtered := make([]card, 0, len(cards))
	for _, cd := range cards {
		if hidden[cd.PropertyID] {
			continue
		}
		filtered = append(filtered, cd)
	}
	cards = filtered

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"start_date":     sd,
		"end_date":       ed,
		"property_count": len(cards),
		"cards":          cards,
	}})
}

// ─── Handler: GA4 property detail ──────────────────────────────────────────

// GAPropertyDetail GET /api/v1/ga4/property-detail?property_id=&start_date=&end_date=
func GAPropertyDetail(c *gin.Context) {
	propertyID := c.Query("property_id")
	if propertyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "property_id 必填"})
		return
	}
	sd, ed, err := parseDateRange(c, 28)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}
	effectiveUserID := c.GetInt64("user_id")
	if v := c.Query("user_id"); v != "" {
		if role, _ := c.Get("role"); role == "admin" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				effectiveUserID = n
			}
		}
	}
	token, err := getGAAccessToken(effectiveUserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": err.Error()})
		return
	}

	dateRange := []map[string]string{{"startDate": sd, "endDate": ed}}

	// Six slices, mirrors SyncGAData but date-range aware.
	type slice struct {
		name string
		body map[string]interface{}
	}
	slices := []slice{
		{"overview", map[string]interface{}{
			"dateRanges": dateRange,
			"metrics": []map[string]string{
				{"name": "sessions"}, {"name": "totalUsers"}, {"name": "newUsers"},
				{"name": "screenPageViews"}, {"name": "bounceRate"}, {"name": "averageSessionDuration"},
				{"name": "engagementRate"},
			},
		}},
		{"channels", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "sessionDefaultChannelGroup"}},
			"metrics":    []map[string]string{{"name": "sessions"}, {"name": "totalUsers"}, {"name": "screenPageViews"}},
			"limit":      10,
		}},
		{"pages", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "pagePath"}},
			"metrics": []map[string]string{
				{"name": "screenPageViews"}, {"name": "totalUsers"}, {"name": "averageSessionDuration"}, {"name": "bounceRate"},
			},
			"orderBys": []map[string]interface{}{
				{"metric": map[string]string{"metricName": "screenPageViews"}, "desc": true},
			},
			"limit": 20,
		}},
		{"devices", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "deviceCategory"}},
			"metrics":    []map[string]string{{"name": "sessions"}, {"name": "totalUsers"}},
		}},
		{"countries", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "country"}},
			"metrics":    []map[string]string{{"name": "sessions"}, {"name": "totalUsers"}},
			"orderBys":   []map[string]interface{}{{"metric": map[string]string{"metricName": "sessions"}, "desc": true}},
			"limit":      10,
		}},
		{"trend", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "date"}},
			"metrics":    []map[string]string{{"name": "sessions"}, {"name": "totalUsers"}, {"name": "screenPageViews"}},
			"orderBys":   []map[string]interface{}{{"dimension": map[string]string{"dimensionName": "date"}}},
		}},
	}

	result := make(map[string]json.RawMessage)
	errs := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, s := range slices {
		wg.Add(1)
		go func(name string, body map[string]interface{}) {
			defer wg.Done()
			raw, err := ga4Fetch(token, propertyID, body)
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

	if errs["overview"] != "" {
		log.Printf("[ga4] overview error uid=%d: %v", c.GetInt64("user_id"), errs)
		c.JSON(http.StatusBadGateway, gin.H{"code": -1, "error": "GA4 API: " + errs["overview"]})
		return
	}

	// Opportunistic sync into ga_data, same pattern as GSC.
	go saveGAOverviewAsSync(propertyID, ed, result["overview"])

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"property_id": propertyID,
		"start_date":  sd,
		"end_date":    ed,
		"overview":    result["overview"],
		"channels":    result["channels"],
		"pages":       result["pages"],
		"devices":     result["devices"],
		"countries":   result["countries"],
		"trend":       result["trend"],
		"errors":      errs,
	}})
}

// saveGAOverviewAsSync: match a GA4 property back to one of our local
// sites and write the overview row into ga_data. Best-effort.
func saveGAOverviewAsSync(propertyID, date string, overviewRaw json.RawMessage) {
	if len(overviewRaw) == 0 {
		return
	}
	var resp struct {
		Rows []struct {
			MetricValues []struct {
				Value string `json:"value"`
			} `json:"metricValues"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(overviewRaw, &resp); err != nil || len(resp.Rows) == 0 {
		return
	}
	mv := resp.Rows[0].MetricValues
	if len(mv) < 4 {
		return
	}
	var siteID int64
	db.DB.QueryRow("SELECT id FROM sites WHERE ga_property_id = $1 LIMIT 1", propertyID).Scan(&siteID)
	if siteID == 0 {
		return
	}
	db.DB.Exec(`
		INSERT INTO ga_data (site_id, date, sessions, users, pageviews, bounce_rate, avg_duration) VALUES ($1, $2, $3, $4, $5, $6, 0) ON CONFLICT (site_id, date) DO UPDATE SET sessions = EXCLUDED.sessions, users = EXCLUDED.users, pageviews = EXCLUDED.pageviews, bounce_rate = EXCLUDED.bounce_rate, avg_duration = EXCLUDED.avg_duration`,
		siteID, date,
		int64(parseFloatSafe(mv[0].Value)), // sessions
		int64(parseFloatSafe(mv[1].Value)), // users (totalUsers)
		int64(parseFloatSafe(mv[3].Value)), // pageviews (screenPageViews)
		func() float64 {
			if len(mv) > 4 {
				return parseFloatSafe(mv[4].Value) // bounceRate
			}
			return 0
		}(),
	)
}

// translateGAError turns raw Google API errors into user-friendly Chinese messages.
func translateGAError(raw string) string {
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "admin api has not been used") || strings.Contains(lower, "service_disabled") {
		return "GA4 Admin API 未启用：请在 Google Cloud Console 中启用 Google Analytics Admin API 和 Google Analytics Data API，然后等待几分钟后重试"
	}
	if strings.Contains(lower, "data api has not been used") {
		return "GA4 Data API 未启用：请在 Google Cloud Console 中启用 Google Analytics Data API，然后等待几分钟后重试"
	}
	if strings.Contains(lower, "invalid_grant") || strings.Contains(lower, "token has been expired") {
		return "GA4 授权已过期，请在个人中心重新授权 GA4"
	}
	if strings.Contains(lower, "permission_denied") {
		return "GA4 权限不足：请确认 OAuth 应用已启用 Analytics Admin API 和 Analytics Data API"
	}
	if strings.Contains(lower, "quota") {
		return "GA4 API 配额超限，请稍后重试"
	}
	return "GA4 接口错误：" + raw
}
