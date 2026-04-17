<template>
  <div class="global-data">
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
          <button class="btn btn-refresh" @click="load(true)" :disabled="loading">
            {{ loading ? '加载中...' : '🔄 刷新' }}
          </button>
        </div>
      </div>
      <div class="tb-meta" v-if="dateRange[0] && dateRange[1]">
        <span class="meta-item">📅 {{ dateRange[0] }} → {{ dateRange[1] }}</span>
        <span class="meta-item" v-if="dayCount === 1">单日数据</span>
        <span class="meta-item" v-else>共 {{ dayCount }} 天</span>
        <span class="meta-item">
          👥 <el-select v-model="filterUser" placeholder="按用户筛选（全部）" clearable size="small" style="width:180px" @change="load(false)">
            <el-option v-for="u in users" :key="u.id" :label="u.name" :value="u.id" />
          </el-select>
        </span>
      </div>
    </div>

    <!-- ─── Card grid ─────────────────────────────────────────────────── -->
    <div class="section-title">
      {{ platformLabel }} · 总览
      <span class="site-count-badge" v-if="cards.length">{{ cards.length }} 个站点</span>
      <span class="site-count-badge meta-owner" v-if="summary?.total_owners">
        {{ summary.total_owners }} 位管理员工
      </span>
    </div>

    <div v-if="loading && !cards.length" class="state-placeholder">加载中...</div>
    <div v-else-if="error" class="state-error">
      <div class="err-title">❌ {{ error }}</div>
    </div>
    <div v-else-if="!cards.length" class="state-empty">
      该日期范围内没有数据 — 请确认用户已完成站点同步
    </div>

    <div v-else class="card-grid">
      <div
        v-for="(c, idx) in cards"
        :key="idx"
        :class="['site-card', !c.has_data && 'site-card-empty', c.error && 'site-card-err']"
        @click="openCard(c)"
      >
        <div class="card-head">
          <div class="card-title">{{ c.display_name || c.site_url || c.host_id || c.property_id }}</div>
          <span v-if="c.error" class="card-tag tag-err">错误</span>
          <span v-else-if="!c.has_data" class="card-tag tag-muted">无数据</span>
        </div>
        <div class="card-sub">{{ cardSubtitle(c) }}</div>
        <div class="card-owner" v-if="c.owner_name">
          <span class="owner-icon">👤</span>{{ c.owner_name }}
        </div>
        <div v-if="c.error" class="card-err-msg">{{ c.error }}</div>

        <!-- GSC / Bing / Yandex: clicks / impressions / ctr / position -->
        <template v-else-if="platform !== 'ga4'">
          <div class="card-kpis">
            <div class="kpi">
              <div class="kpi-label">点击</div>
              <div class="kpi-value sv-blue">{{ fmt(c.clicks) }}</div>
            </div>
            <div class="kpi">
              <div class="kpi-label">展示</div>
              <div class="kpi-value sv-green">{{ fmt(c.impressions) }}</div>
            </div>
            <div class="kpi">
              <div class="kpi-label">CTR</div>
              <div class="kpi-value sv-orange">{{ ((c.ctr || 0) * 100).toFixed(2) }}%</div>
            </div>
            <div class="kpi">
              <div class="kpi-label">{{ platform === 'yandex' ? '平均位置' : '平均排名' }}</div>
              <div class="kpi-value sv-purple">{{ (c.position || 0).toFixed(1) }}</div>
            </div>
          </div>
        </template>

        <!-- GA4: sessions / users / newUsers / pageViews -->
        <template v-else>
          <div class="card-kpis">
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

    <!-- Per-user error toast list so admin knows who needs to re-auth -->
    <div v-if="userErrors.length" class="user-errors panel">
      <div class="err-head">⚠ 部分用户数据获取失败</div>
      <div v-for="e in userErrors" :key="e.user_id" class="err-row">
        <span class="err-user">👤 {{ e.owner_name }}</span>
        <span class="err-msg">{{ e.error }}</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

// ── Platforms ────────────────────────────────────────────────────────────
const platforms = [
  { code: 'gsc', label: 'GSC' },
  { code: 'ga4', label: 'GA4' },
  { code: 'bing', label: 'Bing' },
  { code: 'yandex', label: 'Yandex' },
]
const platform = ref('gsc')
const platformLabel = computed(() => platforms.find(p => p.code === platform.value)?.label || '')

// ── Date range + presets ────────────────────────────────────────────────
function ymd(d) {
  const pad = n => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
function daysAgo(n) {
  const d = new Date()
  d.setDate(d.getDate() - n)
  return ymd(d)
}
const dateRange = ref([daysAgo(29), daysAgo(2)])
const activePreset = ref('28d')
const presets = [
  { key: 'recent1', label: '最近一天' },
  { key: '7d', label: '近 7 天' },
  { key: '28d', label: '近 28 天' },
  { key: '90d', label: '近 90 天' },
]
function applyPreset(key) {
  activePreset.value = key
  switch (key) {
    case 'recent1': dateRange.value = [daysAgo(2), daysAgo(2)]; break
    case '7d':      dateRange.value = [daysAgo(8), daysAgo(2)]; break
    case '28d':     dateRange.value = [daysAgo(29), daysAgo(2)]; break
    case '90d':     dateRange.value = [daysAgo(91), daysAgo(2)]; break
  }
  load(false)
}
function onDateChange() {
  activePreset.value = 'custom'
  load(false)
}
function disabledDate(date) {
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

// ── State ───────────────────────────────────────────────────────────────
const loading = ref(false)
const error = ref('')
const cards = ref([])
const summary = ref(null)
const userErrors = ref([])
const users = ref([])
const filterUser = ref(null)

// 5-min in-memory cache keyed by platform:start:end:user
const cache = new Map()
function cacheKey() { return `${platform.value}:${dateRange.value[0]}:${dateRange.value[1]}:${filterUser.value || ''}` }

function switchPlatform(code) {
  if (platform.value === code) return
  platform.value = code
  load(false)
}

async function load(bypassCache) {
  const k = cacheKey()
  if (!bypassCache) {
    const hit = cache.get(k)
    if (hit && Date.now() - hit.at < 5 * 60 * 1000) {
      cards.value = hit.data.cards || []
      summary.value = hit.data.summary || null
      userErrors.value = hit.data.user_errors || []
      return
    }
  }
  loading.value = true
  error.value = ''
  try {
    const res = await api.getGlobalData({
      platform: platform.value,
      start_date: dateRange.value[0],
      end_date: dateRange.value[1],
      user_id: filterUser.value || undefined,
    })
    cards.value = res.data?.cards || []
    summary.value = res.data?.summary || null
    userErrors.value = res.data?.user_errors || []
    cache.set(k, { at: Date.now(), data: res.data })
  } catch (e) {
    error.value = e?.response?.data?.error || '加载失败'
    cards.value = []
  } finally {
    loading.value = false
  }
}

// Provider-side sites aren't rows in our `sites` table so they don't have
// a route to drill into. For now clicking a card just shows a toast with
// the provider's identifier — the admin uses this page for at-a-glance
// health check, not deep per-site analysis. Deep drilldown stays in the
// owner's own 数据管理 page where they can click from.
function openCard(c) {
  const ident = c.site_url || c.property_id || c.host_id || c.display_name
  ElMessage.info(`${c.owner_name || '(未知)'} · ${ident}`)
}

function cardSubtitle(c) {
  switch (platform.value) {
    case 'gsc':    return c.is_domain ? 'Domain Property' : (c.site_url || '')
    case 'ga4':    return c.account_name ? '账户：' + c.account_name : ''
    case 'bing':   return c.site_url || ''
    case 'yandex': return c.host_id || ''
  }
  return ''
}

function fmt(n) {
  if (!n && n !== 0) return '—'
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return String(n)
}

onMounted(async () => {
  try {
    const u = await api.listUsers()
    users.value = u.data || []
  } catch (e) { /* non-fatal */ }
  load(false)
})
</script>

<style scoped>
.global-data { max-width: 1600px; }

.panel {
  background: var(--surface);
  border-radius: var(--r);
  box-shadow: var(--s1);
  border: 1.5px solid var(--border-light);
}

/* ─── Toolbar (copied from DataManagement for consistency) ────────── */
.toolbar { padding: 14px 18px; margin-bottom: 14px; }
.tb-row { display: flex; justify-content: space-between; align-items: center; gap: 14px; flex-wrap: wrap; }
.tb-tabs { display: flex; gap: 8px; }
.tb-date { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.tb-meta { margin-top: 10px; display: flex; gap: 14px; font-size: 12px; color: var(--ink-hint); flex-wrap: wrap; align-items: center; }
.meta-item { display: inline-flex; align-items: center; gap: 6px; }
.meta-owner { color: var(--sun-deep); font-weight: 700; }

.pill {
  padding: 6px 16px; border-radius: 20px;
  border: 1.5px solid var(--border); background: var(--surface);
  font-size: 13px; font-weight: 700; color: var(--ink-mid);
  cursor: pointer; transition: all .15s; font-family: inherit;
}
.pill:hover { border-color: var(--peach); color: var(--sun-deep); }
.pill.active { background: linear-gradient(135deg, var(--sun), var(--coral)); color: #fff; border-color: transparent; box-shadow: var(--s-amber); }
.pill-sm { padding: 5px 12px; font-size: 12px; }
.btn-refresh {
  padding: 6px 14px; border-radius: 8px; border: 1.5px solid var(--border);
  background: var(--surface); font-size: 13px; font-weight: 700; color: var(--ink-mid);
  cursor: pointer; font-family: inherit;
}
.btn-refresh:hover:not(:disabled) { border-color: var(--sun); color: var(--sun-deep); }
.btn-refresh:disabled { opacity: .6; cursor: not-allowed; }

.section-title {
  font-size: 14px; font-weight: 800; color: var(--ink);
  margin: 18px 2px 12px; display: flex; align-items: center; gap: 10px;
}
.site-count-badge {
  font-size: 11px; font-weight: 600; color: var(--sun-deep);
  background: var(--sun-light); padding: 2px 10px; border-radius: 10px;
  border: 1px solid var(--peach);
}

/* ─── Cards (copied from DataManagement) ───────────────────────────── */
.card-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 14px; }
.site-card {
  background: var(--surface);
  border: 1.5px solid var(--border-light);
  border-radius: var(--r);
  padding: 16px 18px;
  cursor: pointer;
  transition: all .15s;
  box-shadow: var(--s1);
}
.site-card:hover { border-color: var(--sun); box-shadow: var(--s-amber); transform: translateY(-1px); }
.site-card-empty { opacity: .75; }
.site-card-empty:hover { opacity: .95; }

.card-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 8px; margin-bottom: 6px; }
.card-title { font-size: 14px; font-weight: 800; color: var(--ink); word-break: break-all; line-height: 1.35; }
.card-tag { font-size: 10.5px; font-weight: 700; padding: 2px 8px; border-radius: 10px; white-space: nowrap; flex-shrink: 0; }
.tag-muted { background: var(--surface-3); color: var(--ink-hint); }
.card-sub { font-size: 11.5px; color: var(--ink-hint); font-family: 'JetBrains Mono', monospace; margin-bottom: 8px; word-break: break-all; min-height: 14px; }
.card-owner {
  display: inline-flex; align-items: center; gap: 4px;
  font-size: 11px; font-weight: 700; color: var(--sun-deep);
  background: var(--sun-light); padding: 2px 8px; border-radius: 8px;
  margin-bottom: 12px; border: 1px solid var(--peach);
}
.owner-icon { font-size: 11px; }

.card-kpis { display: grid; grid-template-columns: repeat(2, 1fr); gap: 10px 18px; }
.kpi-label { font-size: 10.5px; font-weight: 700; color: var(--ink-hint); text-transform: uppercase; letter-spacing: .06em; margin-bottom: 3px; }
.kpi-value { font-size: 16px; font-weight: 800; font-family: 'JetBrains Mono', monospace; }
.sv-blue { color: #3B82F6; }
.sv-green { color: #10B981; }
.sv-orange { color: #F59E0B; }
.sv-purple { color: #8B5CF6; }

.state-placeholder, .state-empty, .state-error { padding: 60px 20px; text-align: center; color: var(--ink-hint); font-size: 14px; }
.state-error { color: #B91C1C; }
.err-title { font-size: 15px; font-weight: 700; margin-bottom: 6px; }

.site-card-err { border-color: #FECACA; background: #FEF2F2; }
.tag-err { background: #FEE2E2; color: #B91C1C; }
.card-err-msg { font-size: 12px; color: #B91C1C; background: #FEF2F2; padding: 8px 10px; border-radius: 6px; margin-top: 6px; }

.user-errors {
  margin-top: 18px; padding: 14px 18px;
  background: #FEF3C7; border: 1.5px solid #FCD34D;
}
.err-head { font-size: 13px; font-weight: 800; color: #92400E; margin-bottom: 8px; }
.err-row { display: flex; gap: 14px; font-size: 12px; padding: 4px 0; }
.err-user { font-weight: 700; color: #92400E; min-width: 120px; }
.err-msg { color: #78350F; font-family: 'JetBrains Mono', monospace; }
</style>
