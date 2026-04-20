<template>
  <div>
    <!-- Tab switcher between daily and weekly reports -->
    <div class="tabs">
      <router-link to="/reports" class="tab-btn" active-class="tab-active" exact-active-class="tab-active">📝 日汇报</router-link>
      <router-link to="/reports/weekly" class="tab-btn">📊 周汇报</router-link>
      <router-link to="/reports/monthly" class="tab-btn">📅 月汇报</router-link>
    </div>

    <div class="page-top">
      <div class="filter-row">
        <span class="filter-label">日期筛选</span>
        <el-date-picker v-model="filterDate" type="date" value-format="YYYY-MM-DD" placeholder="选择日期" @change="loadReports" clearable size="small" />
        <!-- Admin gets team filters (部门 / 组 / 姓名搜索) + direct
             成员 picker. Non-admins see only their own reports so these
             don't appear. -->
        <template v-if="userStore.isAdmin()">
          <span class="filter-label" style="margin-left:8px">成员</span>
          <el-select v-model="filterUser" placeholder="全部成员" clearable size="small" style="width:140px" @change="loadReports">
            <el-option v-for="u in users" :key="u.id" :label="u.name" :value="u.id" />
          </el-select>
          <span class="filter-label" style="margin-left:8px">部门</span>
          <el-select v-model="filterDept" placeholder="全部部门" clearable size="small" style="width:130px" @change="onDeptChange">
            <el-option v-for="d in departments" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
          <span class="filter-label">组</span>
          <el-select v-model="filterGroup" placeholder="全部组" clearable size="small" :disabled="!filterDept" style="width:120px" @change="loadReports">
            <el-option v-for="g in filteredGroups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
          <span class="filter-label">姓名</span>
          <el-input v-model="filterSearch" placeholder="搜索姓名" clearable size="small" style="width:150px" @keyup.enter="loadReports" @clear="loadReports" />
        </template>
      </div>
      <button class="btn btn-primary" @click="$router.push('/reports/write')">📝 写汇报</button>
    </div>

    <div class="panel">
      <table class="table">
        <thead><tr><th>提交人</th><th>日期</th><th>内容摘要</th><th>提交时间</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="r in reports" :key="r.id">
            <td style="font-weight:700">{{ r.user_name }}</td>
            <td><span class="date-badge">{{ r.report_date }}</span></td>
            <td style="color:var(--ink-mid);max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{{ r.content }}</td>
            <td style="font-size:12px;color:var(--ink-soft)">{{ r.created_at }}</td>
            <td><button class="btn btn-xs" @click="viewReport(r)">查看</button></td>
          </tr>
          <tr v-if="!reports.length"><td colspan="5" class="empty-cell">暂无汇报</td></tr>
        </tbody>
      </table>
    </div>

    <el-dialog v-model="showDetail" title="📋 汇报详情" width="720px" top="5vh">
      <div v-if="sel" class="report-detail">
        <div class="report-meta">{{ sel.user_name }} · {{ sel.report_date }}</div>
        <div class="report-content">{{ sel.content }}</div>
        <div v-if="parseTasks(sel.tasks_done).length" class="report-section">
          <h4>✅ 今日完成</h4>
          <ul><li v-for="(t, i) in parseTasks(sel.tasks_done)" :key="i">{{ t }}</li></ul>
        </div>
        <div v-if="parseTasks(sel.tasks_todo).length" class="report-section">
          <h4>📌 明日计划</h4>
          <ul><li v-for="(t, i) in parseTasks(sel.tasks_todo)" :key="i">{{ t }}</li></ul>
        </div>
        <div v-if="sel.issues" class="report-section">
          <h4>⚠️ 问题与风险</h4>
          <p>{{ sel.issues }}</p>
        </div>

        <!-- Evaluations mount. :key ensures the inner component
             reloads when the user opens a different report. -->
        <EvaluationsPanel
          :key="sel.id"
          report-type="daily"
          :report-id="sel.id"
        />
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useUserStore } from '../stores/user'
import api from '../api'
import EvaluationsPanel from '../components/EvaluationsPanel.vue'

const userStore = useUserStore()
const reports = ref([])
const filterDate = ref('')
const filterUser = ref(null)
const users = ref([])
// Admin team filters: 部门 / 组 / 姓名搜索. Members never render these.
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
  loadReports()
}
const showDetail = ref(false)
const sel = ref(null)

async function loadReports() {
  try {
    const params = {}
    if (filterDate.value) params.date = filterDate.value
    if (filterUser.value) params.user_id = filterUser.value
    if (filterDept.value) params.department_id = filterDept.value
    if (filterGroup.value) params.group_id = filterGroup.value
    const q = String(filterSearch.value || '').trim()
    if (q) params.user_search = q
    const res = await api.listReports(params)
    reports.value = res.data
  } catch (e) { console.error(e) }
}

function viewReport(r) { sel.value = r; showDetail.value = true }
function parseTasks(v) { if (!v) return []; try { const a = typeof v === 'string' ? JSON.parse(v) : v; return Array.isArray(a) ? a : [] } catch { return [] } }

onMounted(async () => {
  if (userStore.isAdmin()) {
    try { users.value = (await api.listUsers()).data } catch (e) { console.error(e) }
    try {
      const [dRes, gRes] = await Promise.all([api.listDepartments(), api.listGroups()])
      departments.value = dRes.data || []
      groups.value = gRes.data || []
    } catch (e) { console.error(e) }
  }
  loadReports()
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

.date-badge { font-size:12px; font-weight:700; color:var(--sun); background:var(--sun-light); padding:2px 10px; border-radius:20px; border:1px solid var(--peach); }
.empty-cell { text-align:center; color:var(--ink-hint); padding:30px !important; }

.report-detail { }
.report-meta { font-size:14px; font-weight:700; color:var(--sun); margin-bottom:14px; }
.report-content { white-space:pre-wrap; font-size:14px; line-height:1.8; color:var(--ink); margin-bottom:16px; padding:14px; background:var(--surface-2); border-radius:9px; border:1px solid var(--border-light); }
.report-section { margin-bottom:14px; }
.report-section h4 { font-size:13px; font-weight:700; margin-bottom:6px; }
.report-section ul { padding-left:20px; font-size:13px; color:var(--ink-mid); }
.report-section li { margin-bottom:4px; }
.report-section p { font-size:13px; color:var(--ink-mid); }
</style>
