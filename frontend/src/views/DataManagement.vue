<template>
  <div class="data-mgmt">
    <!-- ─── Top toolbar: platform tabs + date range ─────────────────── -->
    <div class="panel toolbar">
      <div class="tb-row">
        <div class="tb-tabs">
          <button
            v-for="p in platforms"
            :key="p.code"
            :class="['pill', platform === p.code && 'active']"
            @click="switchPlatform(p.code)"
          >{{ p.label }}</button>
        </div>

        <div class="tb-date">
          <button
            v-for="preset in presets"
            :key="preset.key"
            :class="['pill pill-sm', activePreset === preset.key && 'active']"
            @click="applyPreset(preset.key)"
          >{{ preset.label }}</button>
          <el-date-picker
            v-model="dateRange"
            type="daterange"
            range-separator="~"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            format="YYYY-MM-DD"
            value-format="YYYY-MM-DD"
            size="default"
            :disabled-date="disabledDate"
            @change="onDateChange"
          />
          <button class="btn btn-refresh" @click="refreshCurrent(true)" :disabled="loading">
            {{ loading ? '加载中...' : '🔄 刷新' }}
          </button>
        </div>
      </div>
      <div class="tb-meta" v-if="dateRange[0] && dateRange[1]">
        <span class="meta-item">📅 {{ dateRange[0] }} → {{ dateRange[1] }}</span>
        <span class="meta-item" v-if="dayCount === 1">单日数据</span>
        <span class="meta-item" v-else>共 {{ dayCount }} 天</span>
      </div>
    </div>

    <!-- ─── Overview state: card grid ───────────────────────────────── -->
    <div v-if="view === 'overview'" class="view-overview">
      <div class="section-title">
        {{ platformLabel }} · 总览
        <span class="site-count-badge" v-if="overview.cards.length">{{ overview.cards.length }} 个站点</span>
      </div>

      <div v-if="loading && !overview.cards.length" class="state-placeholder">加载中...</div>
      <div v-else-if="error" class="state-error">
        <div class="err-title">❌ {{ error }}</div>
        <div class="err-hint">如果还没授权该平台，请先到「系统管理 → 数据平台」完成授权</div>
      </div>
      <div v-else-if="!overview.cards.length" class="state-empty">
        暂无站点 — 请确认账号下已有站点 / Property，且完成验证
      </div>

      <div v-else class="card-grid">
        <div
          v-for="(c, idx) in overview.cards"
          :key="idx"
          :class="['site-card', !c.has_data && 'site-card-empty', c.error && 'site-card-err']"
          @click="openDetail(c)"
        >
          <div class="card-head">
            <div class="card-title">{{ c.display_name || c.site_url || c.host_id || c.property_id }}</div>
            <span v-if="c.verified === false" class="card-tag tag-warn">未验证</span>
            <span v-else-if="c.error" class="card-tag tag-err">错误</span>
            <span v-else-if="!c.has_data" class="card-tag tag-muted">无数据</span>
          </div>
          <div v-if="c.error" class="card-err-msg">{{ c.error }}</div>
          <template v-else>
            <div class="card-sub">{{ cardSubtitle(c) }}</div>
            <div class="card-kpis" v-if="platform !== 'ga4'">
              <div class="kpi">
                <div class="kpi-label">点击</div>
                <div class="kpi-value sv-blue">{{ fmt(c.clicks) }}</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">展示</div>
                <div class="kpi-value sv-green">{{ fmt(c.impressions || c.shows) }}</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">CTR</div>
                <div class="kpi-value sv-orange">{{ ((c.ctr || 0) * 100).toFixed(2) }}%</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">平均排名</div>
                <div class="kpi-value sv-purple">{{ (c.position || c.avg_position || 0).toFixed(1) }}</div>
              </div>
            </div>
            <!-- GA4 metrics -->
            <div class="card-kpis" v-else>
              <div class="kpi">
                <div class="kpi-label">会话</div>
                <div class="kpi-value sv-blue">{{ fmt(c.sessions) }}</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">用户</div>
                <div class="kpi-value sv-green">{{ fmt(c.total_users) }}</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">新用户</div>
                <div class="kpi-value sv-orange">{{ fmt(c.new_users) }}</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">浏览量</div>
                <div class="kpi-value sv-purple">{{ fmt(c.screen_page_views) }}</div>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>

    <!-- ─── Detail state ────────────────────────────────────────────── -->
    <div v-else-if="view === 'detail'" class="view-detail">
      <div class="detail-head">
        <button class="btn btn-back" @click="backToOverview">← 返回总览</button>
        <div class="detail-title">
          <span class="title-platform">{{ platformLabel }}</span>
          <span class="title-sep">·</span>
          <span class="title-site">{{ detail.siteName }}</span>
        </div>
      </div>

      <div v-if="loading" class="state-placeholder">加载中...</div>
      <div v-else-if="error" class="state-error">
        <div class="err-title">❌ {{ error }}</div>
      </div>
      <template v-else>
        <div class="kpi-strip">
          <div v-for="kpi in detailKPIs" :key="kpi.label" class="panel kpi-card">
            <div class="kpi-label">{{ kpi.label }}</div>
            <div :class="['kpi-value', 'kpi-big', kpi.class]">{{ kpi.value }}</div>
          </div>
        </div>

        <div class="panel trend-panel" v-if="detail.trendRows.length >= 2">
          <div class="section-title">每日趋势</div>
          <div class="trend-chart-wrap">
            <svg :viewBox="`0 0 ${trendW} ${trendH}`" preserveAspectRatio="none" class="trend-svg">
              <defs>
                <linearGradient id="trendGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stop-color="var(--sun)" stop-opacity="0.4" />
                  <stop offset="100%" stop-color="var(--sun)" stop-opacity="0" />
                </linearGradient>
              </defs>
              <polygon :points="trendAreaPoints" fill="url(#trendGrad)" />
              <polyline :points="trendLinePoints" fill="none" stroke="var(--sun)" stroke-width="2" vector-effect="non-scaling-stroke" />
            </svg>
          </div>
          <div class="trend-labels">
            <span>{{ detail.trendRows[0]?.date }}</span>
            <span>{{ detail.trendRows[detail.trendRows.length - 1]?.date }}</span>
          </div>
        </div>

        <div class="two-col">
          <div class="panel" v-if="detail.keywords.length">
            <div class="section-title">热门关键词 Top 20</div>
            <table class="table">
              <thead>
                <tr><th>#</th><th>关键词</th><th>点击</th><th>展示</th><th>CTR</th><th>排名</th></tr>
              </thead>
              <tbody>
                <tr v-for="(r, i) in detail.keywords" :key="i">
                  <td class="rk">{{ i + 1 }}</td>
                  <td class="kw-cell" :title="r.query">{{ r.query }}</td>
                  <td class="num sv-blue">{{ fmt(r.clicks) }}</td>
                  <td class="num sv-green">{{ fmt(r.impressions) }}</td>
                  <td class="num sv-orange">{{ ((r.ctr || 0) * 100).toFixed(2) }}%</td>
                  <td class="num sv-purple">{{ (r.position || 0).toFixed(1) }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="panel" v-if="detail.pages.length">
            <div class="section-title">热门页面 Top 20</div>
            <table class="table">
              <thead>
                <tr><th>#</th><th>页面</th><th>点击</th><th>展示</th><th>CTR</th></tr>
              </thead>
              <tbody>
                <tr v-for="(r, i) in detail.pages" :key="i">
                  <td class="rk">{{ i + 1 }}</td>
                  <td class="path-cell" :title="r.page">{{ cleanPath(r.page) }}</td>
                  <td class="num sv-blue">{{ fmt(r.clicks) }}</td>
                  <td class="num sv-green">{{ fmt(r.impressions) }}</td>
                  <td class="num sv-orange">{{ ((r.ctr || 0) * 100).toFixed(2) }}%</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div class="two-col">
          <div class="panel" v-if="detail.devices.length">
            <div class="section-title">设备分布</div>
            <div class="bar-list">
              <div v-for="d in detail.devices" :key="d.label" class="bar-row">
                <div class="bar-head">
                  <span class="bar-name">{{ d.label }}</span>
                  <span class="bar-val">{{ fmt(d.clicks) }} · {{ d.pct.toFixed(1) }}%</span>
                </div>
                <div class="bar-track"><div class="bar-fill" :style="`width: ${d.pct}%`"></div></div>
              </div>
            </div>
          </div>

          <div class="panel" v-if="detail.countries.length">
            <div class="section-title">国家 / 地区 Top 10</div>
            <div class="bar-list">
              <div v-for="c in detail.countries" :key="c.label" class="bar-row">
                <div class="bar-head">
                  <span class="bar-name">{{ (c.label || '').toUpperCase() }}</span>
                  <span class="bar-val">{{ fmt(c.clicks) }} · {{ c.pct.toFixed(1) }}%</span>
                </div>
                <div class="bar-track"><div class="bar-fill bar-green" :style="`width: ${c.pct}%`"></div></div>
              </div>
            </div>
          </div>
        </div>

        <div v-if="!detail.keywords.length && !detail.pages.length && !detail.trendRows.length" class="state-empty">
          该日期范围内无数据
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

// ── Platforms ────────────────────────────────────────────────────────────
const platforms = [
  { code: 'gsc', label: 'GSC' },
  { code: 'ga4', label: 'GA4' },
  { code: 'bing', label: 'Bing' },
  { code: 'yandex', label: 'Yandex' },
]
const platform = ref<'gsc' | 'ga4' | 'bing' | 'yandex'>('gsc')
const platformLabel = computed(() => platforms.find(p => p.code === platform.value)?.label || '')

// ── Date range + presets ────────────────────────────────────────────────
// Default: last 28 days ending yesterday-2 (GSC data lags ~2 days; we
// keep the same end-date rule for all platforms for consistency in the UI).
function ymd(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
function daysAgo(n: number): string {
  const d = new Date()
  d.setDate(d.getDate() - n)
  return ymd(d)
}
const dateRange = ref<[string, string]>([daysAgo(29), daysAgo(2)])
const activePreset = ref<string>('28d')
const presets = [
  { key: 'yesterday', label: '昨天' },
  { key: '7d', label: '近 7 天' },
  { key: '28d', label: '近 28 天' },
  { key: '90d', label: '近 90 天' },
]
function applyPreset(key: string) {
  activePreset.value = key
  switch (key) {
    case 'yesterday': dateRange.value = [daysAgo(1), daysAgo(1)]; break
    case '7d':       dateRange.value = [daysAgo(8), daysAgo(2)]; break
    case '28d':      dateRange.value = [daysAgo(29), daysAgo(2)]; break
    case '90d':      dateRange.value = [daysAgo(91), daysAgo(2)]; break
  }
  refreshCurrent()
}
function onDateChange() {
  activePreset.value = 'custom'
  refreshCurrent()
}
// GSC data only goes back ~16 months; UI blocks picking earlier.
function disabledDate(date: Date): boolean {
  const earliest = new Date()
  earliest.setMonth(earliest.getMonth() - 16)
  return date > new Date() || date < earliest
}
const dayCount = computed(() => {
  if (!dateRange.value[0] || !dateRange.value[1]) return 0
  const s = new Date(dateRange.value[0]).getTime()
  const e = new Date(dateRange.value[1]).getTime()
  return Math.round((e - s) / 86400000) + 1
})

// ── View state ──────────────────────────────────────────────────────────
const view = ref<'overview' | 'detail'>('overview')
const loading = ref(false)
const error = ref('')

const overview = reactive({ cards: [] as any[] })
const detail = reactive({
  siteKey: '' as string,
  siteName: '' as string,
  kpis: {} as Record<string, number>,
  trendRows: [] as any[],
  keywords: [] as any[],
  pages: [] as any[],
  devices: [] as { label: string; clicks: number; pct: number }[],
  countries: [] as { label: string; clicks: number; pct: number }[],
})

// ── 5-minute in-memory cache ────────────────────────────────────────────
// Key: platform:startDate:endDate:siteKey (empty for overview).
type CacheEntry = { at: number; data: any }
const cacheTTL = 5 * 60 * 1000
const cache = new Map<string, CacheEntry>()
function cacheKey(extra = '') {
  return `${platform.value}:${dateRange.value[0]}:${dateRange.value[1]}:${extra}`
}
function cacheGet(k: string): any | null {
  const hit = cache.get(k)
  if (!hit) return null
  if (Date.now() - hit.at > cacheTTL) { cache.delete(k); return null }
  return hit.data
}
function cacheSet(k: string, data: any) {
  cache.set(k, { at: Date.now(), data })
}

// ── Platform switch + main load ─────────────────────────────────────────
function switchPlatform(code: any) {
  if (platform.value === code) return
  platform.value = code
  view.value = 'overview'
  error.value = ''
  loadOverview()
}

async function refreshCurrent(bypassCache = false) {
  if (bypassCache) {
    // Drop cache entries for the current platform+range so an explicit
    // refresh honours the user's "I want fresh data" intent.
    const prefix = `${platform.value}:${dateRange.value[0]}:${dateRange.value[1]}:`
    for (const k of Array.from(cache.keys())) {
      if (k.startsWith(prefix)) cache.delete(k)
    }
  }
  if (view.value === 'overview') await loadOverview()
  else await loadDetail(detail.siteKey, detail.siteName)
}

async function loadOverview() {
  loading.value = true
  error.value = ''
  const key = cacheKey('')
  const cached = cacheGet(key)
  if (cached) {
    overview.cards = cached.cards || []
    loading.value = false
    return
  }
  const params = { start_date: dateRange.value[0], end_date: dateRange.value[1] }
  try {
    let res: any
    switch (platform.value) {
      case 'gsc':    res = await api.dashGSCOverview(params); break
      case 'ga4':    res = await api.dashGAOverview(params); break
      case 'bing':   res = await api.dashBingOverview(params); break
      case 'yandex': res = await api.dashYandexOverview(params); break
    }
    const data = res.data || { cards: [] }
    overview.cards = data.cards || []
    cacheSet(key, data)
  } catch (e: any) {
    error.value = e?.response?.data?.error || e?.message || '加载失败'
    overview.cards = []
  } finally {
    loading.value = false
  }
}

// ── Detail view ─────────────────────────────────────────────────────────
function cardSubtitle(c: any): string {
  switch (platform.value) {
    case 'gsc':    return c.is_domain ? 'Domain Property' : (c.site_url || '')
    case 'ga4':    return c.account_name ? `账户：${c.account_name}` : ''
    case 'bing':   return c.site_url || ''
    case 'yandex': return c.host_id || ''
  }
  return ''
}

function openDetail(c: any) {
  let siteKey = ''
  switch (platform.value) {
    case 'gsc':    siteKey = c.site_url; break
    case 'ga4':    siteKey = c.property_id; break
    case 'bing':   siteKey = c.site_url; break
    case 'yandex': siteKey = c.host_id; break
  }
  if (!siteKey) { ElMessage.warning('无法打开详情：站点标识缺失'); return }
  if (c.error) { ElMessage.warning(`该站点当前不可访问：${c.error}`); return }
  view.value = 'detail'
  detail.siteName = c.display_name || siteKey
  loadDetail(siteKey, c.display_name || siteKey)
}

function backToOverview() {
  view.value = 'overview'
  error.value = ''
}

async function loadDetail(siteKey: string, siteName: string) {
  loading.value = true
  error.value = ''
  detail.siteKey = siteKey
  detail.siteName = siteName
  detail.kpis = {}
  detail.trendRows = []
  detail.keywords = []
  detail.pages = []
  detail.devices = []
  detail.countries = []

  const key = cacheKey(siteKey)
  const cached = cacheGet(key)
  if (cached) {
    digestDetail(cached)
    loading.value = false
    return
  }

  const baseParams = { start_date: dateRange.value[0], end_date: dateRange.value[1] }
  try {
    let res: any
    switch (platform.value) {
      case 'gsc':    res = await api.dashGSCSiteDetail({ ...baseParams, site_url: siteKey }); break
      case 'ga4':    res = await api.dashGAPropertyDetail({ ...baseParams, property_id: siteKey }); break
      case 'bing':   res = await api.dashBingSiteDetail({ ...baseParams, site_url: siteKey }); break
      case 'yandex': res = await api.dashYandexHostDetail({ ...baseParams, host_id: siteKey }); break
    }
    cacheSet(key, res.data)
    digestDetail(res.data)
  } catch (e: any) {
    error.value = e?.response?.data?.error || e?.message || '加载详情失败'
  } finally {
    loading.value = false
  }
}

// digestDetail normalizes each platform's raw response into the shared
// shape our UI expects. Anything we can't parse stays empty — the
// template guards every section with v-if.
function digestDetail(d: any) {
  if (!d) return
  switch (platform.value) {
    case 'gsc':    return digestGSC(d)
    case 'ga4':    return digestGA(d)
    case 'bing':   return digestBing(d)
    case 'yandex': return digestYandex(d)
  }
}

function digestGSC(d: any) {
  const ov = (d.overview?.rows || [])[0] || {}
  detail.kpis = {
    clicks: ov.clicks || 0, impressions: ov.impressions || 0,
    ctr: ov.ctr || 0, position: ov.position || 0,
  }
  detail.trendRows = (d.trend?.rows || []).map((r: any) => ({
    date: r.keys?.[0] || '',
    clicks: r.clicks || 0, impressions: r.impressions || 0,
    ctr: r.ctr || 0,
  }))
  detail.keywords = (d.keywords?.rows || []).map((r: any) => ({
    query: r.keys?.[0] || '',
    clicks: r.clicks || 0, impressions: r.impressions || 0,
    ctr: r.ctr || 0, position: r.position || 0,
  }))
  detail.pages = (d.pages?.rows || []).map((r: any) => ({
    page: r.keys?.[0] || '',
    clicks: r.clicks || 0, impressions: r.impressions || 0,
    ctr: r.ctr || 0, position: r.position || 0,
  }))
  detail.devices = barize((d.devices?.rows || []).map((r: any) => ({
    label: deviceLabel(r.keys?.[0]), clicks: r.clicks || 0,
  })))
  detail.countries = barize((d.countries?.rows || []).slice(0, 10).map((r: any) => ({
    label: r.keys?.[0] || '', clicks: r.clicks || 0,
  })))
}

function digestGA(d: any) {
  const toNum = (v: any) => parseFloat(v?.value ?? v ?? '0') || 0
  const ovRow = (d.overview?.rows || [])[0]
  if (ovRow?.metricValues) {
    const m = ovRow.metricValues
    detail.kpis = {
      sessions: toNum(m[0]), users: toNum(m[1]), newUsers: toNum(m[2]),
      pageviews: toNum(m[3]), bounceRate: toNum(m[4]),
      engagementRate: toNum(m[6]),
    }
  }
  detail.trendRows = (d.trend?.rows || []).map((r: any) => {
    const raw = r.dimensionValues?.[0]?.value || ''
    const dt = raw.length === 8
      ? `${raw.slice(0, 4)}-${raw.slice(4, 6)}-${raw.slice(6, 8)}` : raw
    return {
      date: dt,
      // re-use the clicks/impressions field for sessions/users so the
      // generic chart/KPI code keeps working without per-platform branches.
      clicks: toNum(r.metricValues?.[0]),
      impressions: toNum(r.metricValues?.[1]),
      ctr: 0,
    }
  })
  detail.keywords = [] // GA4 has no query dimension by default
  detail.pages = (d.pages?.rows || []).map((r: any) => ({
    page: r.dimensionValues?.[0]?.value || '',
    clicks: toNum(r.metricValues?.[0]),
    impressions: toNum(r.metricValues?.[1]),
    ctr: 0, position: 0,
  }))
  detail.devices = barize((d.devices?.rows || []).map((r: any) => ({
    label: deviceLabel(r.dimensionValues?.[0]?.value),
    clicks: toNum(r.metricValues?.[0]),
  })))
  detail.countries = barize((d.countries?.rows || []).slice(0, 10).map((r: any) => ({
    label: r.dimensionValues?.[0]?.value || '',
    clicks: toNum(r.metricValues?.[0]),
  })))
}

function digestBing(d: any) {
  const rows = d.queries?.d || []
  let clicks = 0, impr = 0, wPos = 0
  rows.forEach((r: any) => {
    clicks += r.Clicks || 0
    impr += r.Impressions || 0
    wPos += (r.AvgImpressionPosition || 0) * (r.Impressions || 0)
  })
  detail.kpis = {
    clicks, impressions: impr,
    ctr: impr > 0 ? clicks / impr : 0,
    position: impr > 0 ? wPos / impr : 0,
  }
  detail.keywords = rows.slice(0, 20).map((r: any) => ({
    query: r.Query || '',
    clicks: r.Clicks || 0, impressions: r.Impressions || 0,
    ctr: r.Impressions ? r.Clicks / r.Impressions : 0,
    position: r.AvgImpressionPosition || 0,
  }))
  const pages = d.pages?.d || []
  detail.pages = pages.slice(0, 20).map((r: any) => ({
    page: r.Page || r.Url || '',
    clicks: r.Clicks || 0, impressions: r.Impressions || 0,
    ctr: r.Impressions ? r.Clicks / r.Impressions : 0,
    position: r.AvgImpressionPosition || 0,
  }))
  detail.trendRows = []; detail.devices = []; detail.countries = []
}

function digestYandex(d: any) {
  const ind = d.history?.indicators || {}
  const clicks = sumArr(ind.TOTAL_CLICKS)
  const shows = sumArr(ind.TOTAL_SHOWS)
  detail.kpis = {
    clicks, impressions: shows,
    ctr: shows > 0 ? clicks / shows : 0, position: 0,
  }
  const clicksArr = ind.TOTAL_CLICKS || []
  const showsArr = ind.TOTAL_SHOWS || []
  detail.trendRows = clicksArr.map((p: any, i: number) => ({
    date: p.date || '',
    clicks: p.value || 0,
    impressions: (showsArr[i]?.value) || 0,
    ctr: 0,
  }))
  const popular = d.popular?.queries || d.popular?.popular_queries || []
  detail.keywords = popular.slice(0, 20).map((q: any) => {
    const inds = q.indicators || {}
    const kClicks = inds.TOTAL_CLICKS || 0
    const kShows = inds.TOTAL_SHOWS || 0
    return {
      query: q.query_text || q.query || '',
      clicks: kClicks, impressions: kShows,
      ctr: kShows ? kClicks / kShows : 0,
      position: inds.AVG_SHOW_POSITION || 0,
    }
  })
  detail.pages = []; detail.devices = []; detail.countries = []
}

// ── Helpers ─────────────────────────────────────────────────────────────
function sumArr(arr: any): number {
  if (!Array.isArray(arr)) return 0
  return arr.reduce((s, p) => s + (p.value || 0), 0)
}
function deviceLabel(raw: string): string {
  const lbl: Record<string, string> = {
    MOBILE: '手机', DESKTOP: '桌面', TABLET: '平板',
    mobile: '手机', desktop: '桌面', tablet: '平板',
  }
  return lbl[raw] || raw || '未知'
}
function barize(items: { label: string; clicks: number }[]) {
  const total = items.reduce((s, x) => s + x.clicks, 0) || 1
  return items.map(x => ({ ...x, pct: (x.clicks / total) * 100 }))
}
function cleanPath(u: string): string {
  if (!u) return ''
  return u.replace(/^https?:\/\/[^/]+/, '') || '/'
}
function fmt(n: number | undefined | null): string {
  if (n == null || isNaN(Number(n))) return '0'
  const v = Number(n)
  if (v >= 1e6) return (v / 1e6).toFixed(2) + 'M'
  if (v >= 1e3) return (v / 1e3).toFixed(1) + 'K'
  return Math.round(v).toLocaleString()
}

// ── KPIs for detail strip ───────────────────────────────────────────────
const detailKPIs = computed(() => {
  const k = detail.kpis
  switch (platform.value) {
    case 'gsc':
    case 'bing':
    case 'yandex':
      return [
        { label: '点击', value: fmt(k.clicks), class: 'sv-blue' },
        { label: '展示', value: fmt(k.impressions), class: 'sv-green' },
        { label: 'CTR', value: ((k.ctr || 0) * 100).toFixed(2) + '%', class: 'sv-orange' },
        { label: '平均排名', value: (k.position || 0).toFixed(1), class: 'sv-purple' },
      ]
    case 'ga4':
      return [
        { label: '会话', value: fmt(k.sessions), class: 'sv-blue' },
        { label: '用户', value: fmt(k.users), class: 'sv-green' },
        { label: '浏览量', value: fmt(k.pageviews), class: 'sv-orange' },
        { label: '跳出率', value: ((k.bounceRate || 0) * 100).toFixed(1) + '%', class: 'sv-purple' },
      ]
  }
  return []
})

// ── Trend chart math ────────────────────────────────────────────────────
const trendW = 800
const trendH = 180
const trendLinePoints = computed(() => {
  const rows = detail.trendRows
  if (rows.length < 2) return ''
  const maxV = Math.max(...rows.map(r => r.clicks || 0), 1)
  return rows.map((r, i) => {
    const x = (i / (rows.length - 1)) * trendW
    const y = trendH - ((r.clicks || 0) / maxV) * (trendH - 10) - 5
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
})
const trendAreaPoints = computed(() => {
  const line = trendLinePoints.value
  if (!line) return ''
  return `0,${trendH} ${line} ${trendW},${trendH}`
})

// ── Init ────────────────────────────────────────────────────────────────
onMounted(() => { loadOverview() })
</script>

<style scoped>
.data-mgmt { padding:22px 28px; min-height:calc(100vh - 60px); background:var(--bg); }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:16px 22px; }
.toolbar { margin-bottom:20px; }
.tb-row { display:flex; align-items:center; gap:20px; flex-wrap:wrap; }
.tb-tabs { display:flex; gap:8px; }
.tb-date { display:flex; align-items:center; gap:8px; flex-wrap:wrap; margin-left:auto; }
.tb-meta { display:flex; gap:16px; margin-top:12px; padding-top:10px; border-top:1px dashed var(--border-light); }
.meta-item { font-size:12px; color:var(--ink-hint); font-family:'JetBrains Mono', monospace; }

.pill { padding:6px 16px; border-radius:20px; border:1.5px solid var(--border-light); background:var(--surface-2); font-size:13px; font-weight:700; color:var(--ink-soft); cursor:pointer; font-family:inherit; transition:all .15s; }
.pill.active { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.pill:hover:not(.active) { background:var(--sun-light); color:var(--sun-deep); border-color:var(--peach); }
.pill-sm { padding:5px 12px; font-size:12px; }

.btn { padding:7px 16px; border-radius:8px; border:1.5px solid var(--border-light); background:var(--surface); font-size:13px; font-weight:700; color:var(--ink-mid); cursor:pointer; font-family:inherit; transition:all .15s; }
.btn:hover:not(:disabled) { border-color:var(--sun); color:var(--sun-deep); }
.btn:disabled { opacity:.5; cursor:not-allowed; }
.btn-refresh { background:var(--sun-light); color:var(--sun-deep); border-color:var(--peach); }
.btn-back { background:var(--surface-2); margin-right:18px; }

.section-title { font-size:14px; font-weight:800; color:var(--ink); margin:0 0 14px; display:flex; align-items:center; gap:10px; padding-bottom:6px; border-bottom:1.5px solid var(--border-light); }
.site-count-badge { font-size:11px; font-weight:700; padding:2px 10px; background:var(--sun-light); color:var(--sun-deep); border-radius:10px; }

.view-overview { padding-top:4px; }
.card-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(280px,1fr)); gap:14px; }
.site-card { background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r); padding:16px 18px; cursor:pointer; transition:all .15s; box-shadow:var(--s1); }
.site-card:hover { border-color:var(--sun); box-shadow:var(--s-amber); transform:translateY(-1px); }
.site-card-empty { opacity:.75; }
.site-card-empty:hover { opacity:.95; }
.site-card-err { border-color:#FECACA; background:#FEF2F2; }

.card-head { display:flex; justify-content:space-between; align-items:flex-start; gap:8px; margin-bottom:6px; }
.card-title { font-size:14px; font-weight:800; color:var(--ink); word-break:break-all; line-height:1.35; }
.card-tag { font-size:10.5px; font-weight:700; padding:2px 8px; border-radius:10px; white-space:nowrap; flex-shrink:0; }
.tag-warn { background:#FEF3C7; color:#92400E; }
.tag-err { background:#FEE2E2; color:#B91C1C; }
.tag-muted { background:var(--surface-3); color:var(--ink-hint); }
.card-sub { font-size:11.5px; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; margin-bottom:12px; word-break:break-all; min-height:14px; }
.card-err-msg { font-size:12px; color:#B91C1C; background:#FEF2F2; padding:8px 10px; border-radius:6px; margin-top:6px; }

.card-kpis { display:grid; grid-template-columns:repeat(2,1fr); gap:10px 18px; }
.kpi { }
.kpi-label { font-size:10.5px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.06em; margin-bottom:3px; }
.kpi-value { font-size:16px; font-weight:800; font-family:'JetBrains Mono',monospace; }
.sv-blue { color:#3B82F6; }
.sv-green { color:#10B981; }
.sv-orange { color:#F59E0B; }
.sv-purple { color:#8B5CF6; }

.view-detail { padding-top:4px; }
.detail-head { display:flex; align-items:center; margin-bottom:18px; }
.detail-title { font-size:16px; font-weight:800; color:var(--ink); display:flex; align-items:center; gap:10px; }
.title-platform { background:var(--sun-light); color:var(--sun-deep); padding:3px 12px; border-radius:10px; font-size:13px; }
.title-sep { color:var(--ink-hint); }
.title-site { font-family:'JetBrains Mono',monospace; font-size:14px; word-break:break-all; }

.kpi-strip { display:grid; grid-template-columns:repeat(4,1fr); gap:14px; margin-bottom:18px; }
.kpi-card { text-align:left; padding:16px 20px; }
.kpi-big { font-size:24px; }

.trend-panel { margin-bottom:18px; }
.trend-chart-wrap { height:160px; background:var(--surface-2); border-radius:8px; padding:8px; margin-bottom:6px; }
.trend-svg { width:100%; height:100%; display:block; }
.trend-labels { display:flex; justify-content:space-between; font-size:11px; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; padding:0 8px; }

.two-col { display:grid; grid-template-columns:1fr 1fr; gap:14px; margin-bottom:18px; }
@media (max-width:1000px) { .two-col { grid-template-columns:1fr; } }

.table { width:100%; border-collapse:collapse; }
.table th { padding:9px 12px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.07em; border-bottom:2px solid var(--border); background:var(--surface-3); }
.table td { padding:9px 12px; border-bottom:1px solid var(--border-light); font-size:12.5px; }
.table tr:hover td { background:var(--sun-light); }
.rk { width:36px; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; font-weight:700; }
.num { font-family:'JetBrains Mono',monospace; font-weight:700; text-align:right; white-space:nowrap; }
.kw-cell { max-width:280px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.path-cell { font-family:'JetBrains Mono',monospace; font-size:11.5px; color:var(--ink-soft); max-width:260px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }

.bar-list { display:flex; flex-direction:column; gap:10px; }
.bar-head { display:flex; justify-content:space-between; font-size:12px; margin-bottom:4px; }
.bar-name { font-weight:700; color:var(--ink); }
.bar-val { font-family:'JetBrains Mono',monospace; color:var(--ink-hint); }
.bar-track { height:8px; background:var(--surface-3); border-radius:4px; overflow:hidden; }
.bar-fill { height:100%; background:linear-gradient(90deg,var(--sun),var(--coral)); border-radius:4px; transition:width .3s; }
.bar-green { background:linear-gradient(90deg,#10B981,#059669); }

.state-placeholder,
.state-empty,
.state-error { padding:60px 20px; text-align:center; color:var(--ink-hint); font-size:14px; }
.state-error { color:#B91C1C; }
.err-title { font-size:15px; font-weight:700; margin-bottom:6px; }
.err-hint { font-size:12px; color:var(--ink-hint); }
</style>
