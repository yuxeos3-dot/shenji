package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Helper: AI call for GEO ─────────────────────────────────────────────────

// geoCallAI uses user's AI config with round-robin failover.
// Falls back to system config if user has no slots configured.
func geoCallAI(userID int64, prompt string, maxTokens int) (string, error) {
	content, _, err := CallAIWithUserConfig(userID, prompt, maxTokens)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(content), nil
}

// ─── Stats ───────────────────────────────────────────────────────────────────

func GeoStats(c *gin.Context) {
	siteID := c.Query("site_id")
	contentType := c.DefaultQuery("content_type", "geo")
	var totalArticles, drafts, published, reviewed int
	if siteID != "" {
		if !canAccessSite(c, siteID) {
			return
		}
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=$1 AND content_type=$2", siteID, contentType).Scan(&totalArticles)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=$1 AND status='draft' AND content_type=$2", siteID, contentType).Scan(&drafts)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=$1 AND status='published' AND content_type=$2", siteID, contentType).Scan(&published)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=$1 AND review_status='pending' AND content_type=$2", siteID, contentType).Scan(&reviewed)
	} else {
		role, _ := c.Get("role")
		if role == "admin" {
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE content_type=$1", contentType).Scan(&totalArticles)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE status='draft' AND content_type=$1", contentType).Scan(&drafts)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE status='published' AND content_type=$1", contentType).Scan(&published)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE review_status='pending' AND content_type=$1", contentType).Scan(&reviewed)
		} else {
			userID := c.GetInt64("user_id")
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE content_type=$1 AND site_id IN (SELECT id FROM sites WHERE created_by=$2)", contentType, userID).Scan(&totalArticles)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE status='draft' AND content_type=$1 AND site_id IN (SELECT id FROM sites WHERE created_by=$2)", contentType, userID).Scan(&drafts)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE status='published' AND content_type=$1 AND site_id IN (SELECT id FROM sites WHERE created_by=$2)", contentType, userID).Scan(&published)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE review_status='pending' AND content_type=$1 AND site_id IN (SELECT id FROM sites WHERE created_by=$2)", contentType, userID).Scan(&reviewed)
		}
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{
		"total_articles": totalArticles, "drafts": drafts, "published": published,
		"pending_review": reviewed,
	}})
}

// ─── Title Libraries ─────────────────────────────────────────────────────────

// GeoListTitleLibs only returns libs belonging to the caller. Admin
// sees their own libs too — the product rule is "每个人管自己的"。
// Orphan rows (user_id=0, from before the column existed) are treated
// as admin-owned to avoid losing access to legacy data.
func GeoListTitleLibs(c *gin.Context) {
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	isAdmin := role == "admin"
	siteID := c.Query("site_id")
	contentType := c.DefaultQuery("content_type", "geo")
	// Ensure default library exists for this user+site+content_type combo.
	// Default libraries are auto-created, undeletable, and act as the target
	// for "pull keywords from 4 platforms" actions.
	ensureDefaultGeoLib(uid, siteID, contentType)

	q := "SELECT id, name, description, title_count, COALESCE(is_default,FALSE), created_at FROM geo_title_libs WHERE content_type = $1"
	args := []interface{}{contentType}
	// Per-user isolation: admin sees user_id=self OR user_id=0 (orphan),
	// member sees only user_id=self.
	if isAdmin {
		q += fmt.Sprintf(" AND (user_id = $%d OR user_id = 0)", len(args)+1)
		args = append(args, uid)
	} else {
		q += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, uid)
	}
	if siteID != "" {
		q += fmt.Sprintf(" AND site_id = $%d", len(args)+1)
		args = append(args, siteID)
	}
	q += " ORDER BY is_default DESC, id DESC LIMIT 200"
	rows, err := db.DB.Query(q, args...)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	var libs []gin.H
	for rows.Next() {
		var id, cnt int64
		var name, desc, cat string
		var isDefault bool
		rows.Scan(&id, &name, &desc, &cnt, &isDefault, &cat)
		libs = append(libs, gin.H{
			"id": id, "name": name, "description": desc,
			"title_count": cnt, "is_default": isDefault, "created_at": cat,
		})
	}
	if libs == nil {
		libs = []gin.H{}
	}
	c.JSON(200, gin.H{"code": 0, "data": libs})
}

func GeoCreateTitleLib(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		SiteID      int64  `json:"site_id"`
		ContentType string `json:"content_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "name 必填"})
		return
	}
	if req.ContentType == "" {
		req.ContentType = "geo"
	}
	uid := c.GetInt64("user_id")
	var id int64
	err := db.DB.QueryRow(
		"INSERT INTO geo_title_libs (name, description, site_id, content_type, user_id) VALUES ($1, $2, $3, $4, $5) RETURNING id",
		req.Name, req.Description, req.SiteID, req.ContentType, uid,
	).Scan(&id)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "创建失败"})
		return
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"id": id}})
}

// assertGeoLibOwnership returns the lib row + ok=true only when the
// caller is allowed to touch it. Members: must be user_id owner.
// Admin: can touch their own OR legacy orphan (user_id=0).
func assertGeoLibOwnership(c *gin.Context, libID string) (int64, int64, string, bool) {
	uid := c.GetInt64("user_id")
	role, _ := c.Get("role")
	var owner, siteID int64
	var contentType string
	err := db.DB.QueryRow(
		"SELECT COALESCE(user_id,0), COALESCE(site_id,0), COALESCE(content_type,'geo') FROM geo_title_libs WHERE id=$1",
		libID,
	).Scan(&owner, &siteID, &contentType)
	if err != nil {
		c.JSON(404, gin.H{"code": -1, "error": "关键词库不存在"})
		return 0, 0, "", false
	}
	allowed := owner == uid || (role == "admin" && owner == 0)
	if !allowed {
		c.JSON(403, gin.H{"code": -1, "error": "无权访问该关键词库（非本人创建）"})
		return 0, 0, "", false
	}
	return owner, siteID, contentType, true
}

func GeoDeleteTitleLib(c *gin.Context) {
	id := c.Param("id")
	contentType := c.DefaultQuery("content_type", "geo")
	_, libSiteID, ct, ok := assertGeoLibOwnership(c, id)
	if !ok {
		return
	}
	if ct != contentType {
		c.JSON(403, gin.H{"code": -1, "error": "无权删除"})
		return
	}
	// Default libraries are auto-created per user+site, cannot be deleted
	var isDefault bool
	db.DB.QueryRow("SELECT COALESCE(is_default,FALSE) FROM geo_title_libs WHERE id=$1", id).Scan(&isDefault)
	if isDefault {
		c.JSON(400, gin.H{"code": -1, "error": "默认库不可删除"})
		return
	}
	if libSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", libSiteID)) {
		return
	}
	db.DB.Exec("DELETE FROM geo_titles WHERE lib_id = $1", id)
	db.DB.Exec("DELETE FROM geo_title_libs WHERE id = $1", id)
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

// ensureDefaultGeoLib creates the default library for the caller's
// (user, site, content_type) combination if it doesn't already exist.
// Idempotent via UNIQUE partial index. Named "geo库" or "link库" based
// on content_type.
func ensureDefaultGeoLib(userID int64, siteID string, contentType string) {
	if userID <= 0 || siteID == "" {
		return
	}
	var exists int
	db.DB.QueryRow(
		"SELECT 1 FROM geo_title_libs WHERE user_id=$1 AND COALESCE(site_id,0)::text=$2 AND content_type=$3 AND is_default=TRUE LIMIT 1",
		userID, siteID, contentType,
	).Scan(&exists)
	if exists == 1 {
		return
	}
	name := "geo库"
	desc := "默认关键词库 · 自动包含该站点所有4大平台关键词"
	if contentType == "extlink" {
		name = "link库"
		desc = "默认外链关键词库 · 自动包含该站点所有4大平台关键词"
	}
	db.DB.Exec(
		"INSERT INTO geo_title_libs (name, description, site_id, content_type, user_id, is_default) VALUES ($1, $2, $3::int, $4, $5, TRUE) ON CONFLICT DO NOTHING",
		name, desc, siteID, contentType, userID,
	)
}

// GeoImportFromPlatforms pulls all keywords from the 4 platforms
// (GSC/Bing/Yandex deep tables) into the target library.
// POST /v1/geo/title-libs/:id/import-platforms
func GeoImportFromPlatforms(c *gin.Context) {
	libID := c.Param("id")
	_, libSiteID, _, ok := assertGeoLibOwnership(c, libID)
	if !ok {
		return
	}
	var req struct {
		SiteID int64 `json:"site_id"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.SiteID == 0 {
		req.SiteID = libSiteID
	}
	if req.SiteID == 0 {
		c.JSON(400, gin.H{"code": -1, "error": "site_id 必填"})
		return
	}
	if !canAccessSite(c, fmt.Sprintf("%d", req.SiteID)) {
		return
	}

	// Collect distinct queries across all 4 platforms
	keywords := map[string]bool{}

	// GSC (deep)
	if rows, err := db.DB.Query(
		`SELECT DISTINCT query FROM gsc_query_page_daily
		 WHERE site_id=$1 AND query<>'' AND search_type='web'
		 LIMIT 5000`, req.SiteID,
	); err == nil {
		for rows.Next() {
			var q string
			if rows.Scan(&q) == nil && q != "" {
				keywords[q] = true
			}
		}
		rows.Close()
	}

	// Bing deep
	if rows, err := db.DB.Query(
		`SELECT DISTINCT query FROM bing_query_daily
		 WHERE site_id=$1 AND query<>'' LIMIT 5000`, req.SiteID,
	); err == nil {
		for rows.Next() {
			var q string
			if rows.Scan(&q) == nil && q != "" {
				keywords[q] = true
			}
		}
		rows.Close()
	}

	// Yandex deep
	if rows, err := db.DB.Query(
		`SELECT DISTINCT query FROM yandex_query_daily
		 WHERE site_id=$1 AND query<>'' LIMIT 5000`, req.SiteID,
	); err == nil {
		for rows.Next() {
			var q string
			if rows.Scan(&q) == nil && q != "" {
				keywords[q] = true
			}
		}
		rows.Close()
	}

	// Fallback to basic gsc_data.keywords_json / bing_data.queries_json / yandex_data.keywords_json
	// if deep tables have no data yet.
	if len(keywords) == 0 {
		var gscKW string
		db.DB.QueryRow("SELECT COALESCE(keywords_json::text,'') FROM gsc_data WHERE site_id=$1 ORDER BY date DESC LIMIT 1", req.SiteID).Scan(&gscKW)
		var gscRows []struct {
			Keys []string `json:"keys"`
		}
		if gscKW != "" && gscKW != "null" {
			_ = json.Unmarshal([]byte(gscKW), &gscRows)
			for _, r := range gscRows {
				if len(r.Keys) > 0 && r.Keys[0] != "" {
					keywords[r.Keys[0]] = true
				}
			}
		}
	}

	if len(keywords) == 0 {
		c.JSON(400, gin.H{"code": -1, "error": "该站点暂无关键词数据, 请先在「数据管理」同步4大平台"})
		return
	}

	// Dedupe against existing titles in this lib
	existing := map[string]bool{}
	if rows, err := db.DB.Query("SELECT title FROM geo_titles WHERE lib_id=$1", libID); err == nil {
		for rows.Next() {
			var t string
			rows.Scan(&t)
			existing[t] = true
		}
		rows.Close()
	}

	inserted := 0
	for kw := range keywords {
		if existing[kw] {
			continue
		}
		if _, err := db.DB.Exec(
			"INSERT INTO geo_titles (lib_id, title, keyword) VALUES ($1, $2, $3)",
			libID, kw, kw,
		); err == nil {
			inserted++
		}
	}
	db.DB.Exec("UPDATE geo_title_libs SET title_count = (SELECT COUNT(*) FROM geo_titles WHERE lib_id = $1) WHERE id = $2", libID, libID)

	c.JSON(200, gin.H{"code": 0, "data": gin.H{
		"imported":     inserted,
		"source_total": len(keywords),
		"skipped":      len(keywords) - inserted,
	}})
}

func GeoListTitles(c *gin.Context) {
	libID := c.Param("id")
	_, listLibSiteID, _, ok := assertGeoLibOwnership(c, libID)
	if !ok {
		return
	}
	if listLibSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", listLibSiteID)) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "100"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 500 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM geo_titles WHERE lib_id = $1", libID).Scan(&total)

	rows, err := db.DB.Query("SELECT id, lib_id, title, keyword, is_used, used_at, created_at FROM geo_titles WHERE lib_id = $1 ORDER BY id LIMIT $2 OFFSET $3", libID, perPage, offset)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	var titles []gin.H
	for rows.Next() {
		var id, libIDVal, isUsed int64
		var title, keyword, cat string
		var usedAt *string
		rows.Scan(&id, &libIDVal, &title, &keyword, &isUsed, &usedAt, &cat)
		titles = append(titles, gin.H{"id": id, "lib_id": libIDVal, "title": title, "keyword": keyword, "is_used": isUsed, "used_at": usedAt, "created_at": cat})
	}
	if titles == nil {
		titles = []gin.H{}
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"items": titles, "total": total, "page": page, "per_page": perPage}})
}

func GeoAddTitles(c *gin.Context) {
	libID := c.Param("id")
	_, addLibSiteID, _, ok := assertGeoLibOwnership(c, libID)
	if !ok {
		return
	}
	if addLibSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", addLibSiteID)) {
		return
	}
	var req struct {
		Titles []struct {
			Title   string `json:"title"`
			Keyword string `json:"keyword"`
		} `json:"titles"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Titles) == 0 {
		c.JSON(400, gin.H{"code": -1, "error": "titles 数组不能为空"})
		return
	}
	count := 0
	for _, t := range req.Titles {
		if strings.TrimSpace(t.Title) == "" {
			continue
		}
		if _, err := db.DB.Exec("INSERT INTO geo_titles (lib_id, title, keyword) VALUES ($1, $2, $3)",
			libID, strings.TrimSpace(t.Title), strings.TrimSpace(t.Keyword)); err == nil {
			count++
		}
	}
	db.DB.Exec("UPDATE geo_title_libs SET title_count = (SELECT COUNT(*) FROM geo_titles WHERE lib_id = $1) WHERE id = $2", libID, libID)
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"added": count}})
}

func GeoImportTitles(c *gin.Context) {
	libID := c.Param("id")
	_, impLibSiteID, _, ok := assertGeoLibOwnership(c, libID)
	if !ok {
		return
	}
	if impLibSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", impLibSiteID)) {
		return
	}
	var req struct {
		Text string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "text 必填"})
		return
	}
	lines := strings.Split(req.Text, "\n")
	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		title := strings.TrimSpace(parts[0])
		keyword := ""
		if len(parts) > 1 {
			keyword = strings.TrimSpace(parts[1])
		}
		if title == "" {
			continue
		}
		if _, err := db.DB.Exec("INSERT INTO geo_titles (lib_id, title, keyword) VALUES ($1, $2, $3)",
			libID, title, keyword); err == nil {
			count++
		}
	}
	db.DB.Exec("UPDATE geo_title_libs SET title_count = (SELECT COUNT(*) FROM geo_titles WHERE lib_id = $1) WHERE id = $2", libID, libID)
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"imported": count}})
}

func GeoDeleteTitle(c *gin.Context) {
	id := c.Param("id")
	var libID int64
	db.DB.QueryRow("SELECT lib_id FROM geo_titles WHERE id = $1", id).Scan(&libID)
	if libID > 0 {
		_, titleLibSiteID, _, ok := assertGeoLibOwnership(c, fmt.Sprintf("%d", libID))
		if !ok {
			return
		}
		if titleLibSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", titleLibSiteID)) {
			return
		}
	}
	db.DB.Exec("DELETE FROM geo_titles WHERE id = $1", id)
	if libID > 0 {
		db.DB.Exec("UPDATE geo_title_libs SET title_count = (SELECT COUNT(*) FROM geo_titles WHERE lib_id = $1) WHERE id = $2", libID, libID)
	}
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

// GeoImportFromGSC imports top keywords from GSC data into a keyword library.
func GeoImportFromGSC(c *gin.Context) {
	libID := c.Param("id")
	// Lib must belong to the caller (or be admin-owned orphan).
	if _, _, _, ok := assertGeoLibOwnership(c, libID); !ok {
		return
	}
	var req struct {
		SiteID int64 `json:"site_id" binding:"required"`
		Limit  int   `json:"limit"` // optional, 0 = all (position 1-99)
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "site_id 必填"})
		return
	}
	if !canAccessSite(c, fmt.Sprintf("%d", req.SiteID)) {
		return
	}

	// Get latest GSC data for this site
	var kwJSON string
	err := db.DB.QueryRow("SELECT keywords_json FROM gsc_data WHERE site_id = $1 ORDER BY date DESC LIMIT 1", req.SiteID).Scan(&kwJSON)
	if err != nil || kwJSON == "" || kwJSON == "[]" {
		c.JSON(400, gin.H{"code": -1, "error": "该站点无 GSC 关键词数据，请先在数据中心同步 GSC"})
		return
	}

	// GSC API stores rows as: [{"keys":["keyword"], "clicks":5, "impressions":100, "ctr":0.05, "position":3.5}, ...]
	type gscRow struct {
		Keys        []string `json:"keys"`
		Clicks      int      `json:"clicks"`
		Impressions int      `json:"impressions"`
		CTR         float64  `json:"ctr"`
		Position    float64  `json:"position"`
	}
	var rows []gscRow
	if err := json.Unmarshal([]byte(kwJSON), &rows); err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "GSC 数据解析失败: " + err.Error()})
		return
	}

	maxLimit := req.Limit
	if maxLimit <= 0 || maxLimit > 500 {
		maxLimit = 500 // default: import all
	}

	count := 0
	for _, row := range rows {
		if len(row.Keys) == 0 || row.Keys[0] == "" {
			continue
		}
		// Filter: only import keywords ranked 1-99
		if row.Position < 1 || row.Position > 99 {
			continue
		}
		if count >= maxLimit {
			break
		}
		keyword := row.Keys[0]
		// Skip duplicates
		var exists int
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_titles WHERE lib_id=$1 AND title=$2", libID, keyword).Scan(&exists)
		if exists > 0 {
			continue
		}
		if _, err := db.DB.Exec("INSERT INTO geo_titles (lib_id, title, keyword) VALUES ($1, $2, $3)",
			libID, keyword, keyword); err == nil {
			count++
		}
	}
	db.DB.Exec("UPDATE geo_title_libs SET title_count = (SELECT COUNT(*) FROM geo_titles WHERE lib_id = $1) WHERE id = $2", libID, libID)
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"imported": count, "source_total": len(rows)}})
}

// ─── Articles ────────────────────────────────────────────────────────────────
//
// NOTE: Prompt 模板、知识库、生成任务 tables were dropped in the wizard
// refactor. Per-keyword generation now flows through geo_workflow_jobs
// (see geo_workflow.go). Articles still carry a task_id column for
// historical rows but new articles are created with task_id=0.

// ─── Articles ────────────────────────────────────────────────────────────────

func GeoListArticles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	where := "1=1"
	args := []interface{}{}

	// Always filter by content_type
	contentType := c.DefaultQuery("content_type", "geo")
	where += fmt.Sprintf(" AND a.content_type = $%d", len(args)+1)
	args = append(args, contentType)

	if v := c.Query("site_id"); v != "" {
		if !canAccessSite(c, v) {
			return
		}
		where += fmt.Sprintf(" AND a.site_id = $%d", len(args)+1)
		args = append(args, v)
	} else {
		role, _ := c.Get("role")
		if role != "admin" {
			userID := c.GetInt64("user_id")
			where += " AND a.site_id IN (SELECT id FROM sites WHERE created_by = $1)"
			args = append(args, userID)
		}
	}
	if v := c.Query("task_id"); v != "" {
		where += fmt.Sprintf(" AND a.task_id = $%d", len(args)+1)
		args = append(args, v)
	}
	if v := c.Query("status"); v != "" {
		where += fmt.Sprintf(" AND a.status = $%d", len(args)+1)
		args = append(args, v)
	}
	if v := c.Query("review_status"); v != "" {
		where += fmt.Sprintf(" AND a.review_status = $%d", len(args)+1)
		args = append(args, v)
	}

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles a WHERE "+where, args...).Scan(&total)

	limitClause := fmt.Sprintf("LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, perPage, offset)
	// List intentionally omits the full `content` column — it can be 100+
	// KiB per row. Callers render excerpt; GeoGetArticle fetches content
	// on demand when the user opens a single article.
	rows, err := db.DB.Query(`SELECT a.id, a.task_id, a.title, a.excerpt, a.keywords,
		COALESCE(a.meta_title,''), a.meta_description,
		a.status, a.review_status, a.ai_model, a.word_count, a.created_at, a.published_at
		FROM geo_articles a
		WHERE `+where+` ORDER BY a.id DESC ` + limitClause, args...)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	var list []gin.H
	for rows.Next() {
		var id, taskID, wordCount int64
		var title, excerpt, kw, mt, md, status, reviewStatus, aiModel, cat string
		var pubAt *string
		if err := rows.Scan(&id, &taskID, &title, &excerpt, &kw, &mt, &md, &status, &reviewStatus, &aiModel, &wordCount, &cat, &pubAt); err != nil {
			continue
		}
		list = append(list, gin.H{
			"id": id, "task_id": taskID, "title": title, "excerpt": excerpt,
			"keywords": kw, "meta_title": mt, "meta_description": md, "status": status,
			"review_status": reviewStatus, "ai_model": aiModel, "word_count": wordCount,
			"created_at": cat, "published_at": pubAt,
		})
	}
	if list == nil {
		list = []gin.H{}
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"items": list, "total": total, "page": page, "per_page": perPage}})
}

func GeoGetArticle(c *gin.Context) {
	id := c.Param("id")
	var articleSiteID int64
	if err := db.DB.QueryRow("SELECT site_id FROM geo_articles WHERE id=$1", id).Scan(&articleSiteID); err != nil {
		c.JSON(404, gin.H{"code": -1, "error": "文章不存在"})
		return
	}
	if articleSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", articleSiteID)) {
		return
	}
	var aID, taskID, titleID, wordCount int64
	var title, content, excerpt, kw, metaTitle, md, category, status, reviewStatus, reviewerNote, aiModel, slug, createdAt string
	var pubAt *string
	err := db.DB.QueryRow(`SELECT id, task_id, title_id, title, slug, content, excerpt, keywords,
		COALESCE(meta_title,''), meta_description, category, status, review_status, reviewer_note,
		ai_model, word_count, created_at, published_at
		FROM geo_articles WHERE id=$1`, id).
		Scan(&aID, &taskID, &titleID, &title, &slug, &content, &excerpt, &kw,
			&metaTitle, &md, &category, &status, &reviewStatus, &reviewerNote,
			&aiModel, &wordCount, &createdAt, &pubAt)
	if err != nil {
		c.JSON(404, gin.H{"code": -1, "error": "文章不存在"})
		return
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{
		"id": aID, "task_id": taskID, "title_id": titleID, "title": title, "slug": slug,
		"content": content, "excerpt": excerpt, "keywords": kw,
		"meta_title": metaTitle, "meta_description": md,
		"category": category, "status": status, "review_status": reviewStatus,
		"reviewer_note": reviewerNote, "ai_model": aiModel, "word_count": wordCount,
		"created_at": createdAt, "published_at": pubAt,
	}})
}

func GeoUpdateArticle(c *gin.Context) {
	id := c.Param("id")
	var updSiteID int64
	if err := db.DB.QueryRow("SELECT site_id FROM geo_articles WHERE id=$1", id).Scan(&updSiteID); err != nil {
		c.JSON(404, gin.H{"code": -1, "error": "文章不存在"})
		return
	}
	if updSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", updSiteID)) {
		return
	}
	var req struct {
		Title           string `json:"title"`
		Content         string `json:"content"`
		Keywords        string `json:"keywords"`
		MetaTitle       string `json:"meta_title"`
		MetaDescription string `json:"meta_description"`
		Category        string `json:"category"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	wc := utf8.RuneCountInString(req.Content)
	if _, err := db.DB.Exec(`UPDATE geo_articles
		SET title=$1, content=$2, keywords=$3, meta_title=$4, meta_description=$5, category=$6, word_count=$7,
		    updated_at=CURRENT_TIMESTAMP
		WHERE id=$8`,
		req.Title, req.Content, req.Keywords, req.MetaTitle, req.MetaDescription, req.Category, wc, id); err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "更新失败"})
		return
	}
	c.JSON(200, gin.H{"code": 0, "data": "更新成功"})
}

func GeoDeleteArticle(c *gin.Context) {
	id := c.Param("id")
	contentType := c.DefaultQuery("content_type", "geo")
	var ct string
	var delArtSiteID int64
	if err := db.DB.QueryRow("SELECT content_type, COALESCE(site_id,0) FROM geo_articles WHERE id=$1", id).Scan(&ct, &delArtSiteID); err != nil {
		c.JSON(404, gin.H{"code": -1, "error": "文章不存在"})
		return
	}
	if ct != contentType {
		c.JSON(403, gin.H{"code": -1, "error": "无权删除"})
		return
	}
	if delArtSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", delArtSiteID)) {
		return
	}
	db.DB.Exec("DELETE FROM geo_articles WHERE id = $1", id)
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

func GeoReviewArticle(c *gin.Context) {
	id := c.Param("id")
	var revSiteID int64
	if err := db.DB.QueryRow("SELECT site_id FROM geo_articles WHERE id=$1", id).Scan(&revSiteID); err != nil {
		c.JSON(404, gin.H{"code": -1, "error": "文章不存在"})
		return
	}
	if revSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", revSiteID)) {
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"` // approved / rejected
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Status != "approved" && req.Status != "rejected") {
		c.JSON(400, gin.H{"code": -1, "error": "status 必须是 approved 或 rejected"})
		return
	}
	db.DB.Exec("UPDATE geo_articles SET review_status=$1, reviewer_note=$2, updated_at=CURRENT_TIMESTAMP WHERE id=$3",
		req.Status, req.Note, id)
	c.JSON(200, gin.H{"code": 0, "data": "审核完成"})
}

func GeoPublishArticle(c *gin.Context) {
	id := c.Param("id")
	var pubSiteID int64
	if err := db.DB.QueryRow("SELECT site_id FROM geo_articles WHERE id=$1", id).Scan(&pubSiteID); err != nil {
		c.JSON(404, gin.H{"code": -1, "error": "文章不存在"})
		return
	}
	if pubSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", pubSiteID)) {
		return
	}

	// #48: Only approved articles can be published.
	var reviewStatus string
	db.DB.QueryRow("SELECT COALESCE(review_status,'') FROM geo_articles WHERE id=$1", id).Scan(&reviewStatus)
	if reviewStatus != "approved" {
		c.JSON(400, gin.H{"code": -1, "error": "文章未通过审核，无法发布"})
		return
	}

	db.DB.Exec("UPDATE geo_articles SET status='published', published_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=$1", id)
	c.JSON(200, gin.H{"code": 0, "data": "已发布"})
}
