<template>
  <div class="deadlinks-page">
    <!-- 站点选择 -->
    <div class="panel selector-bar">
      <span class="selector-label">🔗 选择站点</span>
      <el-select v-model="selectedSiteId" placeholder="请选择站点" style="width:260px" @change="onSiteChange">
        <el-option v-for="s in sites" :key="s.id" :label="s.name + ' (' + s.domain + ')'" :value="s.id" />
      </el-select>
      <button class="btn btn-primary btn-sm" @click="startCheck" :disabled="checking || !selectedSiteId">
        {{ checking ? '检测中...' : '🔍 开始死链检测' }}
      </button>
      <button class="btn btn-sm" style="background:#FEF3C7;border-color:#F59E0B;color:#92400E" @click="retryFailed" :disabled="checking || !selectedSiteId || summary.total_dead===0" v-if="!checking && summary.total_dead>0">
        🔄 失败重查 ({{ summary.total_dead }} 条)
      </button>
      <button class="btn btn-sm" @click="loadResults" :disabled="!selectedSiteId" v-if="!checking">
        ↻ 刷新结果
      </button>
    </div>

    <div v-if="!selectedSiteId" class="empty-hint panel">
      <div class="empty-icon">🔗</div>
      <div>请先选择一个站点进行死链检测</div>
    </div>

    <template v-else>
      <!-- 检测中提示 -->
      <div v-if="checking" class="panel checking-banner">
        <span class="spin">⏳</span>
        正在抓取所有页面链接并逐一检测... 共发现 <strong>{{ totalLinks }}</strong> 条链接，已检测 <strong>{{ checkedLinks }}</strong> 条
        <div class="check-progress-bar"><div class="check-progress-fill" :style="{width: totalLinks ? (checkedLinks/totalLinks*100)+'%' : '0%'}"></div></div>
      </div>

      <!-- 概览卡片 -->
      <div class="summary-row" v-if="summary.total_all>0">
        <div class="summary-card panel">
          <div class="sc-label">检测链接总数</div>
          <div class="sc-value num-blue">{{ summary.total_all }}</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">死链数量</div>
          <div class="sc-value" :class="summary.total_dead>0?'num-red':'num-green'">{{ summary.total_dead }}</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">重定向链接</div>
          <div class="sc-value num-orange">{{ summary.total_redirect }}</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">正常链接</div>
          <div class="sc-value num-green">{{ summary.total_ok }}</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">最近检测</div>
          <div class="sc-value" style="font-size:13px;font-weight:600;color:var(--ink-soft)">{{ lastCheckedAt || '—' }}</div>
        </div>
      </div>

      <!-- 过滤器 -->
      <div class="filter-bar panel" v-if="summary.total_all>0">
        <button :class="['filter-btn', filter==='all'&&'active']" @click="filter='all';page=1;loadResults()">全部 {{ summary.total_all }}</button>
        <button :class="['filter-btn', filter==='dead'&&'active', 'filter-dead']" @click="filter='dead';page=1;loadResults()">💀 死链 {{ summary.total_dead }}</button>
        <button :class="['filter-btn', filter==='redirect'&&'active']" @click="filter='redirect';page=1;loadResults()">↩️ 重定向 {{ summary.total_redirect }}</button>
        <button :class="['filter-btn', filter==='ok'&&'active']" @click="filter='ok';page=1;loadResults()">✅ 正常 {{ summary.total_ok }}</button>
        <el-input v-model="search" placeholder="搜索 URL..." style="width:280px;margin-left:auto" clearable />
      </div>

      <!-- 结果列表 -->
      <div class="panel result-panel" v-if="results.length" v-loading="loading">
        <table class="table">
          <thead>
            <tr>
              <th style="width:48px">状态</th>
              <th>链接 URL</th>
              <th style="width:120px">来源</th>
              <th style="width:90px">响应时间</th>
              <th style="width:140px">检测时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in displayResults" :key="r.id" :class="rowClass(r.status_code)">
              <td>
                <span :class="['status-dot', statusClass(r.status_code)]">{{ r.status_code || '?' }}</span>
              </td>
              <td class="url-cell">
                <a :href="r.url" target="_blank" rel="noopener noreferrer" class="url-link">{{ r.url }}</a>
              </td>
              <td class="source-cell">{{ r.source_page }}</td>
              <td class="ttfb-cell">{{ r.ttfb_ms ? r.ttfb_ms + 'ms' : '—' }}</td>
              <td class="time-cell">{{ r.checked_at }}</td>
            </tr>
            <tr v-if="!results.length">
              <td colspan="5" class="empty-cell">{{ filter==='dead' ? '🎉 没有发现死链！' : '暂无数据' }}</td>
            </tr>
          </tbody>
        </table>
        <div style="margin-top:12px;text-align:center" v-if="totalCount>perPage">
          <el-pagination v-model:current-page="page" :page-size="perPage" :total="totalCount" layout="prev,pager,next,total" background @current-change="loadResults" />
        </div>
      </div>

      <div v-else-if="!checking" class="empty-hint panel">
        <div class="empty-icon">🔍</div>
        <div>点击「开始死链检测」扫描全站链接</div>
        <div style="font-size:12px;color:var(--ink-hint);margin-top:8px">系统会从「页面管理」中抓取所有页面的链接并逐一检测</div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import apiClient from '../api'

const sites = ref([])
const selectedSiteId = ref(null)
const results = ref([])
const loading = ref(false)
const checking = ref(false)
const filter = ref('all')
const search = ref('')
const totalLinks = ref(0)
const checkedLinks = ref(0)
const lastCheckedAt = ref('')
const page = ref(1)
const perPage = 100
const totalCount = ref(0)
const summary = ref({ total_all: 0, total_dead: 0, total_redirect: 0, total_ok: 0 })
let pollTimer = null
let unmounted = false

const displayResults = computed(() => {
  if (!search.value) return results.value
  const q = search.value.toLowerCase()
  return results.value.filter(r => r.url?.toLowerCase().includes(q))
})

function statusClass(code) {
  if (!code || code === 0) return 'dot-unknown'
  if (code >= 200 && code < 300) return 'dot-ok'
  if (code >= 300 && code < 400) return 'dot-redirect'
  return 'dot-dead'
}
function rowClass(code) {
  if (!code || code >= 400) return 'row-dead'
  if (code >= 300) return 'row-redirect'
  return ''
}

function onSiteChange() { results.value = []; lastCheckedAt.value = ''; summary.value = { total_all:0, total_dead:0, total_redirect:0, total_ok:0 }; page.value = 1; loadResults() }

async function loadResults() {
  if (!selectedSiteId.value) return
  loading.value = true
  try {
    const statusMap = { dead: 'dead', redirect: 'redirect', ok: 'ok' }
    const params = { page: page.value, per_page: perPage }
    if (statusMap[filter.value]) params.status_filter = statusMap[filter.value]
    const res = await apiClient.getDeadLinks(selectedSiteId.value, params)
    const d = res.data
    if (d.items !== undefined) {
      results.value = d.items || []
      totalCount.value = d.total || 0
      summary.value = { total_all: d.total_all||0, total_dead: d.total_dead||0, total_redirect: d.total_redirect||0, total_ok: d.total_ok||0 }
    } else {
      results.value = Array.isArray(d) ? d : []
      totalCount.value = results.value.length
    }
    if (results.value.length) lastCheckedAt.value = results.value[0].checked_at
  } catch (e) { console.error(e) }
  loading.value = false
}

async function retryFailed() {
  if (!selectedSiteId.value) return
  checking.value = true
  totalLinks.value = 0
  checkedLinks.value = 0
  try {
    const res = await apiClient.retryDeadLinks(selectedSiteId.value)
    totalLinks.value = res.data?.retrying || 0
    ElMessage.success('正在重查 ' + totalLinks.value + ' 条死链...')
    pollProgress()
  } catch (e) {
    checking.value = false
    ElMessage.error(e?.response?.data?.error || '重查失败')
  }
}

async function startCheck() {
  if (!selectedSiteId.value) return
  checking.value = true
  totalLinks.value = 0
  checkedLinks.value = 0
  results.value = []
  try {
    await apiClient.startDeadLinkCheck(selectedSiteId.value)
    // Poll for progress
    pollProgress()
  } catch {
    checking.value = false
  }
}

async function pollProgress() {
  if (unmounted) return
  try {
    const res = await apiClient.getDeadLinkProgress(selectedSiteId.value)
    const d = res.data
    totalLinks.value = d.total || 0
    checkedLinks.value = d.checked || 0
    if (d.status === 'done') {
      checking.value = false
      await loadResults()
      ElMessage.success(`检测完成，共 ${summary.value.total_all} 条链接，死链 ${summary.value.total_dead} 条`)
    } else if (!unmounted) {
      pollTimer = setTimeout(pollProgress, 2000)
    }
  } catch {
    checking.value = false
  }
}

onMounted(async () => {
  const res = await apiClient.listSites().catch(() => ({ data: [] }))
  sites.value = res.data || []
  if (sites.value.length) { selectedSiteId.value = sites.value[0].id; loadResults() }
})
onUnmounted(() => { unmounted = true; if (pollTimer) clearTimeout(pollTimer) })
</script>

<style scoped>
.deadlinks-page { display:flex; flex-direction:column; gap:16px; }
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:16px 22px; }
.selector-bar { display:flex; align-items:center; gap:12px; flex-wrap:wrap; }
.selector-label { font-size:13.5px; font-weight:700; color:var(--ink-mid); white-space:nowrap; }

.checking-banner { background:var(--warn-bg); border-color:var(--warn-border); color:var(--warn); font-size:13.5px; font-weight:600; display:flex; flex-direction:column; gap:10px; }
.check-progress-bar { height:6px; background:var(--border-light); border-radius:3px; overflow:hidden; }
.check-progress-fill { height:100%; background:linear-gradient(135deg,var(--sun),var(--coral)); border-radius:3px; transition:width .3s; }

.summary-row { display:grid; grid-template-columns:repeat(5,1fr); gap:12px; }
.summary-card { padding:14px 18px; }
.sc-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.07em; margin-bottom:6px; }
.sc-value { font-size:26px; font-weight:800; font-family:'JetBrains Mono',monospace; }
.num-blue { color:var(--info); } .num-green { color:var(--ok); } .num-red { color:var(--err); } .num-orange { color:var(--warn); }

.filter-bar { display:flex; gap:8px; align-items:center; flex-wrap:wrap; padding:12px 18px; }
.filter-btn { padding:5px 14px; border-radius:20px; border:1.5px solid var(--border-light); background:var(--surface-2); font-size:12.5px; font-weight:600; color:var(--ink-soft); cursor:pointer; font-family:inherit; transition:all .15s; }
.filter-btn.active { background:var(--sun-light); color:var(--sun-deep); border-color:var(--peach); }
.filter-dead.active { background:#FFF0F0; color:var(--err); border-color:#FFCDD2; }

.result-panel { padding:0; overflow:hidden; }
.table { width:100%; border-collapse:collapse; }
.table th { padding:10px 14px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.08em; border-bottom:2px solid var(--border); background:var(--surface-3); }
.table td { padding:10px 14px; border-bottom:1px solid var(--border-light); font-size:12.5px; vertical-align:middle; }
.row-dead td { background:#FFF8F8; }
.row-redirect td { background:#FFFBF0; }

.status-dot { display:inline-block; padding:2px 8px; border-radius:5px; font-size:11px; font-weight:800; font-family:'JetBrains Mono',monospace; }
.dot-ok { background:var(--ok-bg); color:var(--ok); }
.dot-redirect { background:var(--warn-bg); color:var(--warn); }
.dot-dead { background:var(--err-bg); color:var(--err); }
.dot-unknown { background:var(--surface-3); color:var(--ink-hint); }

.url-cell { max-width:400px; } .url-link { color:var(--info); font-size:12px; word-break:break-all; }
.source-cell { font-size:11.5px; color:var(--ink-soft); }
.ttfb-cell { font-family:'JetBrains Mono',monospace; font-size:12px; color:var(--ink-soft); }
.time-cell { font-size:11.5px; color:var(--ink-hint); white-space:nowrap; }
.empty-cell { text-align:center; color:var(--ink-hint); padding:36px !important; font-size:14px; }
.empty-hint { text-align:center; padding:50px; color:var(--ink-hint); }
.empty-icon { font-size:44px; margin-bottom:12px; }
.spin { animation:spin 1.5s linear infinite; display:inline-block; }
@keyframes spin { to { transform:rotate(360deg); } }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 18px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; }
.btn-sm { padding:6px 14px; font-size:12.5px; }
</style>
