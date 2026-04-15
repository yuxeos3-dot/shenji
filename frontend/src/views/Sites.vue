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
          <div class="cap-title">SEO 深度审计系统 · 工作流 v6</div>
          <div class="cap-subtitle">基于源码级分析，覆盖 24 大类 180+ 检查点，媲美专业 SEO 机构的全自动审计引擎</div>
        </div>
      </div>

      <div class="cap-grid">
        <div class="cap-item">
          <div class="cap-item-icon">🔍</div>
          <div class="cap-item-content">
            <div class="cap-item-title">TDK 与基础 Meta</div>
            <div class="cap-item-desc">检查 Title/Description/Keywords 长度规范、关键词覆盖、跨页面重复、Open Graph、Twitter Card、Canonical 自引用等 20+ 细项</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">📐</div>
          <div class="cap-item-content">
            <div class="cap-item-title">H 标签结构</div>
            <div class="cap-item-desc">H1 唯一性、CSS 隐藏检测、H2-H6 层次跳级、关键词覆盖、各页面类型 H1 模板一致性分析</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">🖼️</div>
          <div class="cap-item-content">
            <div class="cap-item-title">图片 SEO</div>
            <div class="cap-item-desc">alt 属性完整性、无意义 alt 检测、首屏图片禁 lazy-load、width/height 防 CLS、WebP/AVIF 格式检测</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">🔗</div>
          <div class="cap-item-content">
            <div class="cap-item-title">链接与内链架构</div>
            <div class="cap-item-desc">内链数量统计、孤立页面发现、nofollow 合理性、锚文本描述性检测、面包屑导航完整性、分页 rel=next/prev</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">📊</div>
          <div class="cap-item-content">
            <div class="cap-item-title">结构化数据 Schema</div>
            <div class="cap-item-desc">VideoObject / Movie / TVSeries / Article / BreadcrumbList / WebSite SearchAction / AggregateRating / Organization 完整性验证</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">🤖</div>
          <div class="cap-item-content">
            <div class="cap-item-title">robots.txt & Sitemap</div>
            <div class="cap-item-desc">重要路径误屏蔽检测、Sitemap 可访问性、lastmod 真实性验证、子 Sitemap 格式、视频/新闻专项 Sitemap 检测</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">⚡</div>
          <div class="cap-item-content">
            <div class="cap-item-title">Core Web Vitals HTML 信号</div>
            <div class="cap-item-desc">LCP 候选元素 fetchpriority="high"、首屏图片 preload、render-blocking 资源、font-display、关键 CSS 内联</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">🔒</div>
          <div class="cap-item-content">
            <div class="cap-item-title">安全响应头</div>
            <div class="cap-item-desc">HSTS / CSP / X-Frame-Options / X-Content-Type-Options / Server 版本泄露 / X-Powered-By 暴露 / 混合内容检测</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">📱</div>
          <div class="cap-item-content">
            <div class="cap-item-title">移动端适配</div>
            <div class="cap-item-desc">viewport meta、字体大小适配、水平滚动触发元素、点击目标尺寸（≥48px）、PC 与移动端响应一致性</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">🧩</div>
          <div class="cap-item-content">
            <div class="cap-item-title">JavaScript SEO</div>
            <div class="cap-item-desc">纯 CSR SPA 检测（致命）、Vue/React 模板语法暴露、noscript 降级、重要内容 JS 依赖、JSON-LD 动态注入风险</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">↩️</div>
          <div class="cap-item-content">
            <div class="cap-item-title">重定向规范化</div>
            <div class="cap-item-desc">HTTP→HTTPS 跳转、www/non-www 统一、多跳重定向链（301→302→200）、分页第1页规范化、URL 大小写统一</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">🕸️</div>
          <div class="cap-item-content">
            <div class="cap-item-title">爬虫陷阱检测</div>
            <div class="cap-item-desc">分面导航无限 URL、Session ID 出现在 URL、日历归档无限翻页、Sitemap 含参数 URL、软 404 检测</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">🏆</div>
          <div class="cap-item-content">
            <div class="cap-item-title">E-E-A-T 权威性信号</div>
            <div class="cap-item-desc">作者页面/Schema Author 字段、关于我们/联系方式页面、DMCA 声明、外链权威来源、组织信息完整性</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">💾</div>
          <div class="cap-item-content">
            <div class="cap-item-title">资源压缩与缓存</div>
            <div class="cap-item-desc">gzip/br 压缩检测、静态资源 Cache-Control、图片 WebP/AVIF 格式、CSS/JS 压缩状态、第三方脚本负担</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">🎬</div>
          <div class="cap-item-content">
            <div class="cap-item-title">视频/影视站专项</div>
            <div class="cap-item-desc">VideoObject Schema 完整性、视频标题关键词、缩略图声明、视频 Sitemap 存在性、播出平台信息</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">🔄</div>
          <div class="cap-item-content">
            <div class="cap-item-title">重复内容处理</div>
            <div class="cap-item-desc">多 URL 同内容检测、Canonical 正确性、标签页/分类页内容高度重叠、打印版/移动版 Canonical 处理</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">🌐</div>
          <div class="cap-item-content">
            <div class="cap-item-title">国际化 & 多语言</div>
            <div class="cap-item-desc">hreflang 配置正确性、HTML lang 属性、UTF-8 字符集、Canonical 与 hreflang 关系一致性</div>
          </div>
        </div>
        <div class="cap-item">
          <div class="cap-item-icon">👻</div>
          <div class="cap-item-content">
            <div class="cap-item-title">隐藏文本检测</div>
            <div class="cap-item-desc">position:absolute;left:-9999px 关键词堆砌、display:none 包含关键词、同色文字、font-size:0 文字 — Google 惩罚项</div>
          </div>
        </div>
      </div>

      <div class="cap-footer">
        <span class="cap-tag">📋 每次审计输出 35-70+ 独立问题</span>
        <span class="cap-tag">📄 自动生成 Word 审计报告</span>
        <span class="cap-tag">🎯 每个问题附带真实源码证据</span>
        <span class="cap-tag">✅ 每个问题给出可执行 HTML 修复代码</span>
        <span class="cap-tag">⚡ 支持一键实时抓取页面源码</span>
        <span class="cap-tag">📡 对接 GSC / GA / Yandex 数据</span>
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
      <el-table
        :data="sites"
        v-loading="loading"
        class="sunrise-table"
        :header-cell-style="headerCellStyle"
        :row-class-name="tableRowClassName"
      >
        <el-table-column prop="name" label="站点名称" min-width="160">
          <template #default="{ row }">
            <div class="site-name-cell">
              <span class="site-icon">🌐</span>
              <span class="site-name-text">{{ row.name || row.domain }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="domain" label="域名" min-width="180">
          <template #default="{ row }">
            <span class="domain-text">{{ row.domain }}</span>
          </template>
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
            <span v-if="row.last_audit_status" class="audit-badge" :class="'audit-' + row.last_audit_status">
              {{ auditLabel(row.last_audit_status) }}
            </span>
            <span v-else class="no-audit">暂未巡检</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="添加时间" width="160">
          <template #default="{ row }">
            <span class="date-text">{{ row.created_at }}</span>
          </template>
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
    <el-dialog
      v-model="showDialog"
      :title="editId ? '✏️ 编辑站点' : '🌐 添加站点'"
      width="580px"
      class="sunrise-dialog"
      :close-on-click-modal="false"
    >
      <el-form :model="form" label-width="110px" class="sunrise-form">
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
          <el-button class="btn-sunrise" @click="saveSite">
            {{ editId ? '保存修改' : '确认添加' }}
          </el-button>
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
const form = ref({
  domain: '', name: '', gsc_site_url: '', ga_property_id: '',
  yandex_host_id: '', url_category: '', url_detail: '',
  url_news_list: '', url_news_detail: '', url_search: '', url_static: ''
})

const activeSites = computed(() => sites.value.filter(s => s.status === 'active').length)
const auditedSites = computed(() => sites.value.filter(s => s.last_audit_status === 'done').length)

const headerCellStyle = () => ({
  background: 'linear-gradient(135deg, var(--sun), var(--coral))',
  color: '#FFFFFF',
  fontWeight: '800',
  fontSize: '12px',
  letterSpacing: '0.04em',
  border: 'none',
  padding: '14px 12px',
})

function tableRowClassName({ rowIndex }) {
  return rowIndex % 2 === 0 ? 'row-even' : 'row-odd'
}

function auditLabel(status) {
  if (status === 'done') return '✅ 已完成'
  if (status === 'running') return '⏳ 进行中'
  if (status === 'pending') return '🕐 等待中'
  if (status === 'error') return '❌ 失败'
  return status
}

function openAddDialog() {
  editId.value = null
  form.value = {
    domain: '', name: '', gsc_site_url: '', ga_property_id: '',
    yandex_host_id: '', url_category: '', url_detail: '',
    url_news_list: '', url_news_detail: '', url_search: '', url_static: ''
  }
  showDialog.value = true
}

async function loadSites() {
  loading.value = true
  try {
    const res = await api.listSites()
    sites.value = res.data
  } catch {}
  finally { loading.value = false }
}

async function saveSite() {
  if (!form.value.domain) { ElMessage.warning('请填写域名'); return }
  if (!form.value.domain.startsWith('http://') && !form.value.domain.startsWith('https://')) {
    ElMessage.warning('域名必须以 https:// 或 http:// 开头'); return
  }
  if (!form.value.name) { ElMessage.warning('请填写站点名称'); return }
  try {
    if (editId.value) {
      await api.updateSite(editId.value, form.value)
      ElMessage.success('站点信息已更新')
    } else {
      await api.createSite(form.value)
      ElMessage.success('站点添加成功')
    }
    showDialog.value = false
    loadSites()
  } catch {}
}

async function deleteSite(row) {
  try {
    await ElMessageBox.confirm(
      `确定要删除站点「${row.name || row.domain}」吗？删除后不可恢复。`,
      '确认删除',
      { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' }
    )
    await api.deleteSite(row.id)
    ElMessage.success('站点已删除')
    loadSites()
  } catch {}
}

async function triggerAudit(row) {
  try {
    const res = await api.triggerAudit(row.id)
    ElMessage.success(`巡检任务已启动（任务ID：${res.data.task_id}）`)
    loadSites()
  } catch {}
}

onMounted(loadSites)
</script>

<style scoped>
.sites-page { padding: 0; }

.page-header { display:flex; justify-content:space-between; align-items:center; margin-bottom:20px; }

/* ── 系统能力介绍卡片 ── */
.capability-card {
  background: linear-gradient(135deg, #FFFDF8 0%, #FFF8EC 60%, #FFF2D8 100%);
  border: 1.5px solid var(--peach);
  border-radius: var(--r);
  box-shadow: var(--s2);
  padding: 22px 26px;
  margin-bottom: 24px;
}
.cap-header { display:flex; align-items:flex-start; gap:14px; margin-bottom:20px; padding-bottom:16px; border-bottom:1.5px solid var(--peach); }
.cap-icon { font-size:36px; flex-shrink:0; }
.cap-title { font-size:17px; font-weight:800; color:var(--ink); letter-spacing:-.3px; margin-bottom:4px; }
.cap-subtitle { font-size:13px; color:var(--ink-soft); line-height:1.6; }

.cap-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:12px; margin-bottom:18px; }
@media(max-width:1100px) { .cap-grid { grid-template-columns:repeat(2,1fr); } }

.cap-item { display:flex; align-items:flex-start; gap:10px; background:rgba(255,255,255,.7); border-radius:var(--r-sm); padding:12px 14px; border:1px solid var(--border-light); }
.cap-item:hover { background:#fff; box-shadow:var(--s1); }
.cap-item-icon { font-size:20px; flex-shrink:0; margin-top:1px; }
.cap-item-title { font-size:13px; font-weight:700; color:var(--ink); margin-bottom:3px; }
.cap-item-desc { font-size:11.5px; color:var(--ink-soft); line-height:1.55; }

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

.sunrise-table {
  --el-table-border-color: var(--border-light);
  --el-table-row-hover-bg-color: var(--sun-glow);
  --el-table-bg-color: transparent;
  width:100%;
}
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
.btn-cancel:hover { border-color:var(--ink-hint) !important; color:var(--ink) !important; }

:deep(.sunrise-dialog .el-dialog) { border-radius:var(--r) !important; overflow:hidden; box-shadow:0 20px 60px rgba(45,27,14,.12); }
:deep(.sunrise-dialog .el-dialog__header) { background:linear-gradient(135deg,var(--sun),var(--coral)); padding:20px 24px; margin:0; }
:deep(.sunrise-dialog .el-dialog__title) { color:#fff; font-weight:800; font-size:17px; }
:deep(.sunrise-dialog .el-dialog__headerbtn .el-dialog__close) { color:#fff; font-size:18px; }
:deep(.sunrise-dialog .el-dialog__body) { padding:24px; background:var(--surface); }
:deep(.sunrise-dialog .el-dialog__footer) { padding:16px 24px; background:var(--surface-2); border-top:1px solid var(--border-light); }

.dialog-footer { display:flex; justify-content:flex-end; gap:10px; }

.form-section-title { font-size:12px; font-weight:800; letter-spacing:0.06em; color:var(--sun-deep); margin:20px 0 14px; padding-bottom:8px; border-bottom:2px solid var(--sun-glow); display:flex; align-items:center; gap:8px; }
.form-section-title:first-child { margin-top:0; }
.optional-tag { font-size:10px; font-weight:600; background:var(--sun-glow); color:var(--sun-deep); padding:2px 8px; border-radius:10px; }
.url-tip { font-size:12px; color:var(--ink-soft); margin:-8px 0 12px 0; line-height:1.5; }

:deep(.sunrise-form .el-form-item__label) { font-weight:600; color:var(--ink-mid); font-size:13px; }
:deep(.sunrise-form .el-input__wrapper) { border-radius:var(--r-xs); border:1.5px solid var(--border); box-shadow:none !important; padding:4px 12px; transition:border-color .2s; }
:deep(.sunrise-form .el-input__wrapper:hover) { border-color:var(--peach); }
:deep(.sunrise-form .el-input__wrapper.is-focus) { border-color:var(--sun) !important; box-shadow:0 0 0 3px rgba(255,140,0,.12) !important; }
:deep(.sunrise-form .el-input__inner) { font-size:13.5px; color:var(--ink); }
:deep(.sunrise-form .el-input__inner::placeholder) { color:var(--ink-hint); }
</style>
