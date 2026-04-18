package api

// FRW image / video generation API handlers.
//
// URL scheme:
//   GET    /api/v1/frw/config                   — current user's config (masked)
//   PUT    /api/v1/frw/config                   — save user's apikey/token/user_id
//   GET    /api/v1/frw/generations?kind=image   — list user's generation tasks
//   POST   /api/v1/frw/generations              — submit new task
//   GET    /api/v1/frw/generations/:id          — fetch single task (live-polls if still running)
//   POST   /api/v1/frw/generations/:id/publish  — download result to local /static
//   DELETE /api/v1/frw/generations/:id          — delete task

import (
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"seo-system/internal/db"
	"seo-system/internal/frw"

	"github.com/gin-gonic/gin"
)

// frwConfig is what we persist in users.frw_config. Under the per-user
// model there is NO global fallback — each user must configure their
// own apikey via 个人中心. apikey now lives in users.frw_apikey
// (encrypted); bearer token + frw user_id stay in frw_config since
// they're short-lived browser credentials the user re-pastes every
// few hours.
type frwConfig struct {
	APIKey string `json:"apikey"`
	Token  string `json:"token"`   // Bearer
	UserID string `json:"user_id"` // FRW user id
}

// loadFRWConfig reads the user's FRW config: apikey from the dedicated
// users.frw_apikey column (encrypted), bearer + user id from
// users.frw_config. No global fallback.
func loadFRWConfig(userID int64) frwConfig {
	var raw string
	db.DB.QueryRow("SELECT COALESCE(frw_config,'') FROM users WHERE id=?", userID).Scan(&raw)
	var cfg frwConfig
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	// apikey now comes from the encrypted per-user column, never a
	// global setting. Callers get "" when the user hasn't configured it
	// and should refuse to call FRW rather than using a shared key.
	if v := LoadUserFrwApiKey(userID); v != "" {
		cfg.APIKey = v
	}
	return cfg
}

// ─── GET /frw/config ─────────────────────────────────────────────────────

func GetMyFRWConfig(c *gin.Context) {
	userID := c.GetInt64("user_id")
	cfg := loadFRWConfig(userID)
	// Mask sensitive values — UI shows "已配置" vs "未配置" for each.
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"has_apikey":  cfg.APIKey != "",
		"has_token":   cfg.Token != "",
		"user_id":     cfg.UserID, // user_id isn't secret, show it so user can verify
		"apikey_hint": maskSecret(cfg.APIKey),
		"token_hint":  maskSecret(cfg.Token),
	}})
}

func maskSecret(s string) string {
	if len(s) <= 8 {
		if s == "" {
			return ""
		}
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

// ─── PUT /frw/config ─────────────────────────────────────────────────────

func SaveMyFRWConfig(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var in struct {
		APIKey string `json:"apikey"`
		Token  string `json:"token"`
		UserID string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	old := loadFRWConfig(userID)
	// apikey goes to the encrypted users.frw_apikey column; bearer +
	// user_id stay in frw_config (non-secret browser session bits).
	newAPIKey := firstNonEmpty(stripMask(in.APIKey, old.APIKey), old.APIKey)
	cfg := frwConfig{
		APIKey: "", // never stored in frw_config anymore
		Token:  firstNonEmpty(stripMask(in.Token, old.Token), old.Token),
		UserID: firstNonEmpty(strings.TrimSpace(in.UserID), old.UserID),
	}
	js, _ := json.Marshal(cfg)
	if _, err := db.DB.Exec("UPDATE users SET frw_config=? WHERE id=?", string(js), userID); err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "保存失败"})
		return
	}
	if newAPIKey != "" {
		enc, err := EncryptToken(newAPIKey)
		if err != nil {
			c.JSON(500, gin.H{"code": -1, "error": "加密失败"})
			return
		}
		if _, err := db.DB.Exec("UPDATE users SET frw_apikey=? WHERE id=?", enc, userID); err != nil {
			c.JSON(500, gin.H{"code": -1, "error": "保存失败"})
			return
		}
	}
	c.JSON(200, gin.H{"code": 0, "data": "FRW 配置已保存"})
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
func stripMask(v, old string) string {
	v = strings.TrimSpace(v)
	if v == "****" {
		return ""
	}
	if len(old) > 8 && v == old[:4]+"****"+old[len(old)-4:] {
		return ""
	}
	return v
}

// ─── GET /frw/generations ────────────────────────────────────────────────

func ListFRWGenerations(c *gin.Context) {
	userID := c.GetInt64("user_id")
	kind := c.Query("kind") // "image" | "video"; empty = both
	q := "SELECT id, kind, mode, prompt, source_url, frw_task_id, status, progress, result_url, local_path, error_msg, created_at FROM ai_generations WHERE user_id=?"
	args := []interface{}{userID}
	if kind == "image" || kind == "video" {
		q += " AND kind=?"
		args = append(args, kind)
	}
	q += " ORDER BY id DESC LIMIT 200"
	rows, err := db.DB.Query(q, args...)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id int64
		var kd, md, pr, sr, tid, ru, lp, em string
		var st int
		var pg float64
		var ct time.Time
		if err := rows.Scan(&id, &kd, &md, &pr, &sr, &tid, &st, &pg, &ru, &lp, &em, &ct); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id":          id,
			"kind":        kd,
			"mode":        md,
			"prompt":      pr,
			"source_url":  sr,
			"frw_task_id": tid,
			"status":      st,
			"progress":    pg,
			"result_url":  ru,
			"local_path":  lp,
			"error_msg":   em,
			"created_at":  ct,
		})
	}
	c.JSON(200, gin.H{"code": 0, "data": out})
}

// ─── POST /frw/generations ───────────────────────────────────────────────

func SubmitFRWGeneration(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var in struct {
		Mode      string `json:"mode"` // text2image / img2img / text2video / img2video
		Prompt    string `json:"prompt"`
		SourceURL string `json:"source_url"` // for img2img / img2video
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Prompt) == "" {
		c.JSON(400, gin.H{"code": -1, "error": "缺少 prompt"})
		return
	}
	kind := modeToKind(in.Mode)
	if kind == "" {
		c.JSON(400, gin.H{"code": -1, "error": "mode 不合法"})
		return
	}
	if (in.Mode == "img2img" || in.Mode == "img2video") && strings.TrimSpace(in.SourceURL) == "" {
		c.JSON(400, gin.H{"code": -1, "error": "该模式需要参考图 URL"})
		return
	}

	cfg := loadFRWConfig(userID)
	client := frw.New(cfg.APIKey, cfg.Token, cfg.UserID)
	res, err := client.Submit(in.Mode, in.Prompt, in.SourceURL)
	if err != nil {
		c.JSON(400, gin.H{"code": -1, "error": err.Error()})
		return
	}

	r, err := db.DB.Exec(`INSERT INTO ai_generations (user_id, kind, mode, prompt, source_url, frw_task_id, status, progress) VALUES (?,?,?,?,?,?,?,?)`,
		userID, kind, in.Mode, in.Prompt, in.SourceURL, res.TaskID, res.Status, res.Progress)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "保存任务失败: " + err.Error()})
		return
	}
	id, _ := r.LastInsertId()

	// Fire & forget poller.
	go pollFRWTask(id)

	c.JSON(200, gin.H{"code": 0, "data": gin.H{"id": id, "frw_task_id": res.TaskID}})
}

func modeToKind(mode string) string {
	switch mode {
	case "text2image", "img2img":
		return "image"
	case "text2video", "img2video":
		return "video"
	}
	return ""
}

// ─── GET /frw/generations/:id ────────────────────────────────────────────
// Returns the current row; if still pending/running, kicks off a refresh
// poll so the frontend can poll *our* endpoint instead of hitting FRW
// itself. Cheap because the actual polling is throttled per-task.

func GetFRWGeneration(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	row := fetchGenerationRow(id, userID)
	if row == nil {
		c.JSON(404, gin.H{"code": -1, "error": "任务不存在"})
		return
	}
	status, _ := row["status"].(int)
	// If still processing, nudge the poller. pollFRWTask is idempotent
	// thanks to activeFRWPolls.
	if status == 0 || status == 1 {
		go pollFRWTask(id)
	}
	c.JSON(200, gin.H{"code": 0, "data": row})
}

func fetchGenerationRow(id, userID int64) gin.H {
	var kd, md, pr, sr, tid, ru, lp, em string
	var st int
	var pg float64
	var ct time.Time
	err := db.DB.QueryRow(`SELECT kind, mode, prompt, source_url, frw_task_id, status, progress, result_url, local_path, error_msg, created_at FROM ai_generations WHERE id=? AND user_id=?`, id, userID).Scan(&kd, &md, &pr, &sr, &tid, &st, &pg, &ru, &lp, &em, &ct)
	if err != nil {
		return nil
	}
	return gin.H{
		"id": id, "kind": kd, "mode": md, "prompt": pr, "source_url": sr,
		"frw_task_id": tid, "status": st, "progress": pg,
		"result_url": ru, "local_path": lp, "error_msg": em, "created_at": ct,
	}
}

// ─── Poller ──────────────────────────────────────────────────────────────
//
// One goroutine per running task, deduplicated by ID. 10-second poll for
// images, 20-second for videos (matches the openskills defaults). Hard
// timeout at 15 minutes.

var (
	activeFRWPolls   = map[int64]struct{}{}
	activeFRWPollsMu sync.Mutex
)

func pollFRWTask(id int64) {
	activeFRWPollsMu.Lock()
	if _, running := activeFRWPolls[id]; running {
		activeFRWPollsMu.Unlock()
		return
	}
	activeFRWPolls[id] = struct{}{}
	activeFRWPollsMu.Unlock()
	defer func() {
		activeFRWPollsMu.Lock()
		delete(activeFRWPolls, id)
		activeFRWPollsMu.Unlock()
	}()

	// Read task once to grab user_id + frw_task_id + kind.
	var userID int64
	var kind, taskID string
	var status int
	if err := db.DB.QueryRow(`SELECT user_id, kind, frw_task_id, status FROM ai_generations WHERE id=?`, id).Scan(&userID, &kind, &taskID, &status); err != nil {
		return
	}
	if taskID == "" || status == 2 || status == -1 || status == 4 {
		return
	}

	cfg := loadFRWConfig(userID)
	client := frw.New(cfg.APIKey, cfg.Token, cfg.UserID)
	interval := 10 * time.Second
	if kind == "video" {
		interval = 20 * time.Second
	}
	deadline := time.Now().Add(frw.PollTimeout)

	for time.Now().Before(deadline) {
		q, err := client.Query(taskID)
		if err != nil {
			// 401 or network. Persist as failure so the user gets a
			// meaningful message in the UI, then bail.
			db.DB.Exec(`UPDATE ai_generations SET status=-1, error_msg=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, err.Error(), id)
			return
		}
		_, _ = db.DB.Exec(`UPDATE ai_generations SET status=?, progress=?, result_url=?, error_msg=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			q.Status, q.Progress, q.OutData, q.Message, id)
		if q.Status == 2 || q.Status == -1 || q.Status == 4 {
			return
		}
		time.Sleep(interval)
	}
	// Exceeded our own deadline — mark timeout locally so the row doesn't hang.
	db.DB.Exec(`UPDATE ai_generations SET status=4, error_msg='本地轮询超时', updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
}

// ─── POST /frw/generations/:id/publish ───────────────────────────────────
//
// "Publish" in this context = download the FRW-hosted asset into our own
// /static dir so the user has a persistent local copy even after FRW's
// URL expires or the task gets GC'd.

func PublishFRWGeneration(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	row := fetchGenerationRow(id, userID)
	if row == nil {
		c.JSON(404, gin.H{"code": -1, "error": "任务不存在"})
		return
	}
	if s, _ := row["status"].(int); s != 2 {
		c.JSON(400, gin.H{"code": -1, "error": "任务尚未完成"})
		return
	}
	if lp, _ := row["local_path"].(string); lp != "" {
		c.JSON(200, gin.H{"code": 0, "data": gin.H{"local_path": lp, "message": "已发布"}})
		return
	}
	resultURL, _ := row["result_url"].(string)
	if resultURL == "" {
		c.JSON(400, gin.H{"code": -1, "error": "无结果 URL, 无法下载"})
		return
	}

	data, ct, err := frw.Download(resultURL)
	if err != nil {
		c.JSON(502, gin.H{"code": -1, "error": err.Error()})
		return
	}
	kind, _ := row["kind"].(string)
	ext := extFromContentType(ct)
	if ext == "" {
		if kind == "video" {
			ext = ".mp4"
		} else {
			ext = ".jpg"
		}
	}

	dir := filepath.Join("data", "frw", kind)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "创建目录失败: " + err.Error()})
		return
	}
	// Stable filename — one row, one file.
	fname := fmt.Sprintf("u%d_gen%d%s", userID, id, ext)
	full := filepath.Join(dir, fname)
	if err := os.WriteFile(full, data, 0o644); err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "写入失败: " + err.Error()})
		return
	}
	relPath := "/" + filepath.ToSlash(full) // served via a static route
	if _, err := db.DB.Exec(`UPDATE ai_generations SET local_path=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, relPath, id); err != nil {
		log.Printf("[frw] persist local_path failed: %v", err)
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"local_path": relPath, "size": len(data)}})
}

func extFromContentType(ct string) string {
	// Parse off charset/boundary if present.
	if i := strings.Index(ct, ";"); i > 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(strings.ToLower(ct))
	if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
		return exts[0]
	}
	// Common fallbacks — mime.ExtensionsByType is known to miss some.
	switch ct {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	}
	return ""
}

// ─── DELETE /frw/generations/:id ─────────────────────────────────────────

func DeleteFRWGeneration(c *gin.Context) {
	userID := c.GetInt64("user_id")
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	// Scrub local file if any.
	var lp string
	db.DB.QueryRow(`SELECT COALESCE(local_path,'') FROM ai_generations WHERE id=? AND user_id=?`, id, userID).Scan(&lp)
	if lp != "" && strings.HasPrefix(lp, "/data/frw/") {
		cleanPath := strings.TrimPrefix(lp, "/")
		absPath, err := filepath.Abs(cleanPath)
		if err == nil && strings.HasPrefix(absPath, filepath.Join("data", "frw")) {
			os.Remove(absPath)
		}
	}
	res, err := db.DB.Exec(`DELETE FROM ai_generations WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		c.JSON(500, gin.H{"code": -1, "error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(404, gin.H{"code": -1, "error": "任务不存在"})
		return
	}
	c.JSON(200, gin.H{"code": 0, "data": "已删除"})
}
