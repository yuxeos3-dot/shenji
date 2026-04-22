package api

import (
	"crypto/md5"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/db"
)

var (
	reTitle    = regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	reMetaDesc = regexp.MustCompile(`(?i)<meta[^>]+name=["']description["'][^>]+content=["']([^"']*)["']`)
	reH1       = regexp.MustCompile(`(?i)<h1[^>]*>(.*?)</h1>`)
	reCanonical = regexp.MustCompile(`(?i)<link[^>]+rel=["']canonical["'][^>]+href=["']([^"']*)["']`)
	reAnchor   = regexp.MustCompile(`(?i)<a[^>]+href=["']([^"'#][^"']*)["']`)
	reImg      = regexp.MustCompile(`(?i)<img[^>]*>`)
	reImgAlt   = regexp.MustCompile(`(?i)<img[^>]+alt=["'][^"']+["']`)
	reSchema   = regexp.MustCompile(`(?i)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	reWords    = regexp.MustCompile(`\S+`)
	reHTML     = regexp.MustCompile(`<[^>]+>`)
)

// CrawlAndSnapshotPages crawls site_pages and stores content snapshots.
func CrawlAndSnapshotPages(siteID int64) {
	date := time.Now().Format("2006-01-02")

	rows, err := db.DB.Query(`SELECT url, COALESCE(page_type,'') FROM site_pages WHERE site_id=$1`, siteID)
	if err != nil {
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var url, pageType string
		rows.Scan(&url, &pageType)
		if url == "" {
			continue
		}

		result := crawler.FetchPage(crawler.URLSpec{PageType: pageType, URL: url}, 30*time.Second)
		if result.HTML == "" {
			continue
		}

		html := result.HTML
		title := extractFirst(reTitle, html)
		metaDesc := extractFirst(reMetaDesc, html)
		h1 := extractFirst(reH1, html)
		h1Count := len(reH1.FindAllString(html, -1))
		canonical := extractFirst(reCanonical, html)

		// Word count (strip HTML tags, count words)
		textOnly := reHTML.ReplaceAllString(html, " ")
		wordCount := len(reWords.FindAllString(textOnly, -1))

		// Images
		imgCount := len(reImg.FindAllString(html, -1))
		imgAltCount := len(reImgAlt.FindAllString(html, -1))

		// Links
		allLinks := reAnchor.FindAllStringSubmatch(html, -1)
		var domain string
		db.DB.QueryRow("SELECT domain FROM sites WHERE id=$1", siteID).Scan(&domain)
		domainClean := strings.TrimPrefix(strings.TrimPrefix(domain, "https://"), "http://")
		domainClean = strings.TrimRight(domainClean, "/")

		internalCount := 0
		externalCount := 0
		for _, m := range allLinks {
			if len(m) < 2 {
				continue
			}
			href := m[1]
			if strings.HasPrefix(href, "/") || strings.Contains(href, domainClean) {
				internalCount++
			} else if strings.HasPrefix(href, "http") {
				externalCount++
			}
		}

		// Schema types
		schemaTypes := "[]"
		schemas := reSchema.FindAllStringSubmatch(html, -1)
		if len(schemas) > 0 {
			var types []string
			for _, s := range schemas {
				if len(s) > 1 && strings.Contains(s[1], "@type") {
					// Extract @type value
					idx := strings.Index(s[1], `"@type"`)
					if idx < 0 {
						idx = strings.Index(s[1], `'@type'`)
					}
					if idx >= 0 {
						rest := s[1][idx+7:]
						rest = strings.TrimLeft(rest, `: "'`)
						endIdx := strings.IndexAny(rest, `"',}`)
						if endIdx > 0 {
							types = append(types, rest[:endIdx])
						}
					}
				}
			}
			if len(types) > 0 {
				schemaTypes = `["` + strings.Join(types, `","`) + `"]`
			}
		}

		// Content hash
		hash := fmt.Sprintf("%x", md5.Sum([]byte(textOnly[:min(len(textOnly), 5000)])))

		// E-E-A-T signals
		hasAuthor := strings.Contains(strings.ToLower(html), "author") || strings.Contains(html, "byline")
		hasDate := strings.Contains(strings.ToLower(html), "datePublished") || strings.Contains(strings.ToLower(html), "datetime")
		hasSchemaArticle := strings.Contains(html, "Article") && strings.Contains(html, "ld+json")
		hasBreadcrumb := strings.Contains(html, "BreadcrumbList")

		db.DB.Exec(`INSERT INTO page_content_snapshots (site_id, url, snapshot_date, status_code, title, meta_description,
			h1, h1_count, canonical, word_count, image_count, images_with_alt_count, external_link_count, internal_link_count,
			schema_types_json, content_hash, has_author, has_published_date, has_schema_article, has_breadcrumb)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
			ON CONFLICT (site_id, url, snapshot_date) DO UPDATE SET
			status_code=EXCLUDED.status_code, title=EXCLUDED.title, meta_description=EXCLUDED.meta_description,
			h1=EXCLUDED.h1, h1_count=EXCLUDED.h1_count, canonical=EXCLUDED.canonical, word_count=EXCLUDED.word_count,
			image_count=EXCLUDED.image_count, images_with_alt_count=EXCLUDED.images_with_alt_count,
			external_link_count=EXCLUDED.external_link_count, internal_link_count=EXCLUDED.internal_link_count,
			schema_types_json=EXCLUDED.schema_types_json, content_hash=EXCLUDED.content_hash,
			has_author=EXCLUDED.has_author, has_published_date=EXCLUDED.has_published_date,
			has_schema_article=EXCLUDED.has_schema_article, has_breadcrumb=EXCLUDED.has_breadcrumb`,
			siteID, url, date, result.StatusCode, title, metaDesc,
			h1, h1Count, canonical, wordCount, imgCount, imgAltCount, externalCount, internalCount,
			schemaTypes, hash, hasAuthor, hasDate, hasSchemaArticle, hasBreadcrumb)

		// Also extract internal links for link graph
		for _, m := range allLinks {
			if len(m) < 2 {
				continue
			}
			href := m[1]
			if strings.HasPrefix(href, "/") || strings.Contains(href, domainClean) {
				// Normalize to full URL
				targetURL := href
				if strings.HasPrefix(href, "/") {
					targetURL = strings.TrimRight(domain, "/") + href
				}
				db.DB.Exec(`INSERT INTO internal_link_edges (site_id, source_url, target_url, last_seen_at)
					VALUES ($1,$2,$3,NOW())
					ON CONFLICT DO NOTHING`,
					siteID, url, targetURL)
			}
		}

		count++
	}

	// Compute internal link stats
	if count > 0 {
		computeInternalLinkStats(siteID, date)
	}

	log.Printf("[content] site %d: %d pages snapshotted", siteID, count)
}

func computeInternalLinkStats(siteID int64, date string) {
	// Inbound/outbound counts per URL
	db.DB.Exec(`INSERT INTO internal_link_stats (site_id, url, computed_date, inbound_links, outbound_links)
		SELECT $1, url, $2::date, inbound, outbound FROM (
			SELECT COALESCE(i.url, o.url) as url,
				COALESCE(i.cnt, 0) as inbound, COALESCE(o.cnt, 0) as outbound
			FROM (SELECT target_url as url, COUNT(*) as cnt FROM internal_link_edges WHERE site_id=$1 GROUP BY target_url) i
			FULL OUTER JOIN (SELECT source_url as url, COUNT(*) as cnt FROM internal_link_edges WHERE site_id=$1 GROUP BY source_url) o
			ON i.url = o.url
		) sub
		ON CONFLICT (site_id, url, computed_date) DO UPDATE SET
		inbound_links=EXCLUDED.inbound_links, outbound_links=EXCLUDED.outbound_links`, siteID, date)

	// Orphan detection (0 inbound from other pages)
	db.DB.Exec(`UPDATE internal_link_stats SET is_orphan = (inbound_links = 0)
		WHERE site_id=$1 AND computed_date=$2::date`, siteID, date)

	// Graph metrics
	var totalURLs, totalEdges, orphanCount int
	var avgInbound float64
	db.DB.QueryRow(`SELECT COUNT(DISTINCT url), COALESCE(AVG(inbound_links),0), COUNT(*) FILTER (WHERE is_orphan)
		FROM internal_link_stats WHERE site_id=$1 AND computed_date=$2::date`, siteID, date).Scan(&totalURLs, &avgInbound, &orphanCount)
	db.DB.QueryRow(`SELECT COUNT(*) FROM internal_link_edges WHERE site_id=$1`, siteID).Scan(&totalEdges)

	db.DB.Exec(`INSERT INTO site_link_graph_metrics (site_id, computed_date, total_urls, total_edges, avg_inbound, orphan_count)
		VALUES ($1,$2::date,$3,$4,$5,$6)
		ON CONFLICT (site_id, computed_date) DO UPDATE SET
		total_urls=EXCLUDED.total_urls, total_edges=EXCLUDED.total_edges,
		avg_inbound=EXCLUDED.avg_inbound, orphan_count=EXCLUDED.orphan_count`,
		siteID, date, totalURLs, totalEdges, avgInbound, orphanCount)

	log.Printf("[internal-links] site %d: %d urls, %d edges, %d orphans", siteID, totalURLs, totalEdges, orphanCount)
}

func extractFirst(re *regexp.Regexp, html string) string {
	m := re.FindStringSubmatch(html)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
