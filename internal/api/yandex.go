package api

import (
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

const yandexAPI = "https://api.webmaster.yandex.net/v4"

func GetYandexAuthURL(c *gin.Context) {
	clientID := db.GetSetting("yandex_client_id")
	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "Yandex OAuth未配置"})
		return
	}

	state, err := newOAuthState(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "state 生成失败"})
		return
	}
	authURL := fmt.Sprintf(
		"https://oauth.yandex.ru/authorize?response_type=code&client_id=%s&state=%s",
		clientID, url.QueryEscape(state),
	)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"url": authURL}})
}

func YandexCallback(c *gin.Context) {
	if !requireValidState(c) {
		return
	}
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxOAuthResponseBytes)).Decode(&result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "解析Token响应失败"})
		return
	}
	if result.AccessToken == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "Yandex Token 为空"})
		return
	}

	if _, err := db.DB.Exec(
		"INSERT OR REPLACE INTO oauth_tokens (service, access_token, refresh_token, expires_at) VALUES ('yandex', ?, ?, ?)",
		MustEncryptToken(result.AccessToken), MustEncryptToken(result.RefreshToken),
		time.Now().Add(time.Duration(result.ExpiresIn)*time.Second),
	); err != nil {
		log.Printf("[yandex] persist token failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "Token 存储失败"})
		return
	}

	c.Redirect(http.StatusFound, "/?oauth=yandex&status=success")
}

// yandexFetch wraps Yandex Webmaster API calls (like yf() in gsc-dashboard.html)
func yandexFetch(token, path string) (json.RawMessage, error) {
	apiURL := yandexAPI + path

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "OAuth "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
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

// getYandexUserID mirrors loadYandexUser() in gsc-dashboard.html:
// First tries Yandex login API, then Webmaster /user/ API
func getYandexUserID(token string) (string, error) {
	// Step 1: Try login info API
	req, _ := http.NewRequest("GET", "https://login.yandex.ru/info?format=json", nil)
	req.Header.Set("Authorization", "OAuth "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		var info struct {
			ID  string `json:"id"`
			UID string `json:"uid"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			resp.Body.Close()
			return "", fmt.Errorf("解析Yandex用户信息失败: %w", err)
		}
		loginID := info.ID
		if loginID == "" {
			loginID = info.UID
		}
		if loginID != "" {
			// Step 2: Try Webmaster /user/ to get real user_id
			data, err := yandexFetch(token, "/user/")
			if err == nil {
				var wu struct {
					UserID int64 `json:"user_id"`
				}
				if err := json.Unmarshal(data, &wu); err == nil && wu.UserID > 0 {
					return fmt.Sprintf("%d", wu.UserID), nil
				}
			}
			return loginID, nil
		}
	} else if resp != nil {
		resp.Body.Close()
	}

	// Step 3: Direct Webmaster user API
	data, err := yandexFetch(token, "/user/")
	if err != nil {
		return "", fmt.Errorf("无法获取Yandex用户ID: %w", err)
	}
	var u struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.Unmarshal(data, &u); err != nil {
		return "", fmt.Errorf("解析Yandex Webmaster用户ID失败: %w", err)
	}
	if u.UserID == 0 {
		return "", fmt.Errorf("Yandex用户ID为空")
	}
	return fmt.Sprintf("%d", u.UserID), nil
}

func getYandexAccessToken() (string, error) {
	var stored string
	err := db.DB.QueryRow(
		"SELECT access_token FROM oauth_tokens WHERE service = 'yandex' ORDER BY id DESC LIMIT 1",
	).Scan(&stored)
	if err != nil {
		return "", fmt.Errorf("Yandex未授权")
	}
	plain, err := DecryptToken(stored)
	if err != nil {
		return "", fmt.Errorf("Yandex token 解密失败: %w", err)
	}
	return plain, nil
}

// yandexUserIDCacheTTL bounds how long a cached Yandex user id is trusted.
// Without a TTL, swapping the Yandex account leaves sync permanently broken.
const yandexUserIDCacheTTL = 6 * time.Hour

// getYandexUserIDCached gets userID from DB cache first, fetches and saves if missing.
// The cache is keyed to the current access token so rotating accounts invalidates it,
// and expires after yandexUserIDCacheTTL regardless.
func getYandexUserIDCached(token string) (string, error) {
	tokenFingerprint := ""
	if len(token) >= 8 {
		tokenFingerprint = token[:8]
	}
	var cached string
	db.DB.QueryRow("SELECT value FROM settings WHERE key = 'yandex_user_id'").Scan(&cached)
	if cached != "" {
		parts := strings.SplitN(cached, "|", 3)
		// Legacy entries without fingerprint/ts → treat as stale.
		if len(parts) == 3 && parts[0] == tokenFingerprint {
			if ts, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				if time.Since(time.Unix(ts, 0)) < yandexUserIDCacheTTL {
					return parts[2], nil
				}
			}
		}
	}
	userID, err := getYandexUserID(token)
	if err != nil {
		return "", err
	}
	payload := fmt.Sprintf("%s|%d|%s", tokenFingerprint, time.Now().Unix(), userID)
	db.DB.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('yandex_user_id', ?)", payload)
	return userID, nil
}

// SyncYandexData mirrors fetchYandexSiteData() in gsc-dashboard.html:
// Uses search-queries/all/history for trend data (TOTAL_CLICKS, TOTAL_SHOWS, AVG_SHOW_POSITION)
// Uses search-queries/popular for top keywords
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

	userID, err := getYandexUserIDCached(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}

	days := 30
	ed := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	sd := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	encodedHostID := url.PathEscape(yandexHostID)

	historyPath := fmt.Sprintf("/user/%s/hosts/%s/search-queries/all/history?date_from=%s&date_to=%s&query_indicator=TOTAL_CLICKS&query_indicator=TOTAL_SHOWS&query_indicator=AVG_SHOW_POSITION&query_indicator=AVG_CLICK_POSITION",
		userID, encodedHostID, sd, ed)
	popularPath := fmt.Sprintf("/user/%s/hosts/%s/search-queries/popular?date_from=%s&date_to=%s&order_by=TOTAL_CLICKS&limit=20&query_indicator=TOTAL_CLICKS&query_indicator=TOTAL_SHOWS&query_indicator=AVG_SHOW_POSITION&query_indicator=AVG_CLICK_POSITION",
		userID, encodedHostID, sd, ed)
	diagPath := fmt.Sprintf("/user/%s/hosts/%s/diagnostics", userID, encodedHostID)
	indexPath := fmt.Sprintf("/user/%s/hosts/%s/indexing/history", userID, encodedHostID)

	// Fetch all 4 endpoints in parallel
	type fetchResult struct {
		data json.RawMessage
		err  error
	}
	results := make(map[string]fetchResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, req := range []struct{ name, path string }{
		{"history", historyPath},
		{"popular", popularPath},
		{"diag", diagPath},
		{"index", indexPath},
	} {
		wg.Add(1)
		go func(name, path string) {
			defer wg.Done()
			data, err := yandexFetch(token, path)
			mu.Lock()
			results[name] = fetchResult{data, err}
			mu.Unlock()
		}(req.name, req.path)
	}
	wg.Wait()

	histData := results["history"].data
	histErr := results["history"].err
	popData := results["popular"].data
	diagData := results["diag"].data
	diagErr := results["diag"].err
	indexData := results["index"].data
	indexErr := results["index"].err

	// Parse history data — mirrors gsc-dashboard.html logic:
	// const ind=hist.indicators||{};
	// const clicks_arr=ind.TOTAL_CLICKS||[];
	// const shows_arr=ind.TOTAL_SHOWS||[];
	// totalClicks=clicks_arr.reduce((s,d)=>s+(d.value||0),0);
	var totalClicks, totalImpressions int
	var avgPosition float64

	if histErr == nil {
		var hist struct {
			Indicators map[string][]struct {
				Date  string `json:"date"`
				Value int    `json:"value"`
			} `json:"indicators"`
		}
		if err := json.Unmarshal(histData, &hist); err != nil {
			log.Printf("[yandex] unmarshal history failed: %v", err)
		}

		clicksArr := hist.Indicators["TOTAL_CLICKS"]
		showsArr := hist.Indicators["TOTAL_SHOWS"]
		posArr := hist.Indicators["AVG_SHOW_POSITION"]

		for _, d := range clicksArr {
			totalClicks += d.Value
		}
		for _, d := range showsArr {
			totalImpressions += d.Value
		}
		if len(posArr) > 0 {
			sum := 0
			for _, d := range posArr {
				sum += d.Value
			}
			avgPosition = float64(sum) / float64(len(posArr))
		}
	}

	// Parse popular queries for keywords JSON
	keywordsJSON := "[]"
	if popData != nil {
		// Store the raw popular queries data
		var pop struct {
			Queries []json.RawMessage `json:"queries"`
		}
		if err := json.Unmarshal(popData, &pop); err == nil && pop.Queries != nil {
			kw, _ := json.Marshal(pop.Queries)
			keywordsJSON = string(kw)
		}
	}

	// Store errors from diagnostics
	errorsJSON := "[]"
	if diagErr == nil {
		errorsJSON = string(diagData)
	}

	// Fetch indexing count
	indexCount := 0
	if indexErr == nil {
		var idxResp struct {
			Indicators map[string][]struct {
				Value int `json:"value"`
			} `json:"indicators"`
		}
		if err := json.Unmarshal(indexData, &idxResp); err != nil {
			log.Printf("[yandex] unmarshal index data failed: %v", err)
		}
		if docs, ok := idxResp.Indicators["SEARCHABLE"]; ok && len(docs) > 0 {
			indexCount = docs[len(docs)-1].Value
		}
	}

	date := time.Now().Format("2006-01-02")
	db.DB.Exec(
		"INSERT OR REPLACE INTO yandex_data (site_id, date, clicks, impressions, index_count, keywords_json, errors_json) VALUES (?, ?, ?, ?, ?, ?, ?)",
		siteID, date, totalClicks, totalImpressions, indexCount, keywordsJSON, errorsJSON,
	)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"message":     "Yandex数据同步成功",
			"clicks":      totalClicks,
			"impressions": totalImpressions,
			"index_count": indexCount,
			"position":    avgPosition,
		},
	})
}

// matchYandexToGSC mirrors the matching logic from gsc-dashboard.html
func matchYandexToGSC(yandexHost, gscURL string) bool {
	yd := strings.TrimPrefix(yandexHost, "https://")
	yd = strings.TrimPrefix(yd, "http://")
	yd = strings.TrimRight(yd, "/")
	yd = strings.ToLower(yd)
	// Remove port if any
	if idx := strings.LastIndex(yd, ":"); idx > 0 {
		yd = yd[:idx]
	}

	gd := strings.TrimPrefix(gscURL, "sc-domain:")
	gd = strings.TrimPrefix(gd, "https://")
	gd = strings.TrimPrefix(gd, "http://")
	gd = strings.TrimRight(gd, "/")
	gd = strings.ToLower(gd)

	return yd == gd || strings.HasSuffix(yd, "."+gd) || strings.HasSuffix(gd, "."+yd) || strings.Contains(yd, gd) || strings.Contains(gd, yd)
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
		if err := rows.Scan(&date, &clicks, &impressions, &indexCount, &keywordsJSON, &errorsJSON); err != nil {
			continue
		}
		data = append(data, gin.H{
			"date": date, "clicks": clicks, "impressions": impressions,
			"index_count":   indexCount,
			"keywords_json": json.RawMessage(keywordsJSON),
			"errors_json":   json.RawMessage(errorsJSON),
		})
	}
	if data == nil {
		data = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}
