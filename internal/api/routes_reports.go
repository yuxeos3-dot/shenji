package api

import (
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// registerReportRoutes handles daily, weekly, monthly reports and evaluations.
func registerReportRoutes(v1 *gin.RouterGroup) {
	// Daily reports
	v1.GET("/reports", RequirePermission("reports.daily.view"), ListReports)
	v1.POST("/reports", RequirePermission("reports.daily.write"), SubmitReport)
	v1.POST("/reports/polish", RequirePermission("reports.daily.polish"), PolishReport)
	v1.GET("/reports/export", RequirePermission("reports.daily.export"), ExportReports)

	// Weekly reports
	weekly := v1.Group("/reports/weekly")
	{
		weekly.GET("", RequirePermission("reports.weekly.view"), ListWeeklyReports)
		weekly.GET("/all", RequirePermission("reports.weekly.view"), AdminListAllWeeklyReports)
		weekly.POST("/generate", RequirePermission("reports.weekly.write"), GenerateWeeklyReport)
		weekly.GET("/:id", RequirePermission("reports.weekly.view"), GetWeeklyReport)
		weekly.PUT("/:id", RequirePermission("reports.weekly.write"), UpdateWeeklyReport)
		weekly.POST("/:id/submit", RequirePermission("reports.weekly.write"), SubmitWeeklyReport)
		weekly.POST("/:id/polish", RequirePermission("reports.weekly.polish"), PolishWeeklyReport)
		weekly.DELETE("/:id", RequirePermission("reports.weekly.write"), DeleteWeeklyReport)
	}

	// Monthly reports
	monthly := v1.Group("/reports/monthly")
	{
		monthly.GET("", RequirePermission("reports.monthly.view"), ListMonthlyReports)
		monthly.GET("/all", RequirePermission("reports.monthly.view"), AdminListAllMonthlyReports)
		monthly.POST("/generate", RequirePermission("reports.monthly.write"), GenerateMonthlyReport)
		monthly.GET("/:id", RequirePermission("reports.monthly.view"), GetMonthlyReport)
		monthly.PUT("/:id", RequirePermission("reports.monthly.write"), UpdateMonthlyReport)
		monthly.POST("/:id/submit", RequirePermission("reports.monthly.write"), SubmitMonthlyReport)
		monthly.POST("/:id/polish", RequirePermission("reports.monthly.polish"), PolishMonthlyReport)
		monthly.DELETE("/:id", RequirePermission("reports.monthly.write"), DeleteMonthlyReport)
	}

	// Daily report by date (must be after /export to avoid route conflict)
	v1.GET("/reports/:date", RequirePermission("reports.daily.view"), GetReportByDate)

	// Evaluations
	v1.GET("/evaluations", RequirePermission("reports.daily.view"), ListEvaluations)
	v1.POST("/evaluations", middleware.AdminRequired(), CreateEvaluation)
	v1.POST("/evaluations/polish", middleware.AdminRequired(), PolishEvaluation)
	v1.DELETE("/evaluations/:id", middleware.AdminRequired(), DeleteEvaluation)
}
