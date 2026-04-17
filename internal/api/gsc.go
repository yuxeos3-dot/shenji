package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

const gscAPI = "https://www.googleapis.com/webmasters/v3"

// maxOAuthResponseBytes caps provider response size to prevent memory blow-ups.
const maxOAuthResponseBytes = 1 << 20 // 1 MiB

func GetGSCAuthURL(c *gin.Context) {
	clientID := db.GetSetting("gsc_client_id")
	redirectURI := db.GetSetting("gsc_redirect_uri")

	if clientID == "" || redirectURI == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "GSC OAuth未配置"})
		return
	}

	state, err := newOAuthState(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "state 生成失败"})
		return
	}
	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s&access_type=offline&prompt=consent",
		clientID, redirectURI,
		url.QueryEscape("https://www.googleapis.com/auth/webmasters.readonly https://www.googleapis.com/auth/analytics.readonly"),
		url.QueryEscape(state),
	)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"url": authURL}})
}

func GSCCallback(c *gin.Context) {
	if !requireValidState(c) {
		return
	}
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "缺少授权码"})
		return
	}

	token, err := exchangeGoogleToken(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "Token交换失败: " + err.Error()})
		return
	}

	if _, err := db.DB.Exec(
		"INSERT OR REPLACE INTO oauth_tokens (service, access_token, refresh_token, expires_at) VALUES ('gsc', ?, ?, ?)",
		MustEncryptToken(token.AccessToken), MustEncryptToken(token.RefreshToken), token.ExpiresAt,
	); err != nil {
		log.Printf("[gsc] persist token failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "Token 存储失败"})
		return
	}

	c.Redirect(http.StatusFound, "/?oauth=gsc&status=success")
}

type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// SaveGSCToken receives the access token obtained by browser-side OAuth (gsc-dashboard.html)
// and stores it in the DB so server-side GSC sync can use it. Admin-only because
// the token is global: a non-admin push would hijack sync for every site.
func SaveGSCToken(c *gin.Context) {
	if role, _ := c.Get("role"); role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "需要管理员权限"})
		return
	}
	var req struct {
		AccessToken string `json:"access_token" binding:"required"`
		ExpiresIn   int    `json:"expires_in"` // seconds remaining
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "access_token 必填"})
		return
	}
	expiresAt := time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)
	if req.ExpiresIn <= 0 {
		expiresAt = time.Now().Add(55 * time.Minute) // default 55 min
	}
	if _, err := db.DB.Exec(
		"INSERT OR REPLACE INTO oauth_tokens (service, access_token, refresh_token, expires_at) VALUES ('gsc', ?, '', ?)",
		MustEncryptToken(req.AccessToken), expiresAt,
	); err != nil {
		log.Printf("[gsc] save token failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "Token 存储失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "GSC token 已保存"})
}

func exchangeGoogleToken(code string) (*OAuthToken, error) {
	clientID := db.GetSetting("gsc_client_id")
	clientSecret := db.GetSetting("gsc_client_secret")
	redirectURI := db.GetSetting("gsc_redirect_uri")

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", map[string][]string{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURI},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s: %s", result.Error, result.ErrorDesc)
	}

	return &OAuthToken{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}, nil
}

func getGoogleAccessToken() (string, error) {
	var storedAccess, storedRefresh string
	var expiresAt time.Time
	err := db.DB.QueryRow(
		"SELECT access_token, refresh_token, expires_at FROM oauth_tokens WHERE service = 'gsc' ORDER BY id DESC LIMIT 1",
	).Scan(&storedAccess, &storedRefresh, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("GSC未授权")
	}
	accessToken, err := DecryptToken(storedAccess)
	if err != nil {
		return "", fmt.Errorf("GSC token 解密失败: %w", err)
	}
	refreshToken, err := DecryptToken(storedRefresh)
	if err != nil {
		return "", fmt.Errorf("GSC refresh token 解密失败: %w", err)
	}

	if time.Now().Before(expiresAt) {
		return accessToken, nil
	}

	// Refresh token
	clientID := db.GetSetting("gsc_client_id")
	clientSecret := db.GetSetting("gsc_client_secret")

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", map[string][]string{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	newExpiry := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	db.DB.Exec("UPDATE oauth_tokens SET access_token = ?, expires_at = ? WHERE service = 'gsc'",
		MustEncryptToken(result.AccessToken), newExpiry)

	return result.AccessToken, nil
}

// gscFetch wraps GSC API calls with auth header (like gf() in gsc-dashboard.html)
func gscFetch(token, apiURL, method string, body interface{}) (json.RawMessage, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, apiURL, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxOAuthResponseBytes))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// SyncGSCData mirrors the gsc-dashboard.html approach:
// Fetches overview (no dimension), trend (date), keywords (query), pages (page),
// devices (device), countries (country), and previous period for comparison.
func SyncGSCData(c *gin.Context) {
	siteID := c.Param("id")

	var gscSiteURL string
	err := db.DB.QueryRow("SELECT gsc_site_url FROM sites WHERE id = ?", siteID).Scan(&gscSiteURL)
	if err != nil || gscSiteURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "站点未配置GSC URL"})
		return
	}

	token, err := getGoogleAccessToken()
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": err.Error()})
		return
	}

	days := 30
	ed := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	sd := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	encodedURL := url.QueryEscape(gscSiteURL)
	baseURL := fmt.Sprintf("%s/sites/%s/searchAnalytics/query", gscAPI, encodedURL)

	// Parallel fetch: overview, trend, keywords(top20), pages(top20), devices, countries
	// Mirrors gsc-dashboard.html openDetail() approach
	type queryReq struct {
		StartDate  string   `json:"startDate"`
		EndDate    string   `json:"endDate"`
		Dimensions []string `json:"dimensions,omitempty"`
		RowLimit   int      `json:"rowLimit,omitempty"`
	}

	queries := []struct {
		name string
		body queryReq
	}{
		{"overview", queryReq{StartDate: sd, EndDate: ed}},
		{"trend", queryReq{StartDate: sd, EndDate: ed, Dimensions: []string{"date"}, RowLimit: 90}},
		{"keywords", queryReq{StartDate: sd, EndDate: ed, Dimensions: []string{"query"}, RowLimit: 500}},
		{"pages", queryReq{StartDate: sd, EndDate: ed, Dimensions: []string{"page"}, RowLimit: 20}},
		{"devices", queryReq{StartDate: sd, EndDate: ed, Dimensions: []string{"device"}, RowLimit: 10}},
		{"countries", queryReq{StartDate: sd, EndDate: ed, Dimensions: []string{"country"}, RowLimit: 10}},
	}

	results := make(map[string]json.RawMessage)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, q := range queries {
		wg.Add(1)
		go func(name string, body queryReq) {
			defer wg.Done()
			data, err := gscFetch(token, baseURL, "POST", body)
			mu.Lock()
			if err != nil {
				results[name] = json.RawMessage("null")
			} else {
				results[name] = data
			}
			mu.Unlock()
		}(q.name, q.body)
	}
	wg.Wait()

	// Parse overview for totals
	var overview struct {
		Rows []struct {
			Clicks      int     `json:"clicks"`
			Impressions int     `json:"impressions"`
			CTR         float64 `json:"ctr"`
			Position    float64 `json:"position"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(results["overview"], &overview); err != nil {
		log.Printf("[gsc] unmarshal overview failed: %v", err)
	}

	var totalClicks, totalImpressions int
	var avgCTR, avgPosition float64
	if len(overview.Rows) > 0 {
		row := overview.Rows[0]
		totalClicks = row.Clicks
		totalImpressions = row.Impressions
		avgCTR = row.CTR
		avgPosition = row.Position
	}

	// Store all dimensions as JSON
	keywordsJSON := extractRows(results["keywords"])
	pagesJSON := extractRows(results["pages"])
	devicesJSON := extractRows(results["devices"])
	countriesJSON := extractRows(results["countries"])

	db.DB.Exec(
		`INSERT OR REPLACE INTO gsc_data (site_id, date, clicks, impressions, ctr, position, keywords_json, pages_json, devices_json, countries_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, ed, totalClicks, totalImpressions, avgCTR, avgPosition,
		keywordsJSON, pagesJSON, devicesJSON, countriesJSON,
	)
	if sid, err := strconv.ParseInt(siteID, 10, 64); err == nil {
		InvalidateGSC(sid)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"message":     "GSC数据同步成功",
			"clicks":      totalClicks,
			"impressions": totalImpressions,
			"ctr":         avgCTR,
			"position":    avgPosition,
		},
	})
}

// extractRows gets the "rows" field from a GSC API response
func extractRows(raw json.RawMessage) string {
	if raw == nil {
		return "[]"
	}
	var resp struct {
		Rows json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || resp.Rows == nil {
		return "[]"
	}
	return string(resp.Rows)
}

func GetGSCData(c *gin.Context) {
	siteID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days < 1 {
		days = 30
	}
	// Cache-first: CachedGSC lazily loads from DB on miss so the first ever
	// read for a site still works without waiting for a login warm-up.
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": CachedGSC(siteID, days)})
}

// StoreSiteKeywords receives keyword data pushed from browser (gsc-dashboard.html)
// and stores it in gsc_data so backend features can read it without calling GSC API.
func StoreSiteKeywords(c *gin.Context) {
	var req struct {
		SiteURL     string          `json:"site_url" binding:"required"`
		Keywords    json.RawMessage `json:"keywords"`
		Pages       json.RawMessage `json:"pages"`
		Devices     json.RawMessage `json:"devices"`
		Countries   json.RawMessage `json:"countries"`
		Clicks      int             `json:"clicks"`
		Impressions int             `json:"impressions"`
		CTR         float64         `json:"ctr"`
		Position    float64         `json:"position"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	// Find matching site by gsc_site_url or domain
	var siteID int64
	err := db.DB.QueryRow(
		"SELECT id FROM sites WHERE gsc_site_url = ? OR domain = ? LIMIT 1",
		req.SiteURL, req.SiteURL,
	).Scan(&siteID)
	if err != nil {
		// Try partial match (sc-domain vs https://)
		cleanURL := strings.TrimPrefix(strings.TrimPrefix(req.SiteURL, "sc-domain:"), "https://")
		cleanURL = strings.TrimPrefix(cleanURL, "http://")
		cleanURL = strings.TrimRight(cleanURL, "/")
		db.DB.QueryRow(
			"SELECT id FROM sites WHERE REPLACE(REPLACE(REPLACE(gsc_site_url,'sc-domain:',''),'https://',''),'http://','') LIKE ? LIMIT 1",
			"%"+cleanURL+"%",
		).Scan(&siteID)
	}
	if siteID == 0 {
		// No matching site, just store without site_id=0
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": "no matching site"})
		return
	}

	kw := "[]"
	if req.Keywords != nil && string(req.Keywords) != "null" {
		kw = string(req.Keywords)
	}
	pg := "[]"
	if req.Pages != nil && string(req.Pages) != "null" {
		pg = string(req.Pages)
	}
	dv := "{}"
	if req.Devices != nil && string(req.Devices) != "null" {
		dv = string(req.Devices)
	}
	ct := "[]"
	if req.Countries != nil && string(req.Countries) != "null" {
		ct = string(req.Countries)
	}

	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	db.DB.Exec(
		`INSERT OR REPLACE INTO gsc_data (site_id, date, clicks, impressions, ctr, position, keywords_json, pages_json, devices_json, countries_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, date, req.Clicks, req.Impressions, req.CTR, req.Position, kw, pg, dv, ct,
	)
	InvalidateGSC(siteID)

	// keyword_count = number of elements in the keywords JSON array, not comma-split.
	kwCount := 0
	var kwArr []json.RawMessage
	if err := json.Unmarshal([]byte(kw), &kwArr); err == nil {
		kwCount = len(kwArr)
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"site_id": siteID, "keyword_count": kwCount}})
}
