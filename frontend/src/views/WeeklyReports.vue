<template>
  <div>
    <!-- Tab switcher — three sibling views: daily, weekly, monthly. -->
    <div class="tabs">
      <router-link to="/reports" class="tab-btn">📝 日汇报</router-link>
      <router-link to="/reports/weekly" class="tab-btn" active-class="tab-active">📊 周汇报</router-link>
      <router-link to="/reports/monthly" class="tab-btn">📅 月汇报</router-link>
    </div>

    <div class="page-top">
      <div class="filter-row">
        <template v-if="userStore.isAdmin()">
          <span class="filter-label">成员</span>
          <el-select v-model="filterUser" placeholder="全部成员" clearable size="small" style="width:160px" @change="load">
            <el-option v-for="u in users" :key="u.id" :label="u.name + (u.department_name ? ' · ' + u.department_name : '')" :value="u.id" />
          </el-select>
          <span class="filter-label" style="margin-left:8px">部门</span>
          <el-select v-model="filterDept" placeholder="全部部门" clearable size="small" style="width:130px" @change="onDeptChange">
            <el-option v-for="d in departments" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
          <span class="filter-label">组</span>
          <el-select v-model="filterGroup" placeholder="全部组" clearable size="small" :disabled="!filterDept" style="width:120px" @change="load">
            <el-option v-for="g in filteredGroups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
          <span class="filter-label">姓名</span>
          <el-input v-model="filterSearch" placeholder="搜索姓名" clearable size="small" style="width:150px" @keyup.enter="load" @clear="load" />
        </template>
      </div>
      <button class="btn btn-primary" @click="$router.push('/reports/weekly/new')">➕ 生成新周汇报</button>
    </div>

    <div class="panel">
      <table class="table">
        <thead>
          <tr>
            <th v-if="userStore.isAdmin()">提交人</th>
            <th>周起止</th>
            <th>状态</th>
            <th>源日汇报</th>
            <th>提交时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in reports" :key="r.id">
            <td v-if="userStore.isAdmin()" style="font-weight:700">
              {{ r.user_name }}
              <div v-if="r.department_name || r.group_name" class="sub-meta">
                <span v-if="r.department_name">{{ r.department_name }}</span>
                <span v-if="r.group_name"> · {{ r.group_name }}</span>
              </div>
            </td>
            <td>
              <span class="date-badge">{{ r.week_start }} ~ {{ r.week_end }}</span>
            </td>
            <td>
              <span :class="['status-badge', 'sb-' + r.status]">
                {{ r.status === 'submitted' ? '✓ 已提交' : '草稿' }}
              </span>
            </td>
            <td class="mono">{{ srcCount(r) }}</td>
            <td style="font-size:12px;color:var(--ink-soft)">
              {{ r.submitted_at || '—' }}
            </td>
            <td>
              <button class="btn btn-xs" @click="$router.push('/reports/weekly/' + r.id)">
                {{ r.status === 'submitted' ? '查看' : '编辑' }}
              </button>
            </td>
          </tr>
          <tr v-if="!reports.length && !loading">
            <td :colspan="userStore.isAdmin() ? 6 : 5" class="empty-cell">
              暂无周汇报。点击右上角「➕ 生成新周汇报」选一个日期范围，系统会把你的日汇报合成一篇周汇报。
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useUserStore } from '../stores/user'
import api from '../api'

const userStore = useUserStore()
const reports = ref([])
const users = ref([])
const filterUser = ref(null)
// Admin team filters: 部门 / 组 / 姓名. For admin, we route through the
// listAllWeeklyReports endpoint (has these filter params); non-admin
// keeps using listWeeklyReports which is scoped to the caller.
const filterDept = ref(null)
const filterGroup = ref(null)
const filterSearch = ref('')
const departments = ref([])
const groups = ref([])
const filteredGroups = computed(() =>
  groups.value.filter(g => !filterDept.value || g.department_id === filterDept.value)
)
function onDeptChange() {
  filterGroup.value = null
  load()
}
const loading = ref(false)

function srcCount(r) {
  try {
    const arr = JSON.parse(r.source_daily_ids || '[]')
    return Array.isArray(arr) ? `${arr.length} 篇` : '—'
  } catch {
    return '—'
  }
}

async function load() {
  loading.value = true
  try {
    const params = {}
    if (userStore.isAdmin()) {
      // Admin: route through /reports/weekly/all so dept/group/search
      // are honored server-side. user_id filter still works (lives on
      // both endpoints), and with no filters set it returns every
      // user's reports — matching the 汇报管理 mental model.
      if (filterUser.value) params.user_id = filterUser.value
      if (filterDept.value) params.department_id = filterDept.value
      if (filterGroup.value) params.group_id = filterGroup.value
      const q = String(filterSearch.value || '').trim()
      if (q) params.user_search = q
      // When a specific user_id is set, stick with the single-user
      // endpoint for a more focused response; otherwise go team-wide.
      const res = filterUser.value
        ? await api.listWeeklyReports(params)
        : await api.listAllWeeklyReports(params)
      reports.value = res.data || []
    } else {
      const res = await api.listWeeklyReports({})
      reports.value = res.data || []
    }
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  if (userStore.isAdmin()) {
    try { users.value = (await api.listUsers()).data || [] } catch { /* ignore */ }
    try {
      const [dRes, gRes] = await Promise.all([api.listDepartments(), api.listGroups()])
      departments.value = dRes.data || []
      groups.value = gRes.data || []
    } catch { /* filters just stay empty */ }
  }
  await load()
})
</script>

<style scoped>
.tabs { display:flex; gap:8px; margin-bottom:18px; border-bottom:2px solid var(--border-light); }
.tab-btn { padding:10px 20px; font-size:14px; font-weight:700; color:var(--ink-soft); text-decoration:none; border-bottom:2px solid transparent; margin-bottom:-2px; transition:all .15s; }
.tab-btn:hover { color:var(--sun-deep); }
.tab-active { color:var(--sun); border-bottom-color:var(--sun); }

.page-top { display:flex; justify-content:space-between; align-items:center; margin-bottom:18px; }
.filter-row { display:flex; align-items:center; gap:10px; }
.filter-label { font-size:12px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep),var(--sun)); color:#fff; }
.btn-xs { padding:4px 12px; font-size:12px; border-radius:6px; }
.btn-xs:hover { background:var(--sun-glow); border-color:var(--sun); color:var(--sun-deep); }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }

.table { width:100%; border-collapse:collapse; }
.table th { padding:10px 14px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.08em; border-bottom:2px solid var(--border); background:var(--surface-3); }
.table td { padding:10px 14px; border-bottom:1px solid var(--border-light); font-size:13px; }
.table tr:hover td { background:var(--sun-light); }

.date-badge { font-size:12px; font-weight:700; color:var(--sun); background:var(--sun-light); padding:3px 10px; border-radius:14px; border:1px solid var(--peach); font-family:'JetBrains Mono', monospace; }
.status-badge { font-size:11.5px; font-weight:800; padding:3px 10px; border-radius:12px; }
.sb-draft { background:#FEF3C7; color:#92400E; border:1px solid #FCD34D; }
.sb-submitted { background:var(--ok-bg); color:var(--ok); border:1px solid var(--ok-border); }

.sub-meta { font-size:11px; color:var(--ink-hint); font-weight:500; margin-top:2px; }
.mono { font-family:'JetBrains Mono', monospace; font-size:12px; color:var(--ink-soft); }
.empty-cell { text-align:center; color:var(--ink-hint); padding:40px !important; line-height:1.7; }
</style>
