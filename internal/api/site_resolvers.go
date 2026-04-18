package api

// Platform site resolvers.
//
// Before this module, every sync/get handler read per-platform identifier
// fields (gsc_site_url, ga_property_id, yandex_host_id, bing_site_url)
// from our local sites table. Users filled them by hand on the site-edit
// form, and format mismatches (e.g. entering "hl365.com" while GSC's
// property is "sc-domain:hl365.com") silently returned empty data.
//
// These resolvers instead:
//   1. Read only the user-friendly `domain` field from our sites table.
//   2. Call the upstream API to list all properties visible to the
//      authorized account / API key.
//   3. Pick the property whose canonical host matches our domain.
//
// This removes the "platform identifier" fields from the user-facing
// form entirely. If the resolver doesn't find a match, it returns an
// explicit error that the frontend surfaces as "该站点在 X 平台未绑定
// / 未验证".
//
// Each resolver is independently cached per-process (5 min TTL) since
// GSC/GA/Bing/Yandex site lists change rarely but every audit/sync
// would otherwise call the list endpoint too.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// canonicalHost strips scheme, "sc-domain:" prefix, "www.", and any
// trailing slash/port to produce a comparable host string.
// "https://www.hl365.com/"  -> "hl365.com"
// "sc-domain:hl365.com"     -> "hl365.com"
// "http://hl365.com:80/abc" -> "hl365.com"
// "https:hl365.com:443"     -> "hl365.com"    (Yandex's weird host_id format)
// "91dasj.com"              -> "91dasj.com"
func canonicalHost(s string) string {
	// TODO(#97): Add IDN-to-ASCII (Punycode) conversion here so that
	// internationalized domain names (e.g. "例え.jp") are correctly
	// matched against their ASCII form ("xn--r8jz45g.jp"). The
	// golang.org/x/net/idna package is already an indirect dependency
	// in go.mod — import idna and call idna.Lookup.ToASCII(h) after
	// lowercasing. Deferred because it requires testing with real IDN
	// domains and the current user base is ASCII-only.
	h := strings.TrimSpace(s)
	h = strings.TrimPrefix(h, "sc-domain:")
	// Yandex's host_id looks like "https:example.com:443". The colon
	// placement is unusual — normalize it by stripping any "scheme:"
	// and port we can detect.
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	h = strings.TrimPrefix(h, "https:")
	h = strings.TrimPrefix(h, "http:")
	// Drop everything from the first '/' onward (path).
	if idx := strings.Index(h, "/"); idx >= 0 {
		h = h[:idx]
	}
	// Drop port (":80" / ":443").
	if idx := strings.LastIndex(h, ":"); idx > 0 {
		h = h[:idx]
	}
	h = strings.TrimPrefix(h, "www.")
	return strings.ToLower(h)
}

// ─── Tiny in-process cache ───────────────────────────────────────────────

type resolveCacheEntry struct {
	at    time.Time
	value string
}

var (
	resolveCache   = map[string]resolveCacheEntry{}
	resolveCacheMu sync.Mutex
)

const resolveCacheTTL = 5 * time.Minute

func resolveCacheGet(key string) (string, bool) {
	resolveCacheMu.Lock()
	defer resolveCacheMu.Unlock()
	e, ok := resolveCache[key]
	if !ok {
		return "", false
	}
	if time.Since(e.at) > resolveCacheTTL {
		delete(resolveCache, key)
		return "", false
	}
	return e.value, true
}
func resolveCacheSet(key, value string) {
	resolveCacheMu.Lock()
	defer resolveCacheMu.Unlock()
	resolveCache[key] = resolveCacheEntry{at: time.Now(), value: value}
}

// init starts a background goroutine that sweeps expired resolveCache
// entries every 10 minutes, preventing unbounded memory growth in
// long-running processes.
func init() {
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			resolveCacheMu.Lock()
			now := time.Now()
			for k, e := range resolveCache {
				if now.Sub(e.at) > resolveCacheTTL {
					delete(resolveCache, k)
				}
			}
			resolveCacheMu.Unlock()
		}
	}()
}

// ─── GSC resolver ────────────────────────────────────────────────────────

// resolveGSCProperty takes a raw domain (e.g. "hl365.com") and returns
// the actual GSC property identifier we should use when calling
// searchAnalytics/query. It prefers URL-prefix properties (exact https
// match) over domain properties, but returns either if both exist.
func resolveGSCProperty(token, domain string, userID int64) (string, error) {
	dom := canonicalHost(domain)
	if dom == "" {
		return "", fmt.Errorf("站点域名为空")
	}
	cacheKey := fmt.Sprintf("gsc:%d:%s", userID, dom)
	if v, ok := resolveCacheGet(cacheKey); ok {
		return v, nil
	}
	raw, err := gscFetch(token, gscAPI+"/sites", "GET", nil)
	if err != nil {
		return "", fmt.Errorf("获取 GSC 站点列表失败: %w", err)
	}
	var resp struct {
		SiteEntry []struct {
			SiteURL         string `json:"siteUrl"`
			PermissionLevel string `json:"permissionLevel"`
		} `json:"siteEntry"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	var urlPrefixMatch, domainMatch string
	for _, s := range resp.SiteEntry {
		// Restricted users can't query searchAnalytics — skip those.
		if s.PermissionLevel == "siteUnverifiedUser" {
			continue
		}
		sh := canonicalHost(s.SiteURL)
		if sh != dom {
			continue
		}
		if strings.HasPrefix(s.SiteURL, "sc-domain:") {
			if domainMatch == "" {
				domainMatch = s.SiteURL
			}
		} else {
			if urlPrefixMatch == "" {
				urlPrefixMatch = s.SiteURL
			}
		}
	}
	// Prefer URL-prefix property if both exist (they query identically
	// but URL-prefix is more specific).
	winner := urlPrefixMatch
	if winner == "" {
		winner = domainMatch
	}
	if winner == "" {
		return "", fmt.Errorf("GSC 账号下未找到匹配站点 %s 的 property（请确认该站点已在 Search Console 里添加并验证）", dom)
	}
	resolveCacheSet(cacheKey, winner)
	return winner, nil
}

// ─── GA4 resolver ────────────────────────────────────────────────────────

// resolveGAProperty finds a GA4 property under the account whose
// displayName contains the domain. GA4 does not directly expose the
// tracked URL in Admin API; property displayNames are user-chosen.
// Best effort: match case-insensitive substring of the canonical host.
func resolveGAProperty(token, domain string, userID int64) (string, error) {
	dom := canonicalHost(domain)
	if dom == "" {
		return "", fmt.Errorf("站点域名为空")
	}
	cacheKey := fmt.Sprintf("ga:%d:%s", userID, dom)
	if v, ok := resolveCacheGet(cacheKey); ok {
		return v, nil
	}
	raw, err := gaAdminGET(token, "https://analyticsadmin.googleapis.com/v1beta/accountSummaries?pageSize=200")
	if err != nil {
		return "", fmt.Errorf("获取 GA4 属性列表失败: %w", err)
	}
	var resp struct {
		AccountSummaries []struct {
			PropertySummaries []struct {
				Property    string `json:"property"`    // "properties/456"
				DisplayName string `json:"displayName"` // user-chosen
			} `json:"propertySummaries"`
		} `json:"accountSummaries"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	// Two-pass: first exact-host match in displayName, then substring.
	var bestProp string
	for _, a := range resp.AccountSummaries {
		for _, p := range a.PropertySummaries {
			name := strings.ToLower(p.DisplayName)
			if name == dom {
				bestProp = p.Property
				break
			}
		}
		if bestProp != "" {
			break
		}
	}
	if bestProp == "" {
		for _, a := range resp.AccountSummaries {
			for _, p := range a.PropertySummaries {
				name := strings.ToLower(p.DisplayName)
				if name == dom || strings.HasSuffix(name, "."+dom) {
					bestProp = p.Property
					break
				}
			}
			if bestProp != "" {
				break
			}
		}
	}
	if bestProp == "" {
		return "", fmt.Errorf("GA4 未找到匹配站点 %s 的 Property（请将 GA4 Property 命名为域名或包含域名的字符串）", dom)
	}
	// strip "properties/" prefix
	id := strings.TrimPrefix(bestProp, "properties/")
	resolveCacheSet(cacheKey, id)
	return id, nil
}

// ─── Bing resolver ───────────────────────────────────────────────────────

// resolveBingSiteURL finds a verified site under the Bing API key
// whose URL matches the given domain.
func resolveBingSiteURL(apiKey, domain string, userID int64) (string, error) {
	dom := canonicalHost(domain)
	if dom == "" {
		return "", fmt.Errorf("站点域名为空")
	}
	cacheKey := fmt.Sprintf("bing:%d:%s", userID, dom)
	if v, ok := resolveCacheGet(cacheKey); ok {
		return v, nil
	}
	raw, err := bingFetch(apiKey, "GetUserSites", nil)
	if err != nil {
		return "", fmt.Errorf("获取 Bing 站点列表失败: %w", err)
	}
	var resp struct {
		D []struct {
			URL        string `json:"Url"`
			IsVerified bool   `json:"IsVerified"`
		} `json:"d"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	for _, s := range resp.D {
		if !s.IsVerified {
			continue
		}
		if canonicalHost(s.URL) == dom {
			resolveCacheSet(cacheKey, s.URL)
			return s.URL, nil
		}
	}
	return "", fmt.Errorf("Bing Webmaster 未找到匹配站点 %s 的已验证站点", dom)
}

// ─── Yandex resolver ─────────────────────────────────────────────────────

// resolveYandexHost finds a Yandex host_id matching the given domain.
// Yandex host_id looks like "https:hl365.com:443" — our canonicalHost
// normalizer strips the scheme and port for comparison.
func resolveYandexHost(token, yandexUserID, domain string, appUserID int64) (string, error) {
	dom := canonicalHost(domain)
	if dom == "" {
		return "", fmt.Errorf("站点域名为空")
	}
	cacheKey := fmt.Sprintf("yandex:%d:%s", appUserID, dom)
	if v, ok := resolveCacheGet(cacheKey); ok {
		return v, nil
	}
	raw, err := yandexFetch(token, "/user/"+url.PathEscape(userID)+"/hosts")
	if err != nil {
		return "", fmt.Errorf("获取 Yandex 站点列表失败: %w", err)
	}
	var resp struct {
		Hosts []struct {
			HostID         string `json:"host_id"`
			UnicodeHostURL string `json:"unicode_host_url"`
			MainMirrorURL  string `json:"main_mirror_url"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	for _, h := range resp.Hosts {
		for _, candidate := range []string{h.HostID, h.UnicodeHostURL, h.MainMirrorURL} {
			if canonicalHost(candidate) == dom {
				resolveCacheSet(cacheKey, h.HostID)
				return h.HostID, nil
			}
		}
	}
	return "", fmt.Errorf("Yandex Webmaster 未找到匹配站点 %s 的 host", dom)
}
