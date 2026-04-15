package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

func GetGAAuthURL(c *gin.Context) {
	clientID := db.GetSetting("gsc_client_id") // Shared Google OAuth
	redirectURI := strings.Replace(db.GetSetting("gsc_redirect_uri"), "/gsc/", "/ga/", 1)

	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "Google OAuth未配置"})
		return
	}

	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=https://www.googleapis.com/auth/analytics.readonly&access_type=offline&prompt=consent",
		clientID, redirectURI,
	)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"url": authURL}})
}

func GACallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "缺少授权码"})
		return
	}

	clientID := db.GetSetting("gsc_client_id")
	clientSecret := db.GetSetting("gsc_client_secret")
	redirectURI := strings.Replace(db.GetSetting("gsc_redirect_uri"), "/gsc/", "/ga/", 1)

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", map[string][]string{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURI},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "Token交换失败"})
		return
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	db.DB.Exec(
		"INSERT OR REPLACE INTO oauth_tokens (service, access_token, refresh_token, expires_at) VALUES ('ga', ?, ?, ?)",
		result.AccessToken, result.RefreshToken, time.Now().Add(time.Duration(result.ExpiresIn)*time.Second),
	)

	c.Redirect(http.StatusFound, "/?oauth=ga&status=success")
}

func SyncGAData(c *gin.Context) {
	siteID := c.Param("id")

	var gaPropertyID string
	err := db.DB.QueryRow("SELECT ga_property_id FROM sites WHERE id = ?", siteID).Scan(&gaPropertyID)
	if err != nil || gaPropertyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "站点未配置GA Property ID"})
		return
	}

	token, err := getGAAccessToken()
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": err.Error()})
		return
	}

	// Fetch GA4 report
	url := fmt.Sprintf("https://analyticsdata.googleapis.com/v1beta/properties/%s:runReport", gaPropertyID)

	body := map[string]interface{}{
		"dateRanges": []map[string]string{
			{"startDate": "30daysAgo", "endDate": "today"},
		},
		"dimensions": []map[string]string{
			{"name": "sessionDefaultChannelGroup"},
		},
		"metrics": []map[string]string{
			{"name": "sessions"},
			{"name": "totalUsers"},
			{"name": "screenPageViews"},
			{"name": "bounceRate"},
			{"name": "averageSessionDuration"},
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", url, strings.NewReader(string(jsonBody)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "GA请求失败"})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	date := time.Now().Format("2006-01-02")
	channelsJSON := string(respBody)

	db.DB.Exec(
		"INSERT OR REPLACE INTO ga_data (site_id, date, channels_json) VALUES (?, ?, ?)",
		siteID, date, channelsJSON,
	)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "GA数据同步成功"})
}

func getGAAccessToken() (string, error) {
	var accessToken, refreshToken string
	var expiresAt time.Time
	err := db.DB.QueryRow(
		"SELECT access_token, refresh_token, expires_at FROM oauth_tokens WHERE service = 'ga' ORDER BY id DESC LIMIT 1",
	).Scan(&accessToken, &refreshToken, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("GA未授权")
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
	json.NewDecoder(resp.Body).Decode(&result)

	newExpiry := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	db.DB.Exec("UPDATE oauth_tokens SET access_token = ?, expires_at = ? WHERE service = 'ga'",
		result.AccessToken, newExpiry)

	return result.AccessToken, nil
}

func GetGAData(c *gin.Context) {
	siteID := c.Param("id")

	rows, err := db.DB.Query(
		"SELECT date, sessions, users, pageviews, bounce_rate, avg_duration, channels_json, pages_json FROM ga_data WHERE site_id = ? ORDER BY date DESC LIMIT 30",
		siteID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var data []gin.H
	for rows.Next() {
		var date, channelsJSON, pagesJSON string
		var sessions, users, pageviews int
		var bounceRate, avgDuration float64
		rows.Scan(&date, &sessions, &users, &pageviews, &bounceRate, &avgDuration, &channelsJSON, &pagesJSON)
		data = append(data, gin.H{
			"date": date, "sessions": sessions, "users": users, "pageviews": pageviews,
			"bounce_rate": bounceRate, "avg_duration": avgDuration,
			"channels_json": json.RawMessage(channelsJSON), "pages_json": json.RawMessage(pagesJSON),
		})
	}
	if data == nil {
		data = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}
