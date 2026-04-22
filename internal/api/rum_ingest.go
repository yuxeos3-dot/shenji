package api

import (
	"net/http"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// RUMBeaconIngest receives Web Vitals data from frontend JS.
// Public endpoint (no auth) with rate limiting and site_id validation.
// POST /api/rum/beacon
func RUMBeaconIngest(c *gin.Context) {
	var req struct {
		SiteID          int     `json:"siteId" binding:"required"`
		URL             string  `json:"url" binding:"required"`
		MetricName      string  `json:"metric" binding:"required"`
		MetricValue     float64 `json:"value"`
		MetricRating    string  `json:"rating"`
		DeviceType      string  `json:"deviceType"`
		ConnectionType  string  `json:"connectionType"`
		Country         string  `json:"country"`
		SessionID       string  `json:"sessionId"`
		ElementSelector string  `json:"element"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1})
		return
	}

	// Validate site exists
	var exists int
	db.DB.QueryRow("SELECT 1 FROM sites WHERE id=$1", req.SiteID).Scan(&exists)
	if exists != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1})
		return
	}

	db.DB.Exec(`INSERT INTO rum_beacons (site_id, url, metric_name, metric_value, metric_rating,
		device_type, connection_type, country, session_id, element_selector)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		req.SiteID, req.URL, req.MetricName, req.MetricValue, req.MetricRating,
		req.DeviceType, req.ConnectionType, req.Country, req.SessionID, req.ElementSelector)

	c.JSON(http.StatusOK, gin.H{"code": 0})
}

// RUMEventIngest receives UX friction events (rage click, dead click, etc)
// POST /api/rum/event
func RUMEventIngest(c *gin.Context) {
	var req struct {
		SiteID    int     `json:"siteId" binding:"required"`
		URL       string  `json:"url" binding:"required"`
		EventType string  `json:"event" binding:"required"`
		Value     float64 `json:"value"`
		Element   string  `json:"element"`
		Device    string  `json:"device"`
		SessionID string  `json:"sessionId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1})
		return
	}

	db.DB.Exec(`INSERT INTO rum_events (site_id, url, event_type, event_value, element_selector, device_type, session_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		req.SiteID, req.URL, req.EventType, req.Value, req.Element, req.Device, req.SessionID)

	c.JSON(http.StatusOK, gin.H{"code": 0})
}
