<template>
  <div class="sites-page">
    <!-- 页头 -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">站点管理</h1>
        <span class="page-subtitle">管理所有监控站点，触发深度巡检任务</span>
      </div>
      <el-button class="btn-sunrise" @click="openAddDialog">
        <span class="btn-icon">+</span> 添加站点
      </el-button>
    </div>

    <!-- 系统能力介绍卡片 -->
    <div class="capability-card">
      <div class="cap-header">
        <span class="cap-icon">🚀</span>
        <div>
          <div class="cap-title">SEO 深度审计系统 · 工作流 v6 ——&nbsp; 全站源码级分析引擎</div>
          <div class="cap-subtitle">覆盖 28 大类 200+ 检查点，融合 AI 深度分析与白帽 SEO 优化建议，媲美专业 SEO 机构的全自动审计引擎。每次巡检输出 35–70 个独立问题，每个问题附带真实源码证据 + 可执行的 HTML 修复代码，让运营和开发都能直接落地执行。</div>
        </div>
      </div>

      <div class="cap-section-title">📋 检查覆盖范围</div>
      <div class="cap-grid">

        <div class="cap-item">
          <div class="cap-item-icon">🏷️</div>
          <div class="cap-item-content">
            <div class="cap-item-title">TDK 与基础 Meta</div>
            <div class="cap-item-desc">Title 长度（30–60字）、关键词覆盖率、跨页面重复检测；Description 120–160 字规范、实质性内容检查；Open Graph og:title/og:description/og:image 完整性；Twitter Card 配置；Canonical 自引用正确性；meta robots 指令合理性</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">📐</div>
          <div class="cap-item-content">
            <div class="cap-item-title">H 标签结构与关键词布局</div>
            <div class="cap-item-desc">H1 唯一性与存在性、CSS display:none/position:absolute 隐藏 H1 检测（Google 惩罚项）、H2–H6 层级跳跃检测、H 标签关键词密度、各页面类型 H1 模板一致性、H 标签与 Title 关键词呼应</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🖼️</div>
          <div class="cap-item-content">
            <div class="cap-item-title">图片 SEO 全项检测</div>
            <div class="cap-item-desc">alt 属性完整性与描述性（拒绝"image123"类无效 alt）、首屏图片禁止 loading="lazy"（影响 LCP）、图片 width/height 属性防止布局偏移（CLS）、WebP/AVIF 现代格式使用率、图片文件名语义化、og:image 尺寸规范（1200×630）</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🔗</div>
          <div class="cap-item-content">
            <div class="cap-item-title">链接与内链架构</div>
            <div class="cap-item-desc">首页内链数量（建议 >50 条有意义链接）、孤立页面发现（无内链指向的页面）、nofollow 使用合理性分析、锚文本描述性检测（拒绝"点击这里"式锚文本）、面包屑导航完整性、分页 rel=next/prev 规范、导航菜单覆盖核心分类</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">📊</div>
          <div class="cap-item-content">
            <div class="cap-item-title">结构化数据 Schema</div>
            <div class="cap-item-desc">VideoObject / Movie / TVSeries / Article / NewsArticle / BreadcrumbList / WebSite+SearchAction / AggregateRating（含 ratingCount）/ Organization / Author / FAQ / HowTo / ItemList 字段完整性验证；JSON-LD 格式合法性；@type 与页面类型匹配</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🤖</div>
          <div class="cap-item-content">
            <div class="cap-item-title">robots.txt & Sitemap 专项</div>
            <div class="cap-item-desc">重要路径意外屏蔽检测（/category/、/movie/、/article/）、应屏蔽路径未屏蔽（/admin/、/api/、/login/）、Sitemap 格式与可访问性、lastmod 真实性验证（批量伪造检测）、视频/新闻专项 Sitemap、Sitemap URL 含参数爬虫陷阱</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">⚡</div>
          <div class="cap-item-content">
            <div class="cap-item-title">Core Web Vitals HTML 信号</div>
            <div class="cap-item-desc">LCP 候选元素 fetchpriority="high" 检测、首屏关键图片 preload link 标签、render-blocking CSS/JS 资源识别、font-display 字体加载策略、关键 CSS 内联优化、LCP 图片误加 lazy-load（致命性能问题）</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🚀</div>
          <div class="cap-item-content">
            <div class="cap-item-title">页面加载速度优化</div>
            <div class="cap-item-desc">第三方脚本数量与阻塞分析、同步 JS 在 &lt;head&gt; 中加载检测、CSS/JS 文件是否压缩（minified）、图片尺寸是否优化、字体文件加载方式、资源合并与分割策略、preconnect/dns-prefetch 配置</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">💾</div>
          <div class="cap-item-content">
            <div class="cap-item-title">资源压缩与缓存策略</div>
            <div class="cap-item-desc">gzip/brotli 压缩启用状态（Content-Encoding 头检测）、静态资源 Cache-Control 配置、ETag/Last-Modified 协商缓存、CDN 命中率（CF-Cache-Status）、图片 CDN 使用情况、资源版本控制</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🔒</div>
          <div class="cap-item-content">
            <div class="cap-item-title">安全响应头全检</div>
            <div class="cap-item-desc">HSTS（Strict-Transport-Security）强制 HTTPS、CSP（Content-Security-Policy）内容安全策略、X-Frame-Options 点击劫持防护、X-Content-Type-Options MIME 嗅探防护、Referrer-Policy 信息泄露控制、Permissions-Policy 浏览器权限控制</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🕵️</div>
          <div class="cap-item-content">
            <div class="cap-item-title">服务器信息泄露检测</div>
            <div class="cap-item-desc">Server 响应头版本号暴露（如 Apache/2.4.51）、X-Powered-By 框架版本泄露（如 PHP/8.1.0）、X-Generator CMS 版本暴露、X-AspNet-Version 泄露、Via CDN 节点信息、HTML 注释中的敏感路径/版本信息</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🛡️</div>
          <div class="cap-item-content">
            <div class="cap-item-title">HTTPS 与安全合规</div>
            <div class="cap-item-desc">混合内容（HTTP 资源加载在 HTTPS 页面）、内部链接全部使用 HTTPS、form action 安全性、SSL 证书状态、cookie 安全属性（Secure/HttpOnly/SameSite）、敏感接口是否暴露给爬虫</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">💻</div>
          <div class="cap-item-content">
            <div class="cap-item-title">代码质量问题</div>
            <div class="cap-item-desc">HTML 结构合法性（嵌套错误、未闭合标签）、重复 ID 属性、内联样式过度使用、TODO/FIXME/测试代码残留在 HTML 中、console.error 暗示、无效的 href="javascript:void(0)"、空 alt/title 属性批量检测</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🧩</div>
          <div class="cap-item-content">
            <div class="cap-item-title">JavaScript SEO 专项</div>
            <div class="cap-item-desc">纯 CSR SPA 检测（body 只有 &lt;div id="app"&gt; — 致命问题）、Vue/React 模板指令暴露在 HTML 中、noscript 降级内容、重要导航/内容依赖 JS 渲染、JSON-LD 动态注入风险、Service Worker 干扰爬取</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">↩️</div>
          <div class="cap-item-content">
            <div class="cap-item-title">重定向规范化</div>
            <div class="cap-item-desc">HTTP→HTTPS 301 正确跳转、www/non-www 统一规范、多跳重定向链检测（301→302→200 性能损耗）、URL 大小写无规范化、双斜杠 URL、/index.html 冗余、分页第1页重定向到无分页版本</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🕸️</div>
          <div class="cap-item-content">
            <div class="cap-item-title">爬虫陷阱全检</div>
            <div class="cap-item-desc">分面导航无限 URL（?sort=&genre=&year= 组合爆炸）、Session ID 出现在 URL 中、日历归档无限翻页、Sitemap 含参数 URL 陷阱、无限滚动页面爬取盲区、软 404（返回 200 但内容提示不存在）</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">📱</div>
          <div class="cap-item-content">
            <div class="cap-item-title">移动端适配全检</div>
            <div class="cap-item-desc">viewport meta 标签配置正确性、正文字体大小 ≥16px（避免小字体惩罚）、水平滚动触发元素检测、点击目标尺寸 ≥48px（避免误触）、PC 与移动端 UA 响应内容一致性、独立移动站点检测</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🔄</div>
          <div class="cap-item-content">
            <div class="cap-item-title">重复内容处理</div>
            <div class="cap-item-desc">多 URL 访问同一内容检测、Canonical 处理正确性、标签页/分类页内容高度重叠分析、打印版/移动版 Canonical、分页内容重复、参数化 URL 重复内容、404 页面内容是否被索引</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">✍️</div>
          <div class="cap-item-content">
            <div class="cap-item-title">内容质量评估</div>
            <div class="cap-item-desc">首页文字内容 >200 字检测、详情页正文充实度、模板重复内容比例、关键词堆砌（Keyword Stuffing）检测、内容对用户真实价值评估、更新时效性（lastmod 与实际内容一致性）</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">👻</div>
          <div class="cap-item-content">
            <div class="cap-item-title">隐藏文本与黑帽手法检测</div>
            <div class="cap-item-desc">position:absolute;left:-9999px 关键词堆砌、display:none 包含关键词文字、同色文字（前景色=背景色）、font-size:0 隐藏文字、overflow:hidden 裁剪内容 — 以上均为 Google 惩罚项，会导致手动降权</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🏆</div>
          <div class="cap-item-content">
            <div class="cap-item-title">E-E-A-T 权威性信号</div>
            <div class="cap-item-desc">作者页面与 Schema Author 字段完整性、关于我们/联系方式/隐私政策页面、DMCA 版权声明、外链权威来源（Wikipedia/官方机构）、Organization Schema 组织信息、用户评分与评论结构化数据</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🎬</div>
          <div class="cap-item-content">
            <div class="cap-item-title">视频/影视站 SEO 专项</div>
            <div class="cap-item-desc">VideoObject Schema 必填字段（name/description/thumbnailUrl/uploadDate/duration）、视频标题关键词布局、缩略图 CDN 可访问性、视频 Sitemap 完整性、播出平台/来源信息声明、连续剧 TVSeries+TVEpisode 层级</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🌐</div>
          <div class="cap-item-content">
            <div class="cap-item-title">国际化与多语言</div>
            <div class="cap-item-desc">hreflang 标签配置正确性（x-default 必须存在）、HTML lang 属性语言代码规范、UTF-8 字符集声明、Canonical 与 hreflang 关系一致性（不能相互矛盾）、多语言版本 Sitemap 分割策略</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">♿</div>
          <div class="cap-item-content">
            <div class="cap-item-title">无障碍访问（影响 SEO）</div>
            <div class="cap-item-desc">form 元素是否有 label 关联、图片 alt 对屏幕阅读器友好、aria-label 合理使用、skip navigation 链接、语义化 HTML 标签使用（nav/main/article/section/header/footer）</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">📰</div>
          <div class="cap-item-content">
            <div class="cap-item-title">RSS Feed 质量</div>
            <div class="cap-item-desc">Feed 是否存在且可访问、Feed 中域名是否与主域一致（镜像站风险）、Feed 更新时效性、Feed 中 URL 是否为 Canonical URL、Feed 内容完整度</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🔢</div>
          <div class="cap-item-content">
            <div class="cap-item-title">分页 SEO 专项</div>
            <div class="cap-item-desc">分页 Title 差异化（第2页不能与第1页 Title 相同）、分页 Canonical 指向正确、第2+页误设 noindex 检测、分页参数是否在 Sitemap 暴露（爬虫陷阱）、分页面包屑层级</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🗂️</div>
          <div class="cap-item-content">
            <div class="cap-item-title">多域名与权重分散</div>
            <div class="cap-item-desc">全站 HTML 中提取所有自有域名、镜像站 Canonical 是否指向主域、镜像站是否有独立 Sitemap（不应有）、www/non-www 权重分散、子域名策略评估</div>
          </div>
        </div>

        <div class="cap-item">
          <div class="cap-item-icon">🌟</div>
          <div class="cap-item-content">
            <div class="cap-item-title">社交媒体集成</div>
            <div class="cap-item-desc">og:image 尺寸规范（1200×630）、Twitter Card 类型配置（summary_large_image）、Facebook Pixel / GTM 追踪代码、分享按钮是否对页面速度造成影响、微信/微博分享 meta 配置</div>
          </div>
        </div>

      </div>

      <!-- 深度SEO建议模块说明 -->
      <div class="cap-section-title" style="margin-top:6px">🎯 深度白帽 SEO 优化建议（每次巡检独家输出）</div>
      <div class="seo-advice-grid">
        <div class="advice-item">
          <div class="advice-icon">📈</div>
          <div class="advice-content">
            <div class="advice-title">关键词策略建议</div>
            <div class="advice-desc">基于页面现有内容，给出目标关键词布局方案、长尾词挖掘方向、关键词密度调整建议，让内容团队有明确的优化方向</div>
          </div>
        </div>
        <div class="advice-item">
          <div class="advice-icon">🏗️</div>
          <div class="advice-content">
            <div class="advice-title">内链建设方案</div>
            <div class="advice-desc">识别权重高的页面与孤立页面，给出内链传递路径建议，帮助提升重要落地页的页面权重（PageRank 流动）</div>
          </div>
        </div>
        <div class="advice-item">
          <div class="advice-icon">📝</div>
          <div class="advice-content">
            <div class="advice-title">内容优化执行清单</div>
            <div class="advice-desc">基于源码分析，给出每类页面的内容充实建议，包括字数要求、段落结构、多媒体配比、内容独特性提升方向</div>
          </div>
        </div>
        <div class="advice-item">
          <div class="advice-icon">🔧</div>
          <div class="advice-content">
            <div class="advice-title">技术修复优先级排序</div>
            <div class="advice-desc">将所有发现的问题按"影响权重 × 修复难度"矩阵排序，给出先修什么、后修什么的明确执行计划，避免资源浪费</div>
          </div>
        </div>
        <div class="advice-item">
          <div class="advice-icon">📊</div>
          <div class="advice-content">
            <div class="advice-title">竞争对手差距分析</div>
            <div class="advice-desc">根据站点类型对比行业最佳实践，指出与头部同类站点的差距，提供可落地的追赶方案</div>
          </div>
        </div>
        <div class="advice-item">
          <div class="advice-icon">🎓</div>
          <div class="advice-content">
            <div class="advice-title">运营人员可执行手册</div>
            <div class="advice-desc">针对非技术运营人员，用白话解释每个问题为什么影响排名，给出无需开发介入就能执行的优化动作（如修改 Title、补充描述、添加 alt 等）</div>
          </div>
        </div>
      </div>

      <div class="cap-footer">
        <span class="cap-tag">📋 每次输出 35–70 个独立问题</span>
        <span class="cap-tag">📄 自动生成 Word 审计报告</span>
        <span class="cap-tag">🎯 每个问题附真实源码证据</span>
        <span class="cap-tag">✅ 每个修复含可执行 HTML 代码</span>
        <span class="cap-tag">⚡ 实时抓取页面源码分析</span>
        <span class="cap-tag">📡 融合 GSC / GA / Yandex 数据</span>
        <span class="cap-tag">🤖 AI 大模型深度理解语义</span>
        <span class="cap-tag">🎓 白帽 SEO 白话建议</span>
      </div>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card panel">
        <div class="stat-emoji">🌐</div>
        <div class="stat-info">
          <div class="stat-value">{{ sites.length }}</div>
          <div class="stat-label">站点总数</div>
        </div>
      </div>
      <div class="stat-card panel">
        <div class="stat-emoji">✅</div>
        <div class="stat-info">
          <div class="stat-value">{{ activeSites }}</div>
          <div class="stat-label">启用中</div>
        </div>
      </div>
      <div class="stat-card panel">
        <div class="stat-emoji">🔍</div>
        <div class="stat-info">
          <div class="stat-value">{{ auditedSites }}</div>
          <div class="stat-label">已审计</div>
        </div>
      </div>
    </div>

    <!-- 站点列表 -->
    <div class="panel table-panel">
      <el-table :data="sites" v-loading="loading" class="sunrise-table" :header-cell-style="headerCellStyle" :row-class-name="tableRowClassName">
        <el-table-column prop="name" label="站点名称" min-width="160">
          <template #default="{ row }">
            <div class="site-name-cell">
              <span class="site-icon">🌐</span>
              <span class="site-name-text">{{ row.name || row.domain }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="domain" label="域名" min-width="180">
          <template #default="{ row }"><span class="domain-text">{{ row.domain }}</span></template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <span class="status-badge" :class="'status-' + row.status">
              {{ row.status === 'active' ? '🟢 启用' : '⚪ 停用' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="last_audit_status" label="最近巡检" width="120">
          <template #default="{ row }">
            <span v-if="row.last_audit_status" class="audit-badge" :class="'audit-' + row.last_audit_status">{{ auditLabel(row.last_audit_status) }}</span>
            <span v-else class="no-audit">暂未巡检</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="添加时间" width="160">
          <template #default="{ row }"><span class="date-text">{{ row.created_at }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="380" fixed="right">
          <template #default="{ row }">
            <div class="action-btns">
              <el-button class="btn-action btn-audit" size="small" @click="triggerAudit(row)">🔍 立即巡检</el-button>
              <el-button class="btn-action btn-detail" size="small" @click="$router.push(`/sites/${row.id}`)">📋 详情</el-button>
              <el-button class="btn-action btn-data" size="small" @click="$router.push(`/sites/${row.id}/data`)">📊 数据</el-button>
              <el-button class="btn-action btn-delete" size="small" @click="deleteSite(row)">🗑️ 删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 添加/编辑弹窗 -->
    <el-dialog v-model="showDialog" :title="editId ? '✏️ 编辑站点' : '🌐 添加站点'" width="480px" class="sunrise-dialog" :close-on-click-modal="false">
      <el-form :model="form" label-width="100px" class="sunrise-form">
        <div class="form-section-title">基本信息</div>
        <el-form-item label="域名">
          <el-input v-model="form.domain" placeholder="https://example.com（必须包含 https:// 或 http://）" />
        </el-form-item>
        <el-form-item label="站点名称">
          <el-input v-model="form.name" placeholder="我的网站" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button class="btn-cancel" @click="showDialog = false">取 消</el-button>
          <el-button class="btn-sunrise" @click="saveSite">{{ editId ? '保存修改' : '确认添加' }}</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const sites = ref([])
const loading = ref(false)
const showDialog = ref(false)
const editId = ref(null)
const form = ref({ domain: '', name: '', gsc_site_url: '', ga_property_id: '', yandex_host_id: '', url_category: '', url_detail: '', url_news_list: '', url_news_detail: '', url_search: '', url_static: '' })

const activeSites = computed(() => sites.value.filter(s => s.status === 'active').length)
const auditedSites = computed(() => sites.value.filter(s => s.last_audit_status === 'done').length)

const headerCellStyle = () => ({ background: 'linear-gradient(135deg, var(--sun), var(--coral))', color: '#FFFFFF', fontWeight: '800', fontSize: '12px', letterSpacing: '0.04em', border: 'none', padding: '14px 12px' })
function tableRowClassName({ rowIndex }) { return rowIndex % 2 === 0 ? 'row-even' : 'row-odd' }
function auditLabel(status) {
  if (status === 'done') return '✅ 已完成'
  if (status === 'running') return '⏳ 进行中'
  if (status === 'pending') return '🕐 等待中'
  if (status === 'error') return '❌ 失败'
  return status
}
function openAddDialog() {
  editId.value = null
  form.value = { domain: '', name: '', gsc_site_url: '', ga_property_id: '', yandex_host_id: '', url_category: '', url_detail: '', url_news_list: '', url_news_detail: '', url_search: '', url_static: '' }
  showDialog.value = true
}
async function loadSites() { loading.value = true; try { const res = await api.listSites(); sites.value = res.data } catch {} finally { loading.value = false } }
async function saveSite() {
  if (!form.value.domain) { ElMessage.warning('请填写域名'); return }
  if (!form.value.domain.startsWith('http://') && !form.value.domain.startsWith('https://')) { ElMessage.warning('域名必须以 https:// 或 http:// 开头'); return }
  if (!form.value.name) { ElMessage.warning('请填写站点名称'); return }
  try {
    if (editId.value) { await api.updateSite(editId.value, form.value); ElMessage.success('站点信息已更新') }
    else { await api.createSite(form.value); ElMessage.success('站点添加成功') }
    showDialog.value = false; loadSites()
  } catch {}
}
async function deleteSite(row) {
  try {
    await ElMessageBox.confirm(`确定要删除站点「${row.name || row.domain}」吗？删除后不可恢复。`, '确认删除', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
    await api.deleteSite(row.id); ElMessage.success('站点已删除'); loadSites()
  } catch {}
}
async function triggerAudit(row) {
  try { const res = await api.triggerAudit(row.id); ElMessage.success(`巡检任务已启动（任务ID：${res.data.task_id}）`); loadSites() } catch {}
}
onMounted(loadSites)
</script>

<style scoped>
.sites-page { padding: 0; }
.page-header { display:flex; justify-content:space-between; align-items:center; margin-bottom:20px; }
.header-left { display:flex; flex-direction:column; gap:4px; }
.page-title { margin:0; font-size:24px; font-weight:800; color:var(--ink); letter-spacing:-0.02em; }
.page-subtitle { font-size:13px; color:var(--ink-soft); font-weight:500; }

/* ── 能力卡片 ── */
.capability-card { background:linear-gradient(135deg,#FFFDF8 0%,#FFF8EC 50%,#FFF2D8 100%); border:1.5px solid var(--peach); border-radius:var(--r); box-shadow:var(--s2); padding:22px 26px; margin-bottom:24px; }
.cap-header { display:flex; align-items:flex-start; gap:14px; margin-bottom:20px; padding-bottom:16px; border-bottom:1.5px solid var(--peach); }
.cap-icon { font-size:36px; flex-shrink:0; }
.cap-title { font-size:17px; font-weight:800; color:var(--ink); letter-spacing:-.3px; margin-bottom:4px; }
.cap-subtitle { font-size:13px; color:var(--ink-soft); line-height:1.7; max-width:900px; }
.cap-section-title { font-size:14px; font-weight:800; color:var(--sun-deep); margin:0 0 14px; display:flex; align-items:center; gap:6px; }
.cap-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:10px; margin-bottom:22px; }
.cap-item { display:flex; align-items:flex-start; gap:10px; background:rgba(255,255,255,.75); border-radius:var(--r-sm); padding:12px 14px; border:1px solid rgba(255,200,120,.4); transition:all .18s; }
.cap-item:hover { background:#fff; box-shadow:var(--s1); transform:translateY(-1px); }
.cap-item-icon { font-size:20px; flex-shrink:0; margin-top:1px; }
.cap-item-title { font-size:13px; font-weight:700; color:var(--ink); margin-bottom:4px; }
.cap-item-desc { font-size:11.5px; color:var(--ink-soft); line-height:1.6; }

/* ── 深度SEO建议 ── */
.seo-advice-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:10px; margin-bottom:18px; }
.advice-item { display:flex; align-items:flex-start; gap:10px; background:linear-gradient(135deg,rgba(37,99,235,.06),rgba(59,130,246,.04)); border-radius:var(--r-sm); padding:12px 14px; border:1px solid rgba(37,99,235,.15); }
.advice-icon { font-size:22px; flex-shrink:0; }
.advice-title { font-size:13px; font-weight:700; color:#1D4ED8; margin-bottom:4px; }
.advice-desc { font-size:11.5px; color:var(--ink-soft); line-height:1.6; }

.cap-footer { display:flex; flex-wrap:wrap; gap:8px; padding-top:14px; border-top:1.5px solid var(--peach); }
.cap-tag { display:inline-flex; align-items:center; gap:5px; padding:5px 12px; background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-radius:20px; font-size:12px; font-weight:700; box-shadow:0 2px 8px rgba(249,115,22,.25); }

.stats-row { display:grid; grid-template-columns:repeat(3,1fr); gap:18px; margin-bottom:24px; }
.stat-card { display:flex; align-items:center; gap:16px; padding:20px 24px; }
.stat-emoji { font-size:32px; line-height:1; }
.stat-info { display:flex; flex-direction:column; }
.stat-value { font-size:26px; font-weight:800; color:var(--ink); }
.stat-label { font-size:12px; font-weight:600; letter-spacing:0.04em; color:var(--ink-soft); margin-top:2px; }

.panel { background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r); box-shadow:var(--s1); transition:box-shadow .2s; }
.panel:hover { box-shadow:var(--s2); }
.table-panel { overflow:hidden; padding:0; }

.sunrise-table { --el-table-border-color:var(--border-light); --el-table-row-hover-bg-color:var(--sun-glow); --el-table-bg-color:transparent; width:100%; }
:deep(.sunrise-table td.el-table__cell) { padding:13px 12px; border-bottom:1px solid var(--border-light); font-size:13.5px; }
:deep(.sunrise-table .row-even td) { background:var(--surface); }
:deep(.sunrise-table .row-odd td) { background:var(--surface-2); }
:deep(.sunrise-table .el-table__body tr:hover > td) { background:var(--sun-glow) !important; }

.site-name-cell { display:flex; align-items:center; gap:10px; }
.site-icon { font-size:18px; }
.site-name-text { font-weight:700; color:var(--ink); }
.domain-text { color:var(--ink-mid); font-weight:500; font-size:13px; }
.date-text { color:var(--ink-soft); font-size:12.5px; }

.status-badge { display:inline-flex; align-items:center; gap:4px; padding:4px 12px; border-radius:20px; font-size:12.5px; font-weight:700; }
.status-active { background:#E8F5E9; color:var(--ok); }
.status-inactive { background:var(--surface-3); color:var(--ink-hint); }

.audit-badge { display:inline-flex; align-items:center; gap:4px; padding:4px 10px; border-radius:20px; font-size:12px; font-weight:700; }
.audit-done { background:#E8F5E9; color:var(--ok); }
.audit-running { background:#FFF8E1; color:var(--warn); }
.audit-error { background:#FFEBEE; color:var(--err); }
.no-audit { color:var(--ink-hint); font-size:13px; }

.action-btns { display:flex; gap:6px; align-items:center; }
.btn-action { border-radius:var(--r-xs) !important; font-weight:700 !important; font-size:12px !important; padding:5px 10px !important; border:1.5px solid var(--border-light) !important; background:var(--surface) !important; color:var(--ink-mid) !important; transition:all .2s !important; }
.btn-action:hover { border-color:var(--sun) !important; color:var(--sun-deep) !important; background:var(--sun-light) !important; }
.btn-audit { background:var(--sun-light) !important; border-color:var(--peach) !important; color:var(--sun-deep) !important; }
.btn-audit:hover { background:linear-gradient(135deg,var(--sun),var(--coral)) !important; color:#fff !important; border-color:var(--sun) !important; }
.btn-delete { border-color:#FFCDD2 !important; color:var(--err) !important; background:#FFF5F5 !important; }
.btn-delete:hover { background:var(--err) !important; color:#fff !important; border-color:var(--err) !important; }

.btn-sunrise { background:linear-gradient(135deg,var(--sun),var(--coral)) !important; border:none !important; color:#fff !important; font-weight:700 !important; font-size:14px !important; padding:10px 24px !important; border-radius:var(--r-sm) !important; box-shadow:var(--s-amber) !important; transition:all .2s !important; display:inline-flex; align-items:center; gap:6px; }
.btn-sunrise:hover { filter:brightness(1.06); transform:translateY(-1px); box-shadow:0 6px 20px rgba(255,140,0,.35) !important; }
.btn-icon { font-size:18px; font-weight:800; line-height:1; }
.btn-cancel { border:1.5px solid var(--border) !important; background:var(--surface) !important; color:var(--ink-mid) !important; font-weight:600 !important; border-radius:var(--r-sm) !important; padding:10px 24px !important; }

:deep(.sunrise-dialog .el-dialog) { border-radius:var(--r) !important; overflow:hidden; box-shadow:0 20px 60px rgba(45,27,14,.12); }
:deep(.sunrise-dialog .el-dialog__header) { background:linear-gradient(135deg,var(--sun),var(--coral)); padding:20px 24px; margin:0; }
:deep(.sunrise-dialog .el-dialog__title) { color:#fff; font-weight:800; font-size:17px; }
:deep(.sunrise-dialog .el-dialog__headerbtn .el-dialog__close) { color:#fff; font-size:18px; }
:deep(.sunrise-dialog .el-dialog__body) { padding:24px; background:var(--surface); }
:deep(.sunrise-dialog .el-dialog__footer) { padding:16px 24px; background:var(--surface-2); border-top:1px solid var(--border-light); }
.dialog-footer { display:flex; justify-content:flex-end; gap:10px; }
.form-section-title { font-size:12px; font-weight:800; letter-spacing:0.06em; color:var(--sun-deep); margin:0 0 14px; padding-bottom:8px; border-bottom:2px solid var(--sun-glow); }
:deep(.sunrise-form .el-form-item__label) { font-weight:600; color:var(--ink-mid); font-size:13px; }
:deep(.sunrise-form .el-input__wrapper) { border-radius:var(--r-xs); border:1.5px solid var(--border); box-shadow:none !important; padding:4px 12px; transition:border-color .2s; }
:deep(.sunrise-form .el-input__wrapper:hover) { border-color:var(--peach); }
:deep(.sunrise-form .el-input__wrapper.is-focus) { border-color:var(--sun) !important; box-shadow:0 0 0 3px rgba(255,140,0,.12) !important; }
</style>
