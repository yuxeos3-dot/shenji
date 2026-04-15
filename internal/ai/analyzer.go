package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"seo-system/internal/crawler"
	"seo-system/internal/db"
)

// ─── v6 Report Structures ────────────────────────────────────────────────────

type V6Issue struct {
	ID            int    `json:"id"`
	Severity      string `json:"severity"`       // 致命/严重/中等/低
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

### 附加类F 深度白帽SEO优化建议（必须独立成章输出）
基于以上所有发现，以运营视角输出可落地的SEO优化路线图，作为单独一章：
1. **关键词策略**：基于现有TDK和H标签，给出目标关键词布局调整方案和3-5个长尾词方向
2. **内链建设方案**：识别权重高的页面，给出内链传递路径建议和锚文本优化方案
3. **内容优化清单**：首页/列表页/详情页各自的内容充实具体建议（目标字数/段落结构/多媒体配比）
4. **3个月技术修复计划**：按"影响权重×修复难度"优先级排序，给出第1个月/第2个月/第3个月应完成的修复项
5. **运营可立即执行的动作**：无需开发介入，运营人员自己就能做的10条优化项（修改Title/Description/alt/内链锚文本等）
6. **与行业最佳实践的差距**：对比同类头部站点应具备的SEO基础设施，指出3个最关键缺口

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
6. 必须覆盖 24 大类 + 6 附加类，完全合规的大类可以跳过
7. 必须有独立的"深度白帽 SEO 优化建议"章节（附加类 F）
8. 按章节分组，标题格式："一、TDK与基础Meta"、"二、H标签结构"……
9. 最后一章为"深度白帽 SEO 优化建议"
10. 先输出 fatal 和 serious，再 medium 和 low
11. **Token 优先级**：输出接近 30000 tokens 时，优先保留所有 fatal/serious 完整输出；low 级别可压缩 fix_suggestion 到 100 字内，但问题本身不能省略
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

	req, err := http.NewRequest("POST", apiURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, model, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 300 * time.Second} // 5min for deep audit
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

	// Find first { and matching }
	start := -1
	end := -1
	depth := 0
	for i, c := range s {
		if c == '{' {
			if start == -1 {
				start = i
			}
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if start >= 0 && end > start {
		return s[start:end]
	}
	// Try array [...]
	start, end, depth = -1, -1, 0
	for i, c := range s {
		if c == '[' {
			if start == -1 {
				start = i
			}
			depth++
		} else if c == ']' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if start >= 0 && end > start {
		return s[start:end]
	}
	return s
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

	// 1) Walk forward tracking structure state, recording a "rewind point" each
	//    time we finish an element at array depth (i.e. just after `,` or just
	//    before the closing `]` that follows a complete element). If the tail
	//    turns out to be inside a string, we'll rewind to the last safe point.
	rewindTo := -1 // byte offset of last `}` that was followed by `,` at array depth
	braces := 0
	brackets := 0
	inString := false
	escaped := false

	bytes := []byte(s)
	for i := 0; i < len(bytes); i++ {
		c := bytes[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case '{':
			braces++
		case '}':
			braces--
			// Record this position if we're inside at least one array.
			if brackets > 0 {
				rewindTo = i + 1 // position *after* the closing brace
			}
		case '[':
			brackets++
		case ']':
			brackets--
		}
	}

	// 2) If we're still inside a string at EOF, the tail is definitely truncated
	//    mid-value. Rewind to the last clean `}` inside an array (i.e. a fully
	//    written element) and discard everything after it.
	if inString && rewindTo > 0 {
		s = s[:rewindTo]
		// Recount the structure state of the rewound prefix.
		braces, brackets, inString = 0, 0, false
		escaped = false
		for i := 0; i < len(s); i++ {
			c := s[i]
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && inString {
				escaped = true
				continue
			}
			if c == '"' {
				inString = !inString
				continue
			}
			if inString {
				continue
			}
			switch c {
			case '{':
				braces++
			case '}':
				braces--
			case '[':
				brackets++
			case ']':
				brackets--
			}
		}
	}

	// 3) Trim any trailing `,` we may have been left with after rewinding or
	//    after the AI stopping between array elements.
	s = strings.TrimRight(s, " \t\r\n,")

	// 4) Close remaining structure. Close arrays and objects in the order they
	//    were opened (arrays generally live *inside* objects, so we alternate
	//    by looking at the last unmatched opener if we wanted to be precise,
	//    but a best-effort close-arrays-first-then-objects works for the known
	//    V6Report shape).
	for brackets > 0 {
		s += "]"
		brackets--
	}
	for braces > 0 {
		s += "}"
		braces--
	}
	return s
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

	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", apiURL+"/chat/completions", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
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
