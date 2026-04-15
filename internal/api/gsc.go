package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

const gscAPI = "https://www.googleapis.com/webmasters/v3"

func GetGSCAuthURL(c *gin.Context) {
	clientID := db.GetSetting("gsc_client_id")
	redirectURI := db.GetSetting("gsc_redirect_uri")

	if clientID == "" || redirectURI == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "GSC OAuth未配置"})
		return
	}

	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&access_type=offline&prompt=consent",
		clientID, redirectURI,
		url.QueryEscape("https://www.googleapis.com/auth/webmasters.readonly https://www.googleapis.com/auth/analytics.readonly"),
	)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"url": authURL}})
}

func GSCCallback(c *gin.Context) {
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

	db.DB.Exec(
		"INSERT OR REPLACE INTO oauth_tokens (service, access_token, refresh_token, expires_at) VALUES ('gsc', ?, ?, ?)",
		token.AccessToken, token.RefreshToken, token.ExpiresAt,
	)

	c.Redirect(http.StatusFound, "/?oauth=gsc&status=success")
}

type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
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
	var accessToken, refreshToken string
	var expiresAt time.Time
	err := db.DB.QueryRow(
		"SELECT access_token, refresh_token, expires_at FROM oauth_tokens WHERE service = 'gsc' ORDER BY id DESC LIMIT 1",
	).Scan(&accessToken, &refreshToken, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("GSC未授权")
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
		result.AccessToken, newExpiry)

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

	respBody, _ := io.ReadAll(resp.Body)
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
		{"keywords", queryReq{StartDate: sd, EndDate: ed, Dimensions: []string{"query"}, RowLimit: 20}},
		{"pages", queryReq{StartDate: sd, EndDate: ed, Dimensions: []string{"page"}, RowLimit: 20}},
		{"devices", queryReq{StartDate: sd, EndDate: ed, Dimensions: []string{"device"}, RowLimit: 10}},
		{"countries", queryReq{StartDate: sd, EndDate: ed, Dimensions: []string{"country"}, RowLimit: 10}},
	}

	results := make(map[string]json.RawMessage)
	for _, q := range queries {
		data, err := gscFetch(token, baseURL, "POST", q.body)
		if err != nil {
			results[q.name] = json.RawMessage("null")
			continue
		}
		results[q.name] = data
	}

	// Parse overview for totals
	var overview struct {
		Rows []struct {
			Clicks      int     `json:"clicks"`
			Impressions int     `json:"impressions"`
			CTR         float64 `json:"ctr"`
			Position    float64 `json:"position"`
		} `json:"rows"`
	}
	json.Unmarshal(results["overview"], &overview)

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

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"message":    "GSC数据同步成功",
			"clicks":     totalClicks,
			"impressions": totalImpressions,
			"ctr":        avgCTR,
			"position":   avgPosition,
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
	siteID := c.Param("id")
	days := c.DefaultQuery("days", "30")

	rows, err := db.DB.Query(
		"SELECT date, clicks, impressions, ctr, position, keywords_json, pages_json, devices_json, countries_json FROM gsc_data WHERE site_id = ? ORDER BY date DESC LIMIT ?",
		siteID, days,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var data []gin.H
	for rows.Next() {
		var date, keywords, pages, devices, countries string
		var clicks, impressions int
		var ctr, position float64
		if err := rows.Scan(&date, &clicks, &impressions, &ctr, &position, &keywords, &pages, &devices, &countries); err != nil {
			continue
		}
		data = append(data, gin.H{
			"date": date, "clicks": clicks, "impressions": impressions,
			"ctr": ctr, "position": position,
			"keywords_json": json.RawMessage(keywords), "pages_json": json.RawMessage(pages),
			"devices_json": json.RawMessage(devices), "countries_json": json.RawMessage(countries),
		})
	}
	if data == nil {
		data = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}
