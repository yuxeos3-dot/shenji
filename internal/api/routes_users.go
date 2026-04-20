package api

import (
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// registerUserRoutes handles user management and organization CRUD.
func registerUserRoutes(v1 *gin.RouterGroup) {
	users := v1.Group("/users", middleware.AdminRequired())
	{
		users.GET("", ListUsers)
		users.POST("", CreateUser)
		users.PUT("/:id", UpdateUser)
		users.DELETE("/:id", DeleteUser)
		users.GET("/:id/permissions", GetUserPermissions)
		users.PUT("/:id/permissions", SaveUserPermissions)
		users.GET("/permission-tree", GetPermissionTree)

		// Pending registrations
		users.GET("/pending", ListPendingUsers)
		users.POST("/:id/approve", ApprovePendingUser)
		users.POST("/:id/reject", RejectPendingUser)

		// User monitoring
		users.GET("/:id/platform-status", GetUserPlatformStatus)
		users.GET("/:id/overview", GetUserOverview)
	}

	// Organization (departments + groups)
	orgs := v1.Group("/org", middleware.AdminRequired())
	{
		orgs.GET("/departments", ListDepartments)
		orgs.POST("/departments", CreateDepartment)
		orgs.PUT("/departments/:id", UpdateDepartment)
		orgs.DELETE("/departments/:id", DeleteDepartment)
		orgs.GET("/groups", ListGroups)
		orgs.POST("/groups", CreateGroup)
		orgs.PUT("/groups/:id", UpdateGroup)
		orgs.DELETE("/groups/:id", DeleteGroup)
	}
}
