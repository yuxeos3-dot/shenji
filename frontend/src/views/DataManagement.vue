<template>
  <div class="data-mgmt">
    <!-- Top toolbar -->
    <div class="panel toolbar">
      <div class="tb-row">
        <div class="tb-tabs">
          <button v-for="p in platforms" :key="p.code" :class="['pill', platform === p.code && 'active']" @click="switchPlatform(p.code)">{{ p.label }}</button>
        </div>
        <div class="tb-right" v-if="view === 'overview'">
          <button class="btn btn-refresh" @click="loadOverview()" :disabled="loading">{{ loading ? '加载中...' : '🔄 刷新' }}</button>
        </div>
      </div>
    </div>

    <!-- Overview: site cards -->
    <div v-if="view === 'overview'" class="view-overview">
      <div class="section-title">{{ platformLabel }} · 总览
        <span class="badge" v-if="overview.cards.length">{{ overview.cards.length }} 个站点</span>
      </div>
      <div v-if="loading && !overview.cards.length" class="state-placeholder">加载中...</div>
      <div v-else-if="error" class="state-error">❌ {{ error }}</div>
      <div v-else-if="!overview.cards.length" class="state-empty">暂无站点。请先在「系统设置」里授权对应平台。</div>
      <div v-else class="card-grid">
        <div v-for="(c, i) in overview.cards" :key="i" :class="['site-card', c.error && 'card-err']" @click="openDetail(c)">
          <div class="card-name">{{ c.display_name || c.site_url || c.host_id || c.property_id }}</div>
          <div v-if="c.error" class="card-err-msg">{{ c.error }}</div>
          <div v-else class="card-kpis">
            <template v-if="platform !== 'ga4'">
              <div class="ck"><span class="ck-l">点击</span><span class="ck-v sv-blue">{{ fmt(c.clicks) }}</span></div>
              <div class="ck"><span class="ck-l">展示</span><span class="ck-v sv-green">{{ fmt(c.impressions || c.shows) }}</span></div>
              <div class="ck"><span class="ck-l">CTR</span><span class="ck-v sv-orange">{{ pct(c.ctr) }}</span></div>
              <div class="ck"><span class="ck-l">排名</span><span class="ck-v sv-purple">{{ (c.position||c.avg_position||0).toFixed(1) }}</span></div>
            </template>
            <template v-else>
              <div class="ck"><span class="ck-l">会话</span><span class="ck-v sv-blue">{{ fmt(c.sessions) }}</span></div>
              <div class="ck"><span class="ck-l">用户</span><span class="ck-v sv-green">{{ fmt(c.total_users) }}</span></div>
              <div class="ck"><span class="ck-l">PV</span><span class="ck-v sv-orange">{{ fmt(c.screen_page_views) }}</span></div>
              <div class="ck"><span class="ck-l">新用户</span><span class="ck-v sv-purple">{{ fmt(c.new_users) }}</span></div>
            </template>
          </div>
        </div>
      </div>
    </div>

    <!-- Detail view -->
    <div v-if="view === 'detail'" class="view-detail">
      <div class="detail-head">
        <button class="btn" @click="backToOverview">← 返回总览</button>
        <div class="detail-title"><span class="tp">{{ platformLabel }}</span> · {{ detail.siteName }}</div>
        <div class="detail-days">
          <el-select v-model="days" size="small" style="width:110px" @change="loadDetail">
            <el-option :value="7" label="近 7 天" />
            <el-option :value="30" label="近 30 天" />
            <el-option :value="90" label="近 90 天" />
          </el-select>
        </div>
        <button class="btn btn-sync" @click="loadDetail" :disabled="loading">{{ loading ? '加载中...' : '🔄 刷新' }}</button>
      </div>

      <div v-if="loading" class="state-placeholder">加载中...</div>
      <div v-else-if="error" class="state-error">❌ {{ error }}</div>
      <template v-else>
        <div v-if="!d.total_clicks && !d.sessions && !d.trend?.length" class="state-empty">
          暂无数据。
        </div>
        <template v-else>
          <div class="kpi-strip">
            <div v-for="k in kpis" :key="k.label" class="panel kpi-card">
              <div class="kpi-label">{{ k.label }}</div>
              <div :class="['kpi-val', k.cls]">{{ k.value }}</div>
            </div>
          </div>

          <div class="panel" v-if="d.trend?.length >= 2">
            <div class="section-title">📈 每日趋势</div>
            <div class="trend-wrap">
              <svg :viewBox="`0 0 ${tW} ${tH}`" preserveAspectRatio="none" class="trend-svg">
                <defs><linearGradient id="tg" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stop-color="var(--sun)" stop-opacity=".4"/>
                  <stop offset="100%" stop-color="var(--sun)" stop-opacity="0"/>
                </linearGradient></defs>
                <polygon :points="tArea" fill="url(#tg)"/>
                <polyline :points="tLine" fill="none" stroke="var(--sun)" stroke-width="2" vector-effect="non-scaling-stroke"/>
              </svg>
            </div>
            <div class="trend-labels"><span>{{ d.trend[0]?.date }}</span><span>{{ d.trend[d.trend.length-1]?.date }}</span></div>
          </div>

          <div class="panel" v-if="d.appearances?.length">
            <div class="section-title">🎯 SERP 特征</div>
            <div class="serp-cards">
              <div class="serp-card" v-for="a in d.appearances" :key="a.appearance">
                <div class="sc-name">{{ a.appearance }}</div>
                <div class="sc-stats"><span><b>{{ fmt(a.impressions) }}</b> 曝光</span><span><b>{{ fmt(a.clicks) }}</b> 点击</span><span>CTR <b>{{ pct(a.ctr) }}</b></span></div>
              </div>
            </div>
          </div>

          <div class="two-col">
            <div class="panel" v-if="d.top_keywords?.length">
              <div class="section-title">🔑 热门关键词</div>
              <table class="tbl">
                <thead><tr><th>#</th><th>关键词</th><th>点击</th><th>展示</th><th>CTR</th><th>排名</th></tr></thead>
                <tbody><tr v-for="(r,i) in d.top_keywords" :key="i">
                  <td class="rk">{{ i+1 }}</td>
                  <td class="kw">{{ r.query }}</td>
                  <td class="num sv-blue">{{ fmt(r.clicks) }}</td>
                  <td class="num sv-green">{{ fmt(r.impressions) }}</td>
                  <td class="num sv-orange">{{ pct(r.ctr) }}</td>
                  <td class="num sv-purple">{{ (r.position||0).toFixed?.(1)||'-' }}</td>
                </tr></tbody>
              </table>
            </div>
            <div class="panel" v-if="d.top_pages?.length">
              <div class="section-title">📄 热门页面</div>
              <table class="tbl">
                <thead><tr><th>#</th><th>页面</th><th>点击</th><th>展示</th></tr></thead>
                <tbody><tr v-for="(r,i) in d.top_pages" :key="i">
                  <td class="rk">{{ i+1 }}</td>
                  <td class="pg">{{ cleanPath(r.page_url||r.page) }}</td>
                  <td class="num sv-blue">{{ fmt(r.clicks) }}</td>
                  <td class="num sv-green">{{ fmt(r.impressions) }}</td>
                </tr></tbody>
              </table>
            </div>
          </div>

          <div class="two-col">
            <div class="panel" v-if="d.devices?.length">
              <div class="section-title">📱 设备分布</div>
              <div class="bar-list">
                <div v-for="x in withPct(d.devices)" :key="x.label" class="bar-item">
                  <div class="bar-head"><span class="bar-name">{{ x.label }}</span><span class="bar-val">{{ fmt(x.clicks) }} · {{ x.pct.toFixed(1) }}%</span></div>
                  <div class="bar-track"><div class="bar-fill" :style="{width:x.pct+'%'}"></div></div>
                </div>
              </div>
            </div>
            <div class="panel" v-if="d.countries?.length">
              <div class="section-title">🌍 国家/地区</div>
              <div class="bar-list">
                <div v-for="x in withPct(d.countries)" :key="x.label" class="bar-item">
                  <div class="bar-head"><span class="bar-name">{{ (x.label||'').toUpperCase() }}</span><span class="bar-val">{{ fmt(x.clicks) }} · {{ x.pct.toFixed(1) }}%</span></div>
                  <div class="bar-track"><div class="bar-fill bar-green" :style="{width:x.pct+'%'}"></div></div>
                </div>
              </div>
            </div>
          </div>
        </template>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import api from '../api'

defineOptions({ name: 'DataManagement' })

const platforms = [
  { code: 'gsc',    label: '🟦 Google Search Console' },
  { code: 'ga4',    label: '🟩 Google Analytics' },
  { code: 'bing',   label: '🟦 Bing Webmaster' },
  { code: 'yandex', label: '🟥 Yandex Webmaster' },
]
const platform = ref('gsc')
const platformLabel = computed(() => platforms.find(p => p.code === platform.value)?.label || '')

const view = ref<'overview' | 'detail'>('overview')
const loading = ref(false)
const error = ref('')
const days = ref(30)

const overview = ref<{ cards: any[] }>({ cards: [] })
const detail = ref({ siteName: '', siteKey: '' })
const d = ref<any>({})

function fmt(n: number | undefined) {
  if (n == null) return '-'
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return String(Math.round(n))
}
function pct(n: number | undefined) {
  if (n == null) return '-'
  return (n * 100).toFixed(2) + '%'
}
function cleanPath(u: string) {
  if (!u) return ''
  try { return new URL(u).pathname || u } catch { return u }
}
function withPct(rows: any[]) {
  const total = rows.reduce((s, r) => s + (r.clicks || 0), 0) || 1
  return rows.map(r => ({ ...r, pct: ((r.clicks || 0) / total) * 100 }))
}
function daysAgo(n: number) { const d = new Date(); d.setDate(d.getDate() - n); return d.toISOString().slice(0, 10) }

async function loadOverview() {
  loading.value = true
  error.value = ''
  try {
    let res: any
    const params = { start_date: '', end_date: '' }
    switch (platform.value) {
      case 'gsc':    res = await api.dashGSCOverview(params); break
      case 'ga4':    res = await api.dashGAOverview(params); break
      case 'bing':   res = await api.dashBingOverview(params); break
      case 'yandex': res = await api.dashYandexOverview(params); break
    }
    overview.value.cards = res.data?.cards || []
  } catch (e: any) {
    error.value = e?.message || '加载失败'
    overview.value.cards = []
  } finally {
    loading.value = false
  }
}

function switchPlatform(code: string) {
  platform.value = code
  view.value = 'overview'
  loadOverview()
}

function platformSiteKey(c: any): string {
  return ({ gsc: c.site_url, ga4: c.property_id, bing: c.site_url, yandex: c.host_id } as any)[platform.value] || ''
}

async function openDetail(c: any) {
  const siteKey = platformSiteKey(c)
  if (!siteKey || c.error) return
  detail.value.siteKey = siteKey
  detail.value.siteName = c.display_name || siteKey
  view.value = 'detail'
  d.value = {}
  await loadDetail()
}

async function loadDetail() {
  const sk = detail.value.siteKey
  loading.value = true
  error.value = ''
  const baseParams = { start_date: daysAgo(days.value + 1), end_date: daysAgo(2) }
  try {
    let res: any
    switch (platform.value) {
      case 'gsc':    res = await api.dashGSCSiteDetail({ ...baseParams, site_url: sk }); break
      case 'ga4':    res = await api.dashGAPropertyDetail({ ...baseParams, property_id: sk }); break
      case 'bing':   res = await api.dashBingSiteDetail({ ...baseParams, site_url: sk }); break
      case 'yandex': res = await api.dashYandexHostDetail({ ...baseParams, host_id: sk }); break
    }
    if (res?.data) digestPlatformData(res.data)
  } catch (e: any) {
    error.value = e?.response?.data?.error || e?.message || '加载失败'
  } finally {
    loading.value = false
  }
}

function digestPlatformData(raw: any) {
  if (!raw) return
  const p = platform.value
  if (p === 'gsc') {
    const ov = (raw.overview?.rows || [])[0] || {}
    const kws = (raw.keywords?.rows || []).map((r: any) => ({ query: r.keys?.[0]||'', clicks: r.clicks||0, impressions: r.impressions||0, ctr: r.ctr||0, position: r.position||0 }))
    const pgs = (raw.pages?.rows || []).map((r: any) => ({ page_url: r.keys?.[0]||'', clicks: r.clicks||0, impressions: r.impressions||0, ctr: r.ctr||0 }))
    const trend = (raw.trend?.rows || []).map((r: any) => ({ date: r.keys?.[0]||'', clicks: r.clicks||0, impressions: r.impressions||0, ctr: r.ctr||0 }))
    const devs = (raw.devices?.rows || []).map((r: any) => ({ label: r.keys?.[0]||'', clicks: r.clicks||0 }))
    const ctrs = (raw.countries?.rows || []).slice(0,10).map((r: any) => ({ label: r.keys?.[0]||'', clicks: r.clicks||0 }))
    const apps = (raw.appearances?.rows || []).map((r: any) => ({ appearance: r.keys?.[0]||'', clicks: r.clicks||0, impressions: r.impressions||0, ctr: r.ctr||0, position: r.position||0 }))
    d.value = { platform: 'gsc', total_clicks: ov.clicks||0, total_impressions: ov.impressions||0, avg_ctr: ov.ctr||0, avg_position: ov.position||0,
      trend, top_keywords: kws, top_pages: pgs, devices: devs, countries: ctrs, appearances: apps }
  } else if (p === 'ga4') {
    const toNum = (v: any) => parseFloat(v?.value ?? v ?? '0') || 0
    const ovRow = (raw.overview?.rows || [])[0]
    const mv = ovRow?.metricValues || []
    const sessions = toNum(mv[0]), gaUsers = toNum(mv[1]), newUsers = toNum(mv[2]), pvs = toNum(mv[3])
    const br = mv.length > 4 ? toNum(mv[4]) : 0
    const er = mv.length > 6 ? toNum(mv[6]) : 0
    const trend = (raw.trend?.rows || []).map((r: any) => {
      const dt = r.dimensionValues?.[0]?.value || ''
      return { date: dt.length === 8 ? `${dt.slice(0,4)}-${dt.slice(4,6)}-${dt.slice(6,8)}` : dt, clicks: toNum(r.metricValues?.[0]), impressions: toNum(r.metricValues?.[1]) }
    })
    const pgs = (raw.pages?.rows || []).map((r: any) => ({ page_url: r.dimensionValues?.[0]?.value||'', clicks: toNum(r.metricValues?.[0]), impressions: toNum(r.metricValues?.[1]) }))
    const devs = (raw.devices?.rows || []).map((r: any) => ({ label: r.dimensionValues?.[0]?.value||'', clicks: toNum(r.metricValues?.[0]) }))
    const ctrs = (raw.countries?.rows || []).slice(0,10).map((r: any) => ({ label: r.dimensionValues?.[0]?.value||'', clicks: toNum(r.metricValues?.[0]) }))
    d.value = { platform: 'ga4', sessions, users: gaUsers, new_users: newUsers, pageviews: pvs, bounce_rate: br, engagement_rate: er, trend, top_pages: pgs, devices: devs, countries: ctrs }
  } else if (p === 'bing') {
    const rows = (raw.queries?.d || []).filter((r: any) => r && typeof r === 'object')
    let clicks = 0, impr = 0
    rows.forEach((r: any) => { clicks += Number(r.Clicks||0); impr += Number(r.Impressions||0) })
    const kws = rows.slice(0,20).map((r: any) => ({ query: r.Query||'', clicks: Number(r.Clicks||0), impressions: Number(r.Impressions||0), position: Number(r.AvgImpPos||0) }))
    const pgs = (raw.pages?.d || []).slice(0,20).map((r: any) => ({ page_url: r.Page||r.Url||'', clicks: Number(r.Clicks||0), impressions: Number(r.Impressions||0) }))
    d.value = { platform: 'bing', total_clicks: clicks, total_impressions: impr, avg_ctr: impr>0?clicks/impr:0, avg_position: 0, top_keywords: kws, top_pages: pgs }
  } else if (p === 'yandex') {
    const ind = raw.history?.indicators || {}
    const sumA = (a: any[]) => (a||[]).reduce((s: number, x: any) => s + (x.value||0), 0)
    const clicks = sumA(ind.TOTAL_CLICKS); const shows = sumA(ind.TOTAL_SHOWS)
    const trend = (ind.TOTAL_CLICKS||[]).map((x: any, i: number) => ({ date: x.date||'', clicks: x.value||0, impressions: (ind.TOTAL_SHOWS||[])[i]?.value||0 }))
    const pop = raw.popular?.queries || raw.popular?.popular_queries || []
    const kws = pop.slice(0,20).map((q: any) => { const i = q.indicators||{}; return { query: q.query_text||q.query||'', clicks: i.TOTAL_CLICKS||0, impressions: i.TOTAL_SHOWS||0, position: i.AVG_SHOW_POSITION||0 } })
    d.value = { platform: 'yandex', total_clicks: clicks, total_impressions: shows, avg_ctr: shows>0?clicks/shows:0, trend, top_keywords: kws }
  }
}

const kpis = computed(() => {
  const v = d.value
  if (platform.value === 'ga4') return [
    { label: '会话', value: fmt(v.sessions), cls: 'sv-blue' },
    { label: '用户', value: fmt(v.users), cls: 'sv-green' },
    { label: 'PV', value: fmt(v.pageviews), cls: 'sv-orange' },
    { label: '跳出率', value: pct(v.bounce_rate), cls: 'sv-purple' },
  ]
  return [
    { label: '点击', value: fmt(v.total_clicks), cls: 'sv-blue' },
    { label: '展示', value: fmt(v.total_impressions), cls: 'sv-green' },
    { label: 'CTR', value: pct(v.avg_ctr), cls: 'sv-orange' },
    { label: '排名', value: (v.avg_position||0).toFixed?.(1) || '-', cls: 'sv-purple' },
  ]
})

const tW = 800
const tH = 180
const tLine = computed(() => {
  const rows = d.value.trend || []
  if (rows.length < 2) return ''
  const max = Math.max(...rows.map((r: any) => r.clicks||0), 1)
  return rows.map((r: any, i: number) => {
    const x = (i / (rows.length - 1)) * tW
    const y = tH - ((r.clicks||0) / max) * (tH - 10) - 5
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
})
const tArea = computed(() => { const l = tLine.value; return l ? `0,${tH} ${l} ${tW},${tH}` : '' })

function backToOverview() { view.value = 'overview'; error.value = ''; d.value = {} }

onMounted(loadOverview)
</script>

<style scoped>
.data-mgmt { padding:20px 24px; min-height:calc(100vh - 60px); background:var(--bg); }
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:16px 20px; margin-bottom:14px; }
.toolbar { margin-bottom:16px; }
.tb-row { display:flex; align-items:center; gap:16px; flex-wrap:wrap; }
.tb-tabs { display:flex; gap:6px; }
.tb-right { margin-left:auto; }
.pill { padding:6px 16px; border-radius:20px; border:1.5px solid var(--border-light); background:var(--surface-2); font-size:13px; font-weight:700; color:var(--ink-soft); cursor:pointer; transition:all .15s; }
.pill.active { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; }
.pill:hover:not(.active) { background:var(--sun-light); color:var(--sun-deep); }
.btn { padding:7px 14px; border-radius:8px; border:1.5px solid var(--border-light); background:var(--surface); font-size:13px; font-weight:700; color:var(--ink-mid); cursor:pointer; transition:all .15s; }
.btn:hover:not(:disabled) { border-color:var(--sun); color:var(--sun-deep); }
.btn:disabled { opacity:.5; cursor:not-allowed; }
.btn-refresh,.btn-sync { background:var(--sun-light); color:var(--sun-deep); border-color:var(--peach); }
.section-title { font-size:14px; font-weight:800; color:var(--ink); margin:0 0 12px; padding-bottom:6px; border-bottom:1.5px solid var(--border-light); display:flex; align-items:center; gap:8px; }
.badge { font-size:11px; font-weight:700; padding:2px 10px; background:var(--sun-light); color:var(--sun-deep); border-radius:10px; }
.card-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(260px,1fr)); gap:12px; }
.site-card { background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r); padding:14px 16px; cursor:pointer; transition:all .15s; position:relative; }
.site-card:hover { border-color:var(--sun); transform:translateY(-1px); box-shadow:var(--s-amber); }
.card-name { font-size:14px; font-weight:800; color:var(--ink); margin-bottom:10px; word-break:break-all; }
.card-err { border-color:#FECACA; background:#FEF2F2; }
.card-err-msg { font-size:12px; color:#B91C1C; }
.card-kpis { display:grid; grid-template-columns:repeat(2,1fr); gap:8px; }
.ck-l { font-size:10.5px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; }
.ck-v { font-size:15px; font-weight:800; display:block; }
.sv-blue { color:#3B82F6; } .sv-green { color:#10B981; } .sv-orange { color:#F59E0B; } .sv-purple { color:#8B5CF6; }
.detail-head { display:flex; align-items:center; gap:14px; margin-bottom:14px; flex-wrap:wrap; }
.detail-title { font-size:16px; font-weight:800; flex:1; }
.tp { background:var(--sun-light); color:var(--sun-deep); padding:2px 10px; border-radius:8px; font-size:13px; }
.kpi-strip { display:grid; grid-template-columns:repeat(4,1fr); gap:12px; margin-bottom:14px; }
.kpi-card { text-align:left; padding:14px 18px; }
.kpi-label { font-size:10.5px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; margin-bottom:4px; }
.kpi-val { font-size:22px; font-weight:800; }
.two-col { display:grid; grid-template-columns:1fr 1fr; gap:14px; margin-bottom:14px; }
@media (max-width:1000px) { .two-col { grid-template-columns:1fr; } }
.tbl { width:100%; border-collapse:collapse; }
.tbl th { padding:8px 10px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); border-bottom:2px solid var(--border); background:var(--surface-3); }
.tbl td { padding:8px 10px; border-bottom:1px solid var(--border-light); font-size:12.5px; }
.tbl tr:hover td { background:var(--sun-light); }
.rk { width:32px; color:var(--ink-hint); font-weight:700; }
.num { font-weight:700; text-align:right; white-space:nowrap; }
.kw { max-width:250px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.pg { font-size:11.5px; color:var(--ink-soft); max-width:250px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.trend-wrap { height:160px; background:var(--surface-2); border-radius:8px; padding:8px; margin-bottom:4px; }
.trend-svg { width:100%; height:100%; display:block; }
.trend-labels { display:flex; justify-content:space-between; font-size:11px; color:var(--ink-hint); padding:0 8px; }
.serp-cards { display:grid; grid-template-columns:repeat(auto-fill,minmax(260px,1fr)); gap:8px; }
.serp-card { background:#f9fafb; border:1px solid #eee; border-radius:8px; padding:10px 14px; }
.sc-name { font-weight:600; margin-bottom:4px; }
.sc-stats { display:flex; gap:12px; font-size:12px; color:#666; } .sc-stats b { color:#333; }
.bar-list { display:flex; flex-direction:column; gap:8px; }
.bar-head { display:flex; justify-content:space-between; font-size:12px; margin-bottom:3px; }
.bar-name { font-weight:700; } .bar-val { color:var(--ink-hint); }
.bar-track { height:7px; background:var(--surface-3); border-radius:4px; overflow:hidden; }
.bar-fill { height:100%; background:linear-gradient(90deg,var(--sun),var(--coral)); border-radius:4px; }
.bar-green { background:linear-gradient(90deg,#10B981,#059669); }
.state-placeholder,.state-empty,.state-error { padding:50px 20px; text-align:center; color:var(--ink-hint); font-size:14px; }
.state-error { color:#B91C1C; }
</style>
