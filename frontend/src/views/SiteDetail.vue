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

<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import SiteInfo from '../components/site/SiteInfo.vue'
import PageManager from '../components/site/PageManager.vue'
import AuditHistory from '../components/site/AuditHistory.vue'
import ScheduleConfig from '../components/site/ScheduleConfig.vue'

const route = useRoute()
const site = ref({})
const audits = ref([])
const auditing = ref(false)

const sitePages = ref([])
const showSource = ref(false)
const sourceData = ref({})
const fetchingAll = ref(false)

const schedEnabled = ref(false)
const schedMode = ref('daily')
const intervalHours = ref(24)
const dailyHour = ref(9)
const savingSched = ref(false)

const pollTimer = ref(null)

function clearPoll() {
  if (pollTimer.value) {
    clearTimeout(pollTimer.value)
    pollTimer.value = null
  }
}

async function loadData() {
  clearPoll()
  try {
    const [siteRes, auditsRes, pagesRes] = await Promise.allSettled([
      api.getSite(route.params.id),
      api.listAudits(route.params.id),
      api.listSitePages(route.params.id),
    ])
    if (siteRes.status === 'fulfilled') {
      site.value = siteRes.value.data
      const s = siteRes.value.data
      schedEnabled.value = s.audit_schedule_enabled === 'true'
      if (s.audit_interval_hours > 0) {
        schedMode.value = 'interval'
        intervalHours.value = s.audit_interval_hours
      } else {
        schedMode.value = 'daily'
        dailyHour.value = s.audit_daily_hour >= 0 ? s.audit_daily_hour : 9
      }
    }
    if (auditsRes.status === 'fulfilled') {
      audits.value = auditsRes.value.data.map(a => ({ ...a, downloading: false }))
    }
    if (pagesRes.status === 'fulfilled') {
      sitePages.value = (pagesRes.value.data || []).map(p => ({ ...p, fetching: false }))
    }
  } catch (e) { console.error('加载站点数据失败', e) }

  const hasRunning = audits.value.some(a => a.status === 'running' || a.status === 'pending')
  if (hasRunning) pollTimer.value = setTimeout(loadData, 3000)
}

onMounted(loadData)
onUnmounted(clearPoll)
watch(() => route.params.id, (id) => { if (id) { clearPoll(); loadData() } })

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

function copySource() {
  navigator.clipboard.writeText(sourceData.value.html || '').then(() => ElMessage.success('源码已复制'))
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
</style>
