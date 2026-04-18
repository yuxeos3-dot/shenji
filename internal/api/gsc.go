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
	"seo-system/internal/httpx"

	"github.com/gin-gonic/gin"
)

const gscAPI = "https://www.googleapis.com/webmasters/v3"

// maxOAuthResponseBytes caps provider response size to prevent memory blow-ups.
const maxOAuthResponseBytes = 1 << 20 // 1 MiB

// tokenRefreshMu serializes token refresh so two concurrent requests that
// both see an expired token don't double-refresh. Keyed per (user_id,
// service) since each user has their own token row.
var (
	tokenRefreshMu     sync.Mutex
	tokenRefreshLocks  = map[string]*sync.Mutex{}
	tokenRefreshLocksM sync.Mutex
)

// perUserTokenLock returns a mutex keyed by (user_id, service). Kept in
// a map so each (user, service) refresh path serializes independently
// without blocking unrelated refreshes.
func perUserTokenLock(userID int64, service string) *sync.Mutex {
	key := fmt.Sprintf("%d:%s", userID, service)
	tokenRefreshLocksM.Lock()
	defer tokenRefreshLocksM.Unlock()
	if m, ok := tokenRefreshLocks[key]; ok {
		return m
	}
	m := &sync.Mutex{}
	tokenRefreshLocks[key] = m
	return m
}

func GetGSCAuthURL(c *gin.Context) {
	uid := c.GetInt64("user_id")
	clientID, _, redirectURI, err := LoadUserOAuthClient(uid, "gsc")
	if err != nil || clientID == "" || redirectURI == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请先在「个人中心 → GSC」填写 Client ID / Client Secret / Redirect URI"})
		return
	}

	state, err := newOAuthState(uid)
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
	uid, ok := consumeOAuthState(c.Query("state"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "OAuth state 无效或已过期，请重新发起授权"})
		return
	}
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "缺少授权码"})
		return
	}

	token, err := exchangeGoogleToken(uid, "gsc", code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "Token交换失败: " + err.Error()})
		return
	}

	if err := saveUserOAuthToken(uid, "gsc", token); err != nil {
		log.Printf("[gsc] persist token uid=%d: %v", uid, err)
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

// saveUserOAuthToken upserts the access/refresh/expires row for
// (user_id, service). Encrypts both tokens at rest.
func saveUserOAuthToken(userID int64, service string, tok *OAuthToken) error {
	encAccess := MustEncryptToken(tok.AccessToken)
	encRefresh := ""
	if tok.RefreshToken != "" {
		encRefresh = MustEncryptToken(tok.RefreshToken)
	}
	_, err := db.DB.Exec(
		`INSERT INTO user_oauth_tokens (user_id, service, access_token, refresh_token, expires_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, service) DO UPDATE SET
		   access_token = excluded.access_token,
		   refresh_token = CASE WHEN excluded.refresh_token != '' THEN excluded.refresh_token ELSE user_oauth_tokens.refresh_token END,
		   expires_at = excluded.expires_at,
		   updated_at = CURRENT_TIMESTAMP`,
		userID, service, encAccess, encRefresh, tok.ExpiresAt,
	)
	return err
}

// SaveGSCToken (browser-side OAuth backfill) is deprecated under the
// per-user model — each user must go through the normal Authorize flow
// so their credentials land in user_oauth_tokens under their own
// user_id. Kept as a 410 stub so old clients get a clear error.
func SaveGSCToken(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"code": -1, "error": "该接口已停用，请在「个人中心 → GSC」完成 OAuth 授权"})
}

// exchangeGoogleToken swaps an authorization code for access + refresh
// tokens using the user's own OAuth app credentials.
func exchangeGoogleToken(userID int64, service, code string) (*OAuthToken, error) {
	clientID, clientSecret, redirectURI, err := LoadUserOAuthClient(userID, service)
	if err != nil {
		return nil, fmt.Errorf("未配置 %s OAuth: %w", service, err)
	}
	if clientID == "" || clientSecret == "" || redirectURI == "" {
		return nil, fmt.Errorf("请先在「个人中心 → %s」填写 Client ID / Secret / Redirect URI", strings.ToUpper(service))
	}

	resp, err := httpx.Outbound(30*time.Second).PostForm("https://oauth2.googleapis.com/token", url.Values{
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxOAuthResponseBytes)).Decode(&result); err != nil {
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

// getGoogleAccessToken loads the given user's current GSC access token,
// refreshing with their own refresh_token + app credentials when needed.
// Pass userID = 0 is rejected — every sync request must identify the
// owner of the token.
func getGoogleAccessToken(userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("未登录")
	}
	return loadAndRefreshOAuth(userID, "gsc")
}

// loadAndRefreshOAuth is shared by GSC / GA / Yandex.
func loadAndRefreshOAuth(userID int64, service string) (string, error) {
	loadToken := func() (access, refresh string, expiresAt time.Time, err error) {
		var storedAccess, storedRefresh string
		err = db.DB.QueryRow(
			`SELECT access_token, COALESCE(refresh_token,''), expires_at
			 FROM user_oauth_tokens WHERE user_id = ? AND service = ?`,
			userID, service,
		).Scan(&storedAccess, &storedRefresh, &expiresAt)
		if err != nil {
			return "", "", time.Time{}, fmt.Errorf("%s 未授权，请在「个人中心」完成授权", strings.ToUpper(service))
		}
		access, err = DecryptToken(storedAccess)
		if err != nil {
			return "", "", time.Time{}, fmt.Errorf("%s token 解密失败: %w", service, err)
		}
		if storedRefresh != "" {
			refresh, err = DecryptToken(storedRefresh)
			if err != nil {
				return "", "", time.Time{}, fmt.Errorf("%s refresh token 解密失败: %w", service, err)
			}
		}
		return access, refresh, expiresAt, nil
	}

	accessToken, refreshToken, expiresAt, err := loadToken()
	if err != nil {
		return "", err
	}
	if time.Now().Before(expiresAt) {
		return accessToken, nil
	}
	if refreshToken == "" {
		return "", fmt.Errorf("%s token 已过期且无 refresh_token，请在「个人中心 → %s」重新授权", service, strings.ToUpper(service))
	}

	lock := perUserTokenLock(userID, service)
	lock.Lock()
	defer lock.Unlock()

	// Re-check under the lock.
	if accessToken, _, expiresAt, err = loadToken(); err == nil && time.Now().Before(expiresAt) {
		return accessToken, nil
	}

	// Yandex refresh endpoint differs; GSC and GA both use Google's.
	if service == "yandex" {
		return refreshYandexToken(userID, refreshToken)
	}

	clientID, clientSecret, _, err := LoadUserOAuthClient(userID, service)
	if err != nil || clientID == "" || clientSecret == "" {
		return "", fmt.Errorf("%s OAuth 应用凭据缺失，请在「个人中心」填写 Client ID / Secret", service)
	}

	resp, err := httpx.Outbound(30*time.Second).PostForm("https://oauth2.googleapis.com/token", url.Values{
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxOAuthResponseBytes)).Decode(&result); err != nil {
		return "", err
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("%s token refresh 返回为空 (HTTP %d)", service, resp.StatusCode)
	}

	newExpiry := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	if _, err := db.DB.Exec(
		`UPDATE user_oauth_tokens SET access_token = ?, expires_at = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND service = ?`,
		MustEncryptToken(result.AccessToken), newExpiry, userID, service,
	); err != nil {
		log.Printf("[%s] persist refreshed token uid=%d: %v", service, userID, err)
		return "", fmt.Errorf("%s token 存储失败: %w", service, err)
	}

	return result.AccessToken, nil
}

// gscFetch wraps GSC API calls with auth header (like gf() in gsc-dashboard.html)
func gscFetch(token, apiURL, method string, body interface{}) (json.RawMessage, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("GSC 请求体序列化失败: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, apiURL, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := httpx.Outbound(30 * time.Second)
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
	if !canAccessSite(c, siteID) {
		return
	}

	var domain string
	err := db.DB.QueryRow("SELECT domain FROM sites WHERE id = ?", siteID).Scan(&domain)
	if err != nil || domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "站点不存在或无域名"})
		return
	}

	token, err := getGoogleAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": err.Error()})
		return
	}

	// Look up the actual GSC property that corresponds to this domain.
	// User no longer fills a "GSC Site URL" field; the resolver reads
	// the account's properties and picks the best canonical match.
	gscSiteURL, err := resolveGSCProperty(token, domain)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": err.Error()})
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

	if _, err := db.DB.Exec(
		`INSERT OR REPLACE INTO gsc_data (site_id, date, clicks, impressions, ctr, position, keywords_json, pages_json, devices_json, countries_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, ed, totalClicks, totalImpressions, avgCTR, avgPosition,
		keywordsJSON, pagesJSON, devicesJSON, countriesJSON,
	); err != nil {
		log.Printf("[gsc] persist sync data site=%s failed: %v", siteID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "GSC 数据写入失败"})
		return
	}
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
	if !canAccessSite(c, c.Param("id")) {
		return
	}
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
	if !canAccessSite(c, strconv.FormatInt(siteID, 10)) {
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
