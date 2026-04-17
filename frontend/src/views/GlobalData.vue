<template>
  <div class="global-data">
    <div class="hdr">
      <h1>📈 全局数据总览</h1>
      <p class="hdr-sub">
        所有用户绑定的所有站点 · 4 平台（GSC / GA / Yandex / Bing）最近一次同步的汇总数据。
        点击任意站点进入「4 平台数据」查看完整趋势。
      </p>
    </div>

    <!-- Summary strip -->
    <div v-if="summary" class="summary-row">
      <div class="summary-card">
        <div class="s-num">{{ summary.total_sites || 0 }}</div>
        <div class="s-lbl">总站点</div>
      </div>
      <div class="summary-card">
        <div class="s-num">{{ summary.total_owners || 0 }}</div>
        <div class="s-lbl">管理员工</div>
      </div>
      <div class="summary-card">
        <div class="s-num blue">{{ fmtNum(summary.total_gsc_clicks) }}</div>
        <div class="s-lbl">GSC 点击（30天）</div>
      </div>
      <div class="summary-card">
        <div class="s-num">{{ fmtNum(summary.total_impressions) }}</div>
        <div class="s-lbl">GSC 展示（30天）</div>
      </div>
      <div class="summary-card">
        <div class="s-num green">{{ fmtNum(summary.total_sessions) }}</div>
        <div class="s-lbl">GA 会话（30天）</div>
      </div>
      <div class="summary-card">
        <div class="s-num">{{ fmtNum(summary.total_users) }}</div>
        <div class="s-lbl">GA 用户（30天）</div>
      </div>
    </div>

    <!-- Filter + view toggle -->
    <div class="filter-row">
      <el-select
        v-model="filterUser"
        placeholder="按用户筛选（留空看全部）"
        style="width:260px"
        clearable
        @change="load"
      >
        <el-option
          v-for="u in users"
          :key="u.id"
          :label="u.name + ' (' + u.email + ')'"
          :value="u.id"
        />
      </el-select>
      <input class="search-input" v-model="search" placeholder="搜索站点 / 域名..." />
      <el-radio-group v-model="groupByOwner" size="small">
        <el-radio-button :value="true">按用户分组</el-radio-button>
        <el-radio-button :value="false">单表格</el-radio-button>
      </el-radio-group>
      <span style="flex:1"></span>
      <el-button @click="load" :loading="loading">🔄 刷新</el-button>
    </div>

    <!-- Loading / empty / grouped -->
    <div v-if="loading" class="panel empty-hint">加载中...</div>
    <div v-else-if="!filteredSites.length" class="panel empty-hint">
      <div class="empty-icon">📭</div>
      <div>没有找到匹配的站点</div>
    </div>

    <!-- Grouped view -->
    <template v-else-if="groupByOwner">
      <div v-for="grp in grouped" :key="grp.owner_id || 0" class="owner-group panel">
        <div class="owner-head">
          <div class="owner-title">
            👤 {{ grp.owner_name || '（无所属）' }}
            <span class="owner-count">{{ grp.sites.length }} 个站点</span>
          </div>
        </div>
        <SitesTable :sites="grp.sites" />
      </div>
    </template>

    <!-- Flat view -->
    <div v-else class="panel flat-panel">
      <SitesTable :sites="filteredSites" :show-owner="true" />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, h } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const loading = ref(false)
const sites = ref([])
const summary = ref(null)
const users = ref([])
const filterUser = ref(null)
const search = ref('')
const groupByOwner = ref(true)

onMounted(async () => {
  try {
    const u = await api.listUsers()
    users.value = u.data || []
  } catch (e) { /* non-fatal */ }
  load()
})

async function load() {
  loading.value = true
  try {
    const params = {}
    if (filterUser.value) params.user_id = filterUser.value
    const res = await api.getGlobalData(params)
    sites.value = res.data?.sites || []
    summary.value = res.data?.summary || null
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '加载失败')
  } finally {
    loading.value = false
  }
}

const filteredSites = computed(() => {
  const q = (search.value || '').toLowerCase().trim()
  if (!q) return sites.value
  return sites.value.filter(s =>
    (s.name || '').toLowerCase().includes(q) ||
    (s.domain || '').toLowerCase().includes(q) ||
    (s.owner_name || '').toLowerCase().includes(q)
  )
})

const grouped = computed(() => {
  const map = new Map()
  for (const s of filteredSites.value) {
    const key = s.owner_id || 0
    if (!map.has(key)) {
      map.set(key, { owner_id: key, owner_name: s.owner_name, sites: [] })
    }
    map.get(key).sites.push(s)
  }
  return [...map.values()]
})

function fmtNum(n) {
  if (!n && n !== 0) return '—'
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return String(n)
}

// ─── Inline table component ───────────────────────────────────────────
// Inline render via h() so we don't need to maintain a separate .vue file
// for something only used here. Shows one row per site with a cell per
// platform. Empty platform = "—". Row click → /sites/:id/data.
const SitesTable = {
  props: ['sites', 'showOwner'],
  setup(props) {
    return () => h('table', { class: 'data-table' }, [
      h('thead', null, [
        h('tr', null, [
          props.showOwner ? h('th', { style: 'width:140px' }, '所属') : null,
          h('th', null, '站点'),
          h('th', { style: 'width:100px' }, '审计'),
          h('th', { class: 'col-platform' }, 'GSC'),
          h('th', { class: 'col-platform' }, 'GA'),
          h('th', { class: 'col-platform' }, 'Yandex'),
          h('th', { class: 'col-platform' }, 'Bing'),
          h('th', { style: 'width:110px' }, '操作'),
        ]),
      ]),
      h('tbody', null, props.sites.map(s =>
        h('tr', { key: s.site_id }, [
          props.showOwner ? h('td', { class: 'td-owner' }, s.owner_name || '—') : null,
          h('td', { class: 'td-site' }, [
            h('div', { class: 'site-name' }, s.name),
            h('div', { class: 'site-domain' }, s.domain),
          ]),
          h('td', null, [
            h('span', { class: 'badge ' + auditBadge(s.last_audit_status) }, auditLabel(s.last_audit_status)),
          ]),
          platformCell(s.gsc, ['clicks', 'impressions']),
          platformCell(s.ga, ['sessions', 'users']),
          platformCell(s.yandex, ['clicks', 'index_count']),
          platformCell(s.bing, ['clicks', 'index_count']),
          h('td', null, [
            h('a', {
              href: `/sites/${s.site_id}/data`,
              class: 'btn btn-xs btn-primary',
              onClick: (e) => { e.preventDefault(); window.location.href = `/sites/${s.site_id}/data` },
            }, '📊 查看数据'),
          ]),
        ]),
      )),
    ])
  },
}

function platformCell(d, fields) {
  if (!d) return h('td', { class: 'td-platform off' }, '—')
  const lines = fields.map(f => {
    const v = d[f]
    if (v == null) return null
    return h('div', { class: 'p-line' }, [
      h('span', { class: 'p-lbl' }, labelOf(f)),
      h('span', { class: 'p-val' }, fmtNum(v)),
    ])
  }).filter(Boolean)
  const dateLine = d.date ? h('div', { class: 'p-date' }, '同步 ' + String(d.date).slice(5, 10)) : null
  return h('td', { class: 'td-platform' }, [...lines, dateLine])
}

function labelOf(f) {
  return { clicks: '点击', impressions: '展示', sessions: '会话', users: '用户', index_count: '收录' }[f] || f
}

function auditLabel(s) {
  return { done: '完成', error: '失败', running: '进行中' }[s] || '未审计'
}
function auditBadge(s) {
  return { done: 'badge-ok', error: 'badge-err', running: 'badge-warn' }[s] || 'badge-muted'
}
</script>

<style scoped>
.global-data { max-width: 1600px; }
.hdr { margin-bottom: 14px; }
.hdr h1 { font-size: 18px; font-weight: 800; color: var(--ink); }
.hdr-sub { font-size: 13px; color: var(--ink-soft); line-height: 1.7; margin-top: 6px; }

.panel {
  background: var(--surface);
  border-radius: var(--r);
  box-shadow: var(--s1);
  border: 1.5px solid var(--border-light);
}
.empty-hint { text-align: center; color: var(--ink-hint); padding: 60px 30px; font-size: 14px; }
.empty-icon { font-size: 36px; margin-bottom: 10px; }

.summary-row {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 10px;
  margin-bottom: 14px;
}
.summary-card {
  background: var(--surface);
  border: 1.5px solid var(--border-light);
  border-radius: 10px;
  padding: 14px 18px;
  text-align: center;
  box-shadow: var(--s1);
}
.s-num { font-size: 24px; font-weight: 800; color: var(--sun-deep); letter-spacing: -0.5px; }
.s-num.blue { color: #2563eb; }
.s-num.green { color: var(--ok); }
.s-lbl { font-size: 11px; color: var(--ink-hint); margin-top: 4px; font-weight: 600; text-transform: uppercase; letter-spacing: .06em; }

.filter-row {
  display: flex; gap: 10px; align-items: center; margin-bottom: 14px;
  background: var(--surface);
  border: 1.5px solid var(--border-light);
  border-radius: 10px;
  padding: 12px 14px;
  box-shadow: var(--s1);
}
.search-input {
  flex: 0 0 240px; padding: 8px 12px;
  border: 1.5px solid var(--border); border-radius: 8px;
  font-size: 13px; font-family: inherit; outline: none;
}
.search-input:focus { border-color: var(--sun); }

.owner-group { margin-bottom: 14px; padding: 14px 18px; }
.owner-head { margin-bottom: 10px; padding-bottom: 10px; border-bottom: 1.5px solid var(--border-light); }
.owner-title { font-size: 14px; font-weight: 800; color: var(--ink); display: flex; align-items: center; gap: 10px; }
.owner-count { font-size: 11px; font-weight: 600; color: var(--ink-hint); background: var(--surface-3); padding: 2px 10px; border-radius: 10px; }

.flat-panel { padding: 14px 18px; }

:deep(.data-table) { width: 100%; border-collapse: collapse; }
:deep(.data-table th) {
  padding: 8px 10px; text-align: left; font-size: 11px; font-weight: 800;
  color: var(--ink-mid); text-transform: uppercase; letter-spacing: .08em;
  border-bottom: 2px solid var(--border); background: var(--surface-3);
}
:deep(.data-table td) { padding: 10px; border-bottom: 1px solid var(--border-light); font-size: 13px; vertical-align: top; }
:deep(.data-table tr:hover td) { background: var(--sun-light); }
:deep(.col-platform) { width: 110px; }
:deep(.td-site .site-name) { font-weight: 700; color: var(--ink); }
:deep(.td-site .site-domain) { font-size: 11px; color: var(--ink-soft); margin-top: 2px; }
:deep(.td-owner) { font-size: 12px; font-weight: 600; color: var(--ink-mid); }
:deep(.td-platform) { font-size: 11px; line-height: 1.5; }
:deep(.td-platform.off) { color: var(--ink-hint); }
:deep(.p-line) { display: flex; justify-content: space-between; gap: 8px; }
:deep(.p-lbl) { color: var(--ink-hint); }
:deep(.p-val) { font-weight: 700; color: var(--ink); font-variant-numeric: tabular-nums; }
:deep(.p-date) { font-size: 10px; color: var(--ink-hint); margin-top: 3px; font-style: italic; }

:deep(.badge) { display: inline-block; font-size: 11px; font-weight: 800; padding: 2px 10px; border-radius: 20px; }
:deep(.badge-ok) { background: var(--ok-bg); color: var(--ok); border: 1px solid var(--ok-border); }
:deep(.badge-warn) { background: var(--warn-bg); color: var(--warn); border: 1px solid var(--warn-border); }
:deep(.badge-err) { background: var(--err-bg); color: var(--err); border: 1px solid var(--err-border); }
:deep(.badge-muted) { background: var(--surface-3); color: var(--ink-hint); border: 1px solid var(--border); }

:deep(.btn) {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 4px 10px; border-radius: 6px; border: 1.5px solid var(--border);
  background: var(--surface); cursor: pointer; font-family: inherit;
  font-size: 11px; font-weight: 700; color: var(--ink-mid);
  text-decoration: none; transition: all .15s;
}
:deep(.btn:hover) { background: var(--sun-light); border-color: var(--peach); color: var(--sun-deep); }
:deep(.btn-xs) { padding: 3px 10px; font-size: 11px; }
:deep(.btn-primary) {
  background: linear-gradient(135deg, var(--sun), var(--coral));
  color: #fff; border-color: transparent;
}
:deep(.btn-primary:hover) {
  background: linear-gradient(135deg, var(--sun-deep), var(--sun));
  color: #fff;
}
</style>
