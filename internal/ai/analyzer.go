package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/httpx"
	"seo-system/internal/db"
)

// ─── v6 Report Structures ────────────────────────────────────────────────────

type V6Issue struct {
	ID            int    `json:"id"`
	Severity      string `json:"severity"` // 致命/严重/中等/低
	Module        string `json:"module"`
	Title         string `json:"title"`
	Scope         string `json:"scope"`
	EvidenceLabel string `json:"evidence_label"`
	EvidenceCode  string `json:"evidence_code"`
	CurrentStatus string `json:"current_status"`
	FixSuggestion string `json:"fix_suggestion"`
}

type V6Chapter struct {
	Title  string    `json:"title"`
	Issues []V6Issue `json:"issues"`
}

type V6Report struct {
	Domain      string      `json:"domain"`
	GeneratedAt string      `json:"generated_at"`
	TotalIssues int         `json:"total_issues"`
	Fatal       int         `json:"fatal"`
	Serious     int         `json:"serious"`
	Medium      int         `json:"medium"`
	Low         int         `json:"low"`
	Chapters    []V6Chapter `json:"chapters"`
	Summary     string      `json:"summary"`
}

// ─── Legacy structures (kept for scheduler compatibility) ────────────────────

type AnalysisResult struct {
	SEOIssues      json.RawMessage `json:"seo_issues"`
	CodeIssues     json.RawMessage `json:"code_issues"`
	SecurityIssues json.RawMessage `json:"security_issues"`
	PerfIssues     json.RawMessage `json:"perf_issues"`
	Suggestions    string          `json:"suggestions"`
	Summary        string          `json:"summary"`
	V6Report       string          `json:"v6_report"` // full JSON of V6Report
	RawResponse    string          `json:"raw_response"`
}

type GSCDataForPrompt struct {
	Clicks      int     `json:"clicks"`
	Impressions int     `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
	Keywords    string  `json:"keywords_json"`
	Pages       string  `json:"pages_json"`
}

type GADataForPrompt struct {
	Sessions  int    `json:"sessions"`
	Users     int    `json:"users"`
	Pageviews int    `json:"pageviews"`
	Channels  string `json:"channels_json"`
}

type YandexDataForPrompt struct {
	IndexCount int    `json:"index_count"`
	Errors     string `json:"errors_json"`
}

type SitePageForPrompt struct {
	Name          string
	URL           string
	StatusCode    int
	TTFB          int64
	HTML          string
	LastFetchedAt string
}

// ─── Strategy (white-hat SEO) Prompt Builder ─────────────────────────────────

// BuildStrategyPrompt builds a prompt focused ONLY on the white-hat SEO
// strategy chapter. This runs as a second AI call so it's guaranteed to be
// generated even if the technical audit call gets cut off.
func BuildStrategyPrompt(domain string, sitePages []SitePageForPrompt, crawlPages []crawler.PageResult,
	gsc *GSCDataForPrompt, ga *GADataForPrompt, yandex *YandexDataForPrompt) string {

	now := time.Now().Format("2006-01-02 15:04:05")
	prompt := fmt.Sprintf(`你是一位资深 SEO 运营策略师。基于下方站点的真实 HTML 源码和数据，输出一份**深度白帽 SEO 优化路线图**。

## 站点
- 域名：%s
- 分析时间：%s

## 任务（只做这一件事，不做技术问题审计）

输出 10 条独立的 SEO 策略建议，作为 10 个 issue，组成一章"深度白帽 SEO 优化建议"。
每条建议必须可落地、可执行、可衡量。

## 10 条建议的固定题目（严格按此顺序输出，不得合并不得省略）

1. **关键词策略布局** — 基于现有 TDK 和 GSC 关键词，给出：
   - 3 个主攻关键词（当前位置 + 目标位置 + 预期流量）
   - 8 个长尾词方向（拼写示例 + 搜索量估算 + 对应落地页类型）
   - 关键词在 title/H1/首段/内链锚文本中的分布建议（给出具体模板）
   - 关键词与同类头部站点差距分析

2. **内链建设方案** — 识别权重最高的 3 个页面，给出：
   - 内链传递路径图（A → B → C 结构，至少 3 条路径）
   - 每个枢纽页应增加的内链数量和锚文本示例（至少 15 条）
   - 如何打通孤立页面（≥ 3 个具体做法）
   - 面包屑、相关推荐、底部导航的优化方案

3. **内容优化清单** — 针对首页 / 列表页 / 详情页三种类型，各给出：
   - 目标正文字数（最低值 + 推荐值）
   - 段落结构（开头 / 中间 / 结尾怎么写，给模板）
   - 多媒体配比（图片数 / 视频数 / 表格数）
   - 至少 8 个可在 24 小时内手改的具体文案位置
   - E-E-A-T 信号如何在内容中体现

4. **3 个月技术修复优先级** — 给出月度路线图：
   - 第 1 个月：致命+严重级修复清单（按"影响大×成本低"排序，前 10 项）
   - 第 2 个月：中等级 + 结构化数据 + Core Web Vitals
   - 第 3 个月：内容扩展 + 外链建设 + 国际化
   - 每项给出预估工时、负责角色（开发/运营/内容）

5. **运营可立即执行动作** — 无需开发介入、运营自己就能做的 15 条具体动作，每条必须含：
   - 操作位置（哪个页面 / 后台哪个菜单）
   - 操作内容（改成什么，给示例）
   - 预期效果（多久见效 / 指标怎么变）

6. **与行业头部站点的关键差距** — 对比同类头部站点，指出：
   - 5 个最关键的 SEO 基础设施缺口（Schema 类型 / Sitemap 分类 / hreflang / AMP / CDN 等）
   - 每个缺口的补救方案（具体配置代码）
   - 补上后预期带来的流量提升幅度（基于 GSC 数据估算）

7. **结构化数据扩展路线** — 基于站点类型推荐：
   - 应部署的 5+ 种 Schema 类型（Article / VideoObject / BreadcrumbList / FAQPage / Organization / Event 等）
   - 每种 Schema 的 JSON-LD 模板（直接可用）
   - 验证方式（rich results test / schema.org validator）
   - 预期 rich snippet 展示效果

8. **Sitemap 与索引策略** — 给出：
   - Sitemap 分片方案（按类型 / 时间 / 语言分）
   - sitemap_index.xml 的组织结构
   - 更新频率建议（lastmod / changefreq）
   - 在 GSC 中提交和监控索引的 SOP
   - 如何识别和处理 "已发现但未编入索引" 的问题

9. **外链与品牌建设** — 白帽外链策略：
   - 3 类可获取的白帽外链类型（行业目录 / 客座博文 / 资源页链接等）
   - 每类的获取方法和沟通话术模板
   - 品牌词曝光提升方案（UGC / 论坛 / 社媒）
   - 如何监控外链质量和 referral 流量

10. **数据驱动的持续优化 SOP** — 建立长效机制：
    - 每周必看的 5 个 GSC/GA 指标（点击、展示、CTR、位置、跳出率）
    - 每月应做的 3 项定期审计（Core Web Vitals / 索引覆盖 / 关键词排名）
    - 季度性的内容策略复盘流程
    - 如何设置告警（排名下滑 / 收录骤降 / 流量异常）

## GSC / GA 数据参考

`, domain, now)

	if gsc != nil {
		prompt += fmt.Sprintf("GSC: 点击=%d 展示=%d CTR=%.2f%% 排名=%.1f\nTop关键词: %s\nTop页面: %s\n\n",
			gsc.Clicks, gsc.Impressions, gsc.CTR*100, gsc.Position, gsc.Keywords, gsc.Pages)
	}
	if ga != nil {
		prompt += fmt.Sprintf("GA: 会话=%d 用户=%d 浏览=%d 渠道=%s\n\n",
			ga.Sessions, ga.Users, ga.Pageviews, ga.Channels)
	}
	if yandex != nil {
		prompt += fmt.Sprintf("Yandex: 收录=%d 错误=%s\n\n", yandex.IndexCount, yandex.Errors)
	}

	// Feed a compact version of page sources so the strategist has context.
	prompt += "## 页面源码样本（仅供参考，不用深扒每个细节）\n\n"
	for _, p := range sitePages {
		html := p.HTML
		if len(html) > 5000 {
			html = html[:5000] + "\n... [截断]"
		}
		prompt += fmt.Sprintf("### %s\nURL: %s\n```html\n%s\n```\n\n", p.Name, p.URL, html)
	}
	for _, p := range crawlPages {
		if p.HTML == "" {
			continue
		}
		html := p.HTML
		if len(html) > 4000 {
			html = html[:4000] + "\n... [截断]"
		}
		prompt += fmt.Sprintf("### 类型: %s\nURL: %s\n```html\n%s\n```\n\n", p.PageType, p.URL, html)
	}

	prompt += `## 输出格式

**只返回 JSON，不要 markdown 代码块标记。**

{
  "title": "深度白帽 SEO 优化建议",
  "issues": [
    {
      "id": 1,
      "severity": "严重",
      "module": "关键词策略",
      "title": "关键词策略布局",
      "scope": "全站",
      "evidence_label": "当前 TDK + GSC 头部关键词",
      "evidence_code": "（贴出当前 title / H1 / top 关键词，3-8 行）",
      "current_status": "（≤ 50 字，量化当前问题）",
      "fix_suggestion": "（400-700 字详细方案，必须含：关键词清单表、分布位置、预期流量）"
    },
    ... 其余 9 条同样结构 ...
  ]
}

**必须输出 10 个 issue，id 从 1 到 10，顺序严格按上方 1-10 的题目。severity 可以是 严重 或 中等。fix_suggestion 必须具体可执行，给代码/话术/清单，不要讲空话。一条都不许省略或合并。**`
	return prompt
}

// ─── Batched technical audit ─────────────────────────────────────────────────
//
// ai.router.team caps output at ~16k chars per response regardless of
// max_tokens. A full 50-issue audit needs ~35-45k chars, so we split into
// multiple smaller calls, each producing only part of the categories. Each
// batch returns pure chapters (no top-level stats/summary) which the caller
// merges.

// TechBatch defines one batch of the technical audit.
type TechBatch struct {
	Name       string // human-readable, for logs
	Categories string // markdown body describing which 大类 to check this round
}

// TechnicalBatches splits the 24 + 5 附加类 into two halves so each batch's
// output fits comfortably under the 16k-char output cap.
var TechnicalBatches = []TechBatch{
	{
		Name: "技术审计A（基础 + 结构化 + 性能）",
		Categories: `### 第1类 TDK 与基础 Meta
- title 标签：长度(30-60 字)、是否含核心关键词、是否跨页面重复、是否含品牌词
- description：长度(120-160 字)、是否有实质内容、是否与 title 重复
- keywords、og:title/og:description/og:image、twitter:card、canonical

### 第2类 H 标签结构
- H1 是否唯一/存在/与 title 呼应/被 CSS 隐藏
- H2-H6 层次是否合理、是否跳级、是否含目标关键词

### 第3类 图片 SEO
- img alt 完整性与有意义性、loading=lazy 是否误用于首屏、width/height、格式

### 第4类 链接与内链
- href 有效性、内链数量、重要页面是否在导航、nofollow 合理性、锚文本描述性、孤立页面

### 第5类 结构化数据 Schema
- ld+json 存在性、Schema 类型与页面匹配、必填字段完整性、AggregateRating、BreadcrumbList、Organization/WebSite

### 第6类 robots.txt & Sitemap
- robots.txt 存在性、是否误屏蔽、Sitemap 声明、Sitemap 可访问性、lastmod 真实性

### 第7类 Core Web Vitals HTML 信号
- LCP 元素 fetchpriority、首屏 preload、render-blocking CSS/JS、滥用 lazy、font-display、关键 CSS 内联

### 第8类 URL 结构
- 简洁语义化、多余参数、大小写、尾斜杠、分页 URL

### 第9类 安全响应头
- HSTS、X-Frame-Options/CSP、X-Content-Type-Options、Server/X-Powered-By 泄露

### 第16类 资源压缩与缓存
- gzip/br、cache-control、WebP/AVIF、CSS/JS minify、第三方脚本数

### 附加类E 页面加载速度
- 同步 JS 未 async/defer、第三方脚本数、preconnect/dns-prefetch、font-display:swap、@import 串行、大型 JS 整体引入、iframe 阻塞

### 第24类 HTTPS 与安全
- 混合内容、证书、内部链接 HTTPS、form action HTTPS`,
	},
	{
		Name: "技术审计B（内容 + 移动 + 影视 + 附加）",
		Categories: `### 第10类 内容质量
- 首页 >200 字实质内容、详情页正文充实度、模板重复、关键词堆砌、用户价值

### 第11类 移动端适配
- viewport、字号 ≥16px、水平滚动、点击目标 ≥48px

### 第12类 JavaScript SEO
- noscript 降级、重要内容依赖 JS、纯 CSR、JSON-LD 动态注入、模板语法暴露

### 第13类 重定向规范化
- HTTP→HTTPS、www 统一、多跳链、分页第 1 页规范化

### 第14类 爬虫陷阱
- 无限分页、分面 URL 组合、Session ID、日历归档

### 第15类 E-E-A-T 信号
- 作者信息、关于我们/联系方式、DMCA/版权、外链权威来源、组织信息完整性

### 第17类 国际化
- hreflang、HTML lang、UTF-8、canonical 与 hreflang 关系

### 第18类 RSS Feed 质量
- 存在性/可访问性、域名一致性、更新时效、URL canonical

### 第19类 内链分析
- 导航覆盖核心分类、footer 重要链接、面包屑、分页 rel=next/prev、推荐模块

### 第20类 分页 SEO
- 分页 title 差异化、canonical、第 2+ 页是否误 noindex、参数暴露

### 第21类 视频/媒体 SEO（影视站专项）
- 视频标题关键词、VideoObject Schema、缩略图 schema、视频 Sitemap、播出平台信息

### 第22类 404 与软 404
- 404 返回正确状态码、友好提示、软 404、noindex 页面被 sitemap 收录

### 第23类 重复内容
- 多 URL 同内容、canonical 正确性、打印版/移动版 canonical、标签页重叠

### 附加类A 隐藏文本与黑帽手法（Google 惩罚项）
- position:-9999px、display:none 含关键词、同色文字、font-size:0、overflow:hidden 裁剪、clip 隐藏

### 附加类B 关键词堆砌检测
- meta keywords >200 字、正文密度异常、隐藏关键词列表、footer 堆砌

### 附加类C 代码质量问题
- HTML 嵌套合法性、重复 ID、内联 style 过度、注释泄露敏感信息、javascript:void(0)、form 无 action、空 alt/title、遗留 console.log、隐藏 input

### 附加类D 安全漏洞风险
- Server/X-Powered-By/X-Generator 版本、API 密钥/Token 硬编码、敏感目录、混合内容、Cookie 属性、CORS、管理员路径`,
	},
}

// BuildTechPromptBatch builds a focused prompt for one technical batch.
func BuildTechPromptBatch(batch TechBatch, domain string, sitePages []SitePageForPrompt,
	crawlPages []crawler.PageResult, gsc *GSCDataForPrompt, ga *GADataForPrompt, yandex *YandexDataForPrompt) string {

	now := time.Now().Format("2006-01-02 15:04:05")
	var prompt strings.Builder
	fmt.Fprintf(&prompt, `你是一位资深 SEO 技术审计专家。我们已为你预先抓取好所有页面 HTML 源码，你基于源码直接分析，不要声称需要更多资料。

## 站点
- 域名：%s
- 审计时间：%s
- 本次批次：%s

## 本次只检查以下类别（其余类别会由其它调用覆盖，不要越界输出）

%s

---

## 已抓取页面源码（用于证据提取）

`, domain, now, batch.Name, batch.Categories)

	if len(sitePages) > 0 {
		prompt.WriteString("### 用户监控页面\n\n")
		for _, p := range sitePages {
			html := p.HTML
			if len(html) > 15000 {
				html = html[:15000] + "\n... [截断]"
			}
			fmt.Fprintf(&prompt, "#### %s\nURL: %s\n状态码: %d  TTFB: %dms  抓取时间: %s\n\n```html\n%s\n```\n\n---\n\n",
				p.Name, p.URL, p.StatusCode, p.TTFB, p.LastFetchedAt, html)
		}
	}
	if len(crawlPages) > 0 {
		prompt.WriteString("### 系统抓取页面\n\n")
		for _, p := range crawlPages {
			if p.HTML == "" {
				continue
			}
			html := p.HTML
			if len(html) > 12000 {
				html = html[:12000] + "\n... [截断]"
			}
			fmt.Fprintf(&prompt, "#### 类型: %s\nURL: %s  状态: %d  TTFB: %dms\n\n```html\n%s\n```\n\n---\n\n",
				p.PageType, p.URL, p.StatusCode, p.TTFB, html)
		}
	}
	if gsc != nil {
		fmt.Fprintf(&prompt, "\n### GSC\n点击 %d 展示 %d CTR %.2f%% 排名 %.1f\n关键词: %s\n页面: %s\n\n",
			gsc.Clicks, gsc.Impressions, gsc.CTR*100, gsc.Position, gsc.Keywords, gsc.Pages)
	}
	if ga != nil {
		fmt.Fprintf(&prompt, "### GA\n会话 %d 用户 %d 浏览 %d 渠道 %s\n\n", ga.Sessions, ga.Users, ga.Pageviews, ga.Channels)
	}
	if yandex != nil {
		fmt.Fprintf(&prompt, "### Yandex\n收录 %d 错误 %s\n\n", yandex.IndexCount, yandex.Errors)
	}

	prompt.WriteString(`---

## 输出格式（非常重要）

**只返回 JSON，不要 markdown 代码块标记。严禁在 JSON 前后出现任何解释文字。**

输出结构极简：

{"chapters":[{"title":"一、TDK与基础Meta","issues":[{"id":1,"severity":"严重","module":"TDK","title":"首页description缺失","scope":"首页","evidence_label":"首页 <head>","evidence_code":"<head>\n  <title>示例</title>\n</head>","current_status":"首页缺少 meta description","fix_suggestion":"在 <head> 中添加 <meta name=\"description\" content=\"...\">\n• 长度 120-160 字符\n• 每页唯一\n• 验收：用 GSC 覆盖率报告确认无缺失"}]}]}

**严格字段要求：**
- severity 只能是：致命/严重/中等/低
- id 在本批次内连续从 1 开始（不用担心跨批次冲突，后端会重排）
- title ≤ 20 字、scope ≤ 15 字、evidence_label ≤ 20 字
- evidence_code：3-10 行真实源码，不得编造
- current_status：**≤ 40 字**，只说缺什么/错什么，不解释原理不推断后果
- fix_suggestion：**200-400 字详细**，必须含：① 可复制的 HTML/配置代码 ② 3-5 条关键参数 ③ 验收方式

**完整性要求：本批次涉及的每一个大类都必须至少出现一次**（完全合规的大类在该章里放一条 severity=低 的"本类无明显问题"占位 issue）。`)
	return prompt.String()
}

// CallAIChapters calls the AI with a batched prompt and returns just the
// chapters array (no top-level stats). This is the preferred form for the
// split-call architecture.
// ─── Transient-error retry for AI HTTP calls ────────────────────────────────
//
// Upstream AI providers (and intermediate routers like ai.router.team)
// occasionally return 502/503 "upstream_error" blips during Anthropic
// capacity issues. Without retry, a single unlucky second can fail an
// entire audit — and users re-trigger the whole thing manually. We saw
// this cost a full audit run in production.
//
// Strategy: exponential backoff with jitter, 3 attempts total, retrying
// ONLY on transient errors (5xx, connection reset, timeout, context
// deadlines). 4xx errors (bad key, model not found, request too large)
// are NOT retried — they'll just fail repeatedly.
//
// The total worst-case wait is ~15s (2s + 5s + 8s) before giving up, which
// is well within the enclosing audit timeouts.

type aiHTTPResponse struct {
	status int
	body   []byte
}

// postAIWithRetry does a POST with JSON body to the given endpoint and
// retries on transient failures. The httpClient's timeout bounds ONE
// attempt — the retry loop sits outside that.
func postAIWithRetry(httpClient *http.Client, apiURL, apiKey string, body []byte, label string) (*aiHTTPResponse, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest("POST", strings.TrimRight(apiURL, "/")+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxAttempts && isTransientHTTPErr(err) {
				wait := retryDelay(attempt)
				log.Printf("[%s] attempt %d/%d network error: %v; retrying in %v", label, attempt, maxAttempts, err, wait)
				time.Sleep(wait)
				continue
			}
			return nil, fmt.Errorf("AI 请求失败: %w", err)
		}
		data, rerr := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
		resp.Body.Close()
		if rerr != nil {
			lastErr = rerr
			if attempt < maxAttempts {
				wait := retryDelay(attempt)
				log.Printf("[%s] attempt %d/%d read error: %v; retrying in %v", label, attempt, maxAttempts, rerr, wait)
				time.Sleep(wait)
				continue
			}
			return nil, rerr
		}

		// Retry on 5xx (including the specific "upstream_error" that
		// ai.router.team returns when Anthropic is hiccupping) and 429
		// (rate-limited). Don't retry 4xx — those reflect bad input.
		if (resp.StatusCode >= 500 || resp.StatusCode == 429) && attempt < maxAttempts {
			snippet := string(data)
			if len(snippet) > 300 {
				snippet = snippet[:300]
			}
			wait := retryDelay(attempt)
			log.Printf("[%s] attempt %d/%d HTTP %d: %s; retrying in %v", label, attempt, maxAttempts, resp.StatusCode, snippet, wait)
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
			time.Sleep(wait)
			continue
		}

		return &aiHTTPResponse{status: resp.StatusCode, body: data}, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("AI 请求失败：已重试 %d 次", maxAttempts)
}

// isTransientHTTPErr recognizes net errors that warrant a retry (connection
// reset, timeout, EOF during response reading). io.ErrUnexpectedEOF is
// the most common one when an upstream drops mid-response.
func isTransientHTTPErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	// Cover the practical set: connection reset, timeout, EOF, broken pipe,
	// context deadline (client-side timeout). We don't import net.Error
	// interface checks because wrapped errors work easier with string match.
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "timeout") ||
		strings.Contains(s, "EOF") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "deadline exceeded") ||
		strings.Contains(s, "i/o timeout")
}

// retryDelay returns a conservative backoff: 2s, 5s, 8s.
// We don't randomize — multiple parallel audits are rare on this system.
func retryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 2 * time.Second
	case 2:
		return 5 * time.Second
	default:
		return 8 * time.Second
	}
}

func CallAIChapters(prompt string) ([]V6Chapter, string, string, error) {
	apiURL := db.GetSetting("ai_api_url")
	apiKey := db.GetSetting("ai_api_key")
	model := db.GetSetting("ai_model")
	if apiURL == "" {
		apiURL = "https://ai.router.team/v1"
	}
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	maxTok := 16000
	if v := db.GetSetting("ai_max_tokens"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 128000 {
			maxTok = n
		}
	}

	reqBody := map[string]interface{}{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  maxTok,
		"temperature": 0.2,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", model, fmt.Errorf("AI chapters 请求体序列化失败: %w", err)
	}

	client := httpx.Outbound(300 * time.Second)
	result, err := postAIWithRetry(client, apiURL, apiKey, jsonBody, "ai-chapters")
	if err != nil {
		return nil, "", model, err
	}
	if result.status != 200 {
		snippet := string(result.body)
		if len(snippet) > 2000 {
			snippet = snippet[:2000]
		}
		return nil, "", model, fmt.Errorf("AI 错误 HTTP %d: %s", result.status, snippet)
	}
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(result.body, &apiResp); err != nil {
		return nil, "", model, err
	}
	if len(apiResp.Choices) == 0 {
		return nil, "", model, fmt.Errorf("AI 无返回")
	}
	raw := apiResp.Choices[0].Message.Content
	finishReason := apiResp.Choices[0].FinishReason
	content := extractJSON(raw)
	content = repairJSON(content)

	var wrapper struct {
		Chapters []V6Chapter `json:"chapters"`
	}
	if err := json.Unmarshal([]byte(content), &wrapper); err == nil && len(wrapper.Chapters) > 0 {
		// Drop trailing chapters with no issues (likely truncated).
		for len(wrapper.Chapters) > 0 && len(wrapper.Chapters[len(wrapper.Chapters)-1].Issues) == 0 {
			wrapper.Chapters = wrapper.Chapters[:len(wrapper.Chapters)-1]
		}
		return wrapper.Chapters, raw, model, nil
	} else if err != nil {
		// Log detailed diagnosis so we can see what broke. These logs go to
		// journalctl and are crucial for debugging AI output issues.
		headN := 200
		if len(content) < headN {
			headN = len(content)
		}
		tailStart := len(content) - 200
		if tailStart < 0 {
			tailStart = 0
		}
		return nil, raw, model, fmt.Errorf("chapters 解析失败: %v (finish=%s, rawLen=%d, headLen=%d, head=%q, tail=%q)",
			err, finishReason, len(raw), len(content), content[:headN], content[tailStart:])
	}
	return nil, raw, model, fmt.Errorf("AI 返回 chapters 为空（finish=%s, rawLen=%d）", finishReason, len(raw))
}

// ComposeV6Report assembles a full V6Report from batched chapters and the
// optional strategy chapter. Totals/severity counts are computed server-side.
func ComposeV6Report(domain string, chapters []V6Chapter, strategy *V6Chapter) (*V6Report, string) {
	out := &V6Report{
		Domain:      domain,
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
		Chapters:    chapters,
	}
	if strategy != nil && len(strategy.Issues) > 0 {
		out.Chapters = append(out.Chapters, *strategy)
	}

	// Renumber issue IDs globally for display consistency.
	gid := 1
	for ci := range out.Chapters {
		for ii := range out.Chapters[ci].Issues {
			out.Chapters[ci].Issues[ii].ID = gid
			gid++
		}
	}

	// Recompute totals + severity buckets + build a server-side summary.
	var severities []string
	for _, ch := range out.Chapters {
		for _, iss := range ch.Issues {
			out.TotalIssues++
			switch iss.Severity {
			case "致命":
				out.Fatal++
			case "严重":
				out.Serious++
			case "中等":
				out.Medium++
			case "低":
				out.Low++
			}
			severities = append(severities, iss.Severity)
		}
	}

	out.Summary = fmt.Sprintf("共检查 %d 个章节，发现 %d 个问题（致命 %d、严重 %d、中等 %d、低 %d）。建议优先处理致命/严重级别，3 个月内逐步修复全部问题。",
		len(out.Chapters), out.TotalIssues, out.Fatal, out.Serious, out.Medium, out.Low)
	_ = severities

	js, _ := json.Marshal(out)
	return out, string(js)
}

// ─── V6 Prompt Builder ────────────────────────────────────────────────────────

func BuildV6AuditPrompt(domain string, sitePages []SitePageForPrompt, crawlPages []crawler.PageResult,
	gsc *GSCDataForPrompt, ga *GADataForPrompt, yandex *YandexDataForPrompt) string {

	now := time.Now().Format("2006-01-02 15:04:05")

	prompt := fmt.Sprintf(`你是一位资深SEO技术审计专家，专门对娱乐内容站、影视聚合站、新闻资讯站做源码级SEO深度审计。
我们的系统已经预先抓取了以下所有页面的完整HTML源码，你直接基于这些源码进行分析，无需再抓取。

## 站点信息
- 域名：%s
- 审计时间：%s
- 站点类型：待判断（根据源码自动识别）

---

## 核心要求（严格遵守）

1. **严格基于证据**：每个问题必须引用真实HTML源码片段作为evidence_code，不允许编造
2. **穷尽式查找**：目标35-70个独立问题，覆盖所有已抓取页面
3. **可执行建议**：每个fix_suggestion必须含具体HTML代码示例或操作步骤
4. **全面覆盖**：必须检查以下24大类+6个附加类，每类至少输出1条问题

---

## 必须检查的24大类（每类至少检查以下要点）

### 第1类 TDK与基础Meta
- title标签：长度(30-60字)、是否含核心关键词、是否跨页面重复、是否包含品牌词
- description：长度(120-160字)、是否有实质内容、是否与title重复
- keywords：是否存在(现在意义不大但注意堆砌)
- og:title/og:description/og:image是否完整
- twitter:card是否配置
- canonical：是否存在、是否正确、是否自引用

### 第2类 H标签结构
- H1是否唯一、是否存在、是否与title呼应、是否被CSS隐藏
- H2-H6层次是否合理、是否跳级
- H标签是否含目标关键词
- 每类页面H1格式是否一致

### 第3类 图片SEO
- img是否有alt属性、alt是否有意义(非"image"/"img123")
- 是否有loading="lazy"（注意首屏图片不应lazy）
- 是否有width/height防止布局偏移(CLS)
- src是否有效、是否有图片SEO扩展名

### 第4类 链接与内链
- a标签是否有href、href是否有效
- 内链数量(首页建议>50条有意义内链)
- 重要页面是否在导航中
- nofollow使用是否合理
- 锚文本是否描述性(避免"点击这里")
- 是否有孤立页面(无内链指向)

### 第5类 结构化数据(Schema)
- 是否有application/ld+json
- Schema类型是否匹配页面(详情页→VideoObject/Movie/Article，列表页→ItemList，首页→WebSite+SearchAction)
- 必填字段是否完整(name/description/url/image等)
- AggregateRating是否有ratingCount+ratingValue
- BreadcrumbList是否正确
- Organization/WebSite是否在首页声明

### 第6类 robots.txt & Sitemap
- robots.txt是否存在、是否屏蔽重要路径
- 是否意外屏蔽(/category/、/movie/、/article/)
- Sitemap是否在robots.txt中声明
- Sitemap是否可访问、格式是否正确
- lastmod是否真实

### 第7类 性能与Core Web Vitals HTML信号
- LCP候选元素是否有fetchpriority="high"
- 首屏图片是否有preload link
- 是否有render-blocking CSS/JS
- 是否滥用loading="lazy"在首屏图片
- font-display是否设置
- 关键CSS是否内联

### 第8类 URL结构
- URL是否简洁语义化(避免?id=123)
- 是否有多余参数
- 大小写是否统一
- 是否有重复URL(带/不带尾斜杠)
- 分页URL是否规范

### 第9类 安全响应头
- 是否有HSTS
- 是否有X-Frame-Options或CSP frame-ancestors
- 是否有X-Content-Type-Options: nosniff
- Server头是否泄露版本信息
- 是否有X-Powered-By暴露技术栈

### 第10类 内容质量
- 首页是否有实质性文字内容(>200字)
- 详情页正文是否充实
- 是否有大量模板重复内容
- keyword stuffing迹象
- 内容是否对用户有价值

### 第11类 移动端适配
- 是否有viewport meta标签
- viewport content是否正确(width=device-width,initial-scale=1)
- 字体大小是否适合移动端(>=16px)
- 是否有水平滚动条触发元素
- 点击目标是否足够大(>=48px)

### 第12类 JavaScript SEO
- 是否有<noscript>降级
- 重要内容是否依赖JS渲染
- 是否是纯CSR(body只有<div id="app">)
- JSON-LD是否动态注入(影响爬取)
- 是否有Vue/React模板语法暴露在HTML中

### 第13类 重定向规范化
- HTTP→HTTPS重定向是否正确
- www→non-www是否统一
- 是否有多跳重定向链(301→302→200)
- 分页第1页是否重定向到无分页版本

### 第14类 爬虫陷阱
- 是否有无限分页/无限滚动
- 分面导航是否产生大量URL(?sort=&genre=&year=)
- Session ID是否出现在URL中
- 日历归档是否产生无限URL

### 第15类 E-E-A-T信号
- 是否有作者信息(author页面/Schema中的author)
- 是否有关于我们/联系方式页面
- 是否有DMCA/版权声明
- 是否有外链到权威来源
- 媒体信息/组织信息是否完整

### 第16类 资源压缩与缓存
- 是否启用gzip/br压缩(从Content-Encoding判断)
- 静态资源是否有cache-control
- 图片是否使用WebP/AVIF
- CSS/JS是否压缩(minified)
- 是否有不必要的第三方脚本

### 第17类 国际化
- hreflang是否正确配置(多语言站)
- HTML lang属性是否正确
- 字符集是否为UTF-8
- 如果有多语言，canonical和hreflang关系是否正确

### 第18类 RSS Feed质量
- Feed是否存在且可访问
- Feed中域名是否与主域一致(镜像站问题)
- Feed更新是否及时
- Feed中URL是否为canonical URL

### 第19类 内链分析
- 导航菜单是否覆盖核心分类
- footer是否有重要链接
- 面包屑是否存在且正确
- 分页rel=next/prev是否正确(或已改为canonical)
- 是否有推荐/相关内容模块

### 第20类 分页SEO
- 分页页面title是否有差异化
- 分页是否有canonical
- 第2+页是否被错误noindex
- 分页参数是否在robots.txt/sitemap中暴露

### 第21类 视频/媒体SEO（影视站专项）
- 视频标题是否含关键词
- 是否有VideoObject Schema
- 视频缩略图是否有schema声明
- 是否有视频Sitemap
- 播放页是否有播出平台信息

### 第22类 404与软404
- 404页面是否返回正确HTTP状态码(而不是200)
- 404页面是否有友好提示和导航
- 是否有重要页面返回软404(200状态但内容提示不存在)
- 是否有noindex的页面被sitemap收录

### 第23类 重复内容
- 是否有多个URL访问同一内容
- canonical是否正确处理重复
- 打印版/移动版是否有canonical
- 标签页/分类页内容是否高度重叠

### 第24类 HTTPS与安全
- 混合内容(HTTP资源在HTTPS页面)
- 证书是否有效
- 内部链接是否全部使用HTTPS
- form action是否使用HTTPS

### 附加类A 隐藏文本与黑帽手法（Google惩罚项）
- CSS position:absolute;left:-9999px 隐藏关键词
- display:none 包含SEO关键词文字
- 前景色与背景色相同的文字
- font-size:0 隐藏文字
- overflow:hidden 裁剪含关键词内容
- clip:rect(1px,1px,1px,1px) 隐藏内容

### 附加类B 关键词堆砌检测
- meta keywords超过200字
- 正文关键词密度异常（同一词出现>15次/1000字）
- 隐藏的关键词列表区块
- Footer区域堆砌城市/品牌词

### 附加类C 代码质量问题
- HTML嵌套结构合法性（未闭合标签、错误嵌套如p>div）
- 同一页面重复ID属性（违反HTML规范）
- 内联style过度使用（>20处，维护困难）
- HTML注释中暴露敏感信息（路径/版本/TODO/FIXME/API密钥）
- 大量 href="javascript:void(0)" 无效链接
- form元素缺少action属性
- 空alt/title批量检测
- script标签内遗留调试console.log/测试代码
- 未使用的隐藏input字段（潜在信息泄露）

### 附加类D 安全漏洞风险
- Server响应头版本号暴露（Apache/2.4.51、nginx/1.18.0等）
- X-Powered-By泄露框架版本（PHP/8.1.0、Express/4.x等）
- X-Generator暴露CMS版本（WordPress 6.x等）
- HTML源码中API密钥/Token硬编码
- /uploads/、/backup/、/config/ 等敏感目录
- 混合内容（HTTP资源在HTTPS页面加载）
- Cookie缺少Secure/HttpOnly/SameSite属性
- CORS配置过宽（Access-Control-Allow-Origin: *）
- 管理员路径未被robots屏蔽（/admin/、/wp-admin/）

### 附加类E 页面加载速度优化
- 同步JS在<head>中阻塞渲染（应加async/defer属性）
- 第三方脚本数量统计（广告/统计/社交/客服 >5个）
- 未使用preconnect/dns-prefetch提前建立连接
- 字体文件缺少font-display:swap（导致FOIT/FOUT）
- 大量使用@import串行加载CSS
- 图片未使用CDN加速（直接用本站域名）
- 大型JS库整体引入未按需加载
- 第三方iframe（广告/视频）阻塞主线程

（注：白帽 SEO 优化策略章节由另一次独立的 AI 调用生成并合并，本次只需专注技术问题）

---

## ⚠️ 完整性自检清单（输出前必须逐项过一遍）

在你开始写 JSON 前，先在脑子里过一遍下面 29 个检查类，每一类**必须**至少有一次判定结果（有问题就列成 issue，完全合规就在本类最后补一个 severity=低、title="本类无明显问题" 的占位 issue——不要整类跳过不提）：

- [ ] 1. TDK 与基础 Meta
- [ ] 2. H 标签结构
- [ ] 3. 图片 SEO
- [ ] 4. 链接与内链
- [ ] 5. 结构化数据 Schema
- [ ] 6. robots.txt & Sitemap
- [ ] 7. 性能与 Core Web Vitals HTML 信号
- [ ] 8. URL 结构
- [ ] 9. 安全响应头
- [ ] 10. 内容质量
- [ ] 11. 移动端适配
- [ ] 12. JavaScript SEO
- [ ] 13. 重定向规范化
- [ ] 14. 爬虫陷阱
- [ ] 15. E-E-A-T 信号
- [ ] 16. 资源压缩与缓存
- [ ] 17. 国际化 hreflang
- [ ] 18. RSS Feed 质量
- [ ] 19. 内链分析
- [ ] 20. 分页 SEO
- [ ] 21. 视频/媒体 SEO
- [ ] 22. 404 与软 404
- [ ] 23. 重复内容
- [ ] 24. HTTPS 与安全
- [ ] 附加 A. 隐藏文本 / 黑帽手法
- [ ] 附加 B. 关键词堆砌
- [ ] 附加 C. 代码质量
- [ ] 附加 D. 安全漏洞风险
- [ ] 附加 E. 页面加载速度优化

**任何一类没有出现在最终输出里视为违规**。

---

## 已抓取页面源码

`, domain, now)

	// Add site_pages (manually fetched pages)
	if len(sitePages) > 0 {
		prompt += "### 【手动监控页面 — 用户已预先抓取】\n\n"
		for _, p := range sitePages {
			html := p.HTML
			if len(html) > 15000 {
				html = html[:15000] + "\n... [已截断，保留前15000字符]"
			}
			prompt += fmt.Sprintf("#### 页面：%s\n- URL: %s\n- 状态码: %d  TTFB: %dms  抓取时间: %s\n\n```html\n%s\n```\n\n---\n\n",
				p.Name, p.URL, p.StatusCode, p.TTFB, p.LastFetchedAt, html)
		}
	}

	// Add crawl pages (auto-crawled)
	if len(crawlPages) > 0 {
		prompt += "### 【系统自动抓取页面】\n\n"
		for _, p := range crawlPages {
			if p.HTML == "" {
				continue
			}
			html := p.HTML
			if len(html) > 12000 {
				html = html[:12000] + "\n... [已截断，保留前12000字符]"
			}
			errInfo := ""
			if p.Error != "" {
				errInfo = fmt.Sprintf("  错误: %s", p.Error)
			}
			prompt += fmt.Sprintf("#### 页面类型：%s\n- URL: %s\n- 状态码: %d  TTFB: %dms%s\n\n```html\n%s\n```\n\n---\n\n",
				p.PageType, p.URL, p.StatusCode, p.TTFB, errInfo, html)
		}
	}

	// Add analytics data
	if gsc != nil {
		prompt += fmt.Sprintf(`
### GSC数据（最近30天）
- 总点击: %d  总展示: %d  平均CTR: %.2f%%  平均排名: %.1f
- Top关键词: %s
- Top页面: %s

`, gsc.Clicks, gsc.Impressions, gsc.CTR*100, gsc.Position, gsc.Keywords, gsc.Pages)
	}

	if ga != nil {
		prompt += fmt.Sprintf(`
### GA数据（最近30天）
- 会话: %d  用户: %d  页面浏览: %d
- 渠道: %s

`, ga.Sessions, ga.Users, ga.Pageviews, ga.Channels)
	}

	if yandex != nil {
		prompt += fmt.Sprintf("### Yandex数据\n- 收录数: %d  错误: %s\n\n", yandex.IndexCount, yandex.Errors)
	}

	// Output format instruction
	prompt += fmt.Sprintf(`
---

## 输出格式要求

**只返回JSON，不要任何其他内容，不要markdown代码块标记。**

严格按以下JSON结构输出（示例已给出，实际内容必须基于上方源码）：

{
  "domain": "%s",
  "generated_at": "%s",
  "total_issues": 42,
  "fatal": 5,
  "serious": 15,
  "medium": 18,
  "low": 4,
  "summary": "一句话总结站点SEO健康状况",
  "chapters": [
    {
      "title": "一、TDK与基础Meta",
      "issues": [
        {
          "id": 1,
          "severity": "严重",
          "module": "TDK",
          "title": "首页description缺失",
          "scope": "首页",
          "evidence_label": "homepage <head> 源码",
          "evidence_code": "<head>\n  <title>示例站</title>\n  <!-- 无description -->\n</head>",
          "current_status": "首页缺少 meta description。",
          "fix_suggestion": "在 <head> 中添加：\n<meta name=\"description\" content=\"【站点名】提供最新电影、电视剧在线观看，海量高清资源免费播放。\">\n\n要点：\n• 长度 120–160 字符；移动端 SERP 会截到 ~120 字\n• 前 70 字必须包含核心关键词（首页=品牌词+业务词；列表页=分类词+长尾）\n• 每个页面唯一，禁止模板化套用\n• 结尾建议加行动号召（如「立即观看」「免费注册」）\n• 避免硬编码域名出现在 description 文案里（显得不专业）\n• 配合 Open Graph og:description 同步更新\n\n验收：在 Google Search Console → 网页体验 → 覆盖率 中看不到「meta description 缺失」提示；用 https://www.mysitedisplay.com/ 预览 SERP 显示符合预期。"
        }
      ]
    }
  ]
}

**严格要求：**
1. severity 只能是：致命、严重、中等、低
2. id 全局连续，从 1 开始
3. evidence_code 必须是从上方源码中提取的真实片段，不允许编造
4. **字段长度规范（严格遵守，保证问题一个都不漏）：**
   - title：≤ 20 字，一句话点名问题，不加语气词
   - scope：≤ 15 字，如"首页 head"、"分类页"、"全站"
   - evidence_label：≤ 20 字
   - evidence_code：只贴最关键的 3-10 行源码，不要整段 head/body；多行用 \n
   - current_status：**≤ 40 字**，直接说"缺什么/错什么/量化数值"，不解释原理不推断后果
   - summary：≤ 80 字，不要展开
   - fix_suggestion：**可详细 200-400 字**，必须含：① 可复制的 HTML/配置代码 ② 3-5 条关键参数（字数/位置/频次） ③ 验收方式（用什么工具验证）
5. **发现多少报多少，一个都不许漏**。质量优先，不要凑数编造问题。
6. 必须覆盖 24 大类技术检查点，完全合规的大类可以跳过
7. 按章节分组，标题格式："一、TDK与基础Meta"、"二、H标签结构"……
8. 先输出 fatal 和 serious，再 medium 和 low
9. **本次不需要输出"深度白帽 SEO 优化建议"章节**（会由另一次独立调用生成并合并）
10. **Token 优先级**：输出接近 30000 tokens 时，优先保留所有 fatal/serious 完整输出；low 级别可压缩 fix_suggestion 到 100 字内，但问题本身不能省略
`, domain, time.Now().Format("2006-01-02"))

	return prompt
}

// ─── Legacy prompt builder (kept for scheduler) ──────────────────────────────

func BuildAnalysisPrompt(domain string, pages []crawler.PageResult, gsc *GSCDataForPrompt, ga *GADataForPrompt, yandex *YandexDataForPrompt) string {
	sitePages := make([]SitePageForPrompt, 0, len(pages))
	return BuildV6AuditPrompt(domain, sitePages, pages, gsc, ga, yandex)
}

// ─── AI Call ──────────────────────────────────────────────────────────────────

func CallAI(prompt string) (*AnalysisResult, string, error) {
	apiURL := db.GetSetting("ai_api_url")
	apiKey := db.GetSetting("ai_api_key")
	model := db.GetSetting("ai_model")

	if apiURL == "" {
		apiURL = "https://ai.router.team/v1"
	}
	if model == "" {
		model = "claude-sonnet-4-6"
	}

	// Resolve max_tokens with runtime override: settings > env > default.
	// Claude Sonnet 4.6 supports up to 64k output tokens; we default to 32k
	// which is plenty for a full V6 report but still bounded.
	maxTok := 32000
	if v := db.GetSetting("ai_max_tokens"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 128000 {
			maxTok = n
		}
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  maxTok,
		"temperature": 0.2,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, model, fmt.Errorf("构建请求失败: %w", err)
	}

	req, err := http.NewRequest("POST", strings.TrimRight(apiURL, "/")+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, model, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := httpx.Outbound(300 * time.Second) // 5min for deep audit
	resp, err := client.Do(req)
	if err != nil {
		return nil, model, fmt.Errorf("AI请求失败: %w", err)
	}
	defer resp.Body.Close()

	// Cap response body size at 20 MiB to avoid OOM from pathological responses.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, model, fmt.Errorf("读取AI响应失败: %w", err)
	}

	if resp.StatusCode != 200 {
		snippet := string(body)
		if len(snippet) > 2000 {
			snippet = snippet[:2000]
		}
		return nil, model, fmt.Errorf("AI返回错误 (HTTP %d): %s", resp.StatusCode, snippet)
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, model, fmt.Errorf("解析AI响应失败: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		snippet := string(body)
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		return nil, model, fmt.Errorf("AI未返回结果，响应体: %s", snippet)
	}

	finishReason := apiResp.Choices[0].FinishReason
	rawContent := apiResp.Choices[0].Message.Content
	truncated := finishReason == "length"

	// Extract + repair. Repair is rewrite-aware: if the tail is a half-written
	// element we rewind to the last clean issue before closing brackets.
	content := extractJSON(rawContent)
	content = repairJSON(content)

	// Try to parse as V6Report. We now accept "partial" success: as long as
	// we got at least one chapter with at least one issue we return what we
	// have, tagged with a truncation notice in Summary. This prevents the
	// entire audit from being discarded just because the tail got clipped.
	var v6 V6Report
	if err := json.Unmarshal([]byte(content), &v6); err == nil && len(v6.Chapters) > 0 {
		// Prune any trailing chapter whose issues array is empty — that's
		// almost always the one that got truncated and contains no useful data.
		for len(v6.Chapters) > 0 && len(v6.Chapters[len(v6.Chapters)-1].Issues) == 0 {
			v6.Chapters = v6.Chapters[:len(v6.Chapters)-1]
		}
		// Recalculate totals if zero
		if v6.TotalIssues == 0 {
			for _, ch := range v6.Chapters {
				v6.TotalIssues += len(ch.Issues)
				for _, iss := range ch.Issues {
					switch iss.Severity {
					case "致命":
						v6.Fatal++
					case "严重":
						v6.Serious++
					case "中等":
						v6.Medium++
					case "低":
						v6.Low++
					}
				}
			}
		}
		summary := v6.Summary
		if truncated {
			v6.Summary = "【注意：AI 输出因 token 上限被截断，尾部章节可能不完整】\n" + v6.Summary
			summary = v6.Summary
		}
		// Re-serialize clean JSON
		cleanJSON, _ := json.Marshal(v6)
		suggestions := fmt.Sprintf("共发现%d个问题（致命:%d 严重:%d 中等:%d 低:%d）", v6.TotalIssues, v6.Fatal, v6.Serious, v6.Medium, v6.Low)
		if truncated {
			suggestions = "⚠️ 输出被截断，以下为可用部分。" + suggestions
		}
		result := &AnalysisResult{
			Summary:        summary,
			Suggestions:    suggestions,
			SEOIssues:      json.RawMessage("[]"),
			CodeIssues:     json.RawMessage("[]"),
			SecurityIssues: json.RawMessage("[]"),
			PerfIssues:     json.RawMessage("[]"),
			V6Report:       string(cleanJSON),
			RawResponse:    rawContent,
		}
		return result, model, nil
	}

	// If parse failed but the response was definitely truncated, return a
	// targeted error referring to token limit instead of the generic "invalid
	// JSON" one.
	if truncated {
		return &AnalysisResult{RawResponse: rawContent}, model,
			fmt.Errorf("AI输出被截断（finish_reason=length），且修复后仍无法解析。建议在系统设置里提高 ai_max_tokens（当前 %d），或减少监控页面数量", maxTok)
	}

	// Fallback — JSON parse failed. Return the raw response so the caller can
	// persist it for debugging, and surface a preview in the error message.
	preview := content
	if len(preview) > 200 {
		preview = preview[:200]
	}
	return &AnalysisResult{RawResponse: rawContent}, model,
		fmt.Errorf("AI返回内容无法解析为报告格式，内容预览: %s", preview)
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// Strip markdown code blocks: ```json ... ``` or ``` ... ```
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			inner := strings.Join(lines[1:], "\n")
			// Remove trailing ```
			if idx := strings.LastIndex(inner, "```"); idx >= 0 {
				inner = inner[:idx]
			}
			s = strings.TrimSpace(inner)
		}
	}

	// Find first { and matching }, STRING-AWARE so that { and } inside JSON
	// string values (very common in evidence_code / fix_suggestion) don't
	// throw off depth counting.
	start, end := findBalanced(s, '{', '}')
	if start >= 0 && end > start {
		return s[start:end]
	}
	// Fallback: try an array [...]
	start, end = findBalanced(s, '[', ']')
	if start >= 0 && end > start {
		return s[start:end]
	}
	return s
}

// findBalanced scans s for the first balanced pair of openCh/closeCh while
// correctly skipping over characters inside JSON string literals. Returns
// (startIdx, endIdxExclusive) or (-1, -1) if no balanced pair is found.
func findBalanced(s string, openCh, closeCh byte) (int, int) {
	start := -1
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case openCh:
			if start == -1 {
				start = i
			}
			depth++
		case closeCh:
			if depth > 0 {
				depth--
				if depth == 0 {
					return start, i + 1
				}
			}
		}
	}
	return -1, -1
}

// repairJSON repairs JSON that was cut off by the token limit. The key trick:
// when truncation happens mid-string, just closing brackets is wrong because
// the current object / array element is half-written garbage. Instead we walk
// backwards from the tail, find the last point at which the structure was
// *cleanly* between array elements (i.e. after a `}` or `]` at array depth
// inside `chapters.issues`), discard the partial element, and only then close
// the remaining open containers.
func repairJSON(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return s
	}

	// scanState returns (openStack, inString, rewindPoint).
	// openStack is an ordered list of '{'/'[' that were opened but not closed.
	// rewindPoint is the byte offset immediately after the last '}' at depth
	// inside at least one array — a known-clean element boundary.
	scan := func(in string) (stack []byte, inString bool, rewind int) {
		rewind = -1
		escaped := false
		bracketDepth := 0
		for i := 0; i < len(in); i++ {
			c := in[i]
			if escaped {
				escaped = false
				continue
			}
			if inString {
				switch c {
				case '\\':
					escaped = true
				case '"':
					inString = false
				}
				continue
			}
			switch c {
			case '"':
				inString = true
			case '{':
				stack = append(stack, '{')
			case '[':
				stack = append(stack, '[')
				bracketDepth++
			case '}':
				if n := len(stack); n > 0 && stack[n-1] == '{' {
					stack = stack[:n-1]
				}
				if bracketDepth > 0 {
					rewind = i + 1 // position *after* a '}' that sits inside an array
				}
			case ']':
				if n := len(stack); n > 0 && stack[n-1] == '[' {
					stack = stack[:n-1]
				}
				if bracketDepth > 0 {
					bracketDepth--
				}
			}
		}
		return
	}

	stack, inString, rewind := scan(s)

	// If the tail is mid-string, rewind to the last clean element boundary.
	if inString && rewind > 0 {
		s = s[:rewind]
		stack, inString, _ = scan(s)
		_ = inString
	}

	// Drop dangling ',' / whitespace.
	s = strings.TrimRight(s, " \t\r\n,")

	// Close the stack in reverse (LIFO) order — this is the crucial correctness
	// fix: if the open order was { [ { [, the close order must be ] } ] }.
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i] {
		case '{':
			s += "}"
		case '[':
			s += "]"
		}
	}
	return s
}

// CallAIStrategy runs a separate, smaller AI call that produces only the
// "深度白帽 SEO 优化建议" chapter. Returns a parsed V6Chapter so it can be
// appended to the technical report. Best-effort: on any failure returns
// (nil, err) and the caller should keep the technical report without it.
func CallAIStrategy(prompt string) (*V6Chapter, string, error) {
	apiURL := db.GetSetting("ai_api_url")
	apiKey := db.GetSetting("ai_api_key")
	model := db.GetSetting("ai_model")
	if apiURL == "" {
		apiURL = "https://ai.router.team/v1"
	}
	if model == "" {
		model = "claude-sonnet-4-6"
	}

	// Smaller bound for strategy: 6 issues × ~1000 chars ≈ well under 16k tokens
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  16000,
		"temperature": 0.3,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, model, fmt.Errorf("策略 AI 请求体序列化失败: %w", err)
	}

	client := httpx.Outbound(240 * time.Second)
	result, err := postAIWithRetry(client, apiURL, apiKey, jsonBody, "ai-strategy")
	if err != nil {
		return nil, model, fmt.Errorf("策略 AI 请求失败: %w", err)
	}
	if result.status != 200 {
		return nil, model, fmt.Errorf("策略 AI 错误 HTTP %d", result.status)
	}
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(result.body, &apiResp); err != nil {
		return nil, model, err
	}
	if len(apiResp.Choices) == 0 {
		return nil, model, fmt.Errorf("策略 AI 无返回")
	}
	content := extractJSON(apiResp.Choices[0].Message.Content)
	content = repairJSON(content)

	var chapter V6Chapter
	if err := json.Unmarshal([]byte(content), &chapter); err == nil && len(chapter.Issues) > 0 {
		if chapter.Title == "" {
			chapter.Title = "深度白帽 SEO 优化建议"
		}
		return &chapter, model, nil
	}
	return nil, model, fmt.Errorf("策略 AI 返回格式异常")
}

// MergeStrategyChapter appends/replaces the strategy chapter inside a
// v6 JSON blob. Returns the new JSON string; if anything fails returns
// the original unchanged.
func MergeStrategyChapter(v6JSON string, strategy *V6Chapter) string {
	if strategy == nil || v6JSON == "" {
		return v6JSON
	}
	var r V6Report
	if err := json.Unmarshal([]byte(v6JSON), &r); err != nil {
		return v6JSON
	}
	// Remove any existing strategy chapter (match by title containing "白帽" or "策略")
	out := r.Chapters[:0]
	for _, ch := range r.Chapters {
		if strings.Contains(ch.Title, "白帽") || strings.Contains(ch.Title, "优化建议") {
			continue
		}
		out = append(out, ch)
	}
	r.Chapters = append(out, *strategy)

	// Recompute stats (include strategy issues in totals).
	r.TotalIssues, r.Fatal, r.Serious, r.Medium, r.Low = 0, 0, 0, 0, 0
	for _, ch := range r.Chapters {
		r.TotalIssues += len(ch.Issues)
		for _, iss := range ch.Issues {
			switch iss.Severity {
			case "致命":
				r.Fatal++
			case "严重":
				r.Serious++
			case "中等":
				r.Medium++
			case "低":
				r.Low++
			}
		}
	}
	out2, _ := json.Marshal(r)
	return string(out2)
}

func TestAIConnection() error {
	apiURL := db.GetSetting("ai_api_url")
	apiKey := db.GetSetting("ai_api_key")
	model := db.GetSetting("ai_model")

	if apiURL == "" || apiKey == "" {
		return fmt.Errorf("AI API未配置")
	}

	reqBody := map[string]interface{}{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "Say hello in one word."}},
		"max_tokens": 10,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("请求体序列化失败: %w", err)
	}
	req, err := http.NewRequest("POST", strings.TrimRight(apiURL, "/")+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := httpx.Outbound(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("AI返回错误 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
