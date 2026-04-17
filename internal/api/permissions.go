package api

import (
	"encoding/json"
	"log"
	"net/http"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// ─── Permission tree (Magento-style ACL) ─────────────────────────────────────
// Each permission has a code, label, and optional children. Admin has ALL
// permissions implicitly. For member users, only checked permissions apply.

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
		{Code: "sites.pages", Label: "管理页面"},
		{Code: "sites.audit", Label: "触发审计"},
		{Code: "sites.audit_view", Label: "查看审计报告"},
		{Code: "sites.audit_delete", Label: "删除审计记录"},
		{Code: "sites.audit_download", Label: "下载 Word 报告"},
	}},
	{Code: "geo", Label: "GEO 管理", Children: []PermNode{
		{Code: "geo.view", Label: "查看 GEO 模块"},
		{Code: "geo.keywords", Label: "管理关键词库"},
		{Code: "geo.prompts", Label: "管理 Prompt 模板"},
		{Code: "geo.knowledge", Label: "管理知识库"},
		{Code: "geo.tasks", Label: "管理生成任务"},
		{Code: "geo.tasks_run", Label: "执行生成（调用 AI）"},
		{Code: "geo.articles", Label: "查看文章"},
		{Code: "geo.articles_review", Label: "审核文章"},
		{Code: "geo.articles_publish", Label: "发布文章"},
		{Code: "geo.articles_delete", Label: "删除文章"},
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
		{Code: "coord.comment", Label: "添加备注"},
		{Code: "coord.submit", Label: "标记修改完毕（开发）"},
		{Code: "coord.verify", Label: "验收审计完成（关闭）"},
	}},
	{Code: "reports", Label: "每日汇报", Children: []PermNode{
		{Code: "reports.view", Label: "查看汇报列表"},
		{Code: "reports.write", Label: "写汇报"},
		{Code: "reports.polish", Label: "AI 润色"},
		{Code: "reports.export", Label: "导出汇报"},
	}},
	{Code: "users", Label: "用户管理", Children: []PermNode{
		{Code: "users.view", Label: "查看用户列表"},
		{Code: "users.create", Label: "创建用户"},
		{Code: "users.edit", Label: "编辑用户"},
		{Code: "users.delete", Label: "删除用户"},
		{Code: "users.permissions", Label: "设置用户权限"},
	}},
	{Code: "settings", Label: "系统设置", Children: []PermNode{
		{Code: "settings.view", Label: "查看系统设置"},
		{Code: "settings.edit", Label: "修改系统设置"},
		{Code: "settings.test_ai", Label: "测试 AI 接口"},
		{Code: "settings.test_tg", Label: "测试 Telegram"},
	}},
}

// GetPermissionTree returns the full tree for the frontend to render checkboxes.
func GetPermissionTree(c *gin.Context) {
	c.JSON(200, gin.H{"code": 0, "data": PermissionTree})
}

// GetUserPermissions returns the saved permissions for a specific user.
func GetUserPermissions(c *gin.Context) {
	userID := c.Param("id")
	var perms string
	db.DB.QueryRow("SELECT COALESCE(permissions,'') FROM users WHERE id=?", userID).Scan(&perms)
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
	known := knownPermCodes()
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(req.Permissions))
	for _, p := range req.Permissions {
		if _, ok := known[p]; !ok {
			c.JSON(400, gin.H{"code": -1, "error": "未知权限: " + p})
			return
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		cleaned = append(cleaned, p)
	}
	js, _ := json.Marshal(cleaned)
	if _, err := db.DB.Exec("UPDATE users SET permissions=? WHERE id=?", string(js), userID); err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "保存失败"})
		return
	}

	WriteAuditLog(c, "save_permissions", "user", userID, string(js))

	c.JSON(200, gin.H{"code": 0, "data": "权限已保存"})
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
	var permsJSON string
	db.DB.QueryRow("SELECT COALESCE(permissions,'') FROM users WHERE id=?", userID).Scan(&permsJSON)
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
	db.DB.QueryRow("SELECT COALESCE(permissions,'') FROM users WHERE id=?", userID).Scan(&permsJSON)
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
