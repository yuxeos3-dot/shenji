package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

func GetGSCAuthURL(c *gin.Context) {
	clientID := db.GetSetting("gsc_client_id")
	redirectURI := db.GetSetting("gsc_redirect_uri")

	if clientID == "" || redirectURI == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "GSC OAuth未配置"})
		return
	}

	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=https://www.googleapis.com/auth/webmasters.readonly+https://www.googleapis.com/auth/analytics.readonly&access_type=offline&prompt=consent",
		clientID, redirectURI,
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
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
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

	endDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

	keywordsJSON, _ := fetchGSCSearchAnalytics(token, gscSiteURL, startDate, endDate, "query")
	pagesJSON, _ := fetchGSCSearchAnalytics(token, gscSiteURL, startDate, endDate, "page")
	devicesJSON, _ := fetchGSCSearchAnalytics(token, gscSiteURL, startDate, endDate, "device")
	countriesJSON, _ := fetchGSCSearchAnalytics(token, gscSiteURL, startDate, endDate, "country")

	var totalClicks, totalImpressions int
	var avgCTR, avgPosition float64

	type gscRow struct {
		Clicks      int     `json:"clicks"`
		Impressions int     `json:"impressions"`
		CTR         float64 `json:"ctr"`
		Position    float64 `json:"position"`
	}
	var gscRows []gscRow
	json.Unmarshal([]byte(keywordsJSON), &gscRows)
	for _, r := range gscRows {
		totalClicks += r.Clicks
		totalImpressions += r.Impressions
		avgCTR += r.CTR
		avgPosition += r.Position
	}
	if len(gscRows) > 0 {
		avgCTR /= float64(len(gscRows))
		avgPosition /= float64(len(gscRows))
	}

	db.DB.Exec(
		`INSERT OR REPLACE INTO gsc_data (site_id, date, clicks, impressions, ctr, position, keywords_json, pages_json, devices_json, countries_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, endDate, totalClicks, totalImpressions, avgCTR, avgPosition,
		keywordsJSON, pagesJSON, devicesJSON, countriesJSON,
	)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "GSC数据同步成功"})
}

func fetchGSCSearchAnalytics(token, siteURL, startDate, endDate, dimension string) (string, error) {
	apiURL := fmt.Sprintf("https://www.googleapis.com/webmasters/v3/sites/%s/searchAnalytics/query", siteURL)

	body := map[string]interface{}{
		"startDate":  startDate,
		"endDate":    endDate,
		"dimensions": []string{dimension},
		"rowLimit":   20,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "[]", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "[]", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Rows []interface{} `json:"rows"`
	}
	json.Unmarshal(respBody, &result)
	if result.Rows == nil {
		return "[]", nil
	}
	rowsJSON, _ := json.Marshal(result.Rows)
	return string(rowsJSON), nil
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
		rows.Scan(&date, &clicks, &impressions, &ctr, &position, &keywords, &pages, &devices, &countries)
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
