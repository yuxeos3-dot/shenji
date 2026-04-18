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
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=? AND content_type=?", siteID, contentType).Scan(&totalArticles)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=? AND status='draft' AND content_type=?", siteID, contentType).Scan(&drafts)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=? AND status='published' AND content_type=?", siteID, contentType).Scan(&published)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=? AND review_status='pending' AND content_type=?", siteID, contentType).Scan(&reviewed)
	} else {
		role, _ := c.Get("role")
		if role == "admin" {
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE content_type=?", contentType).Scan(&totalArticles)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE status='draft' AND content_type=?", contentType).Scan(&drafts)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE status='published' AND content_type=?", contentType).Scan(&published)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE review_status='pending' AND content_type=?", contentType).Scan(&reviewed)
		} else {
			userID := c.GetInt64("user_id")
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE content_type=? AND site_id IN (SELECT id FROM sites WHERE created_by=?)", contentType, userID).Scan(&totalArticles)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE status='draft' AND content_type=? AND site_id IN (SELECT id FROM sites WHERE created_by=?)", contentType, userID).Scan(&drafts)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE status='published' AND content_type=? AND site_id IN (SELECT id FROM sites WHERE created_by=?)", contentType, userID).Scan(&published)
			db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE review_status='pending' AND content_type=? AND site_id IN (SELECT id FROM sites WHERE created_by=?)", contentType, userID).Scan(&reviewed)
		}
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{
		"total_articles": totalArticles, "drafts": drafts, "published": published,
		"pending_review": reviewed,
	}})
}

// ─── Title Libraries ─────────────────────────────────────────────────────────

func GeoListTitleLibs(c *gin.Context) {
	siteID := c.Query("site_id")
	contentType := c.DefaultQuery("content_type", "geo")
	q := "SELECT id, name, description, title_count, created_at FROM geo_title_libs WHERE content_type = ?"
	args := []interface{}{contentType}
	if siteID != "" {
		q += " AND site_id = ?"
		args = append(args, siteID)
	}
	q += " ORDER BY id DESC LIMIT 200"
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
		rows.Scan(&id, &name, &desc, &cnt, &cat)
		libs = append(libs, gin.H{"id": id, "name": name, "description": desc, "title_count": cnt, "created_at": cat})
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
	res, err := db.DB.Exec("INSERT INTO geo_title_libs (name, description, site_id, content_type) VALUES (?, ?, ?, ?)", req.Name, req.Description, req.SiteID, req.ContentType)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "创建失败"})
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"id": id}})
}

func GeoDeleteTitleLib(c *gin.Context) {
	id := c.Param("id")
	contentType := c.DefaultQuery("content_type", "geo")
	// Only delete if content_type matches, prevents cross-type deletion
	var ct string
	var libSiteID int64
	if err := db.DB.QueryRow("SELECT content_type, COALESCE(site_id,0) FROM geo_title_libs WHERE id=?", id).Scan(&ct, &libSiteID); err != nil {
		c.JSON(404, gin.H{"code": -1, "error": "关键词库不存在"})
		return
	}
	if ct != contentType {
		c.JSON(403, gin.H{"code": -1, "error": "无权删除"})
		return
	}
	if libSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", libSiteID)) {
		return
	}
	db.DB.Exec("DELETE FROM geo_titles WHERE lib_id = ?", id)
	db.DB.Exec("DELETE FROM geo_title_libs WHERE id = ?", id)
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

func GeoListTitles(c *gin.Context) {
	libID := c.Param("id")
	var listLibSiteID int64
	db.DB.QueryRow("SELECT COALESCE(site_id,0) FROM geo_title_libs WHERE id=?", libID).Scan(&listLibSiteID)
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
	db.DB.QueryRow("SELECT COUNT(*) FROM geo_titles WHERE lib_id = ?", libID).Scan(&total)

	rows, err := db.DB.Query("SELECT id, lib_id, title, keyword, is_used, used_at, created_at FROM geo_titles WHERE lib_id = ? ORDER BY id LIMIT ? OFFSET ?", libID, perPage, offset)
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
	var addLibSiteID int64
	db.DB.QueryRow("SELECT COALESCE(site_id,0) FROM geo_title_libs WHERE id=?", libID).Scan(&addLibSiteID)
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
		if _, err := db.DB.Exec("INSERT INTO geo_titles (lib_id, title, keyword) VALUES (?, ?, ?)",
			libID, strings.TrimSpace(t.Title), strings.TrimSpace(t.Keyword)); err == nil {
			count++
		}
	}
	db.DB.Exec("UPDATE geo_title_libs SET title_count = (SELECT COUNT(*) FROM geo_titles WHERE lib_id = ?) WHERE id = ?", libID, libID)
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"added": count}})
}

func GeoImportTitles(c *gin.Context) {
	libID := c.Param("id")
	var impLibSiteID int64
	db.DB.QueryRow("SELECT COALESCE(site_id,0) FROM geo_title_libs WHERE id=?", libID).Scan(&impLibSiteID)
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
		if _, err := db.DB.Exec("INSERT INTO geo_titles (lib_id, title, keyword) VALUES (?, ?, ?)",
			libID, title, keyword); err == nil {
			count++
		}
	}
	db.DB.Exec("UPDATE geo_title_libs SET title_count = (SELECT COUNT(*) FROM geo_titles WHERE lib_id = ?) WHERE id = ?", libID, libID)
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"imported": count}})
}

func GeoDeleteTitle(c *gin.Context) {
	id := c.Param("id")
	var libID int64
	db.DB.QueryRow("SELECT lib_id FROM geo_titles WHERE id = ?", id).Scan(&libID)
	if libID > 0 {
		var titleLibSiteID int64
		db.DB.QueryRow("SELECT COALESCE(site_id,0) FROM geo_title_libs WHERE id=?", libID).Scan(&titleLibSiteID)
		if titleLibSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", titleLibSiteID)) {
			return
		}
	}
	db.DB.Exec("DELETE FROM geo_titles WHERE id = ?", id)
	if libID > 0 {
		db.DB.Exec("UPDATE geo_title_libs SET title_count = (SELECT COUNT(*) FROM geo_titles WHERE lib_id = ?) WHERE id = ?", libID, libID)
	}
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

// GeoImportFromGSC imports top keywords from GSC data into a keyword library.
func GeoImportFromGSC(c *gin.Context) {
	libID := c.Param("id")
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
	err := db.DB.QueryRow("SELECT keywords_json FROM gsc_data WHERE site_id = ? ORDER BY date DESC LIMIT 1", req.SiteID).Scan(&kwJSON)
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
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_titles WHERE lib_id=? AND title=?", libID, keyword).Scan(&exists)
		if exists > 0 {
			continue
		}
		if _, err := db.DB.Exec("INSERT INTO geo_titles (lib_id, title, keyword) VALUES (?, ?, ?)",
			libID, keyword, keyword); err == nil {
			count++
		}
	}
	db.DB.Exec("UPDATE geo_title_libs SET title_count = (SELECT COUNT(*) FROM geo_titles WHERE lib_id = ?) WHERE id = ?", libID, libID)
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
	where += " AND a.content_type = ?"
	args = append(args, contentType)

	if v := c.Query("site_id"); v != "" {
		if !canAccessSite(c, v) {
			return
		}
		where += " AND a.site_id = ?"
		args = append(args, v)
	} else {
		role, _ := c.Get("role")
		if role != "admin" {
			userID := c.GetInt64("user_id")
			where += " AND a.site_id IN (SELECT id FROM sites WHERE created_by = ?)"
			args = append(args, userID)
		}
	}
	if v := c.Query("task_id"); v != "" {
		where += " AND a.task_id = ?"
		args = append(args, v)
	}
	if v := c.Query("status"); v != "" {
		where += " AND a.status = ?"
		args = append(args, v)
	}
	if v := c.Query("review_status"); v != "" {
		where += " AND a.review_status = ?"
		args = append(args, v)
	}

	var total int
	db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles a WHERE "+where, args...).Scan(&total)

	args = append(args, perPage, offset)
	// List intentionally omits the full `content` column — it can be 100+
	// KiB per row. Callers render excerpt; GeoGetArticle fetches content
	// on demand when the user opens a single article.
	rows, err := db.DB.Query(`SELECT a.id, a.task_id, a.title, a.excerpt, a.keywords,
		COALESCE(a.meta_title,''), a.meta_description,
		a.status, a.review_status, a.ai_model, a.word_count, a.created_at, a.published_at
		FROM geo_articles a
		WHERE `+where+` ORDER BY a.id DESC LIMIT ? OFFSET ?`, args...)
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
	if err := db.DB.QueryRow("SELECT site_id FROM geo_articles WHERE id=?", id).Scan(&articleSiteID); err != nil {
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
		FROM geo_articles WHERE id=?`, id).
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
	if err := db.DB.QueryRow("SELECT site_id FROM geo_articles WHERE id=?", id).Scan(&updSiteID); err != nil {
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
		SET title=?, content=?, keywords=?, meta_title=?, meta_description=?, category=?, word_count=?,
		    updated_at=CURRENT_TIMESTAMP
		WHERE id=?`,
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
	if err := db.DB.QueryRow("SELECT content_type, COALESCE(site_id,0) FROM geo_articles WHERE id=?", id).Scan(&ct, &delArtSiteID); err != nil {
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
	db.DB.Exec("DELETE FROM geo_articles WHERE id = ?", id)
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

func GeoReviewArticle(c *gin.Context) {
	id := c.Param("id")
	var revSiteID int64
	if err := db.DB.QueryRow("SELECT site_id FROM geo_articles WHERE id=?", id).Scan(&revSiteID); err != nil {
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
	db.DB.Exec("UPDATE geo_articles SET review_status=?, reviewer_note=?, updated_at=CURRENT_TIMESTAMP WHERE id=?",
		req.Status, req.Note, id)
	c.JSON(200, gin.H{"code": 0, "data": "审核完成"})
}

func GeoPublishArticle(c *gin.Context) {
	id := c.Param("id")
	var pubSiteID int64
	if err := db.DB.QueryRow("SELECT site_id FROM geo_articles WHERE id=?", id).Scan(&pubSiteID); err != nil {
		c.JSON(404, gin.H{"code": -1, "error": "文章不存在"})
		return
	}
	if pubSiteID > 0 && !canAccessSite(c, fmt.Sprintf("%d", pubSiteID)) {
		return
	}

	// #48: Only approved articles can be published.
	var reviewStatus string
	db.DB.QueryRow("SELECT COALESCE(review_status,'') FROM geo_articles WHERE id=?", id).Scan(&reviewStatus)
	if reviewStatus != "approved" {
		c.JSON(400, gin.H{"code": -1, "error": "文章未通过审核，无法发布"})
		return
	}

	db.DB.Exec("UPDATE geo_articles SET status='published', published_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=?", id)
	c.JSON(200, gin.H{"code": 0, "data": "已发布"})
}
