package api

import (
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// registerCoordRoutes handles coordination task management.
func registerCoordRoutes(v1 *gin.RouterGroup) {
	coord := v1.Group("/coord")
	{
		coord.GET("/stats", RequirePermission("coord.view"), CoordStats)
		coord.GET("/scopes", RequirePermission("coord.view"), ListCoordScopes)
		coord.GET("/tasks", RequirePermission("coord.view"), ListCoordTasks)
		coord.POST("/tasks", RequirePermission("coord.create"), CreateCoordTask)
		coord.GET("/tasks/:id", RequirePermission("coord.view"), GetCoordTask)
		coord.PUT("/tasks/:id", RequirePermission("coord.edit"), UpdateCoordTask)
		coord.DELETE("/tasks/:id", RequirePermission("coord.delete"), DeleteCoordTask)
		coord.POST("/tasks/:id/assign", RequirePermission("coord.assign"), AssignCoordTask)

		// Review workflow
		coord.POST("/tasks/:id/review", RequirePermission("coord.review"), SubmitCoordReviewerDecision)
		coord.POST("/tasks/:id/submit", RequirePermission("coord.submit"), SubmitCoordTask)
		coord.POST("/tasks/:id/verify", RequirePermission("coord.verify"), VerifyCoordTask)
		coord.POST("/tasks/:id/comments", RequirePermission("coord.comment"), AddCoordComment)

		// Q&A flow
		coord.POST("/tasks/:id/ask", RequirePermission("coord.comment"), AskCoordTask)
		coord.POST("/tasks/:id/answer", RequirePermission("coord.comment"), AnswerCoordTask)
		coord.POST("/tasks/:id/ping-dev", RequirePermission("coord.ping_dev"), PingDevCoordTask)

		// Attachments
		coord.POST("/tasks/:id/attachments", RequirePermission("coord.attachment"), UploadCoordAttachment)
		coord.GET("/attachments/:id/file", RequirePermission("coord.view"), ServeCoordAttachment)
		coord.DELETE("/attachments/:id", RequirePermission("coord.attachment"), DeleteCoordAttachment)

		// Pre-audit (test pages + AI verdict)
		coord.GET("/tasks/:id/test-pages", RequirePermission("coord.view"), ListCoordTestPages)
		coord.POST("/tasks/:id/test-pages", RequirePermission("coord.test_pages"), CreateCoordTestPage)
		coord.DELETE("/tasks/:id/test-pages/:page_id", RequirePermission("coord.test_pages"), DeleteCoordTestPage)
		coord.POST("/tasks/:id/test-pages/:page_id/fetch", RequirePermission("coord.test_pages"), FetchCoordTestPage)
		coord.GET("/tasks/:id/test-pages/:page_id/source", RequirePermission("coord.view"), GetCoordTestPageSource)
		coord.POST("/tasks/:id/pre-audit", RequirePermission("coord.pre_audit"), middleware.UserRateLimit(3), PreAuditCoordTask)
		coord.POST("/tasks/:id/reject", RequirePermission("coord.reject"), RejectCoordTask)
	}
}
