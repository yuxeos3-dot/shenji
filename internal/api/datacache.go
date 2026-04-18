package api

import (
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// dataCache keeps a warm in-memory snapshot of the 4 platforms' data so the
// "数据管理" page renders instantly even when the SQLite table is large or
// during a brief DB contention spike. The cache is populated:
//   - once at server start (async, post-DB-init)
//   - on every successful login (async, non-blocking)
//   - on cache-miss in a Get*Data handler (lazy, single site)
//
// The cache is invalidated per-(site, platform) after a Sync*Data call, and
// the Cache Management page exposes "clear" buttons for each platform.

type siteDataEntry struct {
	GSC     []gin.H
	GA      []gin.H
	GAExtra json.RawMessage // GA "extra" blob stored alongside rows
	Bing    []gin.H
	Yandex  []gin.H
	At      time.Time
}

var (
	dataCacheMu sync.RWMutex
	dataCache   = map[int64]*siteDataEntry{}
	warmingMu   sync.Mutex // serialize warmings so simultaneous logins don't thrash the DB
)

// ─── Warming ───────────────────────────────────────────────────────────────

// WarmDataCache loads every site's 4-platform snapshot into memory.
// Safe to call repeatedly; later calls overwrite the previous cache atomically.
// Intended to be invoked in a goroutine so the caller never blocks.
func WarmDataCache() {
	if !warmingMu.TryLock() {
		// A warming is already in progress; skip to avoid piling up goroutines.
		return
	}
	defer warmingMu.Unlock()

	start := time.Now()
	ids, err := listAllSiteIDs()
	if err != nil {
		log.Printf("[data-cache] list sites failed: %v", err)
		return
	}

	next := make(map[int64]*siteDataEntry, len(ids))
	for _, id := range ids {
		entry := &siteDataEntry{At: time.Now()}
		entry.GSC = loadGSCRows(id)
		entry.GA, entry.GAExtra = loadGARowsWithExtra(id)
		entry.Bing = loadBingRows(id)
		entry.Yandex = loadYandexRows(id)
		next[id] = entry
	}

	dataCacheMu.Lock()
	dataCache = next
	dataCacheMu.Unlock()

	log.Printf("[data-cache] warmed %d sites in %v", len(ids), time.Since(start))
}

// ensureSiteLoaded lazily warms one site's entry if missing. Called from the
// read handlers so a site that was created after startup still benefits from
// the cache without waiting for the next login.
func ensureSiteLoaded(siteID int64) *siteDataEntry {
	dataCacheMu.RLock()
	entry := dataCache[siteID]
	dataCacheMu.RUnlock()
	if entry != nil {
		return entry
	}

	fresh := &siteDataEntry{At: time.Now()}
	fresh.GSC = loadGSCRows(siteID)
	fresh.GA, fresh.GAExtra = loadGARowsWithExtra(siteID)
	fresh.Bing = loadBingRows(siteID)
	fresh.Yandex = loadYandexRows(siteID)

	dataCacheMu.Lock()
	dataCache[siteID] = fresh
	dataCacheMu.Unlock()
	return fresh
}

// ─── Per-platform loaders (read DB) ─────────────────────────────────────────

func listAllSiteIDs() ([]int64, error) {
	rows, err := db.DB.Query("SELECT id FROM sites ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func loadGSCRows(siteID int64) []gin.H {
	rows, err := db.DB.Query(
		`SELECT date, clicks, impressions, ctr, position, keywords_json, pages_json, devices_json, countries_json
		 FROM gsc_data WHERE site_id = $1 ORDER BY date DESC LIMIT 30`,
		siteID,
	)
	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var date, kw, pg, dv, ct string
		var clicks, impressions int
		var ctr, pos float64
		if err := rows.Scan(&date, &clicks, &impressions, &ctr, &pos, &kw, &pg, &dv, &ct); err != nil {
			continue
		}
		out = append(out, gin.H{
			"date": date, "clicks": clicks, "impressions": impressions,
			"ctr": ctr, "position": pos,
			"keywords_json":  json.RawMessage(kw),
			"pages_json":     json.RawMessage(pg),
			"devices_json":   json.RawMessage(dv),
			"countries_json": json.RawMessage(ct),
		})
	}
	if out == nil {
		out = []gin.H{}
	}
	return out
}

func loadGARowsWithExtra(siteID int64) ([]gin.H, json.RawMessage) {
	rows, err := db.DB.Query(
		`SELECT date, sessions, users, pageviews, bounce_rate, avg_duration, channels_json, pages_json
		 FROM ga_data WHERE site_id = $1 ORDER BY date DESC LIMIT 30`,
		siteID,
	)
	if err != nil {
		return []gin.H{}, nil
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var date, channels, pages string
		var sessions, users, pageviews int
		var bounceRate, avgDuration float64
		if err := rows.Scan(&date, &sessions, &users, &pageviews, &bounceRate, &avgDuration, &channels, &pages); err != nil {
			continue
		}
		out = append(out, gin.H{
			"date": date, "sessions": sessions, "users": users, "pageviews": pageviews,
			"bounce_rate": bounceRate, "avg_duration": avgDuration,
			"channels_json": json.RawMessage(channels),
			"pages_json":    json.RawMessage(pages),
		})
	}
	if out == nil {
		out = []gin.H{}
	}
	var extraStr string
	db.DB.QueryRow("SELECT value FROM settings WHERE key = $1", "ga_extra_"+strconv.FormatInt(siteID, 10)).Scan(&extraStr)
	var extra json.RawMessage
	if extraStr != "" {
		extra = json.RawMessage(extraStr)
	}
	return out, extra
}

func loadBingRows(siteID int64) []gin.H {
	rows, err := db.DB.Query(
		`SELECT date, clicks, impressions, ctr, avg_position, index_count,
		        queries_json, pages_json, crawl_json, errors_json
		 FROM bing_data WHERE site_id = $1 ORDER BY date DESC LIMIT 30`,
		siteID,
	)
	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var date, queries, pages, crawl, errors string
		var clicks, impressions, indexCount int
		var ctr, avgPosition float64
		if err := rows.Scan(&date, &clicks, &impressions, &ctr, &avgPosition, &indexCount, &queries, &pages, &crawl, &errors); err != nil {
			continue
		}
		out = append(out, gin.H{
			"date": date, "clicks": clicks, "impressions": impressions,
			"ctr": ctr, "avg_position": avgPosition, "index_count": indexCount,
			"queries_json": json.RawMessage(queries),
			"pages_json":   json.RawMessage(pages),
			"crawl_json":   json.RawMessage(crawl),
			"errors_json":  json.RawMessage(errors),
		})
	}
	if out == nil {
		out = []gin.H{}
	}
	return out
}

func loadYandexRows(siteID int64) []gin.H {
	rows, err := db.DB.Query(
		`SELECT date, clicks, impressions, index_count, keywords_json, errors_json
		 FROM yandex_data WHERE site_id = $1 ORDER BY date DESC LIMIT 30`,
		siteID,
	)
	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var date, keywords, errors string
		var clicks, impressions, indexCount int
		if err := rows.Scan(&date, &clicks, &impressions, &indexCount, &keywords, &errors); err != nil {
			continue
		}
		out = append(out, gin.H{
			"date": date, "clicks": clicks, "impressions": impressions,
			"index_count":   indexCount,
			"keywords_json": json.RawMessage(keywords),
			"errors_json":   json.RawMessage(errors),
		})
	}
	if out == nil {
		out = []gin.H{}
	}
	return out
}

// ─── Cache reads (used by handlers) ────────────────────────────────────────

// CachedGSC returns a copy of the cached GSC rows, truncated to `days`.
// Always returns a non-nil slice (possibly empty). Lazily warms the site.
func CachedGSC(siteID int64, days int) []gin.H {
	entry := ensureSiteLoaded(siteID)
	return limitRows(entry.GSC, days)
}

// CachedGA returns rows + extra blob. Lazy-warms.
func CachedGA(siteID int64) ([]gin.H, json.RawMessage) {
	entry := ensureSiteLoaded(siteID)
	return entry.GA, entry.GAExtra
}

// CachedBing returns the cached Bing rows for the given site.
func CachedBing(siteID int64) []gin.H {
	entry := ensureSiteLoaded(siteID)
	return entry.Bing
}

// CachedYandex returns the cached Yandex rows for the given site.
func CachedYandex(siteID int64) []gin.H {
	entry := ensureSiteLoaded(siteID)
	return entry.Yandex
}

func limitRows(rows []gin.H, n int) []gin.H {
	if n <= 0 || n >= len(rows) {
		return rows
	}
	return rows[:n]
}

// ─── Invalidation (called by Sync* handlers) ───────────────────────────────

// InvalidateGSC reloads the GSC slice for a single site after a sync write.
func InvalidateGSC(siteID int64) {
	rows := loadGSCRows(siteID)
	dataCacheMu.Lock()
	if e := dataCache[siteID]; e != nil {
		e.GSC = rows
		e.At = time.Now()
	} else {
		dataCache[siteID] = &siteDataEntry{GSC: rows, At: time.Now()}
	}
	dataCacheMu.Unlock()
}

// InvalidateGA reloads GA rows + extra.
func InvalidateGA(siteID int64) {
	rows, extra := loadGARowsWithExtra(siteID)
	dataCacheMu.Lock()
	if e := dataCache[siteID]; e != nil {
		e.GA = rows
		e.GAExtra = extra
		e.At = time.Now()
	} else {
		dataCache[siteID] = &siteDataEntry{GA: rows, GAExtra: extra, At: time.Now()}
	}
	dataCacheMu.Unlock()
}

// InvalidateBing reloads Bing rows.
func InvalidateBing(siteID int64) {
	rows := loadBingRows(siteID)
	dataCacheMu.Lock()
	if e := dataCache[siteID]; e != nil {
		e.Bing = rows
		e.At = time.Now()
	} else {
		dataCache[siteID] = &siteDataEntry{Bing: rows, At: time.Now()}
	}
	dataCacheMu.Unlock()
}

// InvalidateYandex reloads Yandex rows.
func InvalidateYandex(siteID int64) {
	rows := loadYandexRows(siteID)
	dataCacheMu.Lock()
	if e := dataCache[siteID]; e != nil {
		e.Yandex = rows
		e.At = time.Now()
	} else {
		dataCache[siteID] = &siteDataEntry{Yandex: rows, At: time.Now()}
	}
	dataCacheMu.Unlock()
}

// ─── Stats + Clear (exposed to cache.go for the admin UI) ─────────────────

// DataCacheStats returns per-platform (sites_with_data, total_rows) tuples so
// the admin UI can show a meaningful "X sites / Y rows" line for each entry.
func DataCacheStats() map[string]struct {
	Sites int
	Rows  int
	At    time.Time
} {
	out := map[string]struct {
		Sites int
		Rows  int
		At    time.Time
	}{
		"gsc":    {},
		"ga":     {},
		"bing":   {},
		"yandex": {},
	}
	var newest time.Time

	dataCacheMu.RLock()
	defer dataCacheMu.RUnlock()
	for _, e := range dataCache {
		if e.At.After(newest) {
			newest = e.At
		}
		if n := len(e.GSC); n > 0 {
			r := out["gsc"]
			r.Sites++
			r.Rows += n
			out["gsc"] = r
		}
		if n := len(e.GA); n > 0 {
			r := out["ga"]
			r.Sites++
			r.Rows += n
			out["ga"] = r
		}
		if n := len(e.Bing); n > 0 {
			r := out["bing"]
			r.Sites++
			r.Rows += n
			out["bing"] = r
		}
		if n := len(e.Yandex); n > 0 {
			r := out["yandex"]
			r.Sites++
			r.Rows += n
			out["yandex"] = r
		}
	}
	for k, v := range out {
		v.At = newest
		out[k] = v
	}
	return out
}

// ClearDataCachePlatform zeroes only the named platform across every site.
// Pass one of "gsc" / "ga" / "bing" / "yandex". Unknown keys are a no-op.
func ClearDataCachePlatform(platform string) {
	dataCacheMu.Lock()
	defer dataCacheMu.Unlock()
	for _, e := range dataCache {
		switch platform {
		case "gsc":
			e.GSC = nil
		case "ga":
			e.GA = nil
			e.GAExtra = nil
		case "bing":
			e.Bing = nil
		case "yandex":
			e.Yandex = nil
		}
	}
}

// ClearAllDataCache wipes every site and every platform.
func ClearAllDataCache() {
	dataCacheMu.Lock()
	dataCache = map[int64]*siteDataEntry{}
	dataCacheMu.Unlock()
}
