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
	"seo-system/internal/httpx"

	"github.com/gin-gonic/gin"
)

const yandexAPI = "https://api.webmaster.yandex.net/v4"

func GetYandexAuthURL(c *gin.Context) {
	uid := c.GetInt64("user_id")
	clientID, _, _, err := LoadUserOAuthClient(uid, "yandex")
	if err != nil || clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请先在「个人中心 → Yandex」填写 Client ID / Client Secret"})
		return
	}

	state, err := newOAuthState(uid)
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

	clientID, clientSecret, _, err := LoadUserOAuthClient(uid, "yandex")
	if err != nil || clientID == "" || clientSecret == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "Yandex Client ID / Secret 未填写"})
		return
	}

	resp, err := httpx.Outbound(30*time.Second).PostForm("https://oauth.yandex.ru/token", url.Values{
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

	tok := &OAuthToken{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}
	if err := saveUserOAuthToken(uid, "yandex", tok); err != nil {
		log.Printf("[yandex] persist token uid=%d: %v", uid, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "Token 存储失败"})
		return
	}

	c.Redirect(http.StatusFound, "/?oauth=yandex&status=success")
}

// refreshYandexToken performs a refresh_token grant using the user's own
// Yandex OAuth app credentials. Called from loadAndRefreshOAuth.
func refreshYandexToken(userID int64, refreshToken string) (string, error) {
	clientID, clientSecret, _, err := LoadUserOAuthClient(userID, "yandex")
	if err != nil || clientID == "" || clientSecret == "" {
		return "", fmt.Errorf("Yandex OAuth 应用凭据缺失，请在「个人中心 → Yandex」填写")
	}
	resp, err := httpx.Outbound(30*time.Second).PostForm("https://oauth.yandex.ru/token", url.Values{
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
		return "", fmt.Errorf("Yandex token refresh 返回为空 (HTTP %d)", resp.StatusCode)
	}
	newExpiry := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	if _, err := db.DB.Exec(
		`UPDATE user_oauth_tokens SET access_token = $1, expires_at = $2, updated_at = CURRENT_TIMESTAMP
		 WHERE user_id = $3 AND service = 'yandex'`,
		MustEncryptToken(result.AccessToken), newExpiry, userID,
	); err != nil {
		return "", fmt.Errorf("Yandex token 存储失败: %w", err)
	}
	return result.AccessToken, nil
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

	client := httpx.Outbound(15 * time.Second)
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

	client := httpx.Outbound(15 * time.Second)
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

func getYandexAccessToken(userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("未登录")
	}
	return loadAndRefreshOAuth(userID, "yandex")
}

// yandexUserIDCacheTTL bounds how long a cached Yandex user id is trusted.
// Without a TTL, swapping the Yandex account leaves sync permanently broken.
const yandexUserIDCacheTTL = 6 * time.Hour

// ClearYandexUserIDCache forces the next Yandex sync to re-fetch the user id.
// Pass userID=0 to clear all users' caches (admin operation).
func ClearYandexUserIDCache(userID int64) {
	if userID == 0 {
		db.DB.Exec("DELETE FROM settings WHERE key LIKE 'yandex_user_id:%'")
		return
	}
	key := fmt.Sprintf("yandex_user_id:%d", userID)
	db.DB.Exec("DELETE FROM settings WHERE key = $1", key)
}

// YandexUserIDCacheStats returns the count of cached user id rows.
// Pass userID=0 to count all users (admin operation).
func YandexUserIDCacheStats(userID int64) int {
	var count int
	if userID == 0 {
		db.DB.QueryRow("SELECT COUNT(*) FROM settings WHERE key LIKE 'yandex_user_id:%'").Scan(&count)
	} else {
		key := fmt.Sprintf("yandex_user_id:%d", userID)
		var v string
		db.DB.QueryRow("SELECT value FROM settings WHERE key = $1", key).Scan(&v)
		if v != "" {
			count = 1
		}
	}
	return count
}

// getYandexUserIDCached gets userID from DB cache first, fetches and saves if missing.
// The cache is keyed to the current access token so rotating accounts invalidates it,
// and expires after yandexUserIDCacheTTL regardless.
func getYandexUserIDCached(appUserID int64, token string) (string, error) {
	tokenFingerprint := ""
	if len(token) >= 8 {
		tokenFingerprint = token[:8]
	}
	cacheKey := fmt.Sprintf("yandex_user_id:%d", appUserID)
	var cached string
	db.DB.QueryRow("SELECT value FROM settings WHERE key = $1", cacheKey).Scan(&cached)
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
	db.DB.Exec("INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value", cacheKey, payload)
	return userID, nil
}

// SyncYandexData mirrors fetchYandexSiteData() in gsc-dashboard.html:
// Uses search-queries/all/history for trend data (TOTAL_CLICKS, TOTAL_SHOWS, AVG_SHOW_POSITION)
// Uses search-queries/popular for top keywords
func SyncYandexData(c *gin.Context) {
	siteID := c.Param("id")
	if !canAccessSite(c, siteID) {
		return
	}

	var domain string
	err := db.DB.QueryRow("SELECT domain FROM sites WHERE id = $1", siteID).Scan(&domain)
	if err != nil || domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "站点不存在或无域名"})
		return
	}

	token, err := getYandexAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": err.Error()})
		return
	}

	userID, err := getYandexUserIDCached(c.GetInt64("user_id"), token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": err.Error()})
		return
	}

	// Resolve Yandex host_id by domain.
	yandexHostID, err := resolveYandexHost(token, userID, domain, c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": err.Error()})
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
		"INSERT INTO yandex_data (site_id, date, clicks, impressions, index_count, keywords_json, errors_json) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (site_id, date) DO UPDATE SET clicks = EXCLUDED.clicks, impressions = EXCLUDED.impressions, index_count = EXCLUDED.index_count, keywords_json = EXCLUDED.keywords_json, errors_json = EXCLUDED.errors_json",
		siteID, date, totalClicks, totalImpressions, indexCount, keywordsJSON, errorsJSON,
	)
	if sid, err := strconv.ParseInt(siteID, 10, 64); err == nil {
		InvalidateYandex(sid)
	}

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

	return yd == gd || strings.HasSuffix(yd, "."+gd) || strings.HasSuffix(gd, "."+yd)
}

func GetYandexData(c *gin.Context) {
	if !canAccessSite(c, c.Param("id")) {
		return
	}
	siteID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": CachedYandex(siteID)})
}
