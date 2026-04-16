<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push(`/sites/${$route.params.id}`)" class="back-btn">← 返回站点</a>
      <span class="back-sep">·</span>
      <span class="back-url">审计详情</span>
    </div>

    <!-- 任务状态卡片 -->
    <div v-if="task" class="panel panel-amber" style="margin-bottom:18px">
      <div class="task-header">
        <div class="info-grid-3">
          <div class="info-item">
            <span class="info-label">状态</span>
            <span class="badge" :class="task.status==='done'?'badge-ok':task.status==='running'?'badge-warn':'badge-err'">
              {{ task.status==='done'?'✅ 已完成':task.status==='running'?'⏳ 进行中':task.status==='pending'?'🕐 等待中':'❌ 失败' }}
            </span>
          </div>
          <div class="info-item">
            <span class="info-label">进度</span>
            <div style="display:flex;align-items:center;gap:8px">
              <div class="progress-bar" style="width:140px"><div class="progress-fill" :style="{width:task.progress+'%'}"></div></div>
              <span class="progress-text">{{ task.progress }}%</span>
            </div>
          </div>
          <div class="info-item">
            <span class="info-label">完成时间</span>
            <span style="font-size:13px;color:var(--ink-mid)">{{ task.finished_at || '—' }}</span>
          </div>
        </div>
        <button v-if="task.status==='done'" class="btn btn-download" @click="downloadReport" :disabled="downloading">
          {{ downloading ? '生成中...' : '📥 下载 Word 报告' }}
        </button>
      </div>

      <!-- 失败错误信息 -->
      <div v-if="task.status==='error'" class="error-bar">
        ❌ 审计失败原因：{{ task.error_msg || 'AI接口超时或配置错误，请检查系统设置中的AI接口配置，然后重新巡检' }}
      </div>

      <!-- 报告概览 -->
      <div v-if="v6Report" class="report-overview">
        <div class="overview-stat ov-fatal">
          <div class="ov-num">{{ v6Report.fatal }}</div>
          <div class="ov-label">致命</div>
        </div>
        <div class="overview-stat ov-serious">
          <div class="ov-num">{{ v6Report.serious }}</div>
          <div class="ov-label">严重</div>
        </div>
        <div class="overview-stat ov-medium">
          <div class="ov-num">{{ v6Report.medium }}</div>
          <div class="ov-label">中等</div>
        </div>
        <div class="overview-stat ov-low">
          <div class="ov-num">{{ v6Report.low }}</div>
          <div class="ov-label">低</div>
        </div>
        <div class="overview-stat ov-total">
          <div class="ov-num">{{ v6Report.total_issues }}</div>
          <div class="ov-label">总计</div>
        </div>
        <div class="ov-summary" v-if="v6Report.summary">
          <span class="ov-summary-label">总结：</span>{{ v6Report.summary }}
        </div>
      </div>

      <div v-if="task.status==='running'" class="running-hint">
        <span class="spin">⏳</span> 正在深度审计中，使用 SEO 深度审计工作流 v6，覆盖 28 大类 200+ 检查点，请耐心等待（约 2–5 分钟）…
      </div>
    </div>

    <!-- 问题列表（分页） -->
    <template v-if="allIssues.length">
      <!-- 筛选栏 -->
      <div class="filter-bar">
        <span class="filter-label">筛选严重级别：</span>
        <button :class="['filter-btn', filterSev===''&&'filter-active']" @click="filterSev='';page=1">全部 ({{ allIssues.length }})</button>
        <button :class="['filter-btn', 'sev-btn-fatal', filterSev==='致命'&&'filter-active']" @click="filterSev='致命';page=1">致命 ({{ countBySev('致命') }})</button>
        <button :class="['filter-btn', 'sev-btn-serious', filterSev==='严重'&&'filter-active']" @click="filterSev='严重';page=1">严重 ({{ countBySev('严重') }})</button>
        <button :class="['filter-btn', 'sev-btn-medium', filterSev==='中等'&&'filter-active']" @click="filterSev='中等';page=1">中等 ({{ countBySev('中等') }})</button>
        <button :class="['filter-btn', 'sev-btn-low', filterSev==='低'&&'filter-active']" @click="filterSev='低';page=1">低 ({{ countBySev('低') }})</button>
        <button v-if="task && task.status==='done'" class="btn-dl-inline" @click="downloadReport">📥 下载报告</button>
      </div>

      <!-- 问题卡片 -->
      <div v-for="issue in pagedIssues" :key="issue.id" :class="['issue-card', 'issue-'+severityClass(issue.severity)]">
        <div class="issue-header">
          <span :class="['sev-badge', 'sev-'+severityClass(issue.severity)]">{{ issue.severity }}</span>
          <span v-if="issue.module" class="mod-badge">{{ issue.module }}</span>
          <span class="issue-id">#{{ issue.id }}</span>
          <span class="issue-title">{{ issue.title }}</span>
          <span v-if="issue._chapter" class="chapter-tag">{{ issue._chapter }}</span>
        </div>
        <div v-if="issue.scope" class="issue-scope">📍 {{ issue.scope }}</div>
        <div v-if="issue.evidence_label" class="evidence-label">📄 {{ issue.evidence_label }}</div>
        <pre v-if="issue.evidence_code" class="evidence-code">{{ issue.evidence_code }}</pre>
        <div v-if="issue.current_status" class="issue-section">
          <div class="section-label current-label">⚠️ 目前现状</div>
          <div class="section-body">{{ issue.current_status }}</div>
        </div>
        <div v-if="issue.fix_suggestion" class="issue-section">
          <div class="section-label fix-label">✅ 修改建议</div>
          <div class="section-body fix-body">{{ issue.fix_suggestion }}</div>
        </div>
      </div>

      <!-- 分页 -->
      <div class="pagination-bar">
        <span class="page-info">共 {{ filteredIssues.length }} 个问题，第 {{ page }} / {{ totalPages }} 页</span>
        <div class="page-btns">
          <button class="page-btn" @click="page=1" :disabled="page===1">«</button>
          <button class="page-btn" @click="page--" :disabled="page===1">‹</button>
          <button
            v-for="p in pageRange" :key="p"
            :class="['page-btn', p===page&&'page-active']"
            @click="page=p"
          >{{ p }}</button>
          <button class="page-btn" @click="page++" :disabled="page===totalPages">›</button>
          <button class="page-btn" @click="page=totalPages" :disabled="page===totalPages">»</button>
        </div>
      </div>
    </template>

    <!-- 进行中占位 -->
    <div v-else-if="task && (task.status==='running'||task.status==='pending')" class="panel running-panel">
      <div class="running-icon">🔍</div>
      <div class="running-title">SEO 深度审计进行中</div>
      <div class="running-desc">正在按 v6 工作流逐项检查 28 大类 200+ 检查点</div>
      <div class="progress-bar large-progress">
        <div class="progress-fill" :style="{width:(task.progress||0)+'%'}"></div>
      </div>
      <div class="running-note">{{ task.progress }}%　完成后将自动显示完整报告</div>
    </div>

    <!-- 已完成但报告解析失败 -->
    <div v-else-if="result && task && task.status==='done' && !v6Report" class="panel">
      <div class="summary-bar">⚠️ 报告数据需要重新生成，请点击「立即巡检」重新审计以获取完整报告格式。</div>
      <div v-if="result.summary && !result.summary.startsWith('{')" class="suggestions-box">{{ result.summary }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'

const route = useRoute()
const task = ref(null)
const result = ref(null)
const v6Report = ref(null)
const filterSev = ref('')
const page = ref(1)
const PAGE_SIZE = 20
let pollTimer = null

const downloading = ref(false)

async function downloadReport() {
  downloading.value = true
  try {
    const token = localStorage.getItem('token')
    const resp = await fetch(`/api/v1/audits/${route.params.taskId}/report/download`, {
      headers: { Authorization: `Bearer ${token}` }
    })
    if (!resp.ok) { const d = await resp.json(); ElMessage.error(d.error || '下载失败'); return }
    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const el = document.createElement('a')
    el.href = url
    const disp = resp.headers.get('Content-Disposition') || ''
    const match = disp.match(/filename="(.+)"/)
    el.download = match ? match[1] : `SEO审计报告_${route.params.taskId}.docx`
    document.body.appendChild(el); el.click(); document.body.removeChild(el)
    URL.revokeObjectURL(url)
    ElMessage.success('报告下载成功')
  } catch (e) { ElMessage.error('下载失败：' + e.message) }
  finally { downloading.value = false }
}

function severityClass(sev) {
  if (sev === '致命') return 'fatal'
  if (sev === '严重') return 'serious'
  if (sev === '中等') return 'medium'
  return 'low'
}

// Flatten all issues from all chapters, attach chapter name
const allIssues = computed(() => {
  if (!v6Report.value?.chapters) return []
  const list = []
  for (const ch of v6Report.value.chapters) {
    for (const issue of ch.issues || []) {
      list.push({ ...issue, _chapter: ch.title })
    }
  }
  return list
})

const filteredIssues = computed(() => {
  if (!filterSev.value) return allIssues.value
  return allIssues.value.filter(i => i.severity === filterSev.value)
})

const totalPages = computed(() => Math.max(1, Math.ceil(filteredIssues.value.length / PAGE_SIZE)))

const pagedIssues = computed(() => {
  const start = (page.value - 1) * PAGE_SIZE
  return filteredIssues.value.slice(start, start + PAGE_SIZE)
})

const pageRange = computed(() => {
  const total = totalPages.value
  const cur = page.value
  const delta = 2
  const range = []
  for (let i = Math.max(1, cur - delta); i <= Math.min(total, cur + delta); i++) range.push(i)
  return range
})

function countBySev(sev) {
  return allIssues.value.filter(i => i.severity === sev).length
}

async function loadData() {
  try {
    const res = await api.getAudit(route.params.taskId)
    task.value = res.data.task
    result.value = res.data.result
    if (res.data.result?.v6_report) {
      try {
        let raw = res.data.result.v6_report
        // Strip markdown code blocks
        raw = raw.trim()
        if (raw.startsWith('```')) {
          const lines = raw.split('\n')
          raw = lines.slice(1).join('\n')
          const lastTick = raw.lastIndexOf('```')
          if (lastTick >= 0) raw = raw.substring(0, lastTick)
          raw = raw.trim()
        }
        // Find JSON object start
        const start = raw.indexOf('{')
        if (start >= 0) raw = raw.substring(start)
        const parsed = JSON.parse(raw)
        if (parsed.chapters && parsed.chapters.length > 0) {
          v6Report.value = parsed
        }
      } catch (e) {
        console.warn('v6_report parse error:', e)
      }
    }
    if (task.value.status === 'running' || task.value.status === 'pending') {
      pollTimer = setTimeout(loadData, 3000)
    }
  } catch {}
}

onMounted(loadData)
onUnmounted(() => { if (pollTimer) clearTimeout(pollTimer) })
</script>

<style scoped>
.back-row { display:flex; align-items:center; gap:8px; margin-bottom:18px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); }
.back-sep { color:var(--ink-hint); }
.back-url { font-size:14px; font-weight:700; color:var(--ink); }

.error-bar { background:var(--err-bg); border:1.5px solid var(--err-border); border-radius:var(--r-sm); padding:12px 16px; font-size:13.5px; color:var(--err); font-weight:600; margin-top:12px; line-height:1.6; }
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel-amber { background:linear-gradient(135deg,#FFFDF8 0%,#FFF8EC 60%,#FFF2D8 100%); border-color:var(--peach); }

.task-header { display:flex; align-items:flex-start; justify-content:space-between; margin-bottom:16px; flex-wrap:wrap; gap:12px; }
.info-grid-3 { display:grid; grid-template-columns:repeat(3,1fr); gap:14px; flex:1; }
.info-item { display:flex; flex-direction:column; gap:4px; }
.info-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.06em; }

.btn-download { display:inline-flex; align-items:center; gap:7px; padding:10px 20px; background:linear-gradient(135deg,#2563EB,#3B82F6); color:#fff; border-radius:var(--r-sm); font-weight:700; font-size:13.5px; text-decoration:none; box-shadow:0 4px 14px rgba(37,99,235,.3); transition:all .18s; white-space:nowrap; }
.btn-download:hover { filter:brightness(1.1); transform:translateY(-1px); }

.report-overview { display:flex; align-items:center; gap:12px; flex-wrap:wrap; padding-top:14px; border-top:1.5px solid var(--border-light); margin-top:4px; }
.overview-stat { display:flex; flex-direction:column; align-items:center; padding:10px 16px; border-radius:var(--r-sm); min-width:64px; }
.ov-num { font-size:24px; font-weight:900; font-family:'JetBrains Mono',monospace; }
.ov-label { font-size:11px; font-weight:700; color:var(--ink-soft); margin-top:2px; }
.ov-fatal { background:#FFF0F0; } .ov-fatal .ov-num { color:#C0392B; }
.ov-serious { background:#FFF4E6; } .ov-serious .ov-num { color:#E67E22; }
.ov-medium { background:#FFFBE6; } .ov-medium .ov-num { color:#F39C12; }
.ov-low { background:#F0FFF4; } .ov-low .ov-num { color:#27AE60; }
.ov-total { background:var(--sun-light); } .ov-total .ov-num { color:var(--sun-deep); }
.ov-summary { flex:1; font-size:13px; color:var(--ink-mid); min-width:200px; }
.ov-summary-label { font-weight:700; color:var(--ink); }

.running-hint { margin-top:14px; font-size:13px; color:var(--warn); display:flex; align-items:center; gap:8px; }
.spin { animation:spin 1.5s linear infinite; display:inline-block; }
@keyframes spin { to { transform:rotate(360deg); } }

/* 筛选栏 */
.filter-bar { display:flex; align-items:center; gap:8px; flex-wrap:wrap; background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r-sm); padding:12px 16px; margin-bottom:16px; }
.filter-label { font-size:12.5px; font-weight:700; color:var(--ink-mid); white-space:nowrap; }
.filter-btn { padding:5px 14px; border-radius:20px; border:1.5px solid var(--border-light); background:var(--surface-2); font-size:12px; font-weight:600; color:var(--ink-soft); cursor:pointer; font-family:inherit; transition:all .15s; }
.filter-btn:hover { border-color:var(--peach); color:var(--sun-deep); background:var(--sun-light); }
.filter-active { background:var(--sun) !important; color:#fff !important; border-color:var(--sun) !important; }
.sev-btn-fatal.filter-active { background:#C0392B !important; border-color:#C0392B !important; }
.sev-btn-serious.filter-active { background:#E67E22 !important; border-color:#E67E22 !important; }
.sev-btn-medium.filter-active { background:#F39C12 !important; border-color:#F39C12 !important; }
.sev-btn-low.filter-active { background:#27AE60 !important; border-color:#27AE60 !important; }
.btn-dl-inline { margin-left:auto; display:inline-flex; align-items:center; gap:6px; padding:6px 14px; background:#EFF6FF; border:1.5px solid #BFDBFE; color:#1D4ED8; border-radius:var(--r-sm); font-size:12.5px; font-weight:700; text-decoration:none; transition:all .15s; white-space:nowrap; }
.btn-dl-inline:hover { background:#2563EB; color:#fff; border-color:#2563EB; }

/* 问题卡片 */
.issue-card { background:var(--surface); border-radius:var(--r-sm); border:1.5px solid var(--border-light); padding:18px 20px; margin-bottom:12px; box-shadow:var(--s1); }
.issue-fatal { border-left:4px solid #C0392B; }
.issue-serious { border-left:4px solid #E67E22; }
.issue-medium { border-left:4px solid #F39C12; }
.issue-low { border-left:4px solid #27AE60; }
.issue-header { display:flex; align-items:center; gap:8px; flex-wrap:wrap; margin-bottom:10px; }
.sev-badge { display:inline-block; padding:2px 9px; border-radius:4px; font-size:11px; font-weight:800; color:#fff; }
.sev-fatal { background:#C0392B; } .sev-serious { background:#E67E22; } .sev-medium { background:#F39C12; } .sev-low { background:#27AE60; }
.mod-badge { display:inline-block; padding:2px 8px; border-radius:4px; font-size:11px; font-weight:700; background:#34495E; color:#ECF0F1; }
.issue-id { font-size:11px; font-weight:700; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; }
.issue-title { font-size:15px; font-weight:700; color:var(--ink); flex:1; }
.chapter-tag { font-size:11px; color:var(--ink-hint); padding:2px 8px; background:var(--surface-3); border-radius:4px; white-space:nowrap; }
.issue-scope { font-size:12.5px; color:var(--ink-soft); margin-bottom:10px; }
.evidence-label { font-size:12px; font-weight:700; color:#34495E; margin-bottom:6px; }
.evidence-code { background:#2C3E50; color:#ECF0F1; padding:12px 16px; border-radius:var(--r-sm); font-family:'JetBrains Mono',monospace; font-size:12px; line-height:1.6; overflow-x:auto; white-space:pre-wrap; word-break:break-all; margin-bottom:10px; }
.issue-section { margin-top:10px; }
.section-label { font-size:13px; font-weight:700; margin-bottom:6px; }
.current-label { color:#C0392B; } .fix-label { color:#27AE60; }
.section-body { font-size:13px; color:var(--ink-mid); line-height:1.7; white-space:pre-line; }
.fix-body { background:#F0FFF4; padding:12px 14px; border-radius:var(--r-sm); border-left:3px solid #27AE60; }

/* 分页 */
.pagination-bar { display:flex; align-items:center; justify-content:space-between; background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r-sm); padding:12px 16px; margin-top:8px; flex-wrap:wrap; gap:10px; }
.page-info { font-size:13px; color:var(--ink-soft); }
.page-btns { display:flex; gap:4px; }
.page-btn { min-width:34px; height:34px; border-radius:7px; border:1.5px solid var(--border-light); background:var(--surface); color:var(--ink-mid); font-size:13px; font-weight:600; cursor:pointer; font-family:inherit; transition:all .15s; display:flex; align-items:center; justify-content:center; }
.page-btn:hover:not(:disabled) { border-color:var(--sun); color:var(--sun-deep); background:var(--sun-light); }
.page-btn:disabled { opacity:.4; cursor:not-allowed; }
.page-active { background:var(--sun) !important; color:#fff !important; border-color:var(--sun) !important; }

/* running */
.running-panel { text-align:center; padding:60px 40px; }
.running-icon { font-size:48px; margin-bottom:12px; }
.running-title { font-size:20px; font-weight:800; color:var(--ink); margin-bottom:8px; }
.running-desc { font-size:14px; color:var(--ink-soft); margin-bottom:24px; }
.large-progress { width:320px; height:10px; margin:0 auto 12px; }
.running-note { font-size:13px; color:var(--ink-hint); }
.progress-bar { display:inline-block; height:6px; background:var(--border-light); border-radius:3px; overflow:hidden; vertical-align:middle; margin-right:6px; }
.progress-fill { height:100%; background:linear-gradient(135deg,var(--sun),var(--coral)); border-radius:3px; transition:width .3s; }
.progress-text { font-size:11px; font-weight:700; color:var(--ink-soft); }
.badge { display:inline-block; font-size:11px; font-weight:800; padding:3px 10px; border-radius:20px; }
.badge-ok { background:var(--ok-bg); color:var(--ok); border:1px solid var(--ok-border); }
.badge-warn { background:var(--warn-bg); color:var(--warn); border:1px solid var(--warn-border); }
.badge-err { background:var(--err-bg); color:var(--err); border:1px solid var(--err-border); }
.summary-bar { background:var(--sun-light); border-left:4px solid var(--sun); padding:12px 16px; border-radius:0 var(--r-sm) var(--r-sm) 0; font-size:14px; font-weight:600; color:var(--ink); margin-bottom:16px; }
.suggestions-box { background:var(--surface-2); padding:16px 20px; border-radius:var(--r-sm); font-size:13.5px; white-space:pre-line; color:var(--ink-mid); line-height:1.7; }
</style>
