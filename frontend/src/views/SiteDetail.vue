<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push('/sites')" class="back-btn">← 返回站点列表</a>
      <span class="back-sep">·</span>
      <span class="back-url">{{ site.name }} ({{ site.domain }})</span>
    </div>

    <div class="panel panel-amber" style="margin-bottom:18px">
      <div class="panel-header">
        <h2>📋 站点信息</h2>
        <button class="btn btn-primary" @click="triggerAudit" :disabled="auditing">
          {{ auditing ? '巡检中...' : '🔍 立即巡检' }}
        </button>
      </div>
      <div class="info-grid">
        <div class="info-item">
          <span class="info-label">域名</span>
          <span class="info-val">
            <a :href="site.domain" target="_blank" class="domain-link">
              {{ site.domain }} <span class="link-icon">↗</span>
            </a>
          </span>
        </div>
        <div class="info-item"><span class="info-label">名称</span><span class="info-val">{{ site.name }}</span></div>
        <div class="info-item"><span class="info-label">状态</span><span class="info-val"><span class="badge badge-ok">{{ site.status === 'active' ? '启用' : '停用' }}</span></span></div>
      </div>
    </div>

    <!-- 页面管理 -->
    <div class="panel" style="margin-bottom:18px">
      <div class="panel-header">
        <h2>📄 页面管理</h2>
        <div style="display:flex;gap:8px">
          <button class="btn btn-sm btn-fetch-all" @click="fetchAllPages" :disabled="fetchingAll">
            {{ fetchingAll ? '抓取中...' : '⚡ 一键抓取全部' }}
          </button>
          <button class="btn btn-primary btn-sm" @click="showAddPage=true">＋ 添加页面</button>
        </div>
      </div>
      <div class="page-tip">添加需要监控的页面，支持实时抓取 HTML 源码查看</div>

      <!-- 添加页面表单 -->
      <div class="add-page-form" v-if="showAddPage">
        <input class="config-input" v-model="newPage.name" placeholder="页面名称（如：首页、详情页）" style="width:180px" />
        <input class="config-input" v-model="newPage.url" placeholder="完整 URL（https://example.com/page）" style="flex:1" />
        <button class="btn btn-primary btn-sm" @click="addPage" :disabled="addingPage">{{ addingPage ? '添加中...' : '确认添加' }}</button>
        <button class="btn btn-sm" @click="showAddPage=false;newPage={name:'',url:''}">取消</button>
      </div>

      <!-- 页面列表 -->
      <div v-if="sitePages.length" class="pages-list">
        <div class="page-row" v-for="p in sitePages" :key="p.id">
          <div class="page-info">
            <span class="page-name">{{ p.name }}</span>
            <a :href="p.url" target="_blank" class="page-url">{{ p.url }}</a>
          </div>
          <div class="page-meta" v-if="p.last_fetched_at">
            <span :class="['page-status', p.status_code===200?'status-ok':'status-err']">{{ p.status_code }}</span>
            <span class="page-ttfb">{{ p.ttfb_ms }}ms</span>
            <span class="page-time">🕐 {{ p.last_fetched_at }}</span>
          </div>
          <div class="page-meta" v-else>
            <span class="page-no-fetch">尚未抓取</span>
          </div>
          <div class="page-actions">
            <button class="btn btn-sm btn-fetch" @click="fetchPage(p)" :disabled="p.fetching">
              {{ p.fetching ? '抓取中...' : '⚡ 实时抓取' }}
            </button>
            <button class="btn btn-sm btn-source" @click="viewSource(p)" :disabled="!p.last_fetched_at">
              🔍 源码查询
            </button>
            <button class="btn btn-sm btn-del-page" @click="deletePage(p)">🗑️</button>
          </div>
        </div>
      </div>
      <div v-else class="pages-empty">暂无页面，点击「添加页面」开始监控</div>
    </div>

    <!-- 源码查看弹窗 -->
    <el-dialog v-model="showSource" :title="`源码查询 — ${sourceData.name}`" width="80%" class="source-dialog">
      <div class="source-meta" v-if="sourceData.last_fetched_at">
        <span class="source-badge">🕐 抓取时间：{{ sourceData.last_fetched_at }}</span>
        <span class="source-badge">📡 URL：{{ sourceData.url }}</span>
        <span :class="['source-badge', sourceData.status_code===200?'badge-ok-s':'badge-err-s']">
          状态码：{{ sourceData.status_code }}
        </span>
        <span class="source-badge">⏱ 响应：{{ sourceData.ttfb_ms }}ms</span>
      </div>
      <pre class="source-code">{{ sourceData.html }}</pre>
      <template #footer>
        <button class="btn btn-sm" @click="copySource">📋 复制源码</button>
        <button class="btn btn-primary btn-sm" @click="showSource=false">关 闭</button>
      </template>
    </el-dialog>

    <div class="panel">
      <div class="panel-header"><h2>📊 审计历史</h2></div>
      <table class="table">
        <thead><tr><th>ID</th><th>状态</th><th>进度</th><th>开始时间</th><th>完成时间</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="a in audits" :key="a.id">
            <td style="font-weight:800;color:var(--sun)">#{{ a.id }}</td>
            <td><span class="badge" :class="a.status==='done'?'badge-ok':a.status==='running'?'badge-warn':'badge-err'">{{ a.status==='done'?'已完成':a.status==='running'?'进行中':a.status==='pending'?'等待中':'失败' }}</span></td>
            <td><div class="progress-bar"><div class="progress-fill" :style="{width:a.progress+'%'}"></div></div><span class="progress-text">{{ a.progress }}%</span></td>
            <td style="font-size:12px;color:var(--ink-soft)">{{ a.created_at }}</td>
            <td style="font-size:12px;color:var(--ink-soft)">{{ a.finished_at || '—' }}</td>
            <td style="display:flex;gap:6px;align-items:center">
              <button class="btn btn-xs" @click="$router.push(`/sites/${$route.params.id}/audit/${a.id}`)">查看详情</button>
              <button v-if="a.status==='done'" class="btn btn-xs btn-dl" @click="downloadReport(a.id)" :disabled="a.downloading">
                {{ a.downloading ? '生成中...' : '📥 下载报告' }}
              </button>
              <button class="btn btn-xs btn-del-audit" @click="deleteAudit(a)" :disabled="a.status==='running'">🗑️</button>
            </td>
          </tr>
          <tr v-if="!audits.length"><td colspan="6" style="text-align:center;color:var(--ink-hint);padding:30px">暂无审计记录</td></tr>
        </tbody>
      </table>
    </div>
    <div class="panel" style="margin-top:18px">
      <div class="panel-header"><h2>⏰ 站点独立巡检计划</h2></div>
      <div class="schedule-desc">为此站点设置独立的巡检频率，设置后将覆盖全局调度，仅对本站点生效。</div>

      <div class="sched-field">
        <label class="sched-label">启用独立巡检计划</label>
        <label class="toggle-label" :class="schedEnabled?'toggle-on':'toggle-off'" @click="schedEnabled=!schedEnabled">
          <span class="toggle-switch"></span>
          <span class="toggle-text">{{ schedEnabled ? '已启用' : '未启用（跟随全局）' }}</span>
        </label>
      </div>

      <template v-if="schedEnabled">
        <div class="sched-field">
          <label class="sched-label">巡检模式</label>
          <div class="sched-mode-row">
            <div :class="['sched-mode-card', schedMode==='interval'&&'sched-mode-active']" @click="schedMode='interval'">
              <div class="smc-icon">⏱</div>
              <div class="smc-name">按间隔</div>
              <div class="smc-desc">每隔 N 小时巡查一次</div>
            </div>
            <div :class="['sched-mode-card', schedMode==='daily'&&'sched-mode-active']" @click="schedMode='daily'">
              <div class="smc-icon">🕘</div>
              <div class="smc-name">按时间</div>
              <div class="smc-desc">每天固定时间点巡查</div>
            </div>
          </div>
        </div>

        <div class="sched-field" v-if="schedMode==='interval'">
          <label class="sched-label">巡检间隔</label>
          <div class="num-row">
            <button class="num-btn" @click="intervalHours=Math.max(1,intervalHours-1)">－</button>
            <input class="num-input" type="number" v-model.number="intervalHours" min="1" max="168" />
            <button class="num-btn" @click="intervalHours=Math.min(168,intervalHours+1)">＋</button>
            <span class="num-unit">小时巡查一次</span>
          </div>
        </div>

        <div class="sched-field" v-if="schedMode==='daily'">
          <label class="sched-label">每天巡检时间</label>
          <div class="hour-grid">
            <span
              v-for="h in 24" :key="h-1"
              :class="['hour-chip', dailyHour===(h-1)&&'hour-active']"
              @click="dailyHour=h-1"
            >{{ String(h-1).padStart(2,'0') }}时</span>
          </div>
          <div class="sched-hint">当前选择：每天 {{ String(dailyHour).padStart(2,'0') }}:00 自动巡检</div>
        </div>
      </template>

      <div class="sched-actions">
        <button class="btn btn-primary" @click="saveSchedule" :disabled="savingSched">{{ savingSched ? '保存中...' : '💾 保存巡检计划' }}</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const route = useRoute()
const site = ref({})
const audits = ref([])
const auditing = ref(false)
const savingConfig = ref(false)

// Page management
const sitePages = ref([])
const showAddPage = ref(false)
const addingPage = ref(false)
const newPage = ref({ name: '', url: '' })
const showSource = ref(false)
const sourceData = ref({})
const fetchingAll = ref(false)

// Per-site schedule
const schedEnabled = ref(false)
const schedMode = ref('daily')
const intervalHours = ref(24)
const dailyHour = ref(9)
const savingSched = ref(false)

async function loadData() {
  try {
    const [siteRes, auditsRes, pagesRes] = await Promise.all([
      api.getSite(route.params.id),
      api.listAudits(route.params.id),
      api.listSitePages(route.params.id).catch(() => ({ data: [] })),
    ])
    site.value = siteRes.data
    audits.value = auditsRes.data.map(a => ({ ...a, downloading: false }))
    sitePages.value = (pagesRes.data || []).map(p => ({ ...p, fetching: false }))
    // Load schedule settings
    const s = siteRes.data
    schedEnabled.value = s.audit_schedule_enabled === 'true'
    if (s.audit_interval_hours > 0) {
      schedMode.value = 'interval'
      intervalHours.value = s.audit_interval_hours
    } else {
      schedMode.value = 'daily'
      dailyHour.value = s.audit_daily_hour >= 0 ? s.audit_daily_hour : 9
    }
  } catch {}

  // 有进行中任务则3秒后自动刷新
  const hasRunning = audits.value.some(a => a.status === 'running' || a.status === 'pending')
  if (hasRunning) pollTimer.value = setTimeout(loadData, 3000)
}

const pollTimer = ref(null)

onMounted(loadData)
onUnmounted(() => { if (pollTimer.value) clearTimeout(pollTimer.value) })
watch(() => route.params.id, () => { if (route.params.id) { if (pollTimer.value) clearTimeout(pollTimer.value); loadData() } })

async function downloadReport(taskId) {
  const audit = audits.value.find(a => a.id === taskId)
  if (audit) audit.downloading = true
  try {
    const token = localStorage.getItem('token')
    const resp = await fetch(`/api/v1/audits/${taskId}/report/download`, { headers: { Authorization: `Bearer ${token}` } })
    if (!resp.ok) { const data = await resp.json(); ElMessage.error(data.error || '下载失败'); return }
    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const el = document.createElement('a')
    el.href = url
    const disp = resp.headers.get('Content-Disposition') || ''
    const match = disp.match(/filename="(.+)"/)
    el.download = match ? match[1] : `SEO审计报告_${taskId}.docx`
    document.body.appendChild(el); el.click(); document.body.removeChild(el)
    URL.revokeObjectURL(url)
    ElMessage.success('报告下载成功')
  } catch (e) { ElMessage.error('下载失败：' + e.message) }
  finally { if (audit) audit.downloading = false }
}

async function deleteAudit(a) {
  try {
    await ElMessageBox.confirm(`确定要删除审计记录 #${a.id} 吗？删除后不可恢复。`, '确认删除', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
    await api.deleteAudit(a.id)
    ElMessage.success('审计记录已删除')
    loadData()
  } catch {}
}

async function fetchAllPages() {
  if (!sitePages.value.length) { ElMessage.warning('暂无页面，请先添加'); return }
  fetchingAll.value = true
  ElMessage.info(`开始抓取 ${sitePages.value.length} 个页面...`)
  let ok = 0, fail = 0
  for (const p of sitePages.value) {
    p.fetching = true
    try {
      await api.fetchSitePage(route.params.id, p.id)
      ok++
    } catch { fail++ }
    finally { p.fetching = false }
  }
  fetchingAll.value = false
  ElMessage.success(`抓取完成：成功 ${ok} 个${fail ? `，失败 ${fail} 个` : ''}`)
  loadData()
}

async function addPage() {
  if (!newPage.value.name) { ElMessage.warning('请填写页面名称'); return }
  if (!newPage.value.url || !newPage.value.url.startsWith('http')) {
    ElMessage.warning('请填写完整 URL（以 http:// 或 https:// 开头）'); return
  }
  addingPage.value = true
  try {
    await api.createSitePage(route.params.id, newPage.value)
    ElMessage.success('页面添加成功')
    showAddPage.value = false
    newPage.value = { name: '', url: '' }
    loadData()
  } catch {}
  finally { addingPage.value = false }
}

async function deletePage(p) {
  try {
    await api.deleteSitePage(route.params.id, p.id)
    ElMessage.success('已删除')
    loadData()
  } catch {}
}

async function fetchPage(p) {
  p.fetching = true
  try {
    const res = await api.fetchSitePage(route.params.id, p.id)
    ElMessage.success(`抓取完成，状态码：${res.data.status_code}，耗时：${res.data.ttfb_ms}ms`)
    loadData()
  } catch {}
  finally { p.fetching = false }
}

async function viewSource(p) {
  try {
    const res = await api.getSitePageSource(route.params.id, p.id)
    sourceData.value = res.data
    showSource.value = true
  } catch {}
}

function copySource() {
  navigator.clipboard.writeText(sourceData.value.html || '').then(() => ElMessage.success('源码已复制'))
}

async function saveSchedule() {
  savingSched.value = true
  try {
    const payload = {
      ...site.value,
      audit_schedule_enabled: schedEnabled.value ? 'true' : 'false',
      audit_interval_hours: schedMode.value === 'interval' ? intervalHours.value : 0,
      audit_daily_hour: schedMode.value === 'daily' ? dailyHour.value : -1,
    }
    await api.updateSite(route.params.id, payload)
    ElMessage.success('巡检计划已保存')
  } catch {}
  finally { savingSched.value = false }
}

async function triggerAudit() {
  auditing.value = true
  try { await api.triggerAudit(route.params.id); ElMessage.success('巡检已启动 — 将自动抓取最新源码后开始分析'); loadData() } catch {} finally { auditing.value = false }
}
</script>

<style scoped>
.back-row { display:flex; align-items:center; gap:8px; margin-bottom:18px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); }
.back-btn:hover { color:var(--sun-deep); }
.back-sep { color:var(--ink-hint); }
.back-url { font-size:14px; font-weight:700; color:var(--ink); }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel + .panel { margin-top:18px; }
.panel-amber { background:linear-gradient(135deg,#FFFDF8 0%,#FFF8EC 60%,#FFF2D8 100%); border-color:var(--peach); }
.panel-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:18px; }
.panel-header h2 { font-size:15px; font-weight:700; }

.info-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:14px; }
.info-item { display:flex; flex-direction:column; gap:4px; }
.info-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.06em; }
.info-val { font-size:14px; font-weight:600; color:var(--ink); }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); transform:translateY(-1px); box-shadow:var(--s1); }
.btn-primary { background:linear-gradient(135deg,var(--sun) 0%,var(--coral) 100%); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep) 0%,var(--sun) 100%); color:#fff; box-shadow:0 8px 24px rgba(249,115,22,.48); }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; transform:none; }
.btn-del-audit { color:var(--err) !important; border-color:#FFCDD2 !important; background:#FFF5F5 !important; }
.btn-del-audit:hover { background:var(--err) !important; color:#fff !important; border-color:var(--err) !important; }
.btn-del-audit:disabled { opacity:.4; cursor:not-allowed; }
.btn-xs:hover { background:var(--sun-glow); border-color:var(--sun); color:var(--sun-deep); }
.btn-dl { background:#EFF6FF !important; border-color:#BFDBFE !important; color:#1D4ED8 !important; text-decoration:none; }
.btn-dl:hover { background:#2563EB !important; color:#fff !important; border-color:#2563EB !important; }

.table { width:100%; border-collapse:collapse; }
.table th { padding:10px 14px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.08em; border-bottom:2px solid var(--border); background:var(--surface-3); }
.table td { padding:10px 14px; border-bottom:1px solid var(--border-light); font-size:13px; }
.table tr:hover td { background:var(--sun-light); }

.badge { display:inline-block; font-size:11px; font-weight:800; padding:2px 10px; border-radius:20px; text-transform:uppercase; }
.badge-ok { background:var(--ok-bg); color:var(--ok); border:1px solid var(--ok-border); }
.badge-warn { background:var(--warn-bg); color:var(--warn); border:1px solid var(--warn-border); }
.badge-err { background:var(--err-bg); color:var(--err); border:1px solid var(--err-border); }

.progress-bar { display:inline-block; width:80px; height:6px; background:var(--border-light); border-radius:3px; overflow:hidden; vertical-align:middle; margin-right:6px; }
.progress-fill { height:100%; background:linear-gradient(135deg,var(--sun),var(--coral)); border-radius:3px; transition:width .3s; }
.progress-text { font-size:11px; font-weight:700; color:var(--ink-soft); }

.domain-link { color:var(--info); font-weight:700; font-size:14px; display:inline-flex; align-items:center; gap:4px; }
.domain-link:hover { color:var(--sun); text-decoration:underline; }
.link-icon { font-size:12px; opacity:.7; }

/* ── 页面管理 ── */
.page-tip { font-size:12.5px; color:var(--ink-soft); margin-bottom:16px; }
.add-page-form { display:flex; gap:10px; align-items:center; padding:14px 16px; background:var(--surface-3); border-radius:var(--r-sm); margin-bottom:16px; flex-wrap:wrap; }
.pages-list { display:flex; flex-direction:column; gap:0; border:1.5px solid var(--border-light); border-radius:var(--r-sm); overflow:hidden; }
.page-row { display:flex; align-items:center; gap:14px; padding:12px 16px; border-bottom:1px solid var(--border-light); flex-wrap:wrap; }
.page-row:last-child { border-bottom:none; }
.page-row:hover { background:var(--sun-light); }
.page-info { display:flex; flex-direction:column; gap:3px; flex:1; min-width:200px; }
.page-name { font-weight:700; font-size:14px; color:var(--ink); }
.page-url { font-size:12px; color:var(--info); word-break:break-all; }
.page-url:hover { text-decoration:underline; }
.page-meta { display:flex; align-items:center; gap:10px; flex-shrink:0; }
.page-status { font-family:'JetBrains Mono',monospace; font-size:12px; font-weight:700; padding:2px 8px; border-radius:5px; }
.status-ok { background:var(--ok-bg); color:var(--ok); }
.status-err { background:var(--err-bg); color:var(--err); }
.page-ttfb { font-size:12px; color:var(--ink-soft); font-family:'JetBrains Mono',monospace; }
.page-time { font-size:11.5px; color:var(--ink-hint); }
.page-no-fetch { font-size:12px; color:var(--ink-hint); }
.page-actions { display:flex; gap:6px; flex-shrink:0; }
.btn-fetch-all { background:linear-gradient(135deg,#F59E0B,#F97316) !important; color:#fff !important; border-color:transparent !important; box-shadow:0 4px 14px rgba(245,158,11,.4) !important; }
.btn-fetch-all:hover { filter:brightness(1.08); transform:translateY(-1px); }
.btn-fetch-all:disabled { opacity:.6; cursor:not-allowed; transform:none; }
.btn-fetch { background:var(--sun-light) !important; border-color:var(--peach) !important; color:var(--sun-deep) !important; }
.btn-fetch:hover { background:linear-gradient(135deg,var(--sun),var(--coral)) !important; color:#fff !important; }
.btn-fetch:disabled { opacity:.6; cursor:not-allowed; }
.btn-source { background:var(--surface-3) !important; }
.btn-source:disabled { opacity:.4; cursor:not-allowed; }
.btn-del-page { color:var(--err) !important; border-color:#FFCDD2 !important; background:#FFF5F5 !important; }
.btn-del-page:hover { background:var(--err) !important; color:#fff !important; }
.pages-empty { text-align:center; padding:36px; color:var(--ink-hint); font-size:13.5px; }

/* ── 源码弹窗 ── */
.source-meta { display:flex; flex-wrap:wrap; gap:8px; margin-bottom:14px; }
.source-badge { font-size:12px; font-weight:600; padding:4px 10px; background:var(--surface-3); border-radius:6px; color:var(--ink-mid); }
.badge-ok-s { background:var(--ok-bg); color:var(--ok); }
.badge-err-s { background:var(--err-bg); color:var(--err); }
.source-code { background:#1E1E1E; color:#D4D4D4; padding:16px 20px; border-radius:var(--r-sm); font-family:'JetBrains Mono',monospace; font-size:12px; line-height:1.6; overflow:auto; max-height:60vh; white-space:pre-wrap; word-break:break-all; }
:deep(.source-dialog .el-dialog__footer) { display:flex; gap:8px; justify-content:flex-end; }

/* ── 站点配置面板 ── */
.config-section-title { font-size:12px; font-weight:800; letter-spacing:.06em; color:var(--sun-deep); margin-bottom:12px; padding-bottom:8px; border-bottom:2px solid var(--sun-glow); display:flex; align-items:center; gap:10px; }
.config-tip { font-size:11.5px; font-weight:500; color:var(--ink-soft); letter-spacing:0; }
.config-grid { display:grid; grid-template-columns:repeat(2,1fr); gap:14px; }
.config-field { display:flex; flex-direction:column; gap:6px; }
.config-label { font-size:12.5px; font-weight:600; color:var(--ink-mid); }
.config-input { width:100%; padding:9px 12px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13.5px; font-family:inherit; background:var(--surface); color:var(--ink); outline:none; transition:border-color .18s, box-shadow .18s; }
.config-input:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); background:#FFFDF9; }
.btn-sm { padding:6px 14px !important; font-size:12.5px !important; }

/* ── 站点独立巡检计划 ── */
.schedule-desc { font-size:13px; color:var(--ink-soft); line-height:1.6; margin-bottom:18px; padding:12px 14px; background:var(--warn-bg); border:1px solid var(--warn-border); border-radius:var(--r-sm); }
.sched-field { display:flex; flex-direction:column; gap:8px; margin-bottom:18px; }
.sched-label { font-size:13px; font-weight:600; color:var(--ink-mid); }
.sched-hint { font-size:12px; color:var(--ink-hint); }

.sched-mode-row { display:grid; grid-template-columns:1fr 1fr; gap:12px; }
.sched-mode-card { border:2px solid var(--border-light); border-radius:var(--r-sm); padding:14px 16px; cursor:pointer; text-align:center; transition:all .18s; background:var(--surface-2); }
.sched-mode-card:hover { border-color:var(--peach); background:var(--sun-light); }
.sched-mode-active { border-color:var(--sun) !important; background:var(--sun-light) !important; box-shadow:0 0 0 3px rgba(249,115,22,.1); }
.smc-icon { font-size:22px; margin-bottom:5px; }
.smc-name { font-weight:700; font-size:14px; color:var(--ink); margin-bottom:3px; }
.smc-desc { font-size:12px; color:var(--ink-soft); }

.num-row { display:flex; align-items:center; gap:10px; }
.num-btn { width:34px; height:34px; border-radius:8px; border:1.5px solid var(--border); background:var(--surface); color:var(--ink); font-size:18px; font-weight:700; cursor:pointer; display:flex; align-items:center; justify-content:center; transition:all .15s; flex-shrink:0; }
.num-btn:hover { background:var(--sun-light); border-color:var(--sun); color:var(--sun); }
.num-input { width:72px; padding:8px 10px; border:1.5px solid var(--border); border-radius:8px; font-size:14px; font-weight:700; text-align:center; font-family:inherit; outline:none; }
.num-input:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); }
.num-unit { font-size:13px; color:var(--ink-soft); white-space:nowrap; }

.hour-grid { display:flex; flex-wrap:wrap; gap:6px; }
.hour-chip { padding:5px 10px; border-radius:7px; border:1.5px solid var(--border-light); background:var(--surface-2); font-size:12px; font-weight:600; color:var(--ink-soft); cursor:pointer; transition:all .15s; font-family:'JetBrains Mono',monospace; }
.hour-chip:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun); }
.hour-active { background:var(--sun) !important; color:#fff !important; border-color:var(--sun) !important; box-shadow:0 2px 8px rgba(249,115,22,.3); }

.sched-actions { padding-top:6px; display:flex; gap:10px; }

.toggle-label { display:inline-flex; align-items:center; gap:9px; cursor:pointer; user-select:none; }
.toggle-switch { width:42px; height:24px; border-radius:12px; position:relative; flex-shrink:0; transition:background .2s; }
.toggle-on .toggle-switch { background:linear-gradient(135deg,#16A34A,#22C55E); box-shadow:0 2px 8px rgba(22,163,74,.3); }
.toggle-off .toggle-switch { background:#D1C4BA; }
.toggle-switch::after { content:''; position:absolute; top:3px; width:18px; height:18px; border-radius:50%; background:#fff; box-shadow:0 1px 4px rgba(0,0,0,.2); transition:left .2s; }
.toggle-on .toggle-switch::after { left:21px; }
.toggle-off .toggle-switch::after { left:3px; }
.toggle-text { font-weight:700; font-size:13px; }
.toggle-on .toggle-text { color:var(--ok); }
.toggle-off .toggle-text { color:var(--ink-soft); }
</style>
