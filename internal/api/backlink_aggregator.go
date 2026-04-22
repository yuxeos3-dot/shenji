package api

import (
	"log"
	"strings"
	"time"

	"seo-system/internal/db"
)

// AggregateBacklinks merges backlinks from Bing + Yandex into backlinks_all.
// Run weekly. GSC Links API doesn't have a direct list endpoint in v3,
// so we rely on Bing + Yandex as the two official sources.
func AggregateBacklinks(siteID int64) {
	now := time.Now().Format("2006-01-02")

	// 1. From bing_inbound_links → backlinks_all
	bingRows, err := db.DB.Query(`SELECT target_page, source_url, anchor_text, first_seen, last_seen
		FROM bing_inbound_links WHERE site_id=$1`, siteID)
	if err == nil {
		defer bingRows.Close()
		for bingRows.Next() {
			var target, source, anchor string
			var firstSeen, lastSeen *string
			bingRows.Scan(&target, &source, &anchor, &firstSeen, &lastSeen)
			if source == "" {
				continue
			}
			domain := extractDomain(source)
			tld := extractTLD(domain)
			db.DB.Exec(`INSERT INTO backlinks_all (site_id, source_url, source_domain, target_url, anchor_text, data_source, first_seen_at, last_seen_at, source_tld)
				VALUES ($1,$2,$3,$4,$5,'bing',$6,$7,$8)
				ON CONFLICT (site_id, source_url, target_url) DO UPDATE SET
				last_seen_at=COALESCE(EXCLUDED.last_seen_at, backlinks_all.last_seen_at),
				anchor_text=COALESCE(NULLIF(EXCLUDED.anchor_text,''), backlinks_all.anchor_text)`,
				siteID, source, domain, target, anchor, firstSeen, lastSeen, tld)
		}
	}

	// 2. From yandex_external_links → backlinks_all
	yRows, err := db.DB.Query(`SELECT target_url, source_url, first_seen, last_seen
		FROM yandex_external_links WHERE site_id=$1`, siteID)
	if err == nil {
		defer yRows.Close()
		for yRows.Next() {
			var target, source string
			var firstSeen, lastSeen *string
			yRows.Scan(&target, &source, &firstSeen, &lastSeen)
			if source == "" {
				continue
			}
			domain := extractDomain(source)
			tld := extractTLD(domain)
			db.DB.Exec(`INSERT INTO backlinks_all (site_id, source_url, source_domain, target_url, data_source, first_seen_at, last_seen_at, source_tld)
				VALUES ($1,$2,$3,$4,'yandex',$5,$6,$7)
				ON CONFLICT (site_id, source_url, target_url) DO UPDATE SET
				last_seen_at=COALESCE(EXCLUDED.last_seen_at, backlinks_all.last_seen_at)`,
				siteID, source, domain, target, firstSeen, lastSeen, tld)
		}
	}

	// 3. Update referring_domains aggregate
	db.DB.Exec(`INSERT INTO referring_domains (site_id, domain, first_seen, last_seen, backlink_count, dofollow_count, tld)
		SELECT site_id, source_domain, MIN(first_seen_at)::date, MAX(last_seen_at)::date,
			COUNT(*), COUNT(*) FILTER (WHERE rel_attr='dofollow'),
			MAX(source_tld)
		FROM backlinks_all WHERE site_id=$1
		GROUP BY site_id, source_domain
		ON CONFLICT (site_id, domain) DO UPDATE SET
		last_seen=EXCLUDED.last_seen, backlink_count=EXCLUDED.backlink_count, dofollow_count=EXCLUDED.dofollow_count`, siteID)

	// 4. Update backlink_profile_daily
	var totalBL, totalRD, dofollow, nofollow, suspicious int
	db.DB.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT source_domain),
		COUNT(*) FILTER (WHERE rel_attr='dofollow'), COUNT(*) FILTER (WHERE rel_attr!='dofollow'),
		COUNT(*) FILTER (WHERE is_suspicious)
		FROM backlinks_all WHERE site_id=$1`, siteID).Scan(&totalBL, &totalRD, &dofollow, &nofollow, &suspicious)

	db.DB.Exec(`INSERT INTO backlink_profile_daily (site_id, date, total_backlinks, total_referring_domains, dofollow_count, nofollow_count, suspicious_domain_count)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (site_id, date) DO UPDATE SET
		total_backlinks=EXCLUDED.total_backlinks, total_referring_domains=EXCLUDED.total_referring_domains,
		dofollow_count=EXCLUDED.dofollow_count, nofollow_count=EXCLUDED.nofollow_count,
		suspicious_domain_count=EXCLUDED.suspicious_domain_count`,
		siteID, now, totalBL, totalRD, dofollow, nofollow, suspicious)

	log.Printf("[backlinks] site %d: %d total, %d domains, %d suspicious", siteID, totalBL, totalRD, suspicious)
}

func extractDomain(u string) string {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	if idx := strings.Index(u, "/"); idx > 0 {
		u = u[:idx]
	}
	return strings.ToLower(u)
}

func extractTLD(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return "." + parts[len(parts)-1]
	}
	return ""
}
