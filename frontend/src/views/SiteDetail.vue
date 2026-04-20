<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push('/sites')" class="back-btn">← 返回站点列表</a>
      <span class="back-sep">·</span>
      <span class="back-url">{{ site.name }} ({{ site.domain }})</span>
    </div>

    <SiteInfo :site="site" :auditing="auditing" @trigger-audit="triggerAudit" />

    <PageManager
      :pages="sitePages"
      :fetching-all="fetchingAll"
      :site-id="route.params.id"
      @fetch-all="fetchAllPages"
      @fetch-page="fetchPage"
      @view-source="viewSource"
      @delete-page="deletePage"
      @add-page="addPage"
    />

    <!-- 源码查看弹窗 -->
    <el-dialog v-model="showSource" :title="`源码查询 — ${sourceData.name}`" width="80%" class="source-dialog">
      <div class="source-meta" v-if="sourceData.last_fetched_at">
        <span class="source-badge">抓取时间：{{ sourceData.last_fetched_at }}</span>
        <span class="source-badge">URL：{{ sourceData.url }}</span>
        <span :class="['source-badge', sourceData.status_code===200?'badge-ok-s':'badge-err-s']">
          状态码：{{ sourceData.status_code }}
        </span>
        <span class="source-badge">响应：{{ sourceData.ttfb_ms }}ms</span>
      </div>
      <pre class="source-code">{{ sourceData.html }}</pre>
      <template #footer>
        <button class="btn btn-sm" @click="copySource">复制源码</button>
        <button class="btn btn-primary btn-sm" @click="showSource=false">关 闭</button>
      </template>
    </el-dialog>

    <AuditHistory
      :audits="audits"
      @view-audit="id => $router.push(`/sites/${route.params.id}/audit/${id}`)"
      @download-report="downloadReport"
      @delete-audit="deleteAudit"
    />

    <!-- ── 历史错误汇总 ──────────────────────────────────────── -->
    <div class="panel history-panel">
      <div class="hp-head">
        <div>
          <h2 class="hp-title">📒 历史错误汇总</h2>
          <p class="hp-sub">本站点已完成的协同任务 + 手动登记的故障 / 黑名单 / 服务异常事件，按时间倒序。
            手动事件可删除；协同任务请到「协同管理」操作。</p>
        </div>
        <button class="btn btn-primary" @click="openIncidentDialog">＋ 登记事件</button>
      </div>

      <div v-if="!history.length" class="hp-empty">暂无历史记录</div>
      <div v-else class="hp-list">
        <div v-for="h in history" :key="h.id" class="hp-row" :class="'hp-' + h.source">
          <div class="hp-when">{{ fmtDate(h.at) }}</div>
          <div class="hp-icon">{{ h.source === 'coord' ? '✅' : '🚨' }}</div>
          <div class="hp-body">
            <div class="hp-row-head">
              <span class="hp-title-text">{{ h.title }}</span>
              <span v-if="h.severity" class="hp-sev" :class="'sev-' + sevKey(h.severity)">{{ sevLabel(h.severity) }}</span>
              <span v-if="h.scope" class="hp-scope">{{ h.scope }}</span>
              <span v-if="h.module" class="hp-mod">{{ h.module }}</span>
              <span class="hp-source-tag">{{ h.source === 'coord' ? '协同已完成' : '手动登记' }}</span>
            </div>
            <div v-if="h.description || h.note" class="hp-desc">{{ h.description || h.note }}</div>
            <div class="hp-meta">登记人：{{ h.actor_name || '—' }}</div>
          </div>
          <div class="hp-actions">
            <button v-if="h.source === 'manual'" class="btn btn-xs btn-danger-xs" @click="deleteIncident(h)">删除</button>
            <button v-else class="btn btn-xs" @click="$router.push(`/coordination/${h.coord_id}`)">查看任务</button>
          </div>
        </div>
      </div>
    </div>

    <!-- 登记事件弹窗 -->
    <el-dialog v-model="showIncidentDialog" title="📝 登记一个站点事件" width="540px">
      <div class="form-row">
        <label>标题 <span class="req">*</span></label>
        <el-input v-model="incidentForm.title" placeholder="例如：站点 503 / 主域名被封 / 备案被注销" />
      </div>
      <div class="form-row">
        <label>严重程度</label>
        <el-select v-model="incidentForm.severity" style="width:100%">
          <el-option label="🟢 低" value="low" />
          <el-option label="🟡 中" value="medium" />
          <el-option label="🔴 高" value="high" />
        </el-select>
      </div>
      <div class="form-row">
        <label>发生时间（不填默认现在）</label>
        <el-date-picker v-model="incidentForm.occurred_at" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" placeholder="留空 = 现在" style="width:100%" />
      </div>
      <div class="form-row">
        <label>详细描述</label>
        <el-input v-model="incidentForm.description" type="textarea" rows="4" placeholder="可写：现象 / 触发原因 / 临时处理 / 永久修复方案，方便以后复盘参考" />
      </div>
      <template #footer>
        <el-button @click="showIncidentDialog = false">取消</el-button>
        <el-button type="primary" @click="saveIncident" :loading="savingIncident">保存</el-button>
      </template>
    </el-dialog>

    <ScheduleConfig
      :sched-enabled="schedEnabled"
      :sched-mode="schedMode"
      :interval-hours="intervalHours"
      :daily-hour="dailyHour"
      :saving="savingSched"
      @save="saveSchedule"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import { copyToClipboard } from '../utils/clipboard'
import SiteInfo from '../components/site/SiteInfo.vue'
import PageManager from '../components/site/PageManager.vue'
import AuditHistory from '../components/site/AuditHistory.vue'
import ScheduleConfig from '../components/site/ScheduleConfig.vue'

const route = useRoute()
const queryClient = useQueryClient()
const siteId = computed(() => route.params.id)
const auditing = ref(false)

const showSource = ref(false)
const sourceData = ref({})
const fetchingAll = ref(false)

const schedEnabled = ref(false)
const schedMode = ref('daily')
const intervalHours = ref(24)
const dailyHour = ref(9)
const savingSched = ref(false)

const { data: siteData, refetch: refetchSite } = useQuery({
  queryKey: ['site', siteId],
  queryFn: async () => {
    const res = await api.getSite(siteId.value)
    return res.data
  },
  enabled: computed(() => !!siteId.value),
})

const { data: auditsData, refetch: refetchAudits } = useQuery({
  queryKey: ['audits', siteId],
  queryFn: async () => {
    const res = await api.listAudits(siteId.value)
    return res.data
  },
  enabled: computed(() => !!siteId.value),
  // Poll every 2s while any audit is in flight; stop polling once all
  // are terminal (done/error) so we don't hammer the backend forever.
  // The function form re-evaluates after every fetch using the latest data.
  refetchInterval: (query) => {
    const list = query?.state?.data || []
    const running = list.some(a => a.status === 'running' || a.status === 'pending')
    return running ? 2000 : false
  },
  refetchIntervalInBackground: false,
})

// Determine if any audit is running to enable fast polling
const hasRunning = computed(() =>
  (auditsData.value || []).some(a => a.status === 'running' || a.status === 'pending')
)

const { data: pagesData, refetch: refetchPages } = useQuery({
  queryKey: ['sitePages', siteId],
  queryFn: async () => {
    const res = await api.listSitePages(siteId.value)
    return res.data
  },
  enabled: computed(() => !!siteId.value),
})

const site = computed(() => siteData.value || {})
const audits = computed(() =>
  (auditsData.value || []).map(a => ({ ...a, downloading: false }))
)
const sitePages = ref([])

// Sync pages data from query into ref (needs ref for per-item fetching state)
watch(() => pagesData.value, (val) => {
  sitePages.value = (val || []).map(p => ({ ...p, fetching: false }))
}, { immediate: true })

// Sync schedule config from site data
watch(site, (s) => {
  if (!s || !s.id) return
  schedEnabled.value = s.audit_schedule_enabled === 'true'
  if (s.audit_interval_hours > 0) {
    schedMode.value = 'interval'
    intervalHours.value = s.audit_interval_hours
  } else {
    schedMode.value = 'daily'
    dailyHour.value = s.audit_daily_hour >= 0 ? s.audit_daily_hour : 9
  }
}, { immediate: true })

function loadData() {
  refetchSite()
  refetchAudits()
  refetchPages()
  loadHistory()
}

// ── 历史错误汇总 ───────────────────────────────────────────────
const history = ref([])
const showIncidentDialog = ref(false)
const savingIncident = ref(false)
const incidentForm = ref({ title: '', severity: 'medium', occurred_at: '', description: '' })

async function loadHistory() {
  try {
    const res = await api.siteHistory(route.params.id)
    history.value = res.data || []
  } catch { /* leave empty */ }
}

function openIncidentDialog() {
  incidentForm.value = { title: '', severity: 'medium', occurred_at: '', description: '' }
  showIncidentDialog.value = true
}

async function saveIncident() {
  if (!incidentForm.value.title.trim()) {
    ElMessage.warning('请填写事件标题')
    return
  }
  savingIncident.value = true
  try {
    await api.createSiteIncident(route.params.id, incidentForm.value)
    ElMessage.success('已登记')
    showIncidentDialog.value = false
    await loadHistory()
  } catch (e) {
    if (!e?.response) console.error(e)
  } finally {
    savingIncident.value = false
  }
}

async function deleteIncident(h) {
  try {
    await ElMessageBox.confirm(`确认删除事件「${h.title}」？`, '删除', { type: 'warning' })
  } catch { return }
  try {
    await api.deleteSiteIncident(route.params.id, h.incident_id)
    ElMessage.success('已删除')
    await loadHistory()
  } catch (e) { console.error(e) }
}

function fmtDate(s) {
  if (!s) return '—'
  // Backend gives RFC3339-ish; render as local YYYY-MM-DD HH:mm.
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function sevKey(s) {
  if (!s) return 'medium'
  const v = String(s).toLowerCase()
  if (v.includes('致命') || v.includes('high') || v.includes('严重')) return 'high'
  if (v.includes('低') || v.includes('low')) return 'low'
  return 'medium'
}
function sevLabel(s) {
  const k = sevKey(s)
  return { high: '🔴 高', medium: '🟡 中', low: '🟢 低' }[k]
}

async function downloadReport(taskId) {
  const audit = audits.value.find(a => a.id === taskId)
  if (audit) audit.downloading = true
  try {
    const { blob, filename } = await api.fetchAuditReportBlob(taskId)
    const url = URL.createObjectURL(blob)
    const el = document.createElement('a')
    el.href = url
    el.download = filename
    document.body.appendChild(el); el.click(); document.body.removeChild(el)
    URL.revokeObjectURL(url)
    ElMessage.success('报告下载成功')
  } catch (e) { ElMessage.error('下载失败：' + (e?.message || e)) }
  finally { if (audit) audit.downloading = false }
}

async function deleteAudit(a) {
  try {
    await ElMessageBox.confirm(`确定要删除审计记录 #${a.id} 吗？删除后不可恢复。`, '确认删除', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
    await api.deleteAudit(a.id)
    ElMessage.success('审计记录已删除')
    loadData()
  } catch (e) { if (e !== 'cancel') console.error('删除审计失败', e) }
}

async function fetchAllPages() {
  if (!sitePages.value.length) { ElMessage.warning('暂无页面，请先添加'); return }
  fetchingAll.value = true
  ElMessage.info(`开始抓取 ${sitePages.value.length} 个页面...`)
  let ok = 0, fail = 0
  for (const p of sitePages.value) {
    p.fetching = true
    try { await api.fetchSitePage(route.params.id, p.id); ok++ }
    catch (e) { fail++; console.error('抓取页面失败', p.url, e) }
    finally { p.fetching = false }
  }
  fetchingAll.value = false
  ElMessage.success(`抓取完成：成功 ${ok} 个${fail ? `，失败 ${fail} 个` : ''}`)
  loadData()
}

async function addPage(pageData) {
  try {
    await api.createSitePage(route.params.id, pageData)
    ElMessage.success('页面添加成功')
    loadData()
  } catch (e) { console.error('添加页面失败', e) }
}

async function deletePage(p) {
  try {
    await api.deleteSitePage(route.params.id, p.id)
    ElMessage.success('已删除')
    loadData()
  } catch (e) { console.error('删除页面失败', e) }
}

async function fetchPage(p) {
  p.fetching = true
  try {
    const res = await api.fetchSitePage(route.params.id, p.id)
    ElMessage.success(`抓取完成，状态码：${res.data.status_code}，耗时：${res.data.ttfb_ms}ms`)
    loadData()
  } catch (e) { console.error('抓取页面失败', e) }
  finally { p.fetching = false }
}

async function viewSource(p) {
  try {
    const res = await api.getSitePageSource(route.params.id, p.id)
    sourceData.value = res.data
    showSource.value = true
  } catch (e) { console.error('查看源码失败', e) }
}

async function copySource() {
  const ok = await copyToClipboard(sourceData.value.html || '')
  if (ok) ElMessage.success('源码已复制')
  else ElMessage.error('复制失败，请手动选择文本复制')
}

async function saveSchedule(config) {
  savingSched.value = true
  try {
    const payload = {
      ...site.value,
      audit_schedule_enabled: config.enabled ? 'true' : 'false',
      audit_interval_hours: config.mode === 'interval' ? config.intervalHours : 0,
      audit_daily_hour: config.mode === 'daily' ? config.dailyHour : -1,
    }
    await api.updateSite(route.params.id, payload)
    ElMessage.success('巡检计划已保存')
  } catch (e) { console.error('保存巡检计划失败', e) }
  finally { savingSched.value = false }
}

async function triggerAudit() {
  auditing.value = true
  try { await api.triggerAudit(route.params.id); ElMessage.success('巡检已启动'); loadData() }
  catch (e) { console.error('触发巡检失败', e) }
  finally { auditing.value = false }
}
</script>

<style scoped>
.back-row { display:flex; align-items:center; gap:8px; margin-bottom:18px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); }
.back-btn:hover { color:var(--sun-deep); }
.back-sep { color:var(--ink-hint); }
.back-url { font-size:14px; font-weight:700; color:var(--ink); }
.source-meta { display:flex; flex-wrap:wrap; gap:8px; margin-bottom:14px; }
.source-badge { font-size:12px; font-weight:600; padding:4px 10px; background:var(--surface-3); border-radius:6px; color:var(--ink-mid); }
.badge-ok-s { background:var(--ok-bg); color:var(--ok); }
.badge-err-s { background:var(--err-bg); color:var(--err); }
.source-code { background:#1E1E1E; color:#D4D4D4; padding:16px 20px; border-radius:var(--r-sm); font-family:'JetBrains Mono',monospace; font-size:12px; line-height:1.6; overflow:auto; max-height:60vh; white-space:pre-wrap; word-break:break-all; }
:deep(.source-dialog .el-dialog__footer) { display:flex; gap:8px; justify-content:flex-end; }
.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-sm { padding:6px 14px !important; font-size:12.5px !important; }
.btn-primary { background:linear-gradient(135deg,var(--sun) 0%,var(--coral) 100%); color:#fff; border-color:transparent; }
.btn-xs { padding:4px 10px !important; font-size:12px !important; border-radius:6px; }
.btn-danger-xs { color:var(--err); border-color:var(--err); }
.btn-danger-xs:hover { background:var(--err); color:#fff; }

/* History panel */
.panel { background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r); box-shadow:var(--s1); padding:18px 22px; margin-top:18px; }
.history-panel { padding:18px 22px; }
.hp-head { display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:14px; gap:18px; }
.hp-title { margin:0; font-size:17px; font-weight:800; color:var(--ink); }
.hp-sub { margin:4px 0 0; font-size:12.5px; color:var(--ink-soft); line-height:1.7; max-width:680px; }
.hp-empty { padding:30px 0; text-align:center; color:var(--ink-hint); font-size:14px; }
.hp-list { display:flex; flex-direction:column; gap:10px; }
.hp-row { display:grid; grid-template-columns:140px 32px 1fr auto; gap:14px; align-items:flex-start; padding:14px 16px; background:var(--surface-2); border-radius:var(--r-sm); border-left:4px solid var(--border-light); }
.hp-coord { border-left-color:#10B981; }
.hp-manual { border-left-color:#EF4444; }
.hp-when { font-size:12px; color:var(--ink-soft); font-family:'JetBrains Mono',monospace; padding-top:2px; }
.hp-icon { font-size:20px; }
.hp-row-head { display:flex; flex-wrap:wrap; align-items:center; gap:8px; }
.hp-title-text { font-weight:700; color:var(--ink); font-size:14px; }
.hp-sev { padding:2px 8px; border-radius:10px; font-size:11px; font-weight:700; }
.sev-high { background:#FEF2F2; color:#DC2626; }
.sev-medium { background:#FEF3C7; color:#D97706; }
.sev-low { background:#ECFDF5; color:#059669; }
.hp-scope { padding:2px 8px; background:var(--sun-light); color:var(--sun-deep); border-radius:10px; font-size:11px; font-weight:700; border:1px solid var(--peach); }
.hp-mod { padding:2px 8px; background:#34495E; color:#fff; border-radius:6px; font-size:10.5px; font-weight:700; }
.hp-source-tag { padding:2px 8px; background:var(--surface); color:var(--ink-soft); border-radius:6px; font-size:11px; font-weight:600; border:1px solid var(--border-light); }
.hp-desc { margin-top:6px; font-size:13px; color:var(--ink-mid); line-height:1.6; white-space:pre-wrap; }
.hp-meta { margin-top:6px; font-size:11.5px; color:var(--ink-soft); }
.hp-actions { display:flex; align-items:center; }
.form-row { margin-bottom:14px; }
.form-row label { display:block; font-size:13px; font-weight:700; color:var(--ink-mid); margin-bottom:6px; }
.req { color:var(--err); margin-left:2px; }
</style>
