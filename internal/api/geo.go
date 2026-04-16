package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Helper: AI call for GEO ─────────────────────────────────────────────────

func geoCallAI(prompt string, maxTokens int) (string, error) {
	apiURL := db.GetSetting("ai_api_url")
	apiKey := db.GetSetting("ai_api_key")
	model := db.GetSetting("ai_model")
	if apiURL == "" {
		apiURL = "https://ai.router.team/v1"
	}
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	if apiKey == "" {
		return "", fmt.Errorf("AI API key 未配置")
	}
	body := map[string]interface{}{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  maxTokens,
		"temperature": 0.7,
	}
	bj, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", apiURL+"/chat/completions", bytes.NewReader(bj))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("AI HTTP %d: %.500s", resp.StatusCode, string(respBody))
	}
	var parsed struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("AI 无返回")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

// ─── Stats ───────────────────────────────────────────────────────────────────

func GeoStats(c *gin.Context) {
	siteID := c.Query("site_id")
	var totalArticles, drafts, published, reviewed, tasksActive int
	if siteID != "" {
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=?", siteID).Scan(&totalArticles)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=? AND status='draft'", siteID).Scan(&drafts)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=? AND status='published'", siteID).Scan(&published)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE site_id=? AND review_status='pending'", siteID).Scan(&reviewed)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_tasks WHERE site_id=? AND status='active'", siteID).Scan(&tasksActive)
	} else {
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles").Scan(&totalArticles)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE status='draft'").Scan(&drafts)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE status='published'").Scan(&published)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_articles WHERE review_status='pending'").Scan(&reviewed)
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_tasks WHERE status='active'").Scan(&tasksActive)
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{
		"total_articles": totalArticles, "drafts": drafts, "published": published,
		"pending_review": reviewed, "active_tasks": tasksActive,
	}})
}

// ─── Title Libraries ─────────────────────────────────────────────────────────

func GeoListTitleLibs(c *gin.Context) {
	siteID := c.Query("site_id")
	q := "SELECT id, name, description, title_count, created_at FROM geo_title_libs"
	args := []interface{}{}
	if siteID != "" {
		q += " WHERE site_id = ?"
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
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "name 必填"})
		return
	}
	res, err := db.DB.Exec("INSERT INTO geo_title_libs (name, description, site_id) VALUES (?, ?, ?)", req.Name, req.Description, req.SiteID)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "创建失败"})
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"id": id}})
}

func GeoDeleteTitleLib(c *gin.Context) {
	id := c.Param("id")
	db.DB.Exec("DELETE FROM geo_titles WHERE lib_id = ?", id)
	db.DB.Exec("DELETE FROM geo_title_libs WHERE id = ?", id)
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

func GeoListTitles(c *gin.Context) {
	libID := c.Param("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "100"))
	if page < 1 { page = 1 }
	if perPage < 1 || perPage > 500 { perPage = 100 }
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
		Limit  int   `json:"limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "site_id 必填"})
		return
	}
	if req.Limit <= 0 || req.Limit > 500 {
		req.Limit = 100
	}

	// Get latest GSC data for this site
	var kwJSON string
	err := db.DB.QueryRow("SELECT keywords_json FROM gsc_data WHERE site_id = ? ORDER BY date DESC LIMIT 1", req.SiteID).Scan(&kwJSON)
	if err != nil || kwJSON == "" || kwJSON == "[]" {
		c.JSON(400, gin.H{"code": -1, "error": "该站点无 GSC 关键词数据，请先同步 GSC"})
		return
	}

	type gscKW struct {
		Query       string  `json:"query"`
		Clicks      int     `json:"clicks"`
		Impressions int     `json:"impressions"`
		Position    float64 `json:"position"`
	}
	var keywords []gscKW
	if err := json.Unmarshal([]byte(kwJSON), &keywords); err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "GSC 关键词数据解析失败"})
		return
	}

	count := 0
	for i, kw := range keywords {
		if i >= req.Limit || kw.Query == "" {
			break
		}
		// Skip duplicates
		var exists int
		db.DB.QueryRow("SELECT COUNT(*) FROM geo_titles WHERE lib_id=? AND title=?", libID, kw.Query).Scan(&exists)
		if exists > 0 {
			continue
		}
		if _, err := db.DB.Exec("INSERT INTO geo_titles (lib_id, title, keyword) VALUES (?, ?, ?)",
			libID, kw.Query, kw.Query); err == nil {
			count++
		}
	}
	db.DB.Exec("UPDATE geo_title_libs SET title_count = (SELECT COUNT(*) FROM geo_titles WHERE lib_id = ?) WHERE id = ?", libID, libID)
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"imported": count, "source_total": len(keywords)}})
}

// ─── Prompts ─────────────────────────────────────────────────────────────────

func GeoListPrompts(c *gin.Context) {
	rows, err := db.DB.Query("SELECT id, name, type, content, description, created_at, updated_at FROM geo_prompts ORDER BY id DESC LIMIT 200")
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	var list []gin.H
	for rows.Next() {
		var id int64
		var name, typ, content, desc, cat, uat string
		rows.Scan(&id, &name, &typ, &content, &desc, &cat, &uat)
		list = append(list, gin.H{"id": id, "name": name, "type": typ, "content": content, "description": desc, "created_at": cat, "updated_at": uat})
	}
	if list == nil {
		list = []gin.H{}
	}
	c.JSON(200, gin.H{"code": 0, "data": list})
}

func GeoCreatePrompt(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Type        string `json:"type"`
		Content     string `json:"content" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "name 和 content 必填"})
		return
	}
	if req.Type == "" {
		req.Type = "content"
	}
	res, err := db.DB.Exec("INSERT INTO geo_prompts (name, type, content, description) VALUES (?, ?, ?, ?)",
		req.Name, req.Type, req.Content, req.Description)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "创建失败"})
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"id": id}})
}

func GeoUpdatePrompt(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Content     string `json:"content"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	db.DB.Exec("UPDATE geo_prompts SET name=?, type=?, content=?, description=?, updated_at=CURRENT_TIMESTAMP WHERE id=?",
		req.Name, req.Type, req.Content, req.Description, id)
	c.JSON(200, gin.H{"code": 0, "data": "更新成功"})
}

func GeoDeletePrompt(c *gin.Context) {
	db.DB.Exec("DELETE FROM geo_prompts WHERE id = ?", c.Param("id"))
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

// ─── Knowledge Bases ─────────────────────────────────────────────────────────

func GeoListKnowledge(c *gin.Context) {
	rows, err := db.DB.Query("SELECT id, name, char_count, created_at FROM geo_knowledge ORDER BY id DESC LIMIT 200")
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	var list []gin.H
	for rows.Next() {
		var id, cc int64
		var name, cat string
		rows.Scan(&id, &name, &cc, &cat)
		list = append(list, gin.H{"id": id, "name": name, "char_count": cc, "created_at": cat})
	}
	if list == nil {
		list = []gin.H{}
	}
	c.JSON(200, gin.H{"code": 0, "data": list})
}

func GeoCreateKnowledge(c *gin.Context) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "name 和 content 必填"})
		return
	}
	cc := utf8.RuneCountInString(req.Content)
	res, err := db.DB.Exec("INSERT INTO geo_knowledge (name, content, char_count) VALUES (?, ?, ?)", req.Name, req.Content, cc)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "创建失败"})
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"id": id}})
}

func GeoUpdateKnowledge(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	c.ShouldBindJSON(&req)
	cc := utf8.RuneCountInString(req.Content)
	db.DB.Exec("UPDATE geo_knowledge SET name=?, content=?, char_count=? WHERE id=?", req.Name, req.Content, cc, id)
	c.JSON(200, gin.H{"code": 0, "data": "更新成功"})
}

func GeoDeleteKnowledge(c *gin.Context) {
	db.DB.Exec("DELETE FROM geo_knowledge WHERE id = ?", c.Param("id"))
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

// ─── Tasks ───────────────────────────────────────────────────────────────────

func GeoListTasks(c *gin.Context) {
	siteID := c.Query("site_id")
	q := `SELECT t.id, t.name, t.title_lib_id, t.prompt_id, t.knowledge_id, t.status,
		       t.draft_limit, t.need_review, t.is_loop, t.auto_keywords, t.auto_description,
		       t.created_count, t.last_run_at, t.created_at,
		       COALESCE(tl.name,''), COALESCE(p.name,''),
		       COALESCE(SUM(CASE WHEN a.status='draft' THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN a.status='published' THEN 1 ELSE 0 END),0)
		FROM geo_tasks t
		LEFT JOIN geo_title_libs tl ON t.title_lib_id = tl.id
		LEFT JOIN geo_prompts p ON t.prompt_id = p.id
		LEFT JOIN geo_articles a ON a.task_id = t.id`
	args := []interface{}{}
	if siteID != "" {
		q += " WHERE t.site_id = ?"
		args = append(args, siteID)
	}
	q += " GROUP BY t.id ORDER BY t.id DESC LIMIT 200"
	rows, err := db.DB.Query(q, args...)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	var list []gin.H
	for rows.Next() {
		var id, titleLibID, promptID, knowledgeID, draftLimit, needReview, isLoop, autoKW, autoDesc, createdCount int64
		var name, status, cat, tlName, pName string
		var lastRunAt *string
		var draftCount, publishedCount int
		if err := rows.Scan(&id, &name, &titleLibID, &promptID, &knowledgeID, &status,
			&draftLimit, &needReview, &isLoop, &autoKW, &autoDesc,
			&createdCount, &lastRunAt, &cat, &tlName, &pName,
			&draftCount, &publishedCount); err != nil {
			continue
		}
		list = append(list, gin.H{
			"id": id, "name": name, "title_lib_id": titleLibID, "prompt_id": promptID,
			"knowledge_id": knowledgeID, "status": status, "draft_limit": draftLimit,
			"need_review": needReview, "is_loop": isLoop, "auto_keywords": autoKW,
			"auto_description": autoDesc, "created_count": createdCount,
			"last_run_at": lastRunAt, "created_at": cat,
			"title_lib_name": tlName, "prompt_name": pName,
			"draft_count": draftCount, "published_count": publishedCount,
		})
	}
	if list == nil {
		list = []gin.H{}
	}
	c.JSON(200, gin.H{"code": 0, "data": list})
}

func GeoCreateTask(c *gin.Context) {
	var req struct {
		Name            string `json:"name" binding:"required"`
		SiteID          int64  `json:"site_id"`
		TitleLibID      int64  `json:"title_lib_id" binding:"required"`
		PromptID        int64  `json:"prompt_id" binding:"required"`
		KnowledgeID     int64  `json:"knowledge_id"`
		DraftLimit      int    `json:"draft_limit"`
		NeedReview      int    `json:"need_review"`
		IsLoop          int    `json:"is_loop"`
		AutoKeywords    int    `json:"auto_keywords"`
		AutoDescription int    `json:"auto_description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "name, title_lib_id, prompt_id 必填"})
		return
	}
	if req.DraftLimit <= 0 {
		req.DraftLimit = 20
	}
	res, err := db.DB.Exec(`INSERT INTO geo_tasks (name, site_id, title_lib_id, prompt_id, knowledge_id, draft_limit, need_review, is_loop, auto_keywords, auto_description)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Name, req.SiteID, req.TitleLibID, req.PromptID, req.KnowledgeID, req.DraftLimit, req.NeedReview, req.IsLoop, req.AutoKeywords, req.AutoDescription)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "创建失败"})
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"id": id}})
}

func GeoUpdateTask(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name            string `json:"name"`
		TitleLibID      int64  `json:"title_lib_id"`
		PromptID        int64  `json:"prompt_id"`
		KnowledgeID     int64  `json:"knowledge_id"`
		DraftLimit      int    `json:"draft_limit"`
		NeedReview      int    `json:"need_review"`
		IsLoop          int    `json:"is_loop"`
		AutoKeywords    int    `json:"auto_keywords"`
		AutoDescription int    `json:"auto_description"`
	}
	c.ShouldBindJSON(&req)
	db.DB.Exec(`UPDATE geo_tasks SET name=?, title_lib_id=?, prompt_id=?, knowledge_id=?, draft_limit=?, need_review=?, is_loop=?, auto_keywords=?, auto_description=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		req.Name, req.TitleLibID, req.PromptID, req.KnowledgeID, req.DraftLimit, req.NeedReview, req.IsLoop, req.AutoKeywords, req.AutoDescription, id)
	c.JSON(200, gin.H{"code": 0, "data": "更新成功"})
}

func GeoDeleteTask(c *gin.Context) {
	id := c.Param("id")
	db.DB.Exec("DELETE FROM geo_articles WHERE task_id = ?", id)
	db.DB.Exec("DELETE FROM geo_tasks WHERE id = ?", id)
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

func GeoStartTask(c *gin.Context) {
	db.DB.Exec("UPDATE geo_tasks SET status='active', updated_at=CURRENT_TIMESTAMP WHERE id=?", c.Param("id"))
	c.JSON(200, gin.H{"code": 0, "data": "已启动"})
}

func GeoStopTask(c *gin.Context) {
	db.DB.Exec("UPDATE geo_tasks SET status='paused', updated_at=CURRENT_TIMESTAMP WHERE id=?", c.Param("id"))
	c.JSON(200, gin.H{"code": 0, "data": "已暂停"})
}

// GeoRunTask executes one article generation NOW.
func GeoRunTask(c *gin.Context) {
	taskID := c.Param("id")

	// Load task
	var name string
	var siteIDVal, titleLibID, promptID, knowledgeID, autoKW, autoDesc, isLoop int64
	if err := db.DB.QueryRow(`SELECT name, COALESCE(site_id,0), title_lib_id, prompt_id, COALESCE(knowledge_id,0), auto_keywords, auto_description, is_loop FROM geo_tasks WHERE id=?`, taskID).
		Scan(&name, &siteIDVal, &titleLibID, &promptID, &knowledgeID, &autoKW, &autoDesc, &isLoop); err != nil {
		c.JSON(404, gin.H{"code": -1, "error": "任务不存在"})
		return
	}

	// Get next unused title
	var titleRowID int64
	var titleText, keyword string
	titleErr := db.DB.QueryRow("SELECT id, title, keyword FROM geo_titles WHERE lib_id=? AND is_used=0 ORDER BY id LIMIT 1", titleLibID).
		Scan(&titleRowID, &titleText, &keyword)
	if titleErr != nil {
		if isLoop == 1 {
			db.DB.Exec("UPDATE geo_titles SET is_used=0, used_at=NULL WHERE lib_id=?", titleLibID)
			titleErr = db.DB.QueryRow("SELECT id, title, keyword FROM geo_titles WHERE lib_id=? AND is_used=0 ORDER BY id LIMIT 1", titleLibID).
				Scan(&titleRowID, &titleText, &keyword)
		}
		if titleErr != nil {
			c.JSON(400, gin.H{"code": -1, "error": "标题库已用尽（可开启循环模式）"})
			return
		}
	}

	// Load prompt template
	var promptContent string
	if err := db.DB.QueryRow("SELECT content FROM geo_prompts WHERE id=?", promptID).Scan(&promptContent); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "Prompt 模板不存在"})
		return
	}

	// Load knowledge (optional)
	knowledgeText := ""
	if knowledgeID > 0 {
		db.DB.QueryRow("SELECT content FROM geo_knowledge WHERE id=?", knowledgeID).Scan(&knowledgeText)
	}

	// Build prompt
	finalPrompt := promptContent
	finalPrompt = strings.ReplaceAll(finalPrompt, "{{title}}", titleText)
	finalPrompt = strings.ReplaceAll(finalPrompt, "{{keyword}}", keyword)
	finalPrompt = strings.ReplaceAll(finalPrompt, "{{knowledge}}", knowledgeText)

	// Call AI for main content
	content, err := geoCallAI(finalPrompt, 4000)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "AI 生成失败: " + err.Error()})
		return
	}

	// Auto-extract keywords
	keywords := keyword
	if autoKW == 1 && content != "" {
		kw, kerr := geoCallAI(fmt.Sprintf("从以下文章中提取 5-8 个 SEO 关键词，用英文逗号分隔，只输出关键词不要其他内容：\n\n%s", content), 200)
		if kerr == nil && kw != "" {
			keywords = kw
		}
	}

	// Auto-generate meta description
	metaDesc := ""
	if autoDesc == 1 && content != "" {
		md, merr := geoCallAI(fmt.Sprintf("为以下文章生成一段 SEO meta description，不超过 160 字符，直接输出描述文字不要加引号或前缀：\n\n%s", content), 200)
		if merr == nil && md != "" {
			metaDesc = md
		}
	}

	// Generate excerpt
	excerpt := content
	if len([]rune(excerpt)) > 200 {
		excerpt = string([]rune(excerpt)[:200]) + "..."
	}

	// Get AI model name
	aiModel := db.GetSetting("ai_model")
	if aiModel == "" {
		aiModel = "claude-sonnet-4-6"
	}

	wordCount := utf8.RuneCountInString(content)

	// Save article
	res, err := db.DB.Exec(`INSERT INTO geo_articles (task_id, site_id, title_id, title, content, excerpt, keywords, meta_description, ai_model, word_count, status, review_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'draft', 'pending')`,
		taskID, siteIDVal, titleRowID, titleText, content, excerpt, keywords, metaDesc, aiModel, wordCount)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "保存文章失败: " + err.Error()})
		return
	}
	articleID, _ := res.LastInsertId()

	// Mark title as used
	db.DB.Exec("UPDATE geo_titles SET is_used=1, used_at=CURRENT_TIMESTAMP WHERE id=?", titleRowID)

	// Update task stats
	db.DB.Exec("UPDATE geo_tasks SET created_count=created_count+1, last_run_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=?", taskID)

	c.JSON(200, gin.H{"code": 0, "data": gin.H{
		"article_id":       articleID,
		"title":            titleText,
		"word_count":       wordCount,
		"keywords":         keywords,
		"meta_description": metaDesc,
	}})
}

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
	if v := c.Query("site_id"); v != "" {
		where += " AND a.site_id = ?"
		args = append(args, v)
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
	rows, err := db.DB.Query(`SELECT a.id, a.task_id, a.title, a.excerpt, a.keywords, a.meta_description, a.status, a.review_status, a.ai_model, a.word_count, a.created_at, a.published_at, COALESCE(t.name,'')
		FROM geo_articles a LEFT JOIN geo_tasks t ON a.task_id = t.id
		WHERE `+where+` ORDER BY a.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "查询失败"})
		return
	}
	defer rows.Close()
	var list []gin.H
	for rows.Next() {
		var id, taskID, wordCount int64
		var title, excerpt, kw, md, status, reviewStatus, aiModel, cat, taskName string
		var pubAt *string
		rows.Scan(&id, &taskID, &title, &excerpt, &kw, &md, &status, &reviewStatus, &aiModel, &wordCount, &cat, &pubAt, &taskName)
		list = append(list, gin.H{
			"id": id, "task_id": taskID, "title": title, "excerpt": excerpt,
			"keywords": kw, "meta_description": md, "status": status,
			"review_status": reviewStatus, "ai_model": aiModel, "word_count": wordCount,
			"created_at": cat, "published_at": pubAt, "task_name": taskName,
		})
	}
	if list == nil {
		list = []gin.H{}
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"items": list, "total": total, "page": page, "per_page": perPage}})
}

func GeoGetArticle(c *gin.Context) {
	id := c.Param("id")
	var aID, taskID, titleID, wordCount int64
	var title, content, excerpt, kw, md, category, status, reviewStatus, reviewerNote, aiModel, slug, createdAt string
	var pubAt *string
	err := db.DB.QueryRow(`SELECT id, task_id, title_id, title, slug, content, excerpt, keywords, meta_description, category, status, review_status, reviewer_note, ai_model, word_count, created_at, published_at
		FROM geo_articles WHERE id=?`, id).
		Scan(&aID, &taskID, &titleID, &title, &slug, &content, &excerpt, &kw, &md, &category, &status, &reviewStatus, &reviewerNote, &aiModel, &wordCount, &createdAt, &pubAt)
	if err != nil {
		c.JSON(404, gin.H{"code": -1, "error": "文章不存在"})
		return
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{
		"id": aID, "task_id": taskID, "title_id": titleID, "title": title, "slug": slug,
		"content": content, "excerpt": excerpt, "keywords": kw, "meta_description": md,
		"category": category, "status": status, "review_status": reviewStatus,
		"reviewer_note": reviewerNote, "ai_model": aiModel, "word_count": wordCount,
		"created_at": createdAt, "published_at": pubAt,
	}})
}

func GeoUpdateArticle(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Title           string `json:"title"`
		Content         string `json:"content"`
		Keywords        string `json:"keywords"`
		MetaDescription string `json:"meta_description"`
		Category        string `json:"category"`
	}
	c.ShouldBindJSON(&req)
	wc := utf8.RuneCountInString(req.Content)
	db.DB.Exec(`UPDATE geo_articles SET title=?, content=?, keywords=?, meta_description=?, category=?, word_count=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		req.Title, req.Content, req.Keywords, req.MetaDescription, req.Category, wc, id)
	c.JSON(200, gin.H{"code": 0, "data": "更新成功"})
}

func GeoDeleteArticle(c *gin.Context) {
	db.DB.Exec("DELETE FROM geo_articles WHERE id = ?", c.Param("id"))
	c.JSON(200, gin.H{"code": 0, "data": "删除成功"})
}

func GeoReviewArticle(c *gin.Context) {
	id := c.Param("id")
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
	db.DB.Exec("UPDATE geo_articles SET status='published', published_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=?", c.Param("id"))
	c.JSON(200, gin.H{"code": 0, "data": "已发布"})
}
