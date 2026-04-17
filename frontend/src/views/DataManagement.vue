<template>
<div class="dm-page">
  <!-- Site selector + actions -->
  <div class="panel toolbar">
    <span class="toolbar-label">选择站点</span>
    <el-select v-model="selectedSiteId" placeholder="请选择站点" style="width:280px" @change="onSiteChange" :loading="sitesLoading">
      <el-option v-for="s in sites" :key="s.id" :label="s.name + ' (' + s.domain + ')'" :value="s.id" />
    </el-select>
    <div class="tab-pills">
      <button :class="['pill', activeTab==='gsc'&&'active']" @click="activeTab='gsc'">GSC</button>
      <button :class="['pill', activeTab==='ga'&&'active']" @click="activeTab='ga'">GA4</button>
      <button :class="['pill', activeTab==='bing'&&'active']" @click="activeTab='bing'">Bing</button>
      <button :class="['pill', activeTab==='yandex'&&'active']" @click="activeTab='yandex'">Yandex</button>
    </div>
    <button class="btn btn-primary btn-sm" @click="syncCurrent" :disabled="syncing" style="margin-left:auto">
      {{ syncing ? '同步中...' : '↻ 同步当前数据源' }}
    </button>
    <button class="btn btn-sm" @click="loadSiteData" :disabled="!selectedSiteId||loading">↻ 刷新</button>
  </div>

  <!-- Empty state -->
  <div v-if="!selectedSiteId" class="panel empty-hint">
    <div class="empty-icon">📡</div>
    <div>请选择一个站点查看数据</div>
  </div>

  <!-- Loading -->
  <div v-else-if="loading" class="panel empty-hint">
    <div class="spin">⏳</div>
    <div>正在加载数据...</div>
  </div>

  <!-- Content -->
  <template v-else>
    <!-- GSC Tab -->
    <template v-if="activeTab==='gsc'">
      <div class="stat-row" v-if="gscData.length">
        <div class="stat-card panel"><div class="stat-label">总点击</div><div class="stat-value sv-blue">{{ fmt(gscData[0]?.clicks) }}</div></div>
        <div class="stat-card panel"><div class="stat-label">总展示</div><div class="stat-value sv-green">{{ fmt(gscData[0]?.impressions) }}</div></div>
        <div class="stat-card panel"><div class="stat-label">平均 CTR</div><div class="stat-value sv-orange">{{ ((gscData[0]?.ctr||0)*100).toFixed(2) }}%</div></div>
        <div class="stat-card panel"><div class="stat-label">平均排名</div><div class="stat-value sv-purple">{{ (gscData[0]?.position||0).toFixed(1) }}</div></div>
      </div>
      <div class="panel" v-if="gscData.length">
        <div class="section-title">趋势数据（最近 {{ gscData.length }} 次同步）</div>
        <table class="table">
          <thead><tr><th>日期</th><th>点击</th><th>展示</th><th>CTR</th><th>排名</th><th>热门关键词</th></tr></thead>
          <tbody>
            <tr v-for="d in gscData" :key="d.date">
              <td class="dc">{{ d.date }}</td>
              <td class="num sv-blue">{{ fmt(d.clicks) }}</td>
              <td class="num sv-green">{{ fmt(d.impressions) }}</td>
              <td class="num sv-orange">{{ ((d.ctr||0)*100).toFixed(2) }}%</td>
              <td class="num sv-purple">{{ (d.position||0).toFixed(1) }}</td>
              <td class="kw-cell">{{ topKW(d.keywords_json) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="panel empty-hint">暂无 GSC 数据，请点击「同步当前数据源」拉取</div>
    </template>

    <!-- GA4 Tab -->
    <template v-if="activeTab==='ga'">
      <div class="stat-row" v-if="gaData.length">
        <div class="stat-card panel"><div class="stat-label">会话数</div><div class="stat-value sv-blue">{{ fmt(gaData[0]?.sessions) }}</div></div>
        <div class="stat-card panel"><div class="stat-label">用户数</div><div class="stat-value sv-green">{{ fmt(gaData[0]?.users) }}</div></div>
        <div class="stat-card panel"><div class="stat-label">页面浏览</div><div class="stat-value sv-orange">{{ fmt(gaData[0]?.pageviews) }}</div></div>
        <div class="stat-card panel"><div class="stat-label">跳出率</div><div class="stat-value sv-purple">{{ ((gaData[0]?.bounce_rate||0)*100).toFixed(1) }}%</div></div>
        <div class="stat-card panel"><div class="stat-label">平均时长</div><div class="stat-value" style="color:var(--info)">{{ fmtDur(gaData[0]?.avg_duration) }}</div></div>
      </div>
      <div class="two-col" v-if="gaData.length">
        <div class="panel">
          <div class="section-title">流量渠道</div>
          <table class="table" v-if="gaChannels.length"><thead><tr><th>渠道</th><th>会话</th><th>用户</th><th>浏览</th></tr></thead><tbody>
            <tr v-for="ch in gaChannels" :key="ch.dim"><td><span class="channel-badge">{{ ch.dim }}</span></td><td class="num sv-blue">{{ fmt(ch.sessions) }}</td><td class="num sv-green">{{ fmt(ch.users) }}</td><td class="num sv-orange">{{ fmt(ch.pageviews) }}</td></tr>
          </tbody></table>
          <div v-else class="empty-hint small">暂无渠道数据</div>
        </div>
        <div class="panel">
          <div class="section-title">热门页面 Top 20</div>
          <table class="table" v-if="gaPages.length"><thead><tr><th>页面路径</th><th>浏览</th><th>用户</th><th>跳出率</th></tr></thead><tbody>
            <tr v-for="pg in gaPages" :key="pg.path"><td class="path-cell" :title="pg.path">{{ pg.path }}</td><td class="num sv-orange">{{ fmt(pg.views) }}</td><td class="num sv-green">{{ fmt(pg.users) }}</td><td class="num sv-purple">{{ (pg.bounceRate*100).toFixed(1) }}%</td></tr>
          </tbody></table>
          <div v-else class="empty-hint small">暂无页面数据</div>
        </div>
      </div>
      <div class="panel" v-if="gaData.length" style="margin-top:16px">
        <div class="section-title">同步历史</div>
        <table class="table"><thead><tr><th>日期</th><th>会话</th><th>用户</th><th>页面浏览</th><th>跳出率</th><th>平均时长</th></tr></thead><tbody>
          <tr v-for="d in gaData" :key="d.date"><td class="dc">{{ d.date }}</td><td class="num sv-blue">{{ fmt(d.sessions) }}</td><td class="num sv-green">{{ fmt(d.users) }}</td><td class="num sv-orange">{{ fmt(d.pageviews) }}</td><td class="num sv-purple">{{ ((d.bounce_rate||0)*100).toFixed(1) }}%</td><td class="num" style="color:var(--info)">{{ fmtDur(d.avg_duration) }}</td></tr>
        </tbody></table>
      </div>
      <div v-if="!gaData.length" class="panel empty-hint">暂无 GA4 数据。请在站点详情配置 GA4 Property ID，完成 OAuth 授权后点击「同步」</div>
    </template>

    <!-- Bing Tab -->
    <template v-if="activeTab==='bing'">
      <div class="stat-row" v-if="bingData.length">
        <div class="stat-card panel"><div class="stat-label">点击数</div><div class="stat-value sv-blue">{{ fmt(bingData[0]?.clicks) }}</div></div>
        <div class="stat-card panel"><div class="stat-label">展示数</div><div class="stat-value sv-green">{{ fmt(bingData[0]?.impressions) }}</div></div>
        <div class="stat-card panel"><div class="stat-label">CTR</div><div class="stat-value sv-orange">{{ ((bingData[0]?.ctr||0)*100).toFixed(2) }}%</div></div>
        <div class="stat-card panel"><div class="stat-label">平均排名</div><div class="stat-value sv-purple">{{ (bingData[0]?.avg_position||0).toFixed(1) }}</div></div>
        <div class="stat-card panel"><div class="stat-label">收录数</div><div class="stat-value" style="color:var(--info)">{{ fmt(bingData[0]?.index_count) }}</div></div>
      </div>
      <div class="two-col" v-if="bingData.length">
        <div class="panel">
          <div class="section-title">热门关键词</div>
          <table class="table" v-if="bingQueries.length"><thead><tr><th>关键词</th><th>点击</th><th>展示</th><th>CTR</th><th>排名</th></tr></thead><tbody>
            <tr v-for="q in bingQueries" :key="q.Query"><td class="kw-cell">{{ q.Query }}</td><td class="num sv-blue">{{ fmt(q.Clicks) }}</td><td class="num sv-green">{{ fmt(q.Impressions) }}</td><td class="num sv-orange">{{ ((q.CTR||0)*100).toFixed(2) }}%</td><td class="num sv-purple">{{ (q.AvgImpPos||0).toFixed(1) }}</td></tr>
          </tbody></table>
          <div v-else class="empty-hint small">暂无数据</div>
        </div>
        <div class="panel">
          <div class="section-title">热门页面</div>
          <table class="table" v-if="bingPages.length"><thead><tr><th>页面</th><th>点击</th><th>展示</th><th>CTR</th></tr></thead><tbody>
            <tr v-for="p in bingPages" :key="p.Url"><td class="path-cell" :title="p.Url">{{ (p.Url||'').replace(/^https?:\/\/[^/]+/,'') || p.Url }}</td><td class="num sv-blue">{{ fmt(p.Clicks) }}</td><td class="num sv-green">{{ fmt(p.Impressions) }}</td><td class="num sv-orange">{{ ((p.CTR||0)*100).toFixed(2) }}%</td></tr>
          </tbody></table>
          <div v-else class="empty-hint small">暂无数据</div>
        </div>
      </div>
      <div class="panel" v-if="bingData.length" style="margin-top:16px">
        <div class="section-title">同步历史</div>
        <table class="table"><thead><tr><th>日期</th><th>点击</th><th>展示</th><th>CTR</th><th>排名</th><th>收录数</th></tr></thead><tbody>
          <tr v-for="d in bingData" :key="d.date"><td class="dc">{{ d.date }}</td><td class="num sv-blue">{{ fmt(d.clicks) }}</td><td class="num sv-green">{{ fmt(d.impressions) }}</td><td class="num sv-orange">{{ ((d.ctr||0)*100).toFixed(2) }}%</td><td class="num sv-purple">{{ (d.avg_position||0).toFixed(1) }}</td><td class="num" style="color:var(--info)">{{ fmt(d.index_count) }}</td></tr>
        </tbody></table>
      </div>
      <div v-if="!bingData.length" class="panel empty-hint">暂无 Bing 数据。请在站点详情填写「Bing Site URL」，系统设置配置「Bing API Key」后点击同步</div>
    </template>

    <!-- Yandex Tab -->
    <template v-if="activeTab==='yandex'">
      <div class="stat-row" v-if="yandexData.length">
        <div class="stat-card panel"><div class="stat-label">点击数</div><div class="stat-value sv-blue">{{ fmt(yandexData[0]?.clicks) }}</div></div>
        <div class="stat-card panel"><div class="stat-label">展示数</div><div class="stat-value sv-green">{{ fmt(yandexData[0]?.impressions) }}</div></div>
        <div class="stat-card panel"><div class="stat-label">收录数</div><div class="stat-value sv-purple">{{ fmt(yandexData[0]?.index_count) }}</div></div>
      </div>
      <div class="two-col" v-if="yandexData.length">
        <div class="panel">
          <div class="section-title">热门关键词</div>
          <table class="table" v-if="yandexKeywords.length"><thead><tr><th>关键词</th><th>点击</th><th>展示</th><th>排名</th></tr></thead><tbody>
            <tr v-for="k in yandexKeywords" :key="k.query_text"><td class="kw-cell">{{ k.query_text }}</td><td class="num sv-blue">{{ fmt(k.indicators?.TOTAL_CLICKS||0) }}</td><td class="num sv-green">{{ fmt(k.indicators?.TOTAL_SHOWS||0) }}</td><td class="num sv-purple">{{ (k.indicators?.AVG_SHOW_POSITION||0).toFixed(1) }}</td></tr>
          </tbody></table>
          <div v-else class="empty-hint small">暂无关键词数据</div>
        </div>
        <div class="panel">
          <div class="section-title">同步历史</div>
          <table class="table"><thead><tr><th>日期</th><th>点击</th><th>展示</th><th>收录数</th></tr></thead><tbody>
            <tr v-for="d in yandexData" :key="d.date"><td class="dc">{{ d.date }}</td><td class="num sv-blue">{{ fmt(d.clicks) }}</td><td class="num sv-green">{{ fmt(d.impressions) }}</td><td class="num sv-purple">{{ fmt(d.index_count) }}</td></tr>
          </tbody></table>
        </div>
      </div>
      <div v-if="!yandexData.length" class="panel empty-hint">暂无 Yandex 数据，请配置 Yandex Host ID 并完成授权后同步</div>
    </template>
  </template>
</div>
</template>

<script>
// Named export so <KeepAlive include="DataManagement"> in Layout.vue can
// match and preserve this view's state across route changes.
export default { name: 'DataManagement' }
</script>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

// ── State ──
const sites = ref([])
const sitesLoading = ref(false)
const selectedSiteId = ref(null)
const activeTab = ref('gsc')
const loading = ref(false)
const syncing = ref(false)

const gscData = ref([])
const gaData = ref([])
const bingData = ref([])
const yandexData = ref([])

// ── Cached data per site to avoid re-fetching ──
const cache = {}

// ── Formatters ──
const fmt = (n) => { n = Math.round(n || 0); if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'; if (n >= 1000) return (n / 1000).toFixed(1) + 'K'; return n.toLocaleString() }
const fmtDur = (s) => { s = Math.round(s || 0); if (s < 60) return s + 's'; return Math.floor(s / 60) + 'm' + (s % 60) + 's' }

const topKW = (kwJson) => {
  try {
    const d = typeof kwJson === 'string' ? JSON.parse(kwJson) : kwJson
    const rows = d?.rows || d || []
    if (!Array.isArray(rows)) return '—'
    return rows.slice(0, 3).map(r => r.keys?.[0] || '').filter(Boolean).join(', ') || '—'
  } catch { return '—' }
}

// ── Computed: parse JSON sub-data ──
const gaChannels = computed(() => {
  try { const rows = (typeof gaData.value[0]?.channels_json === 'string' ? JSON.parse(gaData.value[0].channels_json) : gaData.value[0]?.channels_json)?.rows || []; return rows.map(r => ({ dim: r.dimensionValues?.[0]?.value || '—', sessions: +r.metricValues?.[0]?.value || 0, users: +r.metricValues?.[1]?.value || 0, pageviews: +r.metricValues?.[2]?.value || 0 })).sort((a, b) => b.sessions - a.sessions) } catch { return [] }
})
const gaPages = computed(() => {
  try { const rows = (typeof gaData.value[0]?.pages_json === 'string' ? JSON.parse(gaData.value[0].pages_json) : gaData.value[0]?.pages_json)?.rows || []; return rows.map(r => ({ path: r.dimensionValues?.[0]?.value || '—', views: +r.metricValues?.[0]?.value || 0, users: +r.metricValues?.[1]?.value || 0, bounceRate: +r.metricValues?.[3]?.value || 0 })) } catch { return [] }
})
const bingQueries = computed(() => {
  try { const r = typeof bingData.value[0]?.queries_json === 'string' ? JSON.parse(bingData.value[0].queries_json) : bingData.value[0]?.queries_json; return r?.d?.Results || [] } catch { return [] }
})
const bingPages = computed(() => {
  try { const r = typeof bingData.value[0]?.pages_json === 'string' ? JSON.parse(bingData.value[0].pages_json) : bingData.value[0]?.pages_json; return r?.d?.Results || [] } catch { return [] }
})
const yandexKeywords = computed(() => {
  try { const r = typeof yandexData.value[0]?.keywords_json === 'string' ? JSON.parse(yandexData.value[0].keywords_json) : yandexData.value[0]?.keywords_json; return Array.isArray(r) ? r.slice(0, 20) : [] } catch { return [] }
})

// ── Data loading with cache ──
async function loadSiteData(forceRefresh = false) {
  if (!selectedSiteId.value) return
  const id = selectedSiteId.value

  // Use cache if available and not forcing refresh
  if (!forceRefresh && cache[id]) {
    gscData.value = cache[id].gsc
    gaData.value = cache[id].ga
    bingData.value = cache[id].bing
    yandexData.value = cache[id].yandex
    return
  }

  loading.value = true
  try {
    const [g, a, b, y] = await Promise.all([
      api.getGSCData(id, 30).catch(() => ({ data: [] })),
      api.getGAData(id).catch(() => ({ data: [] })),
      api.getBingData(id).catch(() => ({ data: [] })),
      api.getYandexData(id).catch(() => ({ data: [] })),
    ])
    gscData.value = g.data || []
    gaData.value = a.data || []
    bingData.value = b.data || []
    yandexData.value = y.data || []

    // Cache it
    cache[id] = { gsc: gscData.value, ga: gaData.value, bing: bingData.value, yandex: yandexData.value }
  } catch (e) {
    console.error(e)
    ElMessage.error('加载数据失败')
  }
  loading.value = false
}

function onSiteChange() {
  gscData.value = []; gaData.value = []; bingData.value = []; yandexData.value = []
  loadSiteData()
}

async function syncCurrent() {
  if (!selectedSiteId.value) return
  syncing.value = true
  const id = selectedSiteId.value
  const tab = activeTab.value
  try {
    if (tab === 'gsc') { await api.syncGSC(id); ElMessage.success('GSC 同步成功') }
    else if (tab === 'ga') { await api.syncGA(id); ElMessage.success('GA4 同步成功') }
    else if (tab === 'bing') { await api.syncBing(id); ElMessage.success('Bing 同步成功') }
    else if (tab === 'yandex') { await api.syncYandex(id); ElMessage.success('Yandex 同步成功') }
    // Clear cache and reload
    delete cache[id]
    await loadSiteData(true)
  } catch (e) {
    console.error(e)
    ElMessage.error('同步失败')
  }
  syncing.value = false
}

onMounted(async () => {
  sitesLoading.value = true
  try {
    const res = await api.listSites()
    sites.value = res.data || []
    if (sites.value.length) {
      selectedSiteId.value = sites.value[0].id
      loadSiteData()
    }
  } catch (e) {
    console.error(e)
    ElMessage.error('加载站点列表失败')
  }
  sitesLoading.value = false
})
</script>

<style scoped>
.dm-page { display:flex; flex-direction:column; gap:16px; }
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:16px 22px; }

/* Toolbar */
.toolbar { display:flex; align-items:center; gap:12px; flex-wrap:wrap; }
.toolbar-label { font-size:13px; font-weight:700; color:var(--ink-mid); white-space:nowrap; }
.tab-pills { display:flex; gap:4px; margin-left:12px; }
.pill { padding:5px 14px; border-radius:20px; border:1.5px solid var(--border-light); background:var(--surface-2); font-size:12.5px; font-weight:700; color:var(--ink-soft); cursor:pointer; font-family:inherit; transition:all .15s; }
.pill.active { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.pill:hover:not(.active) { background:var(--sun-light); color:var(--sun-deep); border-color:var(--peach); }

/* Stat cards */
.stat-row { display:grid; grid-template-columns:repeat(auto-fit, minmax(140px, 1fr)); gap:12px; }
.stat-card { padding:14px 18px; }
.stat-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.07em; margin-bottom:6px; }
.stat-value { font-size:24px; font-weight:800; font-family:'JetBrains Mono',monospace; }
.sv-blue { color:var(--info); } .sv-green { color:var(--ok); } .sv-orange { color:var(--warn); } .sv-purple { color:#7C3AED; }

/* Section */
.section-title { font-size:13px; font-weight:700; color:var(--ink-mid); margin:12px 0 10px; padding-bottom:6px; border-bottom:1.5px solid var(--border-light); }
.two-col { display:grid; grid-template-columns:1fr 1fr; gap:16px; }

/* Table */
.table { width:100%; border-collapse:collapse; }
.table th { padding:9px 12px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.07em; border-bottom:2px solid var(--border); background:var(--surface-3); }
.table td { padding:9px 12px; border-bottom:1px solid var(--border-light); font-size:12.5px; }
.table tr:hover td { background:var(--sun-light); }
.dc { font-weight:600; color:var(--ink); font-size:12px; white-space:nowrap; }
.num { font-family:'JetBrains Mono',monospace; font-size:12px; font-weight:700; text-align:right; }
.kw-cell { font-size:12px; color:var(--ink); max-width:220px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.path-cell { font-size:11.5px; color:var(--ink-soft); max-width:240px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-family:'JetBrains Mono',monospace; }
.channel-badge { display:inline-block; padding:2px 8px; background:var(--sun-light); color:var(--sun-deep); border-radius:5px; font-size:11.5px; font-weight:600; }

/* Empty */
.empty-hint { text-align:center; color:var(--ink-hint); padding:50px 20px; font-size:14px; line-height:1.8; }
.empty-hint.small { padding:20px; font-size:13px; }
.empty-icon { font-size:44px; margin-bottom:12px; }

/* Buttons */
.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 18px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; }
.btn-sm { padding:6px 14px; font-size:12.5px; }

.spin { animation:spin 1.5s linear infinite; display:inline-block; font-size:36px; margin-bottom:12px; }
@keyframes spin { to { transform:rotate(360deg); } }

@media (max-width:900px) {
  .two-col { grid-template-columns:1fr; }
  .toolbar { flex-direction:column; align-items:stretch; }
  .tab-pills { margin-left:0; }
}
</style>
