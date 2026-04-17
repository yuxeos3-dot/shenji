package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	PageTypeHome       = "home"
	PageTypeCategory   = "category"
	PageTypeDetail     = "detail"
	PageTypeNewsList   = "news_list"
	PageTypeNewsDetail = "news_detail"
	PageType404        = "404"
	PageTypeSearch     = "search"
	PageTypeStatic     = "static"

	MaxHTMLSize = 500 * 1024 // 500KB
	GooglebotUA = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
)

type URLSpec struct {
	PageType string
	URL      string
}

type PageResult struct {
	PageType   string
	URL        string
	StatusCode int
	TTFB       int64 // milliseconds
	HTML       string
	HTMLSize   int
	Error      string
}

type SiteConfig struct {
	Domain        string
	URLCategory   string
	URLDetail     string
	URLNewsList   string
	URLNewsDetail string
	URLSearch     string
	URLStatic     string
	URLPagesJSON  string // JSON: [{name, url}, ...]
}

type CustomPage struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func BuildURLList(site SiteConfig) []URLSpec {
	domain := strings.TrimRight(site.Domain, "/")
	if !strings.HasPrefix(domain, "http") {
		domain = "https://" + domain
	}

	urls := []URLSpec{
		{PageType: PageTypeHome, URL: domain + "/"},
	}

	if site.URLCategory != "" {
		urls = append(urls, URLSpec{PageType: PageTypeCategory, URL: ensureAbsURL(domain, site.URLCategory)})
	}
	if site.URLDetail != "" {
		urls = append(urls, URLSpec{PageType: PageTypeDetail, URL: ensureAbsURL(domain, site.URLDetail)})
	}
	if site.URLNewsList != "" {
		urls = append(urls, URLSpec{PageType: PageTypeNewsList, URL: ensureAbsURL(domain, site.URLNewsList)})
	}
	if site.URLNewsDetail != "" {
		urls = append(urls, URLSpec{PageType: PageTypeNewsDetail, URL: ensureAbsURL(domain, site.URLNewsDetail)})
	}

	urls = append(urls, URLSpec{PageType: PageType404, URL: domain + "/this-page-does-not-exist-404-test"})

	if site.URLSearch != "" {
		urls = append(urls, URLSpec{PageType: PageTypeSearch, URL: ensureAbsURL(domain, site.URLSearch)})
	} else {
		urls = append(urls, URLSpec{PageType: PageTypeSearch, URL: domain + "/search?q=test"})
	}

	if site.URLStatic != "" {
		urls = append(urls, URLSpec{PageType: PageTypeStatic, URL: ensureAbsURL(domain, site.URLStatic)})
	} else {
		urls = append(urls, URLSpec{PageType: PageTypeStatic, URL: domain + "/about"})
	}

	// Parse custom pages from JSON
	if site.URLPagesJSON != "" && site.URLPagesJSON != "[]" {
		var customPages []CustomPage
		if err := json.Unmarshal([]byte(site.URLPagesJSON), &customPages); err == nil {
			for _, p := range customPages {
				if p.URL != "" {
					urls = append(urls, URLSpec{
						PageType: "custom_" + p.Name,
						URL:      ensureAbsURL(domain, p.URL),
					})
				}
			}
		}
	}

	return urls
}

func ensureAbsURL(domain, path string) string {
	if strings.HasPrefix(path, "http") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return domain + path
}

func CrawlAllPages(site SiteConfig) []PageResult {
	urls := BuildURLList(site)
	sem := make(chan struct{}, 8)
	results := make([]PageResult, len(urls))
	var wg sync.WaitGroup

	for i, u := range urls {
		wg.Add(1)
		go func(idx int, spec URLSpec) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = FetchPage(spec, 15*time.Second)
		}(i, u)
	}

	wg.Wait()
	return results
}

// ─── SSRF protection ─────────────────────────────────────────────────────────

var errBlockedHost = errors.New("目标地址被安全策略拒绝（内网/环回/元数据端点不可访问）")

// isPrivateIP returns true for private / loopback / link-local / CGNAT / multicast /
// unspecified / IPv6 ULA addresses, plus the AWS/GCP/Azure metadata endpoint.
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsPrivate() {
		return true
	}
	// IPv4 CGNAT 100.64.0.0/10
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1]&0xC0 == 64 {
			return true
		}
		// Cloud metadata service: 169.254.169.254 (covered by IsLinkLocalUnicast)
		if v4[0] == 169 && v4[1] == 254 {
			return true
		}
	}
	return false
}

// ssrfAllowed is resolved once at startup: the escape hatch only works in
// non-release Gin modes, so setting ALLOW_PRIVATE_FETCH=true in production
// has no effect. This prevents accidental internal-network fetches.
var ssrfAllowedCached = func() bool {
	if os.Getenv("GIN_MODE") == "release" {
		return false
	}
	return strings.EqualFold(os.Getenv("ALLOW_PRIVATE_FETCH"), "true")
}()

func ssrfAllowed() bool {
	return ssrfAllowedCached
}

// safeDialContext wraps a base dialer to reject connections to private IPs.
// It resolves the hostname once and dials the chosen IP directly so that a
// subsequent DNS flip (DNS rebinding) cannot swap the IP between check and dial.
func safeDialContext(base *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if ssrfAllowed() {
			return base.DialContext(ctx, network, addr)
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		// If addr is already a literal IP we can validate and dial directly.
		if ip := net.ParseIP(host); ip != nil {
			if isPrivateIP(ip) {
				return nil, errBlockedHost
			}
			return base.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		var chosen net.IP
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return nil, errBlockedHost
			}
			if chosen == nil {
				chosen = ip
			}
		}
		if chosen == nil {
			return nil, errBlockedHost
		}
		// Dial the already-validated IP to close the rebinding race window.
		return base.DialContext(ctx, network, net.JoinHostPort(chosen.String(), port))
	}
}

// validateURL rejects obviously bad URLs before any network work happens.
func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("URL 解析失败: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("仅支持 http / https 协议")
	}
	if u.Host == "" {
		return fmt.Errorf("URL 缺少主机名")
	}
	if ssrfAllowed() {
		return nil
	}
	// If the host is a literal IP, check it here — DNS dial will also check,
	// but this lets us fail fast with a clearer error.
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil && isPrivateIP(ip) {
		return errBlockedHost
	}
	return nil
}

// BuildHTTPClient returns an SSRF-aware HTTP client with the given timeout.
func BuildHTTPClient(timeout time.Duration) *http.Client {
	base := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		DialContext:           safeDialContext(base),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// Redirect target must also pass SSRF check.
			return validateURL(req.URL.String())
		},
	}
}

func FetchPage(spec URLSpec, timeout time.Duration) PageResult {
	result := PageResult{
		PageType: spec.PageType,
		URL:      spec.URL,
	}

	if err := validateURL(spec.URL); err != nil {
		result.Error = err.Error()
		return result
	}

	client := BuildHTTPClient(timeout)

	req, err := http.NewRequest("GET", spec.URL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("User-Agent", GooglebotUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer func() {
		// Drain remaining body so the underlying TCP connection can be
		// reused by the transport's idle pool. Otherwise concurrent
		// crawls eventually exhaust the connection pool on errored
		// reads.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// TTFB is captured at the time the response headers arrive. `client.Do`
	// returns when the HTTP response is available — the body is still
	// streaming, so this is a faithful first-byte timing.
	result.TTFB = time.Since(start).Milliseconds()
	result.StatusCode = resp.StatusCode

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxHTMLSize))
	if err != nil {
		result.Error = "读取响应失败: " + err.Error()
		return result
	}

	result.HTML = string(body)
	result.HTMLSize = len(body)

	return result
}
