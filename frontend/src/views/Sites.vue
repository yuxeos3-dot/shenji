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

      <div class="cap-toggle" @click="capExpanded=!capExpanded">
        <span>📋 检查覆盖范围（28大类 200+ 检查点）</span>
        <span class="cap-toggle-icon">{{ capExpanded ? '▲ 收起' : '▼ 展开查看' }}</span>
      </div>
      <div v-show="capExpanded" class="cap-text">
        每次巡检自动触发以下全部检查，覆盖 28 大类 200+ 检查点：<br><br>

        <strong>SEO 基础</strong>：TDK 查询（Title/Description/Keywords 长度规范、关键词覆盖、跨页面重复）、H 标签查询（H1 唯一性/隐藏检测/H2-H6 层级跳跃）、图片 SEO 查询（alt 完整性与描述性、首屏禁 lazy-load、width/height 防 CLS）、链接与内链查询（孤立页面、锚文本描述性、内链数量）、结构化数据查询（VideoObject/Article/BreadcrumbList/AggregateRating 字段完整性）、Canonical 查询（自引用/重复内容处理）、Open Graph 查询（og:title/og:description/og:image 完整性）。<br><br>

        <strong>技术 SEO</strong>：robots.txt 查询（重要路径误屏蔽、应屏蔽路径未屏蔽）、Sitemap 查询（格式/可访问性/lastmod 真实性/爬虫陷阱参数 URL）、重定向规范化查询（HTTP→HTTPS/www统一/多跳重定向链检测）、爬虫陷阱查询（分面导航无限 URL、Session ID 进 URL、软 404）、移动端适配查询（viewport/字体大小/点击目标尺寸）、分页 SEO 查询（Title 差异化/Canonical/误设 noindex）、JavaScript SEO 查询（纯 CSR SPA 致命检测/模板语法暴露/noscript 降级）、多域名权重查询（镜像站 Canonical/子域名权重分散）。<br><br>

        <strong>性能与速度</strong>：Core Web Vitals 查询（LCP 元素 fetchpriority/首屏图片 preload/render-blocking 资源识别）、加载速度查询（同步 JS 阻塞渲染/第三方脚本数量统计/font-display 策略/preconnect 配置）、资源压缩查询（gzip/brotli 压缩/Cache-Control/ETag/CDN 命中率）。<br><br>

        <strong>安全</strong>：安全响应头查询（HSTS/CSP/X-Frame-Options/X-Content-Type-Options/Referrer-Policy/Permissions-Policy）、服务器信息泄露查询（Server 版本号/X-Powered-By/X-Generator 暴露）、HTTPS 合规查询（混合内容/Cookie Secure+HttpOnly+SameSite 属性/CORS 配置）、敏感信息查询（API 密钥硬编码/敏感路径暴露/注释泄露）。<br><br>

        <strong>代码质量</strong>：代码规范查询（HTML 嵌套合法性/重复 ID/内联样式过度/调试代码残留/TODO-FIXME 注释暴露）、隐藏文本查询（position:-9999px/display:none/同色文字/font-size:0 — Google 手动惩罚项）、关键词堆砌查询（keywords 过长/正文异常密度/隐藏关键词列表）。<br><br>

        <strong>内容质量</strong>：内容充实度查询（首页 >200 字/详情页正文丰富度/模板重复比例）、E-E-A-T 查询（作者信息/关于我们/DMCA/Organization Schema）、国际化查询（hreflang/HTML lang/UTF-8）、RSS Feed 查询（可访问性/域名一致性/更新时效）、无障碍访问查询（form label/aria-label/语义化标签）、视频影视专项查询（VideoObject 必填字段/视频 Sitemap/TVSeries 层级）、社交媒体查询（og:image 比例/Twitter Card/GTM 追踪）。<br><br>

        <strong>深度白帽 SEO 建议</strong>：每次巡检额外输出独立一章可落地的 SEO 优化路线图 —— 关键词策略布局、内链建设方案（PageRank 流动路径）、内容优化清单（字数/结构/多媒体配比）、3 个月技术修复优先级计划、运营人员无需开发介入可立即执行的 10 条动作、与行业头部站点的关键差距分析。让运营人员不但知道有问题，更知道怎么改、改哪里最有效。
      </div>

      <div class="cap-footer">
        <span class="cap-tag">📋 每次输出 40–70 个独立问题</span>
        <span class="cap-tag">📄 自动生成 Word 审计报告</span>
        <span class="cap-tag">🎯 每个问题附真实源码证据</span>
        <span class="cap-tag">✅ 每个修复含可执行 HTML 代码</span>
        <span class="cap-tag">⚡ 实时抓取页面源码分析</span>
        <span class="cap-tag">📡 融合 GSC / GA / Yandex 数据</span>
        <span class="cap-tag">🤖 AI 大模型深度语义分析</span>
        <span class="cap-tag">🎓 白帽 SEO 运营白话建议</span>
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
const capExpanded = ref(false)
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
.cap-toggle { display:flex; justify-content:space-between; align-items:center; cursor:pointer; padding:8px 12px; border-radius:var(--r-sm); background:rgba(255,255,255,.6); border:1px solid var(--peach); margin-bottom:10px; font-size:13px; font-weight:700; color:var(--sun-deep); user-select:none; }
.cap-toggle:hover { background:#fff; }
.cap-toggle-icon { font-size:12px; color:var(--ink-soft); }
.cap-section-title { font-size:14px; font-weight:800; color:var(--sun-deep); margin:0 0 12px; }
.cap-text { font-size:13.5px; color:var(--ink-mid); line-height:1.85; margin-bottom:18px; }
.cap-text strong { color:var(--ink); font-weight:700; }

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
