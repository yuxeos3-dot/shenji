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
	"sync"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

func GetGAAuthURL(c *gin.Context) {
	uid := c.GetInt64("user_id")
	clientID, _, redirectURI, err := LoadUserOAuthClient(uid, "ga")
	if err != nil || clientID == "" || redirectURI == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请先在「个人中心 → GA」填写 Client ID / Secret / Redirect URI"})
		return
	}
	state, err := newOAuthState(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "state 生成失败"})
		return
	}
	scope := "https://www.googleapis.com/auth/analytics.readonly"
	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s&access_type=offline&prompt=consent",
		clientID, redirectURI, scope, url.QueryEscape(state),
	)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"url": authURL}})
}

func GACallback(c *gin.Context) {
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
	tok, err := exchangeGoogleToken(uid, "ga", code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "Token交换失败: " + err.Error()})
		return
	}
	if err := saveUserOAuthToken(uid, "ga", tok); err != nil {
		log.Printf("[ga] persist token uid=%d: %v", uid, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "Token 存储失败"})
		return
	}
	c.Redirect(http.StatusFound, "/?oauth=ga&status=success")
}

func getGAAccessToken(userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("未登录")
	}
	return loadAndRefreshOAuth(userID, "ga")
}

func ga4Fetch(token, propertyID string, body map[string]interface{}) (json.RawMessage, error) {
	apiURL := fmt.Sprintf("https://analyticsdata.googleapis.com/v1beta/properties/%s:runReport", propertyID)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("GA4 请求体序列化失败: %w", err)
	}
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBody))
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
		return nil, fmt.Errorf("GA4 API HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func SyncGAData(c *gin.Context) {
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
	token, err := getGAAccessToken(c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": -1, "error": err.Error()})
		return
	}
	// Resolve GA4 property by domain — user no longer enters a property id.
	gaPropertyID, err := resolveGAProperty(token, domain)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": err.Error()})
		return
	}

	dateRange := []map[string]string{{"startDate": "30daysAgo", "endDate": "yesterday"}}

	type queryDef struct {
		name string
		body map[string]interface{}
	}
	queries := []queryDef{
		// 1. Overview totals
		{"overview", map[string]interface{}{
			"dateRanges": dateRange,
			"metrics": []map[string]string{
				{"name": "sessions"}, {"name": "totalUsers"}, {"name": "newUsers"},
				{"name": "screenPageViews"}, {"name": "bounceRate"}, {"name": "averageSessionDuration"},
				{"name": "engagementRate"},
			},
		}},
		// 2. Channel breakdown
		{"channels", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "sessionDefaultChannelGroup"}},
			"metrics": []map[string]string{
				{"name": "sessions"}, {"name": "totalUsers"}, {"name": "screenPageViews"},
			},
			"limit": 10,
		}},
		// 3. Top pages
		{"pages", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "pagePath"}},
			"metrics": []map[string]string{
				{"name": "screenPageViews"}, {"name": "totalUsers"}, {"name": "averageSessionDuration"},
				{"name": "bounceRate"},
			},
			"orderBys": []map[string]interface{}{
				{"metric": map[string]string{"metricName": "screenPageViews"}, "desc": true},
			},
			"limit": 20,
		}},
		// 4. Device breakdown
		{"devices", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "deviceCategory"}},
			"metrics":    []map[string]string{{"name": "sessions"}, {"name": "totalUsers"}},
		}},
		// 5. Country breakdown
		{"countries", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "country"}},
			"metrics":    []map[string]string{{"name": "sessions"}, {"name": "totalUsers"}},
			"orderBys":   []map[string]interface{}{{"metric": map[string]string{"metricName": "sessions"}, "desc": true}},
			"limit":      10,
		}},
		// 6. Daily trend
		{"trend", map[string]interface{}{
			"dateRanges": dateRange,
			"dimensions": []map[string]string{{"name": "date"}},
			"metrics":    []map[string]string{{"name": "sessions"}, {"name": "totalUsers"}, {"name": "screenPageViews"}},
			"orderBys":   []map[string]interface{}{{"dimension": map[string]string{"dimensionName": "date"}}},
		}},
	}

	// Parallel fetch
	results := make(map[string]json.RawMessage)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, q := range queries {
		wg.Add(1)
		go func(name string, body map[string]interface{}) {
			defer wg.Done()
			data, err := ga4Fetch(token, gaPropertyID, body)
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

	// Parse overview
	var sessions, users, newUsers, pageviews int
	var bounceRate, avgDuration, engagementRate float64
	if results["overview"] != nil {
		var ov struct {
			Rows []struct {
				MetricValues []struct {
					Value string `json:"value"`
				} `json:"metricValues"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(results["overview"], &ov); err != nil {
			log.Printf("[ga] unmarshal overview failed: %v", err)
		}
		if len(ov.Rows) > 0 {
			mv := ov.Rows[0].MetricValues
			if len(mv) > 0 {
				fmt.Sscanf(mv[0].Value, "%d", &sessions)
			}
			if len(mv) > 1 {
				fmt.Sscanf(mv[1].Value, "%d", &users)
			}
			if len(mv) > 2 {
				fmt.Sscanf(mv[2].Value, "%d", &newUsers)
			}
			if len(mv) > 3 {
				fmt.Sscanf(mv[3].Value, "%d", &pageviews)
			}
			if len(mv) > 4 {
				fmt.Sscanf(mv[4].Value, "%f", &bounceRate)
			}
			if len(mv) > 5 {
				fmt.Sscanf(mv[5].Value, "%f", &avgDuration)
			}
			if len(mv) > 6 {
				fmt.Sscanf(mv[6].Value, "%f", &engagementRate)
			}
		}
	}

	toJSON := func(r json.RawMessage) string {
		if r == nil {
			return "null"
		}
		return string(r)
	}

	date := time.Now().Format("2006-01-02")
	db.DB.Exec(`INSERT OR REPLACE INTO ga_data
		(site_id, date, sessions, users, pageviews, bounce_rate, avg_duration, channels_json, pages_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, date, sessions, users, pageviews, bounceRate, avgDuration,
		toJSON(results["channels"]), toJSON(results["pages"]),
	)

	// Store extra GA data in settings-like blob
	extraJSON, _ := json.Marshal(map[string]interface{}{
		"new_users":       newUsers,
		"engagement_rate": engagementRate,
		"devices":         json.RawMessage(toJSON(results["devices"])),
		"countries":       json.RawMessage(toJSON(results["countries"])),
		"trend":           json.RawMessage(toJSON(results["trend"])),
	})
	db.DB.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)",
		fmt.Sprintf("ga_extra_%s", siteID), string(extraJSON))
	if sid, err := strconv.ParseInt(siteID, 10, 64); err == nil {
		InvalidateGA(sid)
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"message":  "GA4 数据同步成功",
		"sessions": sessions, "users": users, "pageviews": pageviews,
		"bounce_rate": bounceRate, "avg_duration": avgDuration,
	}})
}

func GetGAData(c *gin.Context) {
	if !canAccessSite(c, c.Param("id")) {
		return
	}
	siteID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	rows, extra := CachedGA(siteID)
	if extra == nil {
		extra = json.RawMessage("null")
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": rows, "extra": extra})
}
