package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"seo-system/internal/db"
)

// syncGSCBasic pulls GSC overview data and stores to gsc_data.
// This is the non-HTTP version of SyncGSCData for use by SyncAndLoad.
func syncGSCBasic(siteID int64, token, gscSiteURL string) {
	days := 30
	ed := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	sd := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	encodedURL := url.QueryEscape(gscSiteURL)
	baseURL := fmt.Sprintf("%s/sites/%s/searchAnalytics/query", gscAPI, encodedURL)

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
		totalClicks = overview.Rows[0].Clicks
		totalImpressions = overview.Rows[0].Impressions
		avgCTR = overview.Rows[0].CTR
		avgPosition = overview.Rows[0].Position
	}

	db.DB.Exec(`INSERT INTO gsc_data (site_id, date, clicks, impressions, ctr, position, keywords_json, pages_json, devices_json, countries_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (site_id, date) DO UPDATE SET clicks=EXCLUDED.clicks, impressions=EXCLUDED.impressions, ctr=EXCLUDED.ctr, position=EXCLUDED.position, keywords_json=EXCLUDED.keywords_json, pages_json=EXCLUDED.pages_json, devices_json=EXCLUDED.devices_json, countries_json=EXCLUDED.countries_json`,
		siteID, ed, totalClicks, totalImpressions, avgCTR, avgPosition,
		extractRows(results["keywords"]), extractRows(results["pages"]),
		extractRows(results["devices"]), extractRows(results["countries"]),
	)
	InvalidateGSC(siteID)
	log.Printf("[sync-basic] GSC site %d: %d clicks, %d impressions", siteID, totalClicks, totalImpressions)
}

// syncGA4Basic pulls GA4 overview data and stores to ga_data.
func syncGA4Basic(siteID int64, token, propertyID string) {
	dateRange := []map[string]string{{"startDate": "30daysAgo", "endDate": "yesterday"}}
	ovBody := map[string]interface{}{
		"dateRanges": dateRange,
		"metrics": []map[string]string{
			{"name": "sessions"}, {"name": "totalUsers"}, {"name": "newUsers"},
			{"name": "screenPageViews"}, {"name": "bounceRate"}, {"name": "averageSessionDuration"},
		},
	}
	data, err := ga4Fetch(token, propertyID, ovBody)
	if err != nil {
		log.Printf("[sync-basic] GA4 site %d: %v", siteID, err)
		return
	}
	var resp struct {
		Rows []struct {
			MetricValues []struct{ Value string } `json:"metricValues"`
		} `json:"rows"`
	}
	json.Unmarshal(data, &resp)
	if len(resp.Rows) == 0 {
		return
	}
	mv := resp.Rows[0].MetricValues
	var sessions, users, pageviews int
	var bounceRate, avgDuration float64
	if len(mv) > 0 { fmt.Sscanf(mv[0].Value, "%d", &sessions) }
	if len(mv) > 1 { fmt.Sscanf(mv[1].Value, "%d", &users) }
	if len(mv) > 3 { fmt.Sscanf(mv[3].Value, "%d", &pageviews) }
	if len(mv) > 4 { fmt.Sscanf(mv[4].Value, "%f", &bounceRate) }
	if len(mv) > 5 { fmt.Sscanf(mv[5].Value, "%f", &avgDuration) }

	date := time.Now().Format("2006-01-02")
	db.DB.Exec(`INSERT INTO ga_data (site_id, date, sessions, users, pageviews, bounce_rate, avg_duration)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (site_id, date) DO UPDATE SET
		sessions=EXCLUDED.sessions, users=EXCLUDED.users, pageviews=EXCLUDED.pageviews,
		bounce_rate=EXCLUDED.bounce_rate, avg_duration=EXCLUDED.avg_duration`,
		siteID, date, sessions, users, pageviews, bounceRate, avgDuration)
	InvalidateGA(siteID)
	log.Printf("[sync-basic] GA4 site %d: %d sessions", siteID, sessions)
}

// syncBingBasic pulls Bing query stats and stores to bing_data.
func syncBingBasic(siteID int64, apiKey, bingSiteURL string) {
	sd := time.Now().AddDate(0, 0, -30).Format("2006-01-02T00:00:00")
	ed := time.Now().AddDate(0, 0, -1).Format("2006-01-02T00:00:00")
	data, err := bingFetch(apiKey, "GetQueryStats", map[string]string{
		"siteUrl": bingSiteURL, "startDate": sd, "endDate": ed,
	})
	if err != nil {
		return
	}
	var totalClicks, totalImpressions int
	var avgCTR, avgPosition float64
	var qr struct { D struct { Results []struct { Clicks int; Impressions int; CTR float64; AvgImpPos float64 `json:"AvgImpPos"` } } `json:"d"` }
	if json.Unmarshal(data, &qr) == nil {
		for _, r := range qr.D.Results {
			totalClicks += r.Clicks; totalImpressions += r.Impressions
		}
		if len(qr.D.Results) > 0 { avgCTR = qr.D.Results[0].CTR; avgPosition = qr.D.Results[0].AvgImpPos }
	}
	date := time.Now().Format("2006-01-02")
	db.DB.Exec(`INSERT INTO bing_data (site_id, date, clicks, impressions, ctr, avg_position, queries_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (site_id, date) DO UPDATE SET
		clicks=EXCLUDED.clicks, impressions=EXCLUDED.impressions, ctr=EXCLUDED.ctr, avg_position=EXCLUDED.avg_position, queries_json=EXCLUDED.queries_json`,
		siteID, date, totalClicks, totalImpressions, avgCTR, avgPosition, string(data))
	InvalidateBing(siteID)
}

// syncYandexBasic pulls Yandex history and stores to yandex_data.
func syncYandexBasic(siteID int64, token, yandexUserID, hostID string) {
	sd := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	ed := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	encodedHost := url.PathEscape(hostID)
	path := fmt.Sprintf("/user/%s/hosts/%s/search-queries/all/history?date_from=%s&date_to=%s&query_indicator=TOTAL_CLICKS&query_indicator=TOTAL_SHOWS",
		yandexUserID, encodedHost, sd, ed)
	data, err := yandexFetch(token, path)
	if err != nil {
		return
	}
	var hist struct {
		Indicators map[string][]struct{ Value int } `json:"indicators"`
	}
	json.Unmarshal(data, &hist)
	var totalClicks, totalImpressions int
	for _, d := range hist.Indicators["TOTAL_CLICKS"] { totalClicks += d.Value }
	for _, d := range hist.Indicators["TOTAL_SHOWS"] { totalImpressions += d.Value }

	date := time.Now().Format("2006-01-02")
	db.DB.Exec(`INSERT INTO yandex_data (site_id, date, clicks, impressions)
		VALUES ($1,$2,$3,$4) ON CONFLICT (site_id, date) DO UPDATE SET clicks=EXCLUDED.clicks, impressions=EXCLUDED.impressions`,
		siteID, date, totalClicks, totalImpressions)
	InvalidateYandex(siteID)
}
