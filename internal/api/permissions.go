package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Permission tree (Magento-style ACL) ─────────────────────────────────────
// Each permission has a code, label, and optional children. Admin has ALL
// permissions implicitly. For member users, only checked permissions apply.
//
// #40 Enforcement note — permission tree vs route-level guards:
//
// The following tree sections are enforced by AdminRequired() middleware at
// the route group level in main.go, NOT by RequirePermission(). Their tree
// entries exist so the frontend can render them in the admin permission UI,
// but the actual gate is the admin-only middleware:
//
//   users.*       → routes under /v1/users use AdminRequired()
//   org.*         → routes under /v1/org   use AdminRequired()
//   settings.*    → routes under /v1/settings use AdminRequired()
//
// Additionally these specific routes use AdminRequired() directly:
//   reports.evaluate → POST/DELETE /v1/evaluations use AdminRequired()
//   (audit-logs)     → /v1/audit-logs uses AdminRequired() (no tree entry)
//   (cache)          → /v1/cache uses AdminRequired() (no tree entry)
//
// All other tree entries are checked via RequirePermission() on their
// respective routes. If a new permission is added to the tree, a matching
// RequirePermission() call must be added to the route definition.

type PermNode struct {
	Code     string     `json:"code"`
	Label    string     `json:"label"`
	Children []PermNode `json:"children,omitempty"`
}

var PermissionTree = []PermNode{
	{Code: "dashboard", Label: "仪表盘", Children: []PermNode{
		{Code: "dashboard.view", Label: "查看仪表盘"},
		{Code: "dashboard.online", Label: "查看在线用户"},
	}},
	{Code: "data", Label: "数据管理", Children: []PermNode{
		{Code: "data.view", Label: "查看数据面板"},
		{Code: "data.sync_gsc", Label: "同步 GSC 数据"},
		{Code: "data.sync_ga", Label: "同步 GA 数据"},
		{Code: "data.sync_yandex", Label: "同步 Yandex 数据"},
	}},
	{Code: "sites", Label: "站点管理", Children: []PermNode{
		{Code: "sites.view", Label: "查看站点列表"},
		{Code: "sites.create", Label: "创建站点"},
		{Code: "sites.edit", Label: "编辑站点"},
		{Code: "sites.delete", Label: "删除站点"},
		{Code: "sites.pages", Label: "管理页面（增/删/抓取源码）"},
		{Code: "sites.audit", Label: "触发审计"},
		{Code: "sites.audit_view", Label: "查看审计报告"},
		{Code: "sites.audit_delete", Label: "删除审计记录"},
		{Code: "sites.audit_download", Label: "下载 Word 报告"},
	}},
	{Code: "geo", Label: "GEO 管理", Children: []PermNode{
		{Code: "geo.view", Label: "查看 GEO 模块"},
		{Code: "geo.keywords", Label: "管理关键词 / 话题库"},
		{Code: "geo.workflow", Label: "关键词 AI 写作（4 步向导）"},
		{Code: "geo.articles", Label: "查看文章"},
		{Code: "geo.articles_review", Label: "审核文章"},
		{Code: "geo.articles_publish", Label: "发布文章"},
		{Code: "geo.articles_delete", Label: "删除文章"},
	}},
	{Code: "ai_gen", Label: "AI 图片 / 视频生成", Children: []PermNode{
		{Code: "ai_gen.view", Label: "查看生成任务列表"},
		{Code: "ai_gen.submit", Label: "提交新生成任务（消耗 FRW 额度）"},
		{Code: "ai_gen.publish", Label: "发布 / 下载结果到本地"},
	}},
	{Code: "deadlinks", Label: "死链检查", Children: []PermNode{
		{Code: "deadlinks.view", Label: "查看死链结果"},
		{Code: "deadlinks.check", Label: "触发死链检测"},
		{Code: "deadlinks.retry", Label: "失败重查"},
	}},
	{Code: "rewrite", Label: "内容二创", Children: []PermNode{
		{Code: "rewrite.use", Label: "使用内容二创"},
	}},
	{Code: "extlinks", Label: "外链管理", Children: []PermNode{
		{Code: "extlinks.view", Label: "查看外链"},
		{Code: "extlinks.create", Label: "添加外链"},
		{Code: "extlinks.edit", Label: "编辑外链"},
		{Code: "extlinks.delete", Label: "删除外链"},
		{Code: "extlinks.check", Label: "检测外链可用性"},
	}},
	{Code: "spider", Label: "蜘蛛管理", Children: []PermNode{
		{Code: "spider.view", Label: "查看蜘蛛统计 / 日志"},
	}},
	{Code: "coord", Label: "协同管理", Children: []PermNode{
		{Code: "coord.view", Label: "查看协同任务"},
		{Code: "coord.create", Label: "从审计报告创建任务"},
		{Code: "coord.edit", Label: "编辑 / 删除任务"},
		{Code: "coord.assign", Label: "分配给开发（启动）"},
		{Code: "coord.comment", Label: "添加备注 / 附件 / 提问回复"},
		{Code: "coord.submit", Label: "标记修改完毕（开发）"},
		{Code: "coord.test_pages", Label: "管理测试服页面（添加 / 抓取源码）"},
		{Code: "coord.pre_audit", Label: "运行 AI 预审核"},
		{Code: "coord.verify", Label: "验收通过（关闭任务）"},
		{Code: "coord.reject", Label: "打回修改"},
	}},
	{Code: "reports", Label: "汇报管理", Children: []PermNode{
		// Daily
		{Code: "reports.daily.view", Label: "查看日报"},
		{Code: "reports.daily.write", Label: "写日报"},
		{Code: "reports.daily.polish", Label: "日报 AI 润色"},
		{Code: "reports.daily.export", Label: "导出日报"},
		// Weekly
		{Code: "reports.weekly.view", Label: "查看周报"},
		{Code: "reports.weekly.write", Label: "写 / 生成 / 提交周报"},
		{Code: "reports.weekly.polish", Label: "周报 AI 润色"},
		// Monthly
		{Code: "reports.monthly.view", Label: "查看月报"},
		{Code: "reports.monthly.write", Label: "写 / 生成 / 提交月报"},
		{Code: "reports.monthly.polish", Label: "月报 AI 润色"},
		// Evaluation (admin-only, kept in tree for visibility).
		// NOTE: enforced by AdminRequired() on evaluation routes, not RequirePermission().
		{Code: "reports.evaluate", Label: "评价下级汇报（仅管理员）"},
	}},
	// NOTE: users.* enforced by AdminRequired() in main.go, not RequirePermission().
	// Kept in tree for frontend visibility / admin-panel rendering.
	{Code: "users", Label: "用户管理（仅管理员）", Children: []PermNode{
		{Code: "users.view", Label: "查看用户列表"},
		{Code: "users.create", Label: "创建用户"},
		{Code: "users.edit", Label: "编辑用户"},
		{Code: "users.delete", Label: "删除用户"},
		{Code: "users.permissions", Label: "设置用户权限"},
	}},
	// NOTE: org.* enforced by AdminRequired() in main.go, not RequirePermission().
	{Code: "org", Label: "组织结构（仅管理员）", Children: []PermNode{
		{Code: "org.view", Label: "查看部门 / 组"},
		{Code: "org.manage", Label: "管理部门 / 组"},
	}},
	// NOTE: settings.* enforced by AdminRequired() in main.go, not RequirePermission().
	{Code: "settings", Label: "系统设置（仅管理员）", Children: []PermNode{
		{Code: "settings.view", Label: "查看系统设置"},
		{Code: "settings.edit", Label: "修改系统设置"},
		{Code: "settings.test_ai", Label: "测试 AI 接口"},
		{Code: "settings.test_tg", Label: "测试 Telegram"},
	}},
}

// MigrateLegacyPermissions runs once at startup and rewrites every user's
// stored permission list from the old flat codes to the new granular
// codes. Called from main() after DB.Init().
//
// Mapping:
//
//	reports.view    → reports.daily.view + reports.weekly.view + reports.monthly.view
//	reports.write   → reports.daily.write + reports.weekly.write + reports.monthly.write
//	reports.polish  → reports.daily.polish + reports.weekly.polish + reports.monthly.polish
//	reports.export  → reports.daily.export
//	geo.tasks_run   → geo.workflow (batch generation removed; migrate old
//	                  task-runner perm to the new per-keyword wizard perm)
//	geo.prompts / geo.knowledge / geo.tasks → dropped (stripped from lists)
//	coord.verify    → coord.verify + coord.pre_audit + coord.reject + coord.test_pages
//	                  (if a user could verify before, they need the whole
//	                  verify-phase toolbox now that it's split)
//
// Idempotent: already-migrated users (those whose list contains any of the
// new codes) are not re-processed; also dedup on write. Safe to leave in
// place across restarts.
//
// We set a settings flag `perm_migrated_v3` on success so subsequent boots
// skip the scan entirely. The flag lookup is one tiny SQL call; if it's
// set we return immediately.
func MigrateLegacyPermissions() {
	// Skip if already done.
	var done string
	db.DB.QueryRow(`SELECT COALESCE(value,'') FROM settings WHERE key='perm_migrated_v3'`).Scan(&done)
	if done == "1" {
		return
	}

	// Map old code → list of new codes to add alongside.
	expansions := map[string][]string{
		"reports.view":   {"reports.daily.view", "reports.weekly.view", "reports.monthly.view"},
		"reports.write":  {"reports.daily.write", "reports.weekly.write", "reports.monthly.write"},
		"reports.polish": {"reports.daily.polish", "reports.weekly.polish", "reports.monthly.polish"},
		"reports.export": {"reports.daily.export"},
		"geo.tasks_run":  {"geo.workflow"},
		"coord.verify":   {"coord.verify", "coord.pre_audit", "coord.reject", "coord.test_pages"},
	}
	// Old codes that get fully replaced (removed after expansion).
	obsolete := map[string]bool{
		"reports.view":   true,
		"reports.write":  true,
		"reports.polish": true,
		"reports.export": true,
		// 4-step wizard refactor dropped batch-task / per-template flows.
		"geo.tasks_run": true,
		"geo.tasks":     true,
		"geo.prompts":   true,
		"geo.knowledge": true,
	}

	rows, err := db.DB.Query(`SELECT id, COALESCE(permissions,'') FROM users WHERE permissions IS NOT NULL AND permissions <> ''`)
	if err != nil {
		log.Printf("[perm-migration] list users: %v", err)
		return
	}
	defer rows.Close()

	type u struct {
		id    int64
		perms string
	}
	var toUpdate []u
	for rows.Next() {
		var x u
		if err := rows.Scan(&x.id, &x.perms); err != nil {
			continue
		}
		toUpdate = append(toUpdate, x)
	}
	rows.Close()

	migrated := 0
	for _, x := range toUpdate {
		var list []string
		if err := json.Unmarshal([]byte(x.perms), &list); err != nil {
			log.Printf("[perm-migration] user %d: unmarshal failed: %v", x.id, err)
			continue
		}
		changed := false
		seen := map[string]bool{}
		next := make([]string, 0, len(list)+8)
		for _, code := range list {
			// Expand legacy codes into their replacements.
			if adds, ok := expansions[code]; ok {
				changed = true
				for _, a := range adds {
					if !seen[a] {
						seen[a] = true
						next = append(next, a)
					}
				}
			}
			// Obsolete codes (including those that had no expansion) get
			// dropped so the user's list no longer references dead perms.
			if obsolete[code] {
				changed = true
				continue
			}
			if !seen[code] {
				seen[code] = true
				next = append(next, code)
			}
		}
		if !changed {
			continue
		}
		js, _ := json.Marshal(next)
		if _, err := db.DB.Exec(`UPDATE users SET permissions = $1 WHERE id = $2`, string(js), x.id); err != nil {
			log.Printf("[perm-migration] user %d: update failed: %v", x.id, err)
			continue
		}
		migrated++
	}
	// Set completion flag whether or not any user needed migrating.
	db.DB.Exec(`INSERT INTO settings(key, value) VALUES('perm_migrated_v3','1')
		ON CONFLICT(key) DO UPDATE SET value='1'`)
	if migrated > 0 {
		log.Printf("[perm-migration] migrated %d user(s) to v2 permission codes", migrated)
	}
}

// ─── Permission loading / saving ────────────────────────────────────────────

func GetPermissionTree(c *gin.Context) {
	c.JSON(200, gin.H{"code": 0, "data": PermissionTree})
}

// GetUserPermissions returns the saved permissions for a specific user.
func GetUserPermissions(c *gin.Context) {
	userID := c.Param("id")
	var perms string
	db.DB.QueryRow("SELECT COALESCE(permissions,'') FROM users WHERE id=$1", userID).Scan(&perms)
	var list []string
	if perms != "" {
		if err := json.Unmarshal([]byte(perms), &list); err != nil {
			log.Printf("[permissions] unmarshal user %s permissions failed: %v", userID, err)
		}
	}
	if list == nil {
		list = []string{}
	}
	c.JSON(200, gin.H{"code": 0, "data": list})
}

// knownPermCodes returns a flat set of every permission code in PermissionTree.
func knownPermCodes() map[string]struct{} {
	codes := map[string]struct{}{}
	var walk func([]PermNode)
	walk = func(ns []PermNode) {
		for _, n := range ns {
			codes[n.Code] = struct{}{}
			walk(n.Children)
		}
	}
	walk(PermissionTree)
	return codes
}

// SaveUserPermissions saves the checked permissions for a user. Every submitted
// permission must appear in PermissionTree, otherwise we reject the request to
// prevent privilege escalation via arbitrary codes.
func SaveUserPermissions(c *gin.Context) {
	userID := c.Param("id")
	var req struct {
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	cleaned, err := cleanPermissionList(req.Permissions)
	if err != nil {
		c.JSON(400, gin.H{"code": -1, "error": err.Error()})
		return
	}
	js, _ := json.Marshal(cleaned)
	if _, err := db.DB.Exec("UPDATE users SET permissions=$1 WHERE id=$2", string(js), userID); err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "保存失败"})
		return
	}

	WriteAuditLog(c, "save_permissions", "user", userID, string(js))

	c.JSON(200, gin.H{"code": 0, "data": "权限已保存"})
}

// cleanPermissionList validates that every code is in the permission tree
// and de-duplicates. Returns the cleaned slice or an error naming the
// first unknown code. Exposed here so both SaveUserPermissions and the
// initial-perms flow on CreateUser go through the same guardrail.
func cleanPermissionList(codes []string) ([]string, error) {
	known := knownPermCodes()
	seen := map[string]bool{}
	out := make([]string, 0, len(codes))
	for _, p := range codes {
		if _, ok := known[p]; !ok {
			return nil, fmt.Errorf("未知权限: %s", p)
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out, nil
}

// HasPermission checks if the current user has a specific permission.
// Admin users always have all permissions. Non-admins need an EXACT match —
// granting a parent node no longer auto-grants children, since the UI renders
// each child as its own checkbox and implicit wildcards led to surprise
// privilege escalation.
func HasPermission(c *gin.Context, perm string) bool {
	role, _ := c.Get("role")
	if role == "admin" {
		return true
	}
	userID := c.GetInt64("user_id")
	if userID <= 0 {
		// Missing or malformed user_id — never grant permission.
		return false
	}
	var permsJSON string
	if err := db.DB.QueryRow("SELECT COALESCE(permissions,'') FROM users WHERE id=$1", userID).Scan(&permsJSON); err != nil && err != sql.ErrNoRows {
		log.Printf("[permissions] load permissions failed for user %d: %v", userID, err)
		return false
	}
	if permsJSON == "" {
		return false
	}
	var perms []string
	if err := json.Unmarshal([]byte(permsJSON), &perms); err != nil {
		log.Printf("[permissions] unmarshal permissions failed for user %d: %v", userID, err)
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// RequirePermission returns a middleware that checks a specific permission.
func RequirePermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !HasPermission(c, perm) {
			c.JSON(http.StatusForbidden, gin.H{"code": -1, "error": "权限不足：需要 " + perm})
			c.Abort()
			return
		}
		c.Next()
	}
}

// GetMyPermissions returns the current user's permissions (for frontend menu control).
func GetMyPermissions(c *gin.Context) {
	role, _ := c.Get("role")
	if role == "admin" {
		// Admin gets all permission codes
		var all []string
		var collect func(nodes []PermNode)
		collect = func(nodes []PermNode) {
			for _, n := range nodes {
				all = append(all, n.Code)
				collect(n.Children)
			}
		}
		collect(PermissionTree)
		c.JSON(200, gin.H{"code": 0, "data": all, "role": "admin"})
		return
	}
	userID := c.GetInt64("user_id")
	var permsJSON string
	db.DB.QueryRow("SELECT COALESCE(permissions,'') FROM users WHERE id=$1", userID).Scan(&permsJSON)
	var perms []string
	if permsJSON != "" {
		if err := json.Unmarshal([]byte(permsJSON), &perms); err != nil {
			log.Printf("[permissions] unmarshal my-permissions failed: %v", err)
		}
	}
	if perms == nil {
		perms = []string{}
	}
	c.JSON(200, gin.H{"code": 0, "data": perms, "role": role})
}
