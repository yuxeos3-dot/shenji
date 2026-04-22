package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
	GooglebotUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
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
	if strings.EqualFold(os.Getenv("ALLOW_PRIVATE_FETCH"), "true") {
		log.Println("WARNING: ALLOW_PRIVATE_FETCH=true — SSRF protection is DISABLED. Private/internal network addresses are reachable. Do NOT use in production!")
		return true
	}
	return false
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

// sharedTransport is initialized once and reused by every HTTP client
// returned from BuildHTTPClient. Rebuilding a Transport per request (as
// the old code did) discarded the idle connection pool and TLS session
// cache, meaning a 500-page audit opened 500 fresh TCP+TLS handshakes
// against the target. Reusing the transport across all crawler requests
// cuts audit wall time dramatically.
var sharedTransport = func() *http.Transport {
	base := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		DialContext:           safeDialContext(base),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}()

// BuildHTTPClient returns an SSRF-aware HTTP client with the given timeout.
// All returned clients share a single Transport (idle-connection pool),
// so they're cheap to construct — the per-call overhead is a small
// struct allocation.
func BuildHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: sharedTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// Redirect target must also pass SSRF check.
			if err := validateURL(req.URL.String()); err != nil {
				return err
			}
			// Even when ssrfAllowed, verify the redirect target does not
			// resolve to a private IP. This prevents open-redirect chains
			// from reaching internal services.
			host := req.URL.Hostname()
			if ip := net.ParseIP(host); ip != nil {
				if isPrivateIP(ip) {
					return errBlockedHost
				}
			} else {
				ips, err := net.DefaultResolver.LookupIP(req.Context(), "ip", host)
				if err != nil {
					return fmt.Errorf("redirect target DNS lookup failed: %w", err)
				}
				for _, ip := range ips {
					if isPrivateIP(ip) {
						return errBlockedHost
					}
				}
			}
			return nil
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

// SharedTransport returns the SSRF-safe HTTP transport for reuse by
// other packages that need outbound HTTP with private-IP blocking.
func SharedTransport() *http.Transport {
	return sharedTransport
}

// ValidateURL rejects obviously bad URLs before any network work happens.
// Exported so other packages can pre-validate user-supplied URLs.
func ValidateURL(raw string) error {
	return validateURL(raw)
}

// VerifyResult holds the result of an external verification check.
type VerifyResult struct {
	URL        string            `json:"url"`
	Label      string            `json:"label"` // human-readable description
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"` // selected response headers
	Snippet    string            `json:"snippet"`  // first 2KB of body
	RedirectTo string            `json:"redirect_to"` // final URL after redirects
	Error      string            `json:"error"`
	TTFB       int64             `json:"ttfb_ms"`
}

// FetchVerification performs a lightweight GET request and captures
// status code, selected response headers, redirect chain, and a body
// snippet. Used by the audit pipeline's external verification phase.
func FetchVerification(rawURL, label string, timeout time.Duration) VerifyResult {
	result := VerifyResult{URL: rawURL, Label: label}
	
	if err := validateURL(rawURL); err != nil {
		result.Error = err.Error()
		return result
	}

	client := &http.Client{
		Timeout: timeout,
		Transport: sharedTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil // follow redirects
		},
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("User-Agent", GooglebotUA)
	req.Header.Set("Accept", "*/*")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	result.TTFB = time.Since(start).Milliseconds()
	result.StatusCode = resp.StatusCode
	result.RedirectTo = resp.Request.URL.String()

	// Capture selected headers
	result.Headers = make(map[string]string)
	for _, h := range []string{
		"Server", "Content-Type", "Cache-Control", "ETag",
		"Strict-Transport-Security", "Content-Security-Policy",
		"X-Frame-Options", "X-Content-Type-Options",
		"Referrer-Policy", "Permissions-Policy",
		"X-Powered-By", "X-Generator",
		"Content-Encoding", "Vary",
	} {
		if v := resp.Header.Get(h); v != "" {
			result.Headers[h] = v
		}
	}

	// Read body snippet (first 2KB)
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	result.Snippet = string(snippet)

	return result
}

// FetchVerificationNoFollow is like FetchVerification but does NOT follow redirects.
// Used to verify HTTP→HTTPS and www→naked domain redirects.
func FetchVerificationNoFollow(rawURL, label string, timeout time.Duration) VerifyResult {
	result := VerifyResult{URL: rawURL, Label: label}

	if err := validateURL(rawURL); err != nil {
		result.Error = err.Error()
		return result
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: sharedTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow
		},
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("User-Agent", GooglebotUA)

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	result.TTFB = time.Since(start).Milliseconds()
	result.StatusCode = resp.StatusCode
	if loc := resp.Header.Get("Location"); loc != "" {
		result.RedirectTo = loc
	}

	result.Headers = make(map[string]string)
	for _, h := range []string{"Location", "Server", "Strict-Transport-Security", "Content-Type"} {
		if v := resp.Header.Get(h); v != "" {
			result.Headers[h] = v
		}
	}

	return result
}
