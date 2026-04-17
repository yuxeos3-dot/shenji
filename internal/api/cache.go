package api

import (
	"net/http"

	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
)

// CacheEntry describes one in-memory cache surface so the admin UI can render it.
type CacheEntry struct {
	Key         string `json:"key"`          // stable identifier for clear-target
	Label       string `json:"label"`        // human name
	Description string `json:"description"`  // what's in it / what breaks when you clear
	Size        int    `json:"size"`         // current item count
	Extra       string `json:"extra,omitempty"` // optional second metric
	CanClear    bool   `json:"can_clear"`    // false = read-only / safety-locked
}

// ListCaches returns every in-memory cache the admin can inspect or clear.
// Keep the key list here in sync with the switch in ClearCache below.
func ListCaches(c *gin.Context) {
	ipCount, _ := middleware.IPWhitelistCacheStats()
	lockTracked, lockActive := LockoutStats()
	dlTracked, dlRunning := DeadlinkProgressStats()

	entries := []CacheEntry{
		{
			Key: "ip_whitelist", Label: "IP 白名单缓存",
			Description: "从 settings 表读出的 IP/CIDR 列表，30 秒过期。清空后下一次请求会重读 DB。",
			Size:        ipCount,
			CanClear:    true,
		},
		{
			Key: "online_users", Label: "在线用户会话",
			Description: "最近 5 分钟内每个 (user, IP) 请求活动记录。清空会清空「在线用户」展示，不影响登录状态。",
			Size:        middleware.OnlineUsersSize(),
			CanClear:    true,
		},
		{
			Key: "login_limiter", Label: "登录限流计数（按 IP）",
			Description: "最近失败登录尝试时间戳。清空后触发过限流的 IP 立即重置。",
			Size:        middleware.LoginLimiterStats(),
			CanClear:    true,
		},
		{
			Key: "account_lockout", Label: "账户锁定（按邮箱）",
			Description: "连续失败后锁定的账户。清空会立即解锁所有被锁账户。",
			Size:        lockTracked,
			Extra:       intToStr(lockActive, "个锁定中"),
			CanClear:    true,
		},
		{
			Key: "heavy_rate_limiter", Label: "用户重操作限流",
			Description: "触发审计/死链/二创等限流的用户记录。清空后所有用户的限流立即重置。",
			Size:        middleware.HeavyLimiterStats(),
			CanClear:    true,
		},
		{
			Key: "oauth_state", Label: "OAuth state 令牌",
			Description: "发往 GSC/GA/Yandex 授权前下发的 CSRF state，10 分钟过期。清空会让所有正在授权的流程失败，需要重新发起。",
			Size:        OAuthStateStats(),
			CanClear:    true,
		},
		{
			Key: "yandex_user_id", Label: "Yandex user_id 缓存",
			Description: "Yandex Webmaster API 查询所需 user_id，绑定当前 access_token，6 小时过期。清空后下次同步会重新调 /user/ 拉取。",
			Size:        YandexUserIDCacheStats(),
			CanClear:    true,
		},
		{
			Key: "deadlink_progress", Label: "死链任务进度",
			Description: "每个站点正在运行 / 刚完成的死链任务状态。清空只会移除已完成的记录，运行中的任务不受影响。",
			Size:        dlTracked,
			Extra:       intToStr(dlRunning, "个运行中"),
			CanClear:    true,
		},
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": entries})
}

// ClearCache dispatches on :key to the right cache's Clear function.
func ClearCache(c *gin.Context) {
	key := c.Param("key")
	switch key {
	case "ip_whitelist":
		middleware.InvalidateIPWhitelistCache()
	case "online_users":
		middleware.ClearOnlineUsers()
	case "login_limiter":
		middleware.ClearLoginLimiter()
	case "heavy_rate_limiter":
		middleware.ClearHeavyLimiter()
	case "account_lockout":
		ClearLockouts()
	case "oauth_state":
		ClearOAuthStates()
	case "yandex_user_id":
		ClearYandexUserIDCache()
	case "deadlink_progress":
		ClearDeadlinkProgress()
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": -1, "error": "未知缓存键"})
		return
	}
	WriteAuditLog(c, "clear_cache", "cache", key, "")
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "已清空"})
}

func intToStr(n int, suffix string) string {
	if n == 0 {
		return ""
	}
	return itoa(n) + " " + suffix
}

// itoa avoids pulling in strconv for a single call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
