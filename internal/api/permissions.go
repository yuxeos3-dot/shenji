package api

import (
	"encoding/json"
	"net/http"
	"strings"

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
		json.Unmarshal([]byte(perms), &list)
	}
	if list == nil {
		list = []string{}
	}
	c.JSON(200, gin.H{"code": 0, "data": list})
}

// SaveUserPermissions saves the checked permissions for a user.
func SaveUserPermissions(c *gin.Context) {
	userID := c.Param("id")
	var req struct {
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": -1, "error": "参数错误"})
		return
	}
	js, _ := json.Marshal(req.Permissions)
	if _, err := db.DB.Exec("UPDATE users SET permissions=? WHERE id=?", string(js), userID); err != nil {
		c.JSON(500, gin.H{"code": -1, "error": "保存失败"})
		return
	}

	WriteAuditLog(c, "save_permissions", "user", userID, string(js))

	c.JSON(200, gin.H{"code": 0, "data": "权限已保存"})
}

// HasPermission checks if the current user has a specific permission.
// Admin users always have all permissions.
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
	json.Unmarshal([]byte(permsJSON), &perms)
	// Check exact match or parent wildcard
	for _, p := range perms {
		if p == perm || strings.HasPrefix(perm, p+".") {
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
		json.Unmarshal([]byte(permsJSON), &perms)
	}
	if perms == nil {
		perms = []string{}
	}
	c.JSON(200, gin.H{"code": 0, "data": perms, "role": role})
}
