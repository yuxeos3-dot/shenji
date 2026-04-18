package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/httpx"

	"github.com/gin-gonic/gin"
)

const bingAPI = "https://ssl.bing.com/webmaster/api.svc/json"

func bingFetch(apiKey, endpoint string, params map[string]string) (json.RawMessage, error) {
	u, err := url.Parse(bingAPI + "/" + endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("apikey", apiKey)
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := httpx.Outbound(20 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Cap response size to protect against upstream misbehavior. Bing's
	// largest normal response (GetRankAndTrafficStats on a busy site)
	// sits comfortably under 1 MiB.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxOAuthResponseBytes))
	if resp.StatusCode != 200 {
		var errResp struct {
			Error struct {
				Code    int    `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		}
		json.Unmarshal(body, &errResp)
		if errResp.Error.Message != "" {
			return nil, fmt.Errorf("Bing API错误: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("Bing API HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func SyncBingData(c *gin.Context) {
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

	apiKey, _ := LoadUserBingKey(c.GetInt64("user_id"))
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "请先在「个人中心 → Bing」填写 API Key"})
		return
	}

	// Resolve actual Bing site URL by domain.
	bingSiteURL, err := resolveBingSiteURL(apiKey, domain, c.GetInt64("user_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": -1, "error": err.Error()})
		return
	}

	// Date range: last 30 days
	ed := time.Now().AddDate(0, 0, -1)
	sd := time.Now().AddDate(0, 0, -30)
	startDate := sd.Format("2006-01-02T00:00:00")
	endDate := ed.Format("2006-01-02T00:00:00")

	type fetchDef struct {
		name     string
		endpoint string
		params   map[string]string
	}

	queries := []fetchDef{
		// Query stats (keywords)
		{"queries", "GetQueryStats", map[string]string{
			"siteUrl":   bingSiteURL,
			"startDate": startDate,
			"endDate":   endDate,
		}},
		// Page stats
		{"pages", "GetPageStats", map[string]string{
			"siteUrl":   bingSiteURL,
			"startDate": startDate,
			"endDate":   endDate,
		}},
		// Crawl stats
		{"crawl", "GetCrawlStats", map[string]string{
			"siteUrl": bingSiteURL,
		}},
		// Index stats
		{"index", "GetIndexStats", map[string]string{
			"siteUrl": bingSiteURL,
		}},
		// Crawl issues / errors
		{"errors", "GetCrawlIssues", map[string]string{
			"siteUrl": bingSiteURL,
		}},
	}

	results := make(map[string]json.RawMessage)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, q := range queries {
		wg.Add(1)
		go func(name, endpoint string, params map[string]string) {
			defer wg.Done()
			data, err := bingFetch(apiKey, endpoint, params)
			mu.Lock()
			if err != nil {
				results[name] = json.RawMessage("null")
			} else {
				results[name] = data
			}
			mu.Unlock()
		}(q.name, q.endpoint, q.params)
	}
	wg.Wait()

	toStr := func(r json.RawMessage) string {
		if r == nil {
			return "null"
		}
		return string(r)
	}

	// Parse query stats for totals
	var totalClicks, totalImpressions int
	var avgCTR, avgPosition float64
	if results["queries"] != nil {
		var qr struct {
			D struct {
				Results []struct {
					Clicks      int     `json:"Clicks"`
					Impressions int     `json:"Impressions"`
					CTR         float64 `json:"CTR"`
					AvgPosition float64 `json:"AvgImpPos"`
				} `json:"Results"`
			} `json:"d"`
		}
		if json.Unmarshal(results["queries"], &qr) == nil {
			for _, r := range qr.D.Results {
				totalClicks += r.Clicks
				totalImpressions += r.Impressions
			}
			if len(qr.D.Results) > 0 {
				avgCTR = qr.D.Results[0].CTR
				avgPosition = qr.D.Results[0].AvgPosition
			}
		}
	}

	// Parse index count
	indexCount := 0
	if results["index"] != nil {
		var ir struct {
			D struct {
				IndexedPageCount int `json:"IndexedPageCount"`
			} `json:"d"`
		}
		if json.Unmarshal(results["index"], &ir) == nil {
			indexCount = ir.D.IndexedPageCount
		}
	}

	date := time.Now().Format("2006-01-02")
	db.DB.Exec(`INSERT OR REPLACE INTO bing_data
		(site_id, date, clicks, impressions, ctr, avg_position, index_count,
		 queries_json, pages_json, crawl_json, errors_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, date, totalClicks, totalImpressions, avgCTR, avgPosition, indexCount,
		toStr(results["queries"]), toStr(results["pages"]),
		toStr(results["crawl"]), toStr(results["errors"]),
	)
	if sid, err := strconv.ParseInt(siteID, 10, 64); err == nil {
		InvalidateBing(sid)
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"message":     "Bing 数据同步成功",
		"clicks":      totalClicks,
		"impressions": totalImpressions,
		"ctr":         avgCTR,
		"position":    avgPosition,
		"index_count": indexCount,
	}})
}

func GetBingData(c *gin.Context) {
	if !canAccessSite(c, c.Param("id")) {
		return
	}
	siteID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": CachedBing(siteID)})
}
