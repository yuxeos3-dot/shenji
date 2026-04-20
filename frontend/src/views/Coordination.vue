<template>
  <div class="coord-page">
    <!-- Header: stats + filters -->
    <div class="panel stats-bar">
      <div class="stat-cell" :class="{active: filter.status===''}" @click="setStatus('')">
        <div class="stat-num">{{ totalAll }}</div>
        <div class="stat-label">全部</div>
      </div>
      <div class="stat-cell pending" :class="{active: filter.status==='pending'}" @click="setStatus('pending')">
        <div class="stat-num">{{ stats.pending || 0 }}</div>
        <div class="stat-label">📋 待分配</div>
      </div>
      <div class="stat-cell review-gate" :class="{active: filter.status==='pending_review'}" @click="setStatus('pending_review')">
        <div class="stat-num">{{ stats.pending_review || 0 }}</div>
        <div class="stat-label">👀 待审核</div>
      </div>
      <div class="stat-cell running" :class="{active: filter.status==='in_progress'}" @click="setStatus('in_progress')">
        <div class="stat-num">{{ stats.in_progress || 0 }}</div>
        <div class="stat-label">🛠 开发中</div>
      </div>
      <div class="stat-cell answer" :class="{active: filter.status==='awaiting_answer'}" @click="setStatus('awaiting_answer')">
        <div class="stat-num">{{ stats.awaiting_answer || 0 }}</div>
        <div class="stat-label">❓ 待回复</div>
      </div>
      <div class="stat-cell review" :class="{active: filter.status==='under_review'}" @click="setStatus('under_review')">
        <div class="stat-num">{{ stats.under_review || 0 }}</div>
        <div class="stat-label">🔍 待验收</div>
      </div>
      <div class="stat-cell done" :class="{active: filter.status==='done'}" @click="setStatus('done')">
        <div class="stat-num">{{ stats.done || 0 }}</div>
        <div class="stat-label">✅ 已完成</div>
      </div>
    </div>

    <!-- Filter / site picker. Admin gets an extra row with team
         filters (部门 / 组 / 姓名搜索) so a manager can slice tasks
         by "whose team" rather than "which site". Members don't see
         team filters — they only see their own sites anyway. -->
    <div class="panel filter-bar">
      <span class="filter-label">站点</span>
      <el-select v-model="filter.site_id" placeholder="全部站点" clearable style="width:260px" @change="reload">
        <el-option v-for="s in sites" :key="s.id" :label="s.name + ' (' + s.domain + ')'" :value="s.id" />
      </el-select>

      <template v-if="userStore.isAdmin()">
        <span class="filter-label" style="margin-left:16px">部门</span>
        <el-select v-model="filter.department_id" placeholder="全部部门" clearable style="width:160px" @change="onDeptChange">
          <el-option v-for="d in departments" :key="d.id" :label="d.name" :value="d.id" />
        </el-select>
        <span class="filter-label">组</span>
        <el-select
          v-model="filter.group_id"
          placeholder="全部组"
          clearable
          :disabled="!filter.department_id"
          style="width:140px"
          @change="reload">
          <el-option v-for="g in filteredGroups" :key="g.id" :label="g.name" :value="g.id" />
        </el-select>
        <span class="filter-label">姓名</span>
        <el-input
          v-model="filter.user_search"
          placeholder="搜索创建人或开发者"
          clearable
          style="width:180px"
          @keyup.enter="reload"
          @clear="reload"
        />
      </template>

      <span class="filter-label" style="margin-left:16px">影响页面</span>
      <el-select v-model="filter.scope" placeholder="全部页面" clearable style="width:160px" @change="reload">
        <el-option v-for="s in scopes" :key="s.scope" :label="`${s.scope} (${s.count})`" :value="s.scope" />
      </el-select>

      <button class="btn btn-sm refresh-btn" @click="reload" :disabled="loading">{{ loading ? '加载中...' : '↻ 刷新' }}</button>
    </div>

    <!-- Site-grouped task list -->
    <div v-if="!tasks.length && !loading" class="empty-hint panel">
      <div class="empty-icon">🤝</div>
      <div>暂无协同任务</div>
      <div class="empty-sub">在「审计详情」页点击问题卡片上的「➕ 加入协同」按钮创建任务</div>
    </div>

    <div v-for="group in groupedBySite" :key="group.site_id" class="panel site-group">
      <div class="site-header">
        <span class="site-icon">🌐</span>
        <span class="site-name">{{ group.site_name }}</span>
        <span class="site-domain">{{ group.site_domain }}</span>
        <span class="site-count">{{ group.tasks.length }} 个任务</span>
      </div>
      <table class="coord-table">
        <thead>
          <tr>
            <th style="width:72px">级别</th>
            <th>问题</th>
            <th style="width:120px">影响页面</th>
            <th style="width:110px">状态</th>
            <th style="width:110px">分配给</th>
            <th style="width:160px">创建时间</th>
            <th style="width:90px">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in group.tasks" :key="t.id" class="coord-row" @click="open(t.id)">
            <td><span :class="['sev-tag', sevClass(t.issue_severity)]">{{ t.issue_severity || '—' }}</span></td>
            <td class="title-cell">
              <div class="issue-title">{{ t.issue_title }}</div>
              <div class="issue-sub" v-if="t.issue_module || t.issue_chapter">
                <span v-if="t.issue_module" class="mod-chip">{{ t.issue_module }}</span>
                <span v-if="t.issue_chapter" class="chapter-chip">{{ t.issue_chapter }}</span>
              </div>
            </td>
            <td>
              <span v-if="t.issue_scope" class="scope-chip">{{ t.issue_scope }}</span>
              <span v-else class="scope-empty">—</span>
            </td>
            <td><span :class="['status-tag', statusClass(t.status)]">{{ statusLabel(t.status) }}</span></td>
            <td>{{ t.assignee_name || '—' }}</td>
            <td class="time-cell">{{ fmtTime(t.created_at) }}</td>
            <td><button class="link-btn" @click.stop="open(t.id)">查看 →</button></td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="total > perPage" class="pager">
      <el-pagination
        v-model:current-page="page"
        :page-size="perPage"
        :total="total"
        layout="prev, pager, next, total"
        background
        @current-change="reload"
      />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import { useUserStore } from '../stores/user'

const router = useRouter()
const userStore = useUserStore()
const sites = ref([])
const tasks = ref([])
const stats = ref({ pending: 0, pending_review: 0, in_progress: 0, under_review: 0, done: 0 })
const loading = ref(false)
// Admin team filters: 部门 / 组 / 姓名. Members don't see these inputs
// so the extra fields stay null and backend treats them as absent.
const filter = ref({
  site_id: null,
  status: '',
  department_id: null,
  group_id: null,
  user_search: '',
  scope: '',
})

// 影响页面 dropdown options come from /v1/coord/scopes — distinct
// scope values + their counts. Refreshed once on mount and on every
// reload so newly-created tasks with new scopes appear.
const scopes = ref([])
async function loadScopes() {
  try {
    const res = await api.coordListScopes()
    scopes.value = res.data || []
  } catch { /* ignore — filter just stays empty */ }
}

const departments = ref([])
const groups = ref([])
const filteredGroups = computed(() =>
  groups.value.filter(g => !filter.value.department_id || g.department_id === filter.value.department_id)
)

function onDeptChange() {
  // Clear group selection when department changes so an orphan
  // (group from a different dept) can't linger in the query.
  filter.value.group_id = null
  reload()
}
const page = ref(1)
const perPage = 100
const total = ref(0)

const totalAll = computed(() =>
  (stats.value.pending || 0) + (stats.value.pending_review || 0) +
  (stats.value.in_progress || 0) +
  (stats.value.awaiting_answer || 0) +
  (stats.value.under_review || 0) + (stats.value.done || 0)
)

// Group tasks by site_id so each site gets its own panel with a
// "Site · N tasks" header — matches the spec ("按照站点区分").
const groupedBySite = computed(() => {
  const map = new Map()
  for (const t of tasks.value) {
    if (!map.has(t.site_id)) {
      map.set(t.site_id, {
        site_id: t.site_id,
        site_name: t.site_name,
        site_domain: t.site_domain,
        tasks: [],
      })
    }
    map.get(t.site_id).tasks.push(t)
  }
  return Array.from(map.values())
})

function statusLabel(s) {
  return {
    pending: '📋 待分配',
    pending_review: '👀 待审核',
    in_progress: '🛠 开发中',
    awaiting_answer: '❓ 待回复',
    under_review: '🔍 待验收',
    done: '✅ 已完成',
  }[s] || s
}
function statusClass(s) { return 's-' + s }
function sevClass(sev) {
  if (sev === '致命') return 'sev-fatal'
  if (sev === '严重') return 'sev-serious'
  if (sev === '中等') return 'sev-medium'
  if (sev === '低') return 'sev-low'
  return 'sev-none'
}
function fmtTime(s) {
  if (!s) return '—'
  // Backend emits UTC RFC3339 or "YYYY-MM-DD HH:MM:SS". Show local time.
  try {
    const d = new Date(s.includes('T') ? s : s.replace(' ', 'T') + 'Z')
    if (Number.isNaN(d.getTime())) return s
    const pad = (n) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
  } catch { return s }
}

function setStatus(s) {
  filter.value.status = s
  page.value = 1
  reload()
}

function open(id) {
  router.push(`/coordination/${id}`)
}

async function reload() {
  loading.value = true
  try {
    const params = { page: page.value, per_page: perPage }
    if (filter.value.site_id) params.site_id = filter.value.site_id
    if (filter.value.status) params.status = filter.value.status
    // Team filters only meaningful for admin — the <template v-if="isAdmin">
    // above is the UI guard, but we also skip sending them when they're
    // falsy so a member's request stays clean.
    if (filter.value.department_id) params.department_id = filter.value.department_id
    if (filter.value.group_id) params.group_id = filter.value.group_id
    const q = String(filter.value.user_search || '').trim()
    if (q) params.user_search = q
    if (filter.value.scope) params.scope = filter.value.scope
    const [tRes, sRes] = await Promise.all([
      api.coordListTasks(params),
      api.coordStats(filter.value.site_id ? { site_id: filter.value.site_id } : {}),
    ])
    tasks.value = tRes.data?.items || []
    total.value = tRes.data?.total || 0
    stats.value = sRes.data || { pending: 0, pending_review: 0, in_progress: 0, under_review: 0, done: 0 }
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

watch(() => filter.value.site_id, () => {
  page.value = 1
  reload()
})

onMounted(async () => {
  try {
    const res = await api.listSites()
    sites.value = res.data || []
  } catch { /* ignore */ }
  // Admin-only team filters need dept + group lookups to populate the
  // select dropdowns. Members never render those controls so skip the
  // round-trip entirely.
  if (userStore.isAdmin()) {
    try {
      const [dRes, gRes] = await Promise.all([
        api.listDepartments(),
        api.listGroups(),
      ])
      departments.value = dRes.data || []
      groups.value = gRes.data || []
    } catch { /* ignore — filters just stay empty */ }
  }
  await loadScopes()
  await reload()
})
</script>

<style scoped>
.coord-page { display:flex; flex-direction:column; gap:16px; }
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:16px 22px; }

.stats-bar { display:grid; grid-template-columns:repeat(7,1fr); gap:12px; padding:14px; }
.stat-cell { text-align:center; padding:16px 12px; border-radius:var(--r-sm); cursor:pointer; transition:all .18s; border:1.5px solid var(--border-light); background:var(--surface-2); }
.stat-cell:hover { transform:translateY(-1px); box-shadow:var(--s1); border-color:var(--peach); }
.stat-cell.active { background:linear-gradient(135deg,var(--sun-light),#FFF); border-color:var(--sun); box-shadow:0 3px 12px rgba(249,115,22,.2); }
.stat-cell .stat-num { font-size:26px; font-weight:800; font-family:'JetBrains Mono',monospace; color:var(--ink); line-height:1; }
.stat-cell .stat-label { font-size:12px; font-weight:700; color:var(--ink-soft); margin-top:6px; }
.stat-cell.pending.active .stat-num { color:#3B82F6; }
.stat-cell.review-gate.active .stat-num { color:#BE185D; }
.stat-cell.running.active .stat-num { color:var(--warn); }
.stat-cell.answer.active .stat-num { color:#B91C1C; }
.stat-cell.review.active .stat-num { color:#7C3AED; }
.stat-cell.done.active .stat-num { color:var(--ok); }

.filter-bar { display:flex; align-items:center; gap:12px; flex-wrap:wrap; }
.filter-label { font-size:13px; font-weight:700; color:var(--ink-mid); }
.refresh-btn { margin-left:auto; }

.site-group { padding:0; overflow:hidden; }
.site-header { display:flex; align-items:center; gap:10px; padding:14px 20px; background:linear-gradient(135deg,var(--sun-light),#FFF); border-bottom:1.5px solid var(--border-light); }
.site-icon { font-size:18px; }
.site-name { font-size:15px; font-weight:800; color:var(--ink); }
.site-domain { font-size:12px; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; }
.site-count { margin-left:auto; font-size:12px; font-weight:700; color:var(--sun-deep); background:var(--sun-light); padding:3px 10px; border-radius:20px; border:1px solid var(--peach); }

.coord-table { width:100%; border-collapse:collapse; }
.coord-table th { padding:10px 14px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.08em; background:var(--surface-3); border-bottom:2px solid var(--border); }
.coord-table td { padding:12px 14px; border-bottom:1px solid var(--border-light); font-size:13px; vertical-align:middle; }
.coord-row { cursor:pointer; transition:background .15s; }
.coord-row:hover td { background:var(--sun-light); }

.sev-tag { display:inline-block; padding:2px 9px; border-radius:4px; font-size:11px; font-weight:800; color:#fff; }
.sev-fatal { background:#C0392B; } .sev-serious { background:#E67E22; } .sev-medium { background:#F39C12; } .sev-low { background:#27AE60; } .sev-none { background:#94A3B8; }

.title-cell .issue-title { font-size:13.5px; font-weight:700; color:var(--ink); margin-bottom:4px; }
.title-cell .issue-sub { display:flex; gap:6px; }
.mod-chip { font-size:10.5px; padding:1px 7px; background:#34495E; color:#ECF0F1; border-radius:3px; font-weight:700; }
.scope-chip { display:inline-block; font-size:11.5px; padding:2px 10px; background:var(--sun-light); color:var(--sun-deep); border:1px solid var(--peach); border-radius:12px; font-weight:700; }
.scope-empty { color:var(--ink-hint); font-size:13px; }
.chapter-chip { font-size:10.5px; padding:1px 7px; background:var(--surface-3); color:var(--ink-soft); border-radius:3px; font-weight:600; }

.status-tag { display:inline-block; padding:3px 9px; border-radius:12px; font-size:11px; font-weight:700; }
.s-pending { background:#DBEAFE; color:#1D4ED8; }
.s-pending_review { background:#FFE4E6; color:#BE185D; }
.s-in_progress { background:#FEF3C7; color:#92400E; }
.s-awaiting_answer { background:#FEE2E2; color:#B91C1C; }
.s-under_review { background:#EDE9FE; color:#6D28D9; }
.s-done { background:var(--ok-bg); color:var(--ok); }

.time-cell { font-size:12px; color:var(--ink-soft); font-family:'JetBrains Mono',monospace; }
.link-btn { background:none; border:none; color:var(--info); cursor:pointer; font-weight:700; font-size:12.5px; font-family:inherit; padding:4px 8px; }
.link-btn:hover { text-decoration:underline; }

.empty-hint { text-align:center; padding:60px 30px; color:var(--ink-hint); }
.empty-icon { font-size:50px; margin-bottom:16px; }
.empty-sub { font-size:13px; margin-top:8px; color:var(--ink-soft); }
.pager { display:flex; justify-content:center; margin-top:12px; }

.btn { display:inline-flex; align-items:center; gap:6px; padding:7px 16px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13px; font-weight:700; color:var(--ink-mid); transition:all .15s; }
.btn:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); }
.btn:disabled { opacity:.6; cursor:not-allowed; }
.btn-sm { padding:6px 14px; font-size:12.5px; }
</style>
