package crawler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func FetchPage(spec URLSpec, timeout time.Duration) PageResult {
	result := PageResult{
		PageType: spec.PageType,
		URL:      spec.URL,
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

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
	defer resp.Body.Close()

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
