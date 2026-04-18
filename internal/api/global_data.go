package api

// Admin global-data endpoint: for each user who has bound the requested
// platform, call the provider API (GSC / GA / Bing / Yandex) with that
// user's own credentials and return every property / site that account
// has access to — tagged with the owner's name. This is the cross-user
// superset of the per-user 数据管理 page.
//
// Important: cards returned are the PROVIDER's sites, not rows from our
// local `sites` table. A user might have registered 1 site with us but
// have 50 properties in their GSC account — all 50 show up here.

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"seo-system/internal/db"

	"github.com/gin-gonic/gin"
)

// GetGlobalDataOverview GET /api/v1/admin/global-data
//
// Query params:
//
//	platform=gsc|ga4|bing|yandex   (required)
//	start_date=YYYY-MM-DD           (required)
//	end_date=YYYY-MM-DD             (required)
//	user_id=N                       (optional; narrows to one owner)
//
// Response:
//
//	{ cards: [...], platform, range, summary }
//
// `cards[]` is the concatenation of every user's provider cards with
// owner_id + owner_name added. Per-user fetches run concurrently (one
// goroutine per user, each internally fans out to up to 8 parallel
// provider calls).
func GetGlobalDataOverview(c *gin.Context) {
	platform := strings.ToLower(c.DefaultQuery("platform", "gsc"))
	switch platform {
	case "gsc", "ga4", "bing", "yandex":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "platform 必须是 gsc / ga4 / bing / yandex"})
		return
	}
	sd := c.Query("start_date")
	ed := c.Query("end_date")
	if !isYMD(sd) || !isYMD(ed) {
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "start_date / end_date 必须是 YYYY-MM-DD"})
		return
	}
	filterUser := c.Query("user_id")

	// Figure out which users have the platform bound. For OAuth platforms
	// we read user_oauth_tokens; for Bing we read user_bing.
	users := listUsersWithPlatformBinding(platform, filterUser)
	if len(users) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"platform": platform,
			"range":    gin.H{"start": sd, "end": ed},
			"cards":    []gin.H{},
			"summary":  gin.H{"total_sites": 0, "total_owners": 0},
		}})
		return
	}

	// Fan out: one goroutine per user, cap total concurrency so 20 users
	// × 8 provider calls don't drown the upstream. 4 users in flight is a
	// reasonable ceiling — admin calls this rarely.
	type userResult struct {
		userID    int64
		ownerName string
		cards     []map[string]interface{}
		errMsg    string
	}
	results := make([]userResult, len(users))
	userSem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, u := range users {
		wg.Add(1)
		userSem <- struct{}{}
		go func(idx int, uid int64, uname string) {
			defer wg.Done()
			defer func() { <-userSem }()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[global-data] user %d (%s) panic: %v", uid, uname, r)
					results[idx] = userResult{userID: uid, ownerName: uname, errMsg: "panic"}
				}
			}()

			var cards []map[string]interface{}
			var err error
			switch platform {
			case "gsc":
				cards, err = gscCardsForUser(uid, sd, ed)
			case "ga4":
				cards, err = gaCardsForUser(uid, sd, ed)
			case "bing":
				cards, err = bingCardsForUser(uid, sd, ed)
			case "yandex":
				cards, err = yandexCardsForUser(uid, sd, ed)
			}
			r := userResult{userID: uid, ownerName: uname, cards: cards}
			if err != nil {
				r.errMsg = err.Error()
				log.Printf("[global-data] user %d (%s) %s failed: %v", uid, uname, platform, err)
			}
			results[idx] = r
		}(i, u.id, u.name)
	}
	wg.Wait()

	// Flatten into final payload. Each card gets owner info so the
	// frontend can render an owner chip on the card.
	out := make([]gin.H, 0, 64)
	var totalClicks, totalImpr, totalSessions, totalUsers float64
	ownerSet := map[int64]bool{}
	for _, r := range results {
		if len(r.cards) == 0 {
			continue
		}
		ownerSet[r.userID] = true
		for _, card := range r.cards {
			card["owner_id"] = r.userID
			card["owner_name"] = r.ownerName
			// Gather totals for the summary strip.
			if v, ok := card["clicks"].(float64); ok {
				totalClicks += v
			}
			if v, ok := card["impressions"].(float64); ok {
				totalImpr += v
			}
			if v, ok := card["sessions"].(float64); ok {
				totalSessions += v
			}
			if v, ok := card["total_users"].(float64); ok {
				totalUsers += v
			}
			out = append(out, gin.H(card))
		}
	}

	// Per-user error summary so the admin can tell "nobody has synced" vs
	// "one user's token expired".
	userErrors := []gin.H{}
	for _, r := range results {
		if r.errMsg != "" {
			userErrors = append(userErrors, gin.H{
				"user_id":    r.userID,
				"owner_name": r.ownerName,
				"error":      r.errMsg,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"platform":    platform,
		"range":       gin.H{"start": sd, "end": ed},
		"cards":       out,
		"user_errors": userErrors,
		"summary": gin.H{
			"total_sites":    len(out),
			"total_owners":   len(ownerSet),
			"total_clicks":   totalClicks,
			"total_impr":     totalImpr,
			"total_sessions": totalSessions,
			"total_users":    totalUsers,
		},
	}})
}

// listUsersWithPlatformBinding returns {id, name} for every user whose
// credential row for the platform exists. Optional filterUserID narrows
// to a single user (admin UI dropdown). Order: name ASC for stable
// rendering.
func listUsersWithPlatformBinding(platform, filterUserID string) []struct {
	id   int64
	name string
} {
	var q string
	switch platform {
	case "gsc":
		q = `SELECT DISTINCT u.id, u.name
		     FROM user_oauth_tokens t
		     JOIN users u ON u.id = t.user_id
		     WHERE t.service = 'gsc' AND t.access_token != ''`
	case "ga4":
		q = `SELECT DISTINCT u.id, u.name
		     FROM user_oauth_tokens t
		     JOIN users u ON u.id = t.user_id
		     WHERE t.service = 'ga' AND t.access_token != ''`
	case "yandex":
		q = `SELECT DISTINCT u.id, u.name
		     FROM user_oauth_tokens t
		     JOIN users u ON u.id = t.user_id
		     WHERE t.service = 'yandex' AND t.access_token != ''`
	case "bing":
		q = `SELECT u.id, u.name
		     FROM user_bing b
		     JOIN users u ON u.id = b.user_id
		     WHERE b.api_key != ''`
	default:
		return nil
	}
	args := []interface{}{}
	if filterUserID != "" {
		q += fmt.Sprintf(" AND u.id = $%d", len(args)+1)
		args = append(args, filterUserID)
	}
	q += " ORDER BY u.name ASC"

	rows, err := db.DB.Query(q, args...)
	if err != nil {
		log.Printf("[global-data] list bound users (%s): %v", platform, err)
		return nil
	}
	defer rows.Close()

	out := []struct {
		id   int64
		name string
	}{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		out = append(out, struct {
			id   int64
			name string
		}{id, name})
	}
	return out
}

// isYMD validates YYYY-MM-DD strictly — no timezones, no times.
func isYMD(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, r := range s {
		if i == 4 || i == 7 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
