package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

func GetYandexAuthURL(c *gin.Context) {
	clientID := db.GetSetting("yandex_client_id")
	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "Yandex OAuth未配置"})
		return
	}

	authURL := fmt.Sprintf(
		"https://oauth.yandex.ru/authorize?response_type=code&client_id=%s",
		clientID,
	)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"url": authURL}})
}

func YandexCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "缺少授权码"})
		return
	}

	clientID := db.GetSetting("yandex_client_id")
	clientSecret := db.GetSetting("yandex_client_secret")

	resp, err := http.PostForm("https://oauth.yandex.ru/token", map[string][]string{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
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
		"INSERT OR REPLACE INTO oauth_tokens (service, access_token, refresh_token, expires_at) VALUES ('yandex', ?, ?, ?)",
		result.AccessToken, result.RefreshToken, time.Now().Add(time.Duration(result.ExpiresIn)*time.Second),
	)

	c.Redirect(http.StatusFound, "/?oauth=yandex&status=success")
}

func SyncYandexData(c *gin.Context) {
	siteID := c.Param("id")

	var yandexHostID string
	err := db.DB.QueryRow("SELECT yandex_host_id FROM sites WHERE id = ?", siteID).Scan(&yandexHostID)
	if err != nil || yandexHostID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "站点未配置Yandex Host ID"})
		return
	}

	token, err := getYandexAccessToken()
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": err.Error()})
		return
	}

	// Get user ID first
	userID, err := getYandexUserID(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "获取Yandex用户ID失败"})
		return
	}

	// Fetch indexing data
	indexCount := fetchYandexIndexCount(token, userID, yandexHostID)

	// Fetch search queries
	keywordsJSON := fetchYandexKeywords(token, userID, yandexHostID)

	// Fetch diagnostics
	errorsJSON := fetchYandexDiagnostics(token, userID, yandexHostID)

	date := time.Now().Format("2006-01-02")
	db.DB.Exec(
		"INSERT OR REPLACE INTO yandex_data (site_id, date, index_count, keywords_json, errors_json) VALUES (?, ?, ?, ?, ?)",
		siteID, date, indexCount, keywordsJSON, errorsJSON,
	)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "Yandex数据同步成功"})
}

func getYandexAccessToken() (string, error) {
	var accessToken string
	var expiresAt time.Time
	err := db.DB.QueryRow(
		"SELECT access_token, expires_at FROM oauth_tokens WHERE service = 'yandex' ORDER BY id DESC LIMIT 1",
	).Scan(&accessToken, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("Yandex未授权")
	}
	return accessToken, nil
}

func getYandexUserID(token string) (string, error) {
	req, _ := http.NewRequest("GET", "https://api.webmaster.yandex.net/v4/user", nil)
	req.Header.Set("Authorization", "OAuth "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		UserID int64 `json:"user_id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return fmt.Sprintf("%d", result.UserID), nil
}

func fetchYandexIndexCount(token, userID, hostID string) int {
	url := fmt.Sprintf("https://api.webmaster.yandex.net/v4/user/%s/hosts/%s/indexing/history", userID, hostID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "OAuth "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	var result struct {
		Indicators map[string][]struct {
			Value int `json:"value"`
		} `json:"indicators"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if docs, ok := result.Indicators["SEARCHABLE"]; ok && len(docs) > 0 {
		return docs[len(docs)-1].Value
	}
	return 0
}

func fetchYandexKeywords(token, userID, hostID string) string {
	url := fmt.Sprintf("https://api.webmaster.yandex.net/v4/user/%s/hosts/%s/query-analytics/list?limit=20&order_by=CLICKS", userID, hostID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "OAuth "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "[]"
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func fetchYandexDiagnostics(token, userID, hostID string) string {
	url := fmt.Sprintf("https://api.webmaster.yandex.net/v4/user/%s/hosts/%s/diagnostics", userID, hostID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "OAuth "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "[]"
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func GetYandexData(c *gin.Context) {
	siteID := c.Param("id")

	rows, err := db.DB.Query(
		"SELECT date, clicks, impressions, index_count, keywords_json, errors_json FROM yandex_data WHERE site_id = ? ORDER BY date DESC LIMIT 30",
		siteID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()

	var data []gin.H
	for rows.Next() {
		var date, keywordsJSON, errorsJSON string
		var clicks, impressions, indexCount int
		rows.Scan(&date, &clicks, &impressions, &indexCount, &keywordsJSON, &errorsJSON)
		data = append(data, gin.H{
			"date": date, "clicks": clicks, "impressions": impressions,
			"index_count": indexCount,
			"keywords_json": json.RawMessage(keywordsJSON),
			"errors_json":   json.RawMessage(errorsJSON),
		})
	}
	if data == nil {
		data = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}
