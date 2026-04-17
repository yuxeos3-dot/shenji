package api

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// parsePageQuery extracts `page` and `per_page` query parameters with safe
// defaults and hard caps. Any malformed value falls back to the default.
// Callers should compute `offset := (page-1) * perPage`.
func parsePageQuery(c *gin.Context, defaultPerPage, maxPerPage int) (page, perPage int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ = strconv.Atoi(c.DefaultQuery("per_page", strconv.Itoa(defaultPerPage)))
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return
}

// parseISODate validates a YYYY-MM-DD string. Returns the original string on
// success so callers can pass it straight to the DB, and an error otherwise.
func parseISODate(raw string) (string, error) {
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return "", err
	}
	return t.Format("2006-01-02"), nil
}
