package api

// Per-user platform-overview helpers, shared between:
//   1. The user-scoped dashboards (GSCOverview / GAOverview / BingOverview /
//      YandexOverview) — they call these with the caller's own user id.
//   2. The admin global-data endpoint — loops over every user who has
//      bound the platform and merges all their provider-side sites
//      with the owner's name attached.
//
// These helpers DO NOT set any gin.Context state; they return cards or
// an error string directly so the caller decides how to surface them.
// Concurrency per user is capped at 8 (same as existing handlers).

import (
	"encoding/json"
	"net/url"
	"sync"
)

// gscCardsForUser lists all GSC properties the given user has access to,
// and for each runs a searchAnalytics overview query for [sd, ed].
// Returns cards as []map so callers can merge / enrich. If token fetch
// fails (user hasn't bound GSC), returns (nil, nil) — the caller treats
// that as "no contribution" rather than an error.
func gscCardsForUser(userID int64, sd, ed string) ([]map[string]interface{}, error) {
	token, err := getGoogleAccessToken(userID)
	if err != nil {
		return nil, nil // not bound → silent skip
	}

	rawSites, err := gscFetch(token, gscAPI+"/sites", "GET", nil)
	if err != nil {
		return nil, err
	}
	var sitesResp struct {
		SiteEntry []struct {
			SiteURL         string `json:"siteUrl"`
			PermissionLevel string `json:"permissionLevel"`
		} `json:"siteEntry"`
	}
	if err := json.Unmarshal(rawSites, &sitesResp); err != nil {
		return nil, err
	}

	cards := make([]map[string]interface{}, len(sitesResp.SiteEntry))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	for i, site := range sitesResp.SiteEntry {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, siteURL, perm string) {
			defer wg.Done()
			defer func() { <-sem }()
			card := map[string]interface{}{
				"site_url":     siteURL,
				"display_name": displayNameForGSCSite(siteURL),
				"permission":   perm,
				"is_domain":    len(siteURL) >= 10 && siteURL[:10] == "sc-domain:",
				"has_data":     false,
			}
			// Restricted users can't query searchAnalytics.
			if perm == "siteUnverifiedUser" {
				card["error"] = "未验证"
				cards[idx] = card
				return
			}
			encoded := url.QueryEscape(siteURL)
			q := map[string]interface{}{"startDate": sd, "endDate": ed}
			raw, err := gscFetch(token, gscAPI+"/sites/"+encoded+"/searchAnalytics/query", "POST", q)
			if err != nil {
				card["error"] = err.Error()
				cards[idx] = card
				return
			}
			var r struct {
				Rows []struct {
					Clicks      float64 `json:"clicks"`
					Impressions float64 `json:"impressions"`
					CTR         float64 `json:"ctr"`
					Position    float64 `json:"position"`
				} `json:"rows"`
			}
			if err := json.Unmarshal(raw, &r); err != nil {
				card["error"] = "解析失败"
				cards[idx] = card
				return
			}
			if len(r.Rows) > 0 {
				card["clicks"] = r.Rows[0].Clicks
				card["impressions"] = r.Rows[0].Impressions
				card["ctr"] = r.Rows[0].CTR
				card["position"] = r.Rows[0].Position
				card["has_data"] = r.Rows[0].Clicks > 0 || r.Rows[0].Impressions > 0
			}
			cards[idx] = card
		}(i, site.SiteURL, site.PermissionLevel)
	}
	wg.Wait()
	return cards, nil
}

// gaCardsForUser lists all GA4 properties + overview metrics for a user.
// Mirrors GAOverview but returns cards directly.
func gaCardsForUser(userID int64, sd, ed string) ([]map[string]interface{}, error) {
	token, err := getGAAccessToken(userID)
	if err != nil {
		return nil, nil
	}

	rawProps, err := gaAdminGET(token, "https://analyticsadmin.googleapis.com/v1beta/accountSummaries?pageSize=200")
	if err != nil {
		return nil, err
	}
	var propsResp struct {
		AccountSummaries []struct {
			DisplayName       string `json:"displayName"`
			PropertySummaries []struct {
				Property    string `json:"property"`
				DisplayName string `json:"displayName"`
			} `json:"propertySummaries"`
		} `json:"accountSummaries"`
	}
	if err := json.Unmarshal(rawProps, &propsResp); err != nil {
		return nil, err
	}

	type propRef struct{ id, disp, acct string }
	var propList []propRef
	for _, a := range propsResp.AccountSummaries {
		for _, p := range a.PropertySummaries {
			id := p.Property
			if len(id) > len("properties/") && id[:len("properties/")] == "properties/" {
				id = id[len("properties/"):]
			}
			propList = append(propList, propRef{id: id, disp: p.DisplayName, acct: a.DisplayName})
		}
	}

	cards := make([]map[string]interface{}, len(propList))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	body := map[string]interface{}{
		"dateRanges": []map[string]string{{"startDate": sd, "endDate": ed}},
		"metrics": []map[string]string{
			{"name": "sessions"}, {"name": "totalUsers"}, {"name": "newUsers"},
			{"name": "screenPageViews"}, {"name": "bounceRate"}, {"name": "engagementRate"},
		},
	}
	for i, p := range propList {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, pr propRef) {
			defer wg.Done()
			defer func() { <-sem }()
			card := map[string]interface{}{
				"property_id":  pr.id,
				"display_name": pr.disp,
				"account_name": pr.acct,
				"has_data":     false,
			}
			raw, err := ga4Fetch(token, pr.id, body)
			if err != nil {
				card["error"] = err.Error()
				cards[idx] = card
				return
			}
			var r struct {
				Rows []struct {
					MetricValues []struct {
						Value string `json:"value"`
					} `json:"metricValues"`
				} `json:"rows"`
			}
			if err := json.Unmarshal(raw, &r); err != nil {
				card["error"] = "解析失败"
				cards[idx] = card
				return
			}
			if len(r.Rows) > 0 && len(r.Rows[0].MetricValues) >= 6 {
				sessions := parseFloatSafe(r.Rows[0].MetricValues[0].Value)
				users := parseFloatSafe(r.Rows[0].MetricValues[1].Value)
				card["sessions"] = sessions
				card["total_users"] = users
				card["new_users"] = parseFloatSafe(r.Rows[0].MetricValues[2].Value)
				card["screen_page_views"] = parseFloatSafe(r.Rows[0].MetricValues[3].Value)
				card["bounce_rate"] = parseFloatSafe(r.Rows[0].MetricValues[4].Value)
				card["engagement_rate"] = parseFloatSafe(r.Rows[0].MetricValues[5].Value)
				card["has_data"] = sessions > 0 || users > 0
			}
			cards[idx] = card
		}(i, p)
	}
	wg.Wait()
	return cards, nil
}

// bingCardsForUser lists all verified Bing sites + overview per site.
func bingCardsForUser(userID int64, sd, ed string) ([]map[string]interface{}, error) {
	apiKey, err := LoadUserBingKey(userID)
	if err != nil || apiKey == "" {
		return nil, nil
	}

	rawSites, err := bingFetch(apiKey, "GetUserSites", nil)
	if err != nil {
		return nil, err
	}
	var sitesResp struct {
		D []struct {
			URL        string `json:"Url"`
			IsVerified bool   `json:"IsVerified"`
		} `json:"d"`
	}
	if err := json.Unmarshal(rawSites, &sitesResp); err != nil {
		return nil, err
	}

	cards := []map[string]interface{}{}
	// Bing's stats endpoints don't cleanly support date range across all
	// sites in one call, so we do them serially per site. Accounts rarely
	// have more than a handful of sites so this is fine.
	for _, site := range sitesResp.D {
		if !site.IsVerified {
			continue
		}
		card := map[string]interface{}{
			"site_url":     site.URL,
			"display_name": site.URL,
			"has_data":     false,
		}
		raw, err := bingFetch(apiKey, "GetRankAndTrafficStats", map[string]string{
			"siteUrl": site.URL,
		})
		if err != nil {
			card["error"] = err.Error()
			cards = append(cards, card)
			continue
		}
		var r struct {
			D []struct {
				Clicks      float64 `json:"Clicks"`
				Impressions float64 `json:"Impressions"`
				AvgClickPos float64 `json:"AvgClickPosition"`
				AvgImpPos   float64 `json:"AvgImpressionPosition"`
			} `json:"d"`
		}
		if err := json.Unmarshal(raw, &r); err == nil {
			var c, imp float64
			var posSum float64
			var posCount float64
			for _, row := range r.D {
				c += row.Clicks
				imp += row.Impressions
				if row.AvgImpPos > 0 {
					posSum += row.AvgImpPos
					posCount++
				}
			}
			card["clicks"] = c
			card["impressions"] = imp
			if imp > 0 {
				card["ctr"] = c / imp
			} else {
				card["ctr"] = 0.0
			}
			if posCount > 0 {
				card["position"] = posSum / posCount
			} else {
				card["position"] = 0.0
			}
			card["has_data"] = c > 0 || imp > 0
		}
		cards = append(cards, card)
	}
	return cards, nil
}

// yandexCardsForUser lists all verified hosts + overview from search-queries/all/history.
func yandexCardsForUser(userID int64, sd, ed string) ([]map[string]interface{}, error) {
	token, err := getYandexAccessToken(userID)
	if err != nil {
		return nil, nil
	}
	yandexUserID, err := getYandexUserIDCached(userID, token)
	if err != nil {
		return nil, err
	}

	rawHosts, err := yandexFetch(token, "/user/"+url.PathEscape(yandexUserID)+"/hosts")
	if err != nil {
		return nil, err
	}
	var hostsResp struct {
		Hosts []struct {
			HostID         string `json:"host_id"`
			UnicodeHostURL string `json:"unicode_host_url"`
			MainMirrorURL  string `json:"main_mirror_url"`
			Verified       bool   `json:"verified"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(rawHosts, &hostsResp); err != nil {
		return nil, err
	}

	cards := make([]map[string]interface{}, 0, len(hostsResp.Hosts))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, h := range hostsResp.Hosts {
		wg.Add(1)
		sem <- struct{}{}
		go func(host struct {
			HostID         string `json:"host_id"`
			UnicodeHostURL string `json:"unicode_host_url"`
			MainMirrorURL  string `json:"main_mirror_url"`
			Verified       bool   `json:"verified"`
		}) {
			defer wg.Done()
			defer func() { <-sem }()

			display := host.UnicodeHostURL
			if display == "" {
				display = host.MainMirrorURL
			}
			if display == "" {
				display = host.HostID
			}
			card := map[string]interface{}{
				"host_id":      host.HostID,
				"display_name": display,
				"has_data":     false,
			}

			encoded := url.PathEscape(host.HostID)
			path := "/user/" + url.PathEscape(yandexUserID) + "/hosts/" + encoded +
				"/search-queries/all/history?date_from=" + sd + "&date_to=" + ed +
				"&query_indicator=TOTAL_CLICKS&query_indicator=TOTAL_SHOWS&query_indicator=AVG_SHOW_POSITION"

			raw, err := yandexFetch(token, path)
			if err != nil {
				// Translate HOST_NOT_LOADED / NOT_VERIFIED / etc. into a
				// short CN message. Mirrors the per-user YandexOverview path;
				// see friendlyYandexError() in yandex_dashboard.go.
				card["error"] = friendlyYandexError(err.Error())
				mu.Lock()
				cards = append(cards, card)
				mu.Unlock()
				return
			}
			var hist struct {
				Indicators map[string][]struct {
					Value float64 `json:"value"`
				} `json:"indicators"`
			}
			if err := json.Unmarshal(raw, &hist); err == nil {
				var c, imp, posSum float64
				var posCount float64
				for _, d := range hist.Indicators["TOTAL_CLICKS"] {
					c += d.Value
				}
				for _, d := range hist.Indicators["TOTAL_SHOWS"] {
					imp += d.Value
				}
				for _, d := range hist.Indicators["AVG_SHOW_POSITION"] {
					posSum += d.Value
					posCount++
				}
				card["clicks"] = c
				card["impressions"] = imp
				if imp > 0 {
					card["ctr"] = c / imp
				} else {
					card["ctr"] = 0.0
				}
				if posCount > 0 {
					card["position"] = posSum / posCount
				} else {
					card["position"] = 0.0
				}
				card["has_data"] = c > 0 || imp > 0
			}
			mu.Lock()
			cards = append(cards, card)
			mu.Unlock()
		}(h)
	}
	wg.Wait()
	return cards, nil
}
