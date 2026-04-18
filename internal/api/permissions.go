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
// Each permission has a code, label, optional children, and an optional
// AdminOnly flag. Admin has ALL permissions implicitly; for members, only
// checked non-admin-only permissions apply.
//
// AdminOnly = true marks a node as administratively reserved — the
// corresponding routes use AdminRequired() middleware and the frontend
// hides the checkbox from the member permission picker. These categories
// cannot be granted to a non-admin user no matter what the request body
// claims.
//
// Currently admin-only (enforced via AdminRequired at the route layer):
//   users.*        → /v1/users/*
//   org.*          → /v1/org/*
//   globaldata.*   → /v1/admin/global-data, /v1/admin/metrics
//   reports.evaluate → POST/DELETE /v1/evaluations
//
// Everything else is grantable via RequirePermission() — members with the
// right code can hit the route, admins always can. If a new permission is
// added to the tree, the corresponding route must route through
// RequirePermission() (unless it's a new admin-only category, in which
// case AdminRequired() is fine).

type PermNode struct {
	Code      string     `json:"code"`
	Label     string     `json:"label"`
	Children  []PermNode `json:"children,omitempty"`
	AdminOnly bool       `json:"admin_only,omitempty"`
}

var PermissionTree = []PermNode{
	{Code: "dashboard", Label: "仪表盘", Children: []PermNode{
		{Code: "dashboard.view", Label: "查看仪表盘"},
		// Online users list is admin-only: it surfaces other employees'
		// real IP addresses and session counts, which members have no
		// business seeing across tenants. Flagged AdminOnly so the
		// member perm picker hides the checkbox; the route itself is
		// now AdminRequired (see main.go) for defense-in-depth.
		{Code: "dashboard.online", Label: "查看在线用户（仅管理员）", AdminOnly: true},
	}},
	{Code: "data", Label: "数据管理", Children: []PermNode{
		{Code: "data.view", Label: "查看数据面板"},
		{Code: "data.sync_gsc", Label: "同步 GSC 数据"},
		{Code: "data.sync_ga", Label: "同步 GA 数据"},
		{Code: "data.sync_yandex", Label: "同步 Yandex 数据"},
		{Code: "data.sync_bing", Label: "同步 Bing 数据"},
	}},
	{Code: "sites", Label: "站点管理", Children: []PermNode{
		{Code: "sites.view", Label: "查看站点列表"},
		{Code: "sites.create", Label: "创建站点"},
		{Code: "sites.edit", Label: "编辑站点"},
		{Code: "sites.delete", Label: "删除站点"},
		{Code: "sites.pages", Label: "管理页面（增/删/抓取源码）"},
		{Code: "sites.export", Label: "导出站点配置（bundle JSON）"},
		{Code: "sites.import", Label: "导入站点配置（bundle JSON）"},
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
		{Code: "ai_gen.delete", Label: "删除生成任务"},
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
	{Code: "push", Label: "推送管理", Children: []PermNode{
		{Code: "push.view", Label: "查看推送日志 / 计划"},
		{Code: "push.submit", Label: "手动推送 Sitemap / URL（单站点）"},
		{Code: "push.submit_all", Label: "全站批量推送 Sitemap（所有站点）"},
		{Code: "push.schedule", Label: "管理推送计划"},
	}},
	{Code: "coord", Label: "协同管理", Children: []PermNode{
		{Code: "coord.view", Label: "查看协同任务"},
		{Code: "coord.create", Label: "从审计报告创建任务"},
		{Code: "coord.edit", Label: "编辑任务（标题 / 问题描述）"},
		{Code: "coord.delete", Label: "删除任务"},
		{Code: "coord.assign", Label: "分配给开发（启动）"},
		{Code: "coord.comment", Label: "添加备注 / 提问回复"},
		{Code: "coord.ping_dev", Label: "📣 发送给开发（推送通知）"},
		{Code: "coord.attachment", Label: "上传 / 删除附件"},
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
		// Evaluate subordinates — admin-only sub, enforced via AdminRequired
		// on POST/DELETE /v1/evaluations. Tree entry is AdminOnly so the
		// member perm picker hides this single row while still showing its
		// parent (reports.*) category as normally grantable.
		{Code: "reports.evaluate", Label: "评价下级汇报", AdminOnly: true},
	}},
	{Code: "settings", Label: "系统设置", Children: []PermNode{
		{Code: "settings.view", Label: "查看系统设置"},
		{Code: "settings.edit", Label: "修改系统设置"},
		{Code: "settings.test_ai", Label: "测试 AI 接口"},
		{Code: "settings.test_tg", Label: "测试 Telegram"},
	}},
	{Code: "audit_log", Label: "审计日志", Children: []PermNode{
		{Code: "audit_log.view", Label: "查看审计日志"},
	}},
	{Code: "cache", Label: "缓存管理", Children: []PermNode{
		{Code: "cache.manage", Label: "查看 / 清理缓存"},
	}},
	// ─── Admin-only categories ─────────────────────────────────────────────
	// These four branches are reserved for admins. Enforcement is via
	// AdminRequired() middleware at the route layer; the AdminOnly flag on
	// each node tells the frontend to hide the checkbox from the "member
	// permission picker" so admins don't see them offered by mistake.
	{Code: "users", Label: "用户管理（仅管理员）", AdminOnly: true, Children: []PermNode{
		{Code: "users.view", Label: "查看用户列表", AdminOnly: true},
		{Code: "users.create", Label: "创建用户", AdminOnly: true},
		{Code: "users.edit", Label: "编辑用户", AdminOnly: true},
		{Code: "users.delete", Label: "删除用户", AdminOnly: true},
		{Code: "users.permissions", Label: "设置用户权限", AdminOnly: true},
	}},
	{Code: "org", Label: "组织结构（仅管理员）", AdminOnly: true, Children: []PermNode{
		{Code: "org.view", Label: "查看部门 / 组", AdminOnly: true},
		{Code: "org.manage", Label: "管理部门 / 组", AdminOnly: true},
	}},
	{Code: "globaldata", Label: "全局数据（仅管理员）", AdminOnly: true, Children: []PermNode{
		{Code: "globaldata.view", Label: "查看全局数据总览", AdminOnly: true},
		{Code: "globaldata.metrics", Label: "查看运营指标", AdminOnly: true},
	}},
}

// MigrateLegacyPermissions runs once at startup and rewrites every user's
// stored permission list from the old flat codes to the new granular
// codes. Called from main() after DB.Init().
//
// Two migration passes run in sequence, each gated by its own flag so
// they're independent and each idempotent:
//
//   v3 (initial granular split): flat reports.* / geo.tasks / coord.verify
//       → per-frequency reports / geo.workflow / full verify-phase toolkit.
//
//   v4 (pre-launch细化 round, 2026-04): tree grew new split codes
//       (coord.delete, coord.ping_dev, coord.attachment, ai_gen.delete,
//       push.submit_all, sites.export, sites.import, data.sync_bing).
//       Users who had the corresponding "parent" perm under the old tree
//       get the new split perms added so nothing silently breaks on upgrade.
//
// Idempotent: each pass writes a settings flag; subsequent boots skip.
// Safe to leave in place across restarts.
func MigrateLegacyPermissions() {
	migrateLegacyPermissionsV3()
	migrateLegacyPermissionsV4()
	migrateLegacyPermissionsV5()
}

// migrateLegacyPermissionsV5 strips every permission code that is now
// flagged AdminOnly from non-admin users' stored lists. The default-
// all-checked behaviour on CreateUser + self-registration handed out
// codes like dashboard.online before that code was marked AdminOnly —
// the UI has since hidden the checkbox, but the database rows still
// carry the grant and GetMyPermissions would keep returning it.
//
// This walks every user with role != 'admin' and filters out any code
// that the current tree flags as AdminOnly. Idempotent, keyed by flag
// perm_migrated_v5.
func migrateLegacyPermissionsV5() {
	var done string
	db.DB.QueryRow(`SELECT COALESCE(value,'') FROM settings WHERE key='perm_migrated_v5'`).Scan(&done)
	if done == "1" {
		return
	}

	adminOnly := adminOnlyCodes()
	if len(adminOnly) == 0 {
		db.DB.Exec(`INSERT INTO settings(key, value) VALUES('perm_migrated_v5','1')
			ON CONFLICT(key) DO UPDATE SET value='1'`)
		return
	}

	rows, err := db.DB.Query(
		`SELECT id, COALESCE(permissions,'') FROM users
		 WHERE role <> 'admin' AND permissions IS NOT NULL AND permissions <> ''`)
	if err != nil {
		log.Printf("[perm-migration-v5] list users: %v", err)
		return
	}
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
			continue
		}
		filtered := make([]string, 0, len(list))
		changed := false
		for _, code := range list {
			if _, bad := adminOnly[code]; bad {
				changed = true
				continue
			}
			filtered = append(filtered, code)
		}
		if !changed {
			continue
		}
		js, _ := json.Marshal(filtered)
		if _, err := db.DB.Exec(`UPDATE users SET permissions=$1 WHERE id=$2`, string(js), x.id); err != nil {
			log.Printf("[perm-migration-v5] user %d update: %v", x.id, err)
			continue
		}
		migrated++
	}
	db.DB.Exec(`INSERT INTO settings(key, value) VALUES('perm_migrated_v5','1')
		ON CONFLICT(key) DO UPDATE SET value='1'`)
	if migrated > 0 {
		log.Printf("[perm-migration-v5] stripped admin-only codes from %d member(s)", migrated)
	}
}

// migrateLegacyPermissionsV3 is the original flat-code → granular-code
// rewrite. See package comment for mapping rules.
func migrateLegacyPermissionsV3() {
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

// migrateLegacyPermissionsV4 grants the new split permissions to users
// who had the corresponding parent perm before the split. The intent is
// "no regression on upgrade" — anyone who could delete tasks yesterday
// via coord.edit should still be able to delete today via the new
// coord.delete, without an admin re-editing every user profile.
//
// Additions (new code → inherited from which existing parent):
//
//	coord.delete       ← coord.edit   (DELETE used to share the edit perm)
//	coord.ping_dev     ← coord.comment(comment covered all conversational ops)
//	coord.attachment   ← coord.comment(same rationale)
//	ai_gen.delete      ← ai_gen.submit(delete used to share the submit perm)
//	push.submit_all    ← push.submit  (cross-site batch used to share single-site perm)
//	sites.export       ← sites.view   (export reads the list you can already see)
//	sites.import       ← sites.create (import creates sites you could already create)
//	data.sync_bing     ← data.sync_gsc(Bing sync inherited the GSC sync perm)
//
// Runs once, keyed by `perm_migrated_v4`. No undo path — roll forward only.
func migrateLegacyPermissionsV4() {
	var done string
	db.DB.QueryRow(`SELECT COALESCE(value,'') FROM settings WHERE key='perm_migrated_v4'`).Scan(&done)
	if done == "1" {
		return
	}

	// Mapping: parent perm → list of new perms to add when the user has the parent.
	inheritance := map[string][]string{
		"coord.edit":    {"coord.delete"},
		"coord.comment": {"coord.ping_dev", "coord.attachment"},
		"ai_gen.submit": {"ai_gen.delete"},
		"push.submit":   {"push.submit_all"},
		"sites.view":    {"sites.export"},
		"sites.create":  {"sites.import"},
		"data.sync_gsc": {"data.sync_bing"},
	}

	rows, err := db.DB.Query(`SELECT id, COALESCE(permissions,'') FROM users WHERE permissions IS NOT NULL AND permissions <> ''`)
	if err != nil {
		log.Printf("[perm-migration-v4] list users: %v", err)
		return
	}
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
			continue
		}
		have := map[string]bool{}
		for _, c := range list {
			have[c] = true
		}
		changed := false
		for parent, adds := range inheritance {
			if !have[parent] {
				continue
			}
			for _, a := range adds {
				if !have[a] {
					have[a] = true
					list = append(list, a)
					changed = true
				}
			}
		}
		if !changed {
			continue
		}
		js, _ := json.Marshal(list)
		if _, err := db.DB.Exec(`UPDATE users SET permissions = $1 WHERE id = $2`, string(js), x.id); err != nil {
			log.Printf("[perm-migration-v4] user %d update: %v", x.id, err)
			continue
		}
		migrated++
	}
	db.DB.Exec(`INSERT INTO settings(key, value) VALUES('perm_migrated_v4','1')
		ON CONFLICT(key) DO UPDATE SET value='1'`)
	if migrated > 0 {
		log.Printf("[perm-migration-v4] added split perms to %d user(s)", migrated)
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

// adminOnlyCodes returns the subset of known codes flagged AdminOnly.
// A node is treated as admin-only if itself or any ancestor has the flag,
// so we don't have to set it on every child individually — though we do
// for defensive clarity in the declaration.
func adminOnlyCodes() map[string]struct{} {
	codes := map[string]struct{}{}
	var walk func(ns []PermNode, inherited bool)
	walk = func(ns []PermNode, inherited bool) {
		for _, n := range ns {
			isAdmin := inherited || n.AdminOnly
			if isAdmin {
				codes[n.Code] = struct{}{}
			}
			walk(n.Children, isAdmin)
		}
	}
	walk(PermissionTree, false)
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

// cleanPermissionList validates that every code is in the permission tree,
// filters out any admin-only codes (members can never hold them no matter
// what the request body says), and de-duplicates. Returns the cleaned
// slice or an error naming the first unknown code. Exposed here so both
// SaveUserPermissions and the initial-perms flow on CreateUser go through
// the same guardrail.
//
// Silently dropping admin-only codes (rather than 400-ing) is deliberate:
// the UI hides those checkboxes and serializes an empty/partial list, so
// a legitimate admin form should never send them. If one slips in via a
// replay or crafted POST, we just drop it and save the legitimate codes.
func cleanPermissionList(codes []string) ([]string, error) {
	known := knownPermCodes()
	adminOnly := adminOnlyCodes()
	seen := map[string]bool{}
	out := make([]string, 0, len(codes))
	for _, p := range codes {
		if _, ok := known[p]; !ok {
			return nil, fmt.Errorf("未知权限: %s", p)
		}
		if _, ok := adminOnly[p]; ok {
			// Silently skip admin-only codes — members can't hold them.
			continue
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
