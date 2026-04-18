package api

// Site bundle export/import.
//
// 导出：把当前用户拥有的所有站点（含每个站点配置的「站点页面」）打包成一段
// JSON 文本，前端可以原样复制走、贴到另一个用户那里通过导入按钮恢复。
//
// 导入：解析这段 JSON，逐个 INSERT。同域名（已经在 sites 表里且 created_by
// 等于当前用户）跳过，不做覆盖也不报错——保证用户重复点导入不会出现奇怪
// 的副作用。
//
// 设计取舍：
//   - 不导出审计任务、抓取的 HTML、push 计划等运行产物。这些是过程数据，
//     新环境重新跑一次就有；塞进去会让一段「方便复制」的字符串瞬间变成
//     几十兆。
//   - 不导出 OAuth/Bing/Telegram 等凭证。每用户独立、加密在 users 表，
//     跨用户搬迁只会泄密。
//   - 域名作为去重锚——同账号下 (domain, created_by) 已有 UNIQUE 约束。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// SiteBundleVersion bumps whenever the export schema changes in a
// non-backwards-compatible way. ImportSites refuses unknown versions
// rather than silently dropping fields.
const SiteBundleVersion = 1

// SiteBundle is the wire format for both export response and import
// request body. Keep field names stable — they end up in user-visible
// JSON.
type SiteBundle struct {
	Version    int          `json:"version"`
	ExportedAt string       `json:"exported_at"`
	Sites      []BundleSite `json:"sites"`
}

type BundleSite struct {
	Domain        string       `json:"domain"`
	Name          string       `json:"name"`
	URLCategory   string       `json:"url_category,omitempty"`
	URLDetail     string       `json:"url_detail,omitempty"`
	URLNewsList   string       `json:"url_news_list,omitempty"`
	URLNewsDetail string       `json:"url_news_detail,omitempty"`
	URLSearch     string       `json:"url_search,omitempty"`
	URLStatic     string       `json:"url_static,omitempty"`
	Pages         []BundlePage `json:"pages,omitempty"`
}

type BundlePage struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ExportSites returns the calling user's full site list (with pages) as
// a JSON document. Admins still only see their own — cross-tenant export
// would be a privacy footgun.
func ExportSites(c *gin.Context) {
	userID := c.GetInt64("user_id")

	rows, err := db.DB.Query(`
		SELECT id, domain, name,
		       COALESCE(url_category,''), COALESCE(url_detail,''),
		       COALESCE(url_news_list,''), COALESCE(url_news_detail,''),
		       COALESCE(url_search,''), COALESCE(url_static,'')
		FROM sites
		WHERE created_by = $1
		ORDER BY id`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "查询站点失败"})
		return
	}
	defer rows.Close()

	bundle := SiteBundle{
		Version:    SiteBundleVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Sites:      []BundleSite{},
	}

	type idAndIndex struct {
		id    int64
		index int
	}
	var siteIndex []idAndIndex

	for rows.Next() {
		var sid int64
		var bs BundleSite
		if err := rows.Scan(
			&sid, &bs.Domain, &bs.Name,
			&bs.URLCategory, &bs.URLDetail,
			&bs.URLNewsList, &bs.URLNewsDetail,
			&bs.URLSearch, &bs.URLStatic,
		); err != nil {
			continue
		}
		siteIndex = append(siteIndex, idAndIndex{id: sid, index: len(bundle.Sites)})
		bundle.Sites = append(bundle.Sites, bs)
	}

	// Pull pages in a second query. Single round-trip per site would be
	// N+1; one big IN-list keeps it to one extra query regardless of size.
	if len(siteIndex) > 0 {
		ids := make([]interface{}, 0, len(siteIndex))
		placeholders := make([]string, 0, len(siteIndex))
		idx := make(map[int64]int, len(siteIndex))
		for i, sx := range siteIndex {
			ids = append(ids, sx.id)
			placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
			idx[sx.id] = sx.index
		}
		pageRows, perr := db.DB.Query(
			"SELECT site_id, name, url FROM site_pages WHERE site_id IN ("+strings.Join(placeholders, ",")+") ORDER BY id",
			ids...,
		)
		if perr == nil {
			defer pageRows.Close()
			for pageRows.Next() {
				var sid int64
				var p BundlePage
				if err := pageRows.Scan(&sid, &p.Name, &p.URL); err != nil {
					continue
				}
				if i, ok := idx[sid]; ok {
					bundle.Sites[i].Pages = append(bundle.Sites[i].Pages, p)
				}
			}
		}
	}

	// Render the bundle as a 2-space indented JSON string so the user can
	// eyeball it before pasting elsewhere.
	body, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": -1, "error": "序列化失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"bundle":      string(body),
		"site_count":  len(bundle.Sites),
		"exported_at": bundle.ExportedAt,
	}})
}

// ImportSitesRequest accepts the bundle either inline (preferred — the
// frontend just dumps the textarea contents into `data`) or as a fully
// parsed object. Inline-string mode is what the UI uses; object mode is
// nicer for scripts that already have the data in memory.
type ImportSitesRequest struct {
	Data   string      `json:"data"`
	Bundle *SiteBundle `json:"bundle"`
}

// ImportSites creates sites + pages from the bundle. Existing (domain,
// created_by) rows are skipped so re-importing is idempotent.
func ImportSites(c *gin.Context) {
	var req ImportSitesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "参数错误"})
		return
	}

	bundle := req.Bundle
	if bundle == nil {
		raw := strings.TrimSpace(req.Data)
		if raw == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "导入内容为空"})
			return
		}
		var parsed SiteBundle
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "JSON 解析失败：" + err.Error()})
			return
		}
		bundle = &parsed
	}

	if bundle.Version != SiteBundleVersion {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":  -1,
			"error": fmt.Sprintf("不支持的版本号 %d（当前支持 %d）", bundle.Version, SiteBundleVersion),
		})
		return
	}
	if len(bundle.Sites) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "没有可导入的站点"})
		return
	}

	userID := c.GetInt64("user_id")
	var sitesCreated, sitesSkipped, pagesCreated int
	var failures []string

	for i, bs := range bundle.Sites {
		domain := strings.TrimSpace(bs.Domain)
		name := strings.TrimSpace(bs.Name)
		if domain == "" || name == "" {
			failures = append(failures, fmt.Sprintf("第 %d 项缺少 domain 或 name", i+1))
			continue
		}
		if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
			failures = append(failures, fmt.Sprintf("%s: 域名必须以 http:// 或 https:// 开头", domain))
			continue
		}

		// Idempotent: if (domain, created_by) already exists, treat as
		// skip rather than error. Avoids "导入了一半就失败" scenarios.
		var existingID int64
		_ = db.DB.QueryRow(
			`SELECT id FROM sites WHERE domain = $1 AND created_by = $2`,
			domain, userID,
		).Scan(&existingID)

		var siteID int64
		if existingID > 0 {
			siteID = existingID
			sitesSkipped++
		} else {
			err := db.DB.QueryRow(
				`INSERT INTO sites (domain, name, url_category, url_detail, url_news_list,
				                    url_news_detail, url_search, url_static, created_by)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
				domain, name,
				bs.URLCategory, bs.URLDetail, bs.URLNewsList, bs.URLNewsDetail,
				bs.URLSearch, bs.URLStatic, userID,
			).Scan(&siteID)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", domain, err))
				continue
			}
			sitesCreated++
		}

		// Pages: skip duplicates within the same site by (site_id, url).
		// No UNIQUE constraint on the table so we check ourselves.
		for _, p := range bs.Pages {
			pname := strings.TrimSpace(p.Name)
			purl := strings.TrimSpace(p.URL)
			if pname == "" || purl == "" {
				continue
			}
			var dupID int64
			_ = db.DB.QueryRow(
				`SELECT id FROM site_pages WHERE site_id = $1 AND url = $2`,
				siteID, purl,
			).Scan(&dupID)
			if dupID > 0 {
				continue
			}
			if _, err := db.DB.Exec(
				`INSERT INTO site_pages (site_id, name, url) VALUES ($1, $2, $3)`,
				siteID, pname, purl,
			); err != nil {
				failures = append(failures, fmt.Sprintf("%s 的页面 %s: %v", domain, purl, err))
				continue
			}
			pagesCreated++
		}
	}

	WriteAuditLog(c, "import_sites", "site_bundle", "",
		fmt.Sprintf("created=%d skipped=%d pages=%d failures=%d",
			sitesCreated, sitesSkipped, pagesCreated, len(failures)))

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"sites_created": sitesCreated,
			"sites_skipped": sitesSkipped,
			"pages_created": pagesCreated,
			"failures":      failures,
		},
	})
}
