<script>
export default { name: 'SpiderManagement' }
</script>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

// ── State ──
const siteSummary = ref([])     // [{site_id, name, domain, hits_7d, spiders_7d, last_visit}]
const selectedSiteId = ref(null)
const overview = ref(null)
const trendRows = ref([])
const trendDays = ref(14)
const logs = ref([])
const logsTotal = ref(0)
const logsPage = ref(1)
const logsPerPage = ref(50)
const logsSpider = ref('')
const loading = ref(false)

const selectedSite = computed(() => siteSummary.value.find(s => s.site_id === selectedSiteId.value))

// ── Loaders ──
async function loadSites() {
  try {
    const res = await api.spiderSiteSummary()
    siteSummary.value = res.data || []
    if (siteSummary.value.length && !selectedSiteId.value) {
      selectedSiteId.value = siteSummary.value[0].site_id
    }
  } catch (e) { console.error(e) }
}

async function loadOverview() {
  if (!selectedSiteId.value) return
  try {
    const res = await api.spiderOverview(selectedSiteId.value)
    overview.value = res.data
  } catch (e) { console.error(e) }
}

async function loadTrend() {
  if (!selectedSiteId.value) return
  try {
    const res = await api.spiderTrend(selectedSiteId.value, { days: trendDays.value })
    trendRows.value = res.data || []
  } catch (e) { console.error(e) }
}

async function loadLogs() {
  if (!selectedSiteId.value) return
  try {
    const res = await api.spiderLogs(selectedSiteId.value, {
      page: logsPage.value,
      per_page: logsPerPage.value,
      spider: logsSpider.value || undefined,
    })
    logs.value = res.data || []
    logsTotal.value = res.total || 0
  } catch (e) { console.error(e) }
}

async function loadAll() {
  loading.value = true
  try {
    await Promise.all([loadOverview(), loadTrend(), loadLogs()])
  } finally { loading.value = false }
}

onMounted(async () => { await loadSites(); await loadAll() })

// Site change → reload everything + reset log pagination
watch(selectedSiteId, () => {
  if (!selectedSiteId.value) return
  logsPage.value = 1
  logsSpider.value = ''
  loadAll()
})
watch(trendDays, loadTrend)
watch([logsPage, logsSpider], loadLogs)

// ── Computed: pivot trend rows ──
const trendPivot = computed(() => {
  const byDate = {}
  const spidersSet = new Set()
  for (const r of trendRows.value) {
    if (!byDate[r.date]) byDate[r.date] = {}
    byDate[r.date][r.spider] = r.hits
    spidersSet.add(r.spider)
  }
  const dates = Object.keys(byDate).sort().reverse()
  const spiders = [...spidersSet].sort()
  return { dates, spiders, byDate }
})

// ── Formatters ──
const fmt = (n) => {
  n = Math.round(n || 0)
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K'
  return n.toLocaleString()
}

const relativeTime = (iso) => {
  if (!iso) return '—'
  const then = new Date(iso.replace(' ', 'T').endsWith('Z') ? iso.replace(' ', 'T') : iso.replace(' ', 'T') + 'Z')
  const diff = (Date.now() - then.getTime()) / 1000
  if (diff < 60) return Math.floor(diff) + ' 秒前'
  if (diff < 3600) return Math.floor(diff / 60) + ' 分钟前'
  if (diff < 86400) return Math.floor(diff / 3600) + ' 小时前'
  if (diff < 2592000) return Math.floor(diff / 86400) + ' 天前'
  return Math.floor(diff / 2592000) + ' 月前'
}

const spiderClass = (name) => {
  const map = {
    Googlebot: 'tag-blue', Bingbot: 'tag-teal', Baiduspider: 'tag-red',
    YandexBot: 'tag-orange', Sogou: 'tag-purple', '360Spider': 'tag-green',
    Bytespider: 'tag-pink', GPTBot: 'tag-dark', ClaudeBot: 'tag-dark',
    PerplexityBot: 'tag-dark',
  }
  return map[name] || 'tag-gray'
}
</script>

<template>
  <div class="spider-page">
    <!-- Site selector + current site summary -->
    <div class="panel toolbar">
      <span class="toolbar-label">选择站点</span>
      <el-select v-model="selectedSiteId" placeholder="请选择站点" style="width:320px">
        <el-option
          v-for="s in siteSummary"
          :key="s.site_id"
          :label="`${s.name} — ${s.domain}`"
          :value="s.site_id"
        >
          <span>{{ s.name }} · {{ s.domain }}</span>
          <span class="opt-hits">{{ fmt(s.hits_7d) }} 次</span>
        </el-option>
      </el-select>
      <div v-if="selectedSite" class="site-meta">
        <span class="meta-chip">最近访问：{{ relativeTime(selectedSite.last_visit) }}</span>
        <span class="meta-chip">近 7 天活跃蜘蛛：{{ selectedSite.spiders_7d }}</span>
      </div>
      <button class="btn btn-sm" @click="loadAll" :disabled="loading" style="margin-left:auto">
        {{ loading ? '加载中...' : '↻ 刷新' }}
      </button>
    </div>

    <!-- Empty state -->
    <div v-if="!selectedSiteId" class="panel empty-hint">
      <div class="empty-icon">🕷️</div>
      <div>请先选择站点查看蜘蛛数据</div>
    </div>

    <!-- Site-scoped stats -->
    <template v-else-if="overview">
      <div class="stat-row">
        <div class="stat-card panel">
          <div class="stat-label">今日 ({{ selectedSite?.domain }})</div>
          <div class="stat-value sv-blue">{{ fmt(overview.total_24h) }}</div>
        </div>
        <div class="stat-card panel">
          <div class="stat-label">近 7 天</div>
          <div class="stat-value sv-green">{{ fmt(overview.total_7d) }}</div>
        </div>
        <div class="stat-card panel">
          <div class="stat-label">近 30 天</div>
          <div class="stat-value sv-orange">{{ fmt(overview.total_30d) }}</div>
        </div>
        <div class="stat-card panel">
          <div class="stat-label">活跃蜘蛛</div>
          <div class="stat-value sv-purple">{{ overview.by_spider?.length || 0 }}</div>
        </div>
      </div>

      <!-- Spider breakdown + last visit -->
      <div class="two-col">
        <div class="panel">
          <div class="section-title">蜘蛛分布（近 7 天）</div>
          <table class="table" v-if="overview.by_spider?.length">
            <thead><tr><th>蜘蛛</th><th>请求数</th><th>占比</th></tr></thead>
            <tbody>
              <tr v-for="r in overview.by_spider" :key="r.spider">
                <td><span class="spider-tag" :class="spiderClass(r.spider)">{{ r.spider }}</span></td>
                <td class="num sv-blue">{{ fmt(r.hits) }}</td>
                <td class="num sv-purple">
                  {{ overview.total_7d ? ((r.hits / overview.total_7d * 100).toFixed(1) + '%') : '—' }}
                </td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty-hint small">此站点近 7 天暂无蜘蛛访问</div>
        </div>
        <div class="panel">
          <div class="section-title">各蜘蛛最后访问</div>
          <table class="table" v-if="overview.last_visit_by_spider?.length">
            <thead><tr><th>蜘蛛</th><th>最后访问</th></tr></thead>
            <tbody>
              <tr v-for="r in overview.last_visit_by_spider" :key="r.spider">
                <td><span class="spider-tag" :class="spiderClass(r.spider)">{{ r.spider }}</span></td>
                <td class="dc">{{ relativeTime(r.at) }}</td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty-hint small">暂无访问历史</div>
        </div>
      </div>

      <!-- Top crawled URLs -->
      <div class="panel" v-if="overview.top_url_7d?.length">
        <div class="section-title">最常被抓取的 URL（近 7 天，Top 10）</div>
        <table class="table">
          <thead><tr><th>URL</th><th style="width:120px">请求数</th></tr></thead>
          <tbody>
            <tr v-for="u in overview.top_url_7d" :key="u.url">
              <td class="path-cell" :title="u.url">{{ u.url }}</td>
              <td class="num sv-blue">{{ fmt(u.hits) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Daily trend -->
      <div class="panel">
        <div class="trend-head">
          <div class="section-title" style="margin:0;padding:0;border:0">每日趋势</div>
          <el-select v-model="trendDays" style="width:120px">
            <el-option :label="'近 7 天'" :value="7" />
            <el-option :label="'近 14 天'" :value="14" />
            <el-option :label="'近 30 天'" :value="30" />
            <el-option :label="'近 90 天'" :value="90" />
          </el-select>
        </div>
        <div class="table-wrap" v-if="trendPivot.dates.length">
          <table class="table">
            <thead>
              <tr>
                <th>日期</th>
                <th v-for="s in trendPivot.spiders" :key="s">
                  <span class="spider-tag" :class="spiderClass(s)">{{ s }}</span>
                </th>
                <th>合计</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="d in trendPivot.dates" :key="d">
                <td class="dc">{{ d }}</td>
                <td v-for="s in trendPivot.spiders" :key="s" class="num sv-blue">
                  {{ trendPivot.byDate[d][s] ? fmt(trendPivot.byDate[d][s]) : '—' }}
                </td>
                <td class="num sv-purple">
                  {{ fmt(Object.values(trendPivot.byDate[d]).reduce((a, b) => a + b, 0)) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="empty-hint small">暂无趋势数据</div>
      </div>

      <!-- Raw logs (this site) -->
      <div class="panel">
        <div class="trend-head">
          <div class="section-title" style="margin:0;padding:0;border:0">请求日志</div>
          <el-select v-model="logsSpider" placeholder="全部蜘蛛" clearable style="width:150px">
            <el-option label="全部" value="" />
            <el-option v-for="s in overview.by_spider" :key="s.spider" :label="s.spider" :value="s.spider" />
          </el-select>
        </div>
        <div class="table-wrap" v-if="logs.length">
          <table class="table">
            <thead>
              <tr><th>时间</th><th>蜘蛛</th><th>URL</th><th>状态</th><th>TTFB</th><th>IP</th></tr>
            </thead>
            <tbody>
              <tr v-for="l in logs" :key="l.id">
                <td class="dc">{{ l.visited_at.replace('T', ' ').replace('Z', '') }}</td>
                <td><span class="spider-tag" :class="spiderClass(l.spider)">{{ l.spider }}</span></td>
                <td class="path-cell" :title="l.url">{{ l.url }}</td>
                <td class="num" :class="{ 'sv-green': l.status_code < 300, 'sv-orange': l.status_code >= 300 && l.status_code < 400, 'sv-red': l.status_code >= 400 }">
                  {{ l.status_code || '—' }}
                </td>
                <td class="num">{{ l.ttfb_ms }}ms</td>
                <td class="mono-cell">{{ l.ip || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="empty-hint small">
          此站点暂无蜘蛛日志。
          <div class="hint-ingest">
            <div>接入方式：向 <code>/api/spider/ingest</code> POST JSON（header 带 <code>X-API-Key</code>）。</div>
            <div>Key 在系统管理 → <code>spider_ingest_api_key</code> 配置；请求体带 <code>site_domain</code> 或 <code>site_id</code>。</div>
          </div>
        </div>
        <div class="pager" v-if="logsTotal > logsPerPage">
          <button class="btn btn-sm" :disabled="logsPage<=1" @click="logsPage--">上一页</button>
          <span class="pager-txt">第 {{ logsPage }} / {{ Math.ceil(logsTotal / logsPerPage) }} 页 · 共 {{ logsTotal }} 条</span>
          <button class="btn btn-sm" :disabled="logsPage * logsPerPage >= logsTotal" @click="logsPage++">下一页</button>
        </div>
      </div>
    </template>

    <!-- All-sites quick overview (below the active site view) -->
    <div class="panel" v-if="siteSummary.length">
      <div class="section-title">所有站点近 7 天蜘蛛总览</div>
      <table class="table">
        <thead>
          <tr><th>站点</th><th>蜘蛛种类</th><th>请求数</th><th>最近访问</th><th></th></tr>
        </thead>
        <tbody>
          <tr
            v-for="s in siteSummary" :key="s.site_id"
            :class="{ 'row-active': s.site_id === selectedSiteId }"
          >
            <td class="site-cell">{{ s.name }} <span class="site-dom">{{ s.domain }}</span></td>
            <td class="num">{{ s.spiders_7d }}</td>
            <td class="num sv-blue">{{ fmt(s.hits_7d) }}</td>
            <td class="dc">{{ relativeTime(s.last_visit) }}</td>
            <td>
              <button
                v-if="s.site_id !== selectedSiteId"
                class="btn btn-sm"
                @click="selectedSiteId = s.site_id"
              >查看</button>
              <span v-else class="now-viewing">当前</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.spider-page { display: flex; flex-direction: column; gap: 16px; }
.panel { background: var(--surface); border-radius: var(--r); box-shadow: var(--s1); border: 1.5px solid var(--border-light); padding: 16px 22px; }

.toolbar { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; }
.toolbar-label { font-size: 13px; font-weight: 700; color: var(--ink-mid); white-space: nowrap; }
.site-meta { display: flex; gap: 8px; flex-wrap: wrap; }
.meta-chip { font-size: 12px; font-weight: 600; padding: 4px 10px; background: var(--surface-3); border-radius: 6px; color: var(--ink-mid); }
.opt-hits { float: right; font-size: 11px; color: var(--ink-hint); font-weight: 600; font-family: 'JetBrains Mono', monospace; }

.stat-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; }
.stat-card { padding: 14px 18px; }
.stat-label { font-size: 11px; font-weight: 700; color: var(--ink-hint); text-transform: uppercase; letter-spacing: .07em; margin-bottom: 6px; }
.stat-value { font-size: 24px; font-weight: 800; font-family: 'JetBrains Mono', monospace; }
.sv-blue { color: var(--info); } .sv-green { color: var(--ok); } .sv-orange { color: var(--warn); } .sv-purple { color: #7C3AED; } .sv-red { color: var(--err); }

.section-title { font-size: 13px; font-weight: 700; color: var(--ink-mid); margin: 0 0 10px; padding-bottom: 6px; border-bottom: 1.5px solid var(--border-light); }
.two-col { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }

.trend-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; gap: 12px; flex-wrap: wrap; }
.table-wrap { overflow-x: auto; }

.table { width: 100%; border-collapse: collapse; }
.table th { padding: 9px 12px; text-align: left; font-size: 11px; font-weight: 800; color: var(--ink-mid); text-transform: uppercase; letter-spacing: .07em; border-bottom: 2px solid var(--border); background: var(--surface-3); white-space: nowrap; }
.table td { padding: 9px 12px; border-bottom: 1px solid var(--border-light); font-size: 12.5px; }
.table tr:hover td { background: var(--sun-light); }
.table tr.row-active td { background: var(--sun-light); font-weight: 600; }
.dc { font-weight: 600; color: var(--ink); font-size: 12px; white-space: nowrap; }
.num { font-family: 'JetBrains Mono', monospace; font-size: 12px; font-weight: 700; text-align: right; }
.site-cell { font-weight: 600; }
.site-dom { font-size: 11px; color: var(--ink-hint); font-weight: 500; margin-left: 6px; }
.path-cell { font-size: 12px; color: var(--ink-soft); max-width: 380px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: 'JetBrains Mono', monospace; }
.mono-cell { font-family: 'JetBrains Mono', monospace; font-size: 11.5px; color: var(--ink-soft); }
.now-viewing { font-size: 11px; color: var(--sun-deep); font-weight: 700; }

.spider-tag { display: inline-block; padding: 2px 9px; border-radius: 10px; font-size: 11px; font-weight: 700; color: #fff; }
.tag-blue { background: #2563EB; }
.tag-teal { background: #0D9488; }
.tag-red { background: #DC2626; }
.tag-orange { background: #EA580C; }
.tag-purple { background: #7C3AED; }
.tag-green { background: #16A34A; }
.tag-pink { background: #DB2777; }
.tag-dark { background: #111827; }
.tag-gray { background: #6B7280; }

.empty-hint { text-align: center; color: var(--ink-hint); padding: 50px 20px; font-size: 14px; line-height: 1.8; }
.empty-hint.small { padding: 20px; font-size: 13px; }
.empty-icon { font-size: 44px; margin-bottom: 12px; }
.hint-ingest { margin-top: 12px; font-size: 12px; color: var(--ink-soft); line-height: 1.8; }
.hint-ingest code { background: var(--surface-2); padding: 2px 6px; border-radius: 4px; font-family: 'JetBrains Mono', monospace; color: var(--sun-deep); }

.btn { display: inline-flex; align-items: center; gap: 6px; padding: 7px 16px; border-radius: 9px; border: 1.5px solid var(--border); background: var(--surface); cursor: pointer; font-family: inherit; font-size: 13px; font-weight: 700; color: var(--ink-mid); transition: all .15s; }
.btn:hover { background: var(--sun-light); border-color: var(--peach); color: var(--sun-deep); }
.btn:disabled { opacity: .55; cursor: not-allowed; }
.btn-sm { padding: 6px 14px; font-size: 12.5px; }

.pager { display: flex; justify-content: center; align-items: center; gap: 12px; margin-top: 14px; padding-top: 12px; border-top: 1.5px dashed var(--border-light); }
.pager-txt { font-size: 12px; color: var(--ink-soft); }

@media (max-width: 900px) {
  .two-col { grid-template-columns: 1fr; }
  .stat-row { grid-template-columns: repeat(2, 1fr); }
}
</style>
