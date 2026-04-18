<template>
  <div>
    <!-- Department + group management panel.
         Admins use this to maintain the two-level org chart. Users
         attach to a (department, group) pair in the form below. -->
    <div class="panel dept-panel">
      <div class="panel-head">
        <div class="panel-title">🏢 部门与组管理</div>
        <div class="panel-actions">
          <button class="btn btn-sm" @click="openAddDept">+ 添加部门</button>
          <button class="btn btn-sm" @click="openAddGroup" :disabled="!departments.length">+ 添加组</button>
        </div>
      </div>
      <div v-if="!departments.length" class="empty-cell" style="padding:22px">
        暂无部门。点击「+ 添加部门」开始建立组织结构。
      </div>
      <div v-else class="dept-list">
        <div v-for="d in departments" :key="d.id" class="dept-card">
          <div class="dept-card-head">
            <div class="dept-name">
              <span class="dept-icon">🏢</span>
              {{ d.name }}
              <span class="dept-count">{{ d.user_count }} 人 · {{ d.group_count }} 组</span>
            </div>
            <div class="dept-actions">
              <button class="btn btn-xs" @click="openEditDept(d)">✏</button>
              <button class="btn btn-xs btn-danger-xs" @click="deleteDept(d)">🗑</button>
            </div>
          </div>
          <div v-if="groupsByDept[d.id]?.length" class="group-chip-row">
            <div v-for="g in groupsByDept[d.id]" :key="g.id" class="group-chip">
              <span class="group-name">{{ g.name }}</span>
              <span class="group-count">{{ g.user_count }} 人</span>
              <button class="chip-btn" @click="openEditGroup(g)" title="编辑组">✏</button>
              <button class="chip-btn chip-danger" @click="deleteGroup(g)" title="删除组">✕</button>
            </div>
          </div>
          <div v-else class="empty-groups">该部门下还没有组</div>
        </div>
      </div>
    </div>

    <div class="page-top">
      <div>
        <button
          v-if="pendingUsers.length > 0"
          class="btn btn-pending"
          @click="showPending = true"
        >
          📋 待审核注册（{{ pendingUsers.length }}）
        </button>
      </div>
      <button class="btn btn-primary" @click="openAdd">👤 添加用户</button>
    </div>

    <!-- ── Pending registrations dialog ─────────────────────────── -->
    <el-dialog v-model="showPending" title="📋 注册申请审核" width="900px" top="5vh">
      <div v-if="!pendingUsers.length" class="empty-cell" style="padding:40px;text-align:center">
        没有待审核的注册申请
      </div>
      <table v-else class="table">
        <thead>
          <tr>
            <th>申请时间</th>
            <th>花名 / 工号</th>
            <th>TG</th>
            <th>部门 / 组</th>
            <th>邀请码</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in pendingUsers" :key="p.id">
            <td style="font-size:12px;color:var(--ink-soft)">{{ p.created_at }}</td>
            <td>
              <div style="font-weight:700">{{ p.nickname }}</div>
              <div style="font-size:12px;color:var(--ink-soft)">{{ p.employee_id }}</div>
            </td>
            <td style="font-size:12px">
              <div>{{ p.tg_name }}</div>
              <div v-if="p.tg_id" style="color:var(--ink-soft)">{{ p.tg_id }}</div>
            </td>
            <td style="font-size:12px">
              {{ p.department_name || '—' }}
              <span v-if="p.group_name" style="color:var(--ink-soft)">· {{ p.group_name }}</span>
            </td>
            <td style="font-size:12px;font-family:monospace;color:var(--ink-soft)">{{ p.invite_code }}</td>
            <td>
              <div class="action-btns">
                <button class="btn btn-xs btn-primary" @click="openApprove(p)">通过</button>
                <button class="btn btn-xs btn-danger-xs" @click="rejectPending(p)">拒绝</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </el-dialog>

    <!-- ── Approve dialog (with permission picker) ──────────────── -->
    <el-dialog v-model="showApprove" :title="'✅ 审核通过 — ' + (approveUser?.nickname || '')" width="650px" top="5vh">
      <div class="perm-hint">
        通过后，该用户可立即登录，并拥有下列勾选的权限。<strong>默认全部勾选</strong>，不想给的取消勾选即可。
      </div>
      <div class="perm-tree">
        <div v-for="group in grantablePermTree" :key="group.code" class="perm-group">
          <div class="perm-group-head">
            <el-checkbox
              :model-value="approvePerms.length > 0 && (group.children || []).every(c => approvePerms.includes(c.code))"
              :indeterminate="(() => {
                const n = (group.children || []).filter(c => approvePerms.includes(c.code)).length
                return n > 0 && n < (group.children || []).length
              })()"
              @change="toggleApproveGroup(group, $event)">
              <span class="perm-group-label">{{ group.label }}</span>
            </el-checkbox>
          </div>
          <div class="perm-children">
            <el-checkbox
              v-for="child in (group.children || []).filter(c => !c.admin_only)"
              :key="child.code"
              :model-value="approvePerms.includes(child.code)"
              @change="toggleApprovePerm(child.code, $event)"
              class="perm-item">
              {{ child.label }}
            </el-checkbox>
          </div>
        </div>
      </div>
      <template #footer>
        <el-button @click="showApprove = false">取消</el-button>
        <el-button @click="approvePerms = allPermCodes()">全选</el-button>
        <el-button @click="approvePerms = []">清空</el-button>
        <el-button type="primary" @click="confirmApprove" :loading="approving">通过审核</el-button>
      </template>
    </el-dialog>

    <div class="panel">
      <table class="table">
        <thead><tr><th>姓名</th><th>邮箱</th><th>角色</th><th>部门 / 组</th><th>Telegram ID</th><th>最后登录</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="u in users" :key="u.id">
            <td style="font-weight:700">{{ u.name }}</td>
            <td style="color:var(--ink-mid)">{{ u.email }}</td>
            <td><span class="role-badge" :class="u.role==='admin'?'role-admin':'role-member'">{{ u.role }}</span></td>
            <td style="font-size:12px">
              <template v-if="u.department_name">
                <span class="dept-inline">{{ u.department_name }}</span>
                <span v-if="u.group_name" class="group-inline">· {{ u.group_name }}</span>
              </template>
              <span v-else style="color:var(--ink-hint)">—</span>
            </td>
            <td style="font-size:12px;color:var(--ink-soft)">{{ u.telegram_id || '—' }}</td>
            <td style="font-size:12px;color:var(--ink-soft)">{{ u.last_login || '—' }}</td>
            <td>
              <div class="action-btns">
                <button class="btn btn-xs" @click="openEdit(u)">编辑</button>
                <button class="btn btn-xs btn-perm" @click="openPerms(u)" v-if="u.role!=='admin'">权限</button>
                <button class="btn btn-xs btn-status" @click="openStatus(u)">配置状态</button>
                <button class="btn btn-xs btn-danger-xs" @click="deleteUser(u)">删除</button>
              </div>
            </td>
          </tr>
          <tr v-if="!users.length"><td colspan="7" class="empty-cell">暂无用户</td></tr>
        </tbody>
      </table>
    </div>

    <el-dialog v-model="showDialog" :title="editId ? '✏️ 编辑用户' : '👤 添加用户'" width="480px" @close="clearFieldErrors">
      <div class="form-group">
        <label>姓名</label>
        <input v-model="form.name" @keyup.enter="saveUser" :class="{'has-err': fieldErrs.name}" />
        <div v-if="fieldErrs.name" class="field-err">{{ fieldErrs.name }}</div>
      </div>
      <div class="form-group">
        <label>邮箱</label>
        <input v-model="form.email" type="email" @keyup.enter="saveUser" :class="{'has-err': fieldErrs.email}" />
        <div v-if="fieldErrs.email" class="field-err">{{ fieldErrs.email }}</div>
      </div>
      <div v-if="!editId" class="form-group">
        <label>密码 <span style="font-weight:400;color:var(--ink-hint);font-size:12px">（至少 12 位，含大小写字母和数字）</span></label>
        <input v-model="form.password" type="password" @keyup.enter="saveUser" :class="{'has-err': fieldErrs.password}" />
        <div v-if="fieldErrs.password" class="field-err">{{ fieldErrs.password }}</div>
      </div>
      <div class="form-group">
        <label>角色</label>
        <el-select v-model="form.role" style="width:100%" @change="onRoleChange">
          <el-option label="管理员" value="admin" />
          <el-option label="成员" value="member" />
        </el-select>
      </div>
      <div class="form-row">
        <div class="form-group" style="flex:1">
          <label>部门</label>
          <el-select v-model="form.department_id" clearable placeholder="未指定" style="width:100%" @change="onDeptChange">
            <el-option v-for="d in departments" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
        </div>
        <div class="form-group" style="flex:1">
          <label>组</label>
          <el-select v-model="form.group_id" clearable placeholder="未指定" style="width:100%" :disabled="!form.department_id">
            <el-option v-for="g in (groupsByDept[form.department_id] || [])" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
        </div>
      </div>
      <div class="form-group"><label>Telegram ID</label><input v-model="form.telegram_id" /></div>

      <!-- Initial permissions (新建时可选; 管理员不需要，自带全权限) -->
      <div v-if="!editId && form.role !== 'admin'" class="form-group">
        <label>
          权限
          <span style="font-weight:400;color:var(--ink-hint);font-size:12px;margin-left:8px">
            可选；不勾选则保存后在列表里点「权限」再设置
          </span>
        </label>
        <div class="inline-perm-box">
          <div class="inline-perm-head">
            <span class="inline-perm-count">
              已选 {{ initialPerms.length }} 项
            </span>
            <span class="inline-perm-links">
              <a @click.prevent="initialPerms = allPermCodes()">全选</a>
              <a @click.prevent="initialPerms = []">清空</a>
            </span>
          </div>
          <div class="inline-perm-groups">
            <!-- Only iterate grantable (non-admin-only) groups. Admin-only
                 categories (users / org / globaldata + reports.evaluate
                 sub) are reserved for the admin role and never appear as
                 checkable options for members. -->
            <div v-for="g in grantablePermTree" :key="g.code" class="inline-perm-group">
              <div class="ipg-head">
                <el-checkbox
                  :model-value="isInlineGroupChecked(g)"
                  :indeterminate="isInlineGroupIndeterminate(g)"
                  @change="toggleInlineGroup(g, $event)">
                  <span class="ipg-label">{{ g.label }}</span>
                </el-checkbox>
              </div>
              <div class="ipg-children">
                <el-checkbox
                  v-for="child in (g.children || []).filter(c => !c.admin_only)"
                  :key="child.code"
                  :model-value="initialPerms.includes(child.code)"
                  @change="toggleInlinePerm(child.code, $event)"
                  class="ipg-item">
                  {{ child.label }}
                </el-checkbox>
              </div>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <el-button @click="showDialog=false">取消</el-button>
        <el-button type="primary" @click="saveUser">保存</el-button>
      </template>
    </el-dialog>

    <!-- Department dialog -->
    <el-dialog v-model="showDeptDialog" :title="deptEditId ? '编辑部门' : '新增部门'" width="380px">
      <div class="form-group">
        <label>部门名称</label>
        <input v-model="deptForm.name" @keyup.enter="saveDept" placeholder="例如：技术部" />
      </div>
      <template #footer>
        <el-button @click="showDeptDialog=false">取消</el-button>
        <el-button type="primary" @click="saveDept">保存</el-button>
      </template>
    </el-dialog>

    <!-- Group dialog -->
    <el-dialog v-model="showGroupDialog" :title="groupEditId ? '编辑组' : '新增组'" width="420px">
      <div class="form-group">
        <label>所属部门</label>
        <el-select v-model="groupForm.department_id" placeholder="选择部门" style="width:100%">
          <el-option v-for="d in departments" :key="d.id" :label="d.name" :value="d.id" />
        </el-select>
      </div>
      <div class="form-group">
        <label>组名称</label>
        <input v-model="groupForm.name" @keyup.enter="saveGroup" placeholder="例如：前端组" />
      </div>
      <template #footer>
        <el-button @click="showGroupDialog=false">取消</el-button>
        <el-button type="primary" @click="saveGroup">保存</el-button>
      </template>
    </el-dialog>

    <!-- Permissions Dialog -->
    <el-dialog v-model="showPerms" :title="'🔐 权限设置 — ' + (permUser?.name||'')" width="650px" top="5vh">
      <div class="perm-hint">勾选该用户可以访问的功能。管理员拥有所有权限，无需单独设置。</div>
      <div class="perm-tree">
        <!-- Skip admin-only branches here too — they can't be granted to
             members no matter what you tick. Backend silently filters
             them on save, but hiding them removes the confusion. -->
        <div v-for="group in grantablePermTree" :key="group.code" class="perm-group">
          <div class="perm-group-head">
            <el-checkbox
              :model-value="isGroupChecked(group)"
              :indeterminate="isGroupIndeterminate(group)"
              @change="toggleGroup(group, $event)">
              <span class="perm-group-label">{{ group.label }}</span>
            </el-checkbox>
          </div>
          <div class="perm-children">
            <el-checkbox
              v-for="child in (group.children || []).filter(c => !c.admin_only)"
              :key="child.code"
              :model-value="checkedPerms.includes(child.code)"
              @change="togglePerm(child.code, $event)"
              class="perm-item">
              {{ child.label }}
            </el-checkbox>
          </div>
        </div>
      </div>
      <template #footer>
        <el-button @click="showPerms=false">取消</el-button>
        <el-button type="warning" @click="checkAll">全选</el-button>
        <el-button @click="checkedPerms=[]">清空</el-button>
        <el-button type="primary" @click="savePerms" :loading="savingPerms">保存权限</el-button>
      </template>
    </el-dialog>

    <!-- Platform status (admin monitoring). Read-only summary of which
         platforms the target user has configured. Secrets never leave
         the server — we only show booleans + token expiry. -->
    <el-dialog v-model="showStatus" :title="'📡 配置状态 — ' + (statusUser?.name || '')" width="540px">
      <div v-if="!statusData" class="empty-cell" style="padding:30px">加载中...</div>
      <div v-else class="status-list">
        <div class="status-row">
          <span class="status-name">🤖 AI 接口</span>
          <span :class="['status-tag', statusData.ai?.bound ? 'ok' : 'off']">
            {{ statusData.ai?.bound ? `已配置 (${statusData.ai.slots} 组)` : '未配置' }}
          </span>
        </div>
        <div class="status-row" v-for="svc in ['gsc', 'ga', 'yandex']" :key="svc">
          <span class="status-name">
            {{ svc === 'gsc' ? '🟦 Google Search Console' : svc === 'ga' ? '🟩 Google Analytics' : '🟥 Yandex' }}
          </span>
          <span :class="['status-tag', statusData[svc]?.bound ? 'ok' : (statusData[svc]?.has_client_id ? 'partial' : 'off')]">
            <template v-if="statusData[svc]?.bound">
              已授权{{ statusData[svc].expires_at ? ' · 至 ' + String(statusData[svc].expires_at).slice(0, 16) : '' }}
            </template>
            <template v-else-if="statusData[svc]?.has_client_id">仅填写应用凭据</template>
            <template v-else>未配置</template>
          </span>
        </div>
        <div class="status-row">
          <span class="status-name">🟦 Bing Webmaster</span>
          <span :class="['status-tag', statusData.bing?.bound ? 'ok' : 'off']">
            {{ statusData.bing?.bound ? '已配置' : '未配置' }}
          </span>
        </div>
        <div class="status-row">
          <span class="status-name">📱 Telegram</span>
          <span :class="['status-tag', statusData.telegram?.bound ? 'ok' : 'off']">
            {{ statusData.telegram?.bound ? '已配置' : '未配置' }}
          </span>
        </div>
        <div class="status-row">
          <span class="status-name">🎨 FRW 图/视频</span>
          <span :class="['status-tag', statusData.frw?.bound ? 'ok' : 'off']">
            {{ statusData.frw?.bound ? '已配置' : '未配置' }}
          </span>
        </div>
        <div class="status-row">
          <span class="status-name">🌐 管理的站点数</span>
          <span class="status-tag muted">{{ statusData.sites || 0 }} 个</span>
        </div>
      </div>
      <template #footer>
        <el-button @click="showStatus = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const users = ref([])
const departments = ref([])
const groups = ref([])
// Guard flag so a rapid sequence of mutations (e.g. clicking Save twice) can't
// fire overlapping `loadAll()` calls that finish out of order and leave the
// UI showing stale data.
const loading = ref(false)
const showDialog = ref(false)
const editId = ref(null)
const form = ref({ name:'', email:'', password:'', role:'member', telegram_id:'', department_id:null, group_id:null })
// Field-level validation errors, keyed by json field name (email/password/name/...).
// Populated from the backend's `fields` key when a POST returns 400. We show each
// message right under its input instead of stacking global toasts.
const fieldErrs = ref({})
function clearFieldErrors() { fieldErrs.value = {} }

// Inline permission picker state (only used when creating a NEW member).
// Reuses the same permTree fetched for the stand-alone permission dialog
// further down — we just need to make sure it's loaded by the time the
// user opens the create dialog.
const initialPerms = ref([])
function isInlineGroupChecked(g) {
  return g.children.length > 0 && g.children.every(c => initialPerms.value.includes(c.code))
}
function isInlineGroupIndeterminate(g) {
  const n = g.children.filter(c => initialPerms.value.includes(c.code)).length
  return n > 0 && n < g.children.length
}
function toggleInlineGroup(g, val) {
  const codes = g.children.map(c => c.code)
  if (val) {
    initialPerms.value = [...new Set([...initialPerms.value, ...codes])]
  } else {
    initialPerms.value = initialPerms.value.filter(c => !codes.includes(c))
  }
}
function toggleInlinePerm(code, val) {
  if (val) initialPerms.value = [...initialPerms.value, code]
  else initialPerms.value = initialPerms.value.filter(c => c !== code)
}
// grantablePermTree hides every node marked admin_only (both branches and
// leaves) — these are reserved for the admin role and aren't even an
// option in the member permission picker. Matches cleanPermissionList's
// server-side filter so the UI can't promise something the backend drops.
const grantablePermTree = computed(() =>
  (permTree.value || [])
    .filter(g => !g.admin_only)
    .map(g => ({ ...g, children: (g.children || []).filter(c => !c.admin_only) }))
)

// All grantable leaf codes — used by the create dialog to default-check
// every non-admin-only permission, and by 全选 to re-select everything.
function allPermCodes() {
  const out = []
  for (const g of grantablePermTree.value) {
    for (const c of g.children) out.push(c.code)
  }
  return out
}

// Group-by-department map so the form's cascading dropdown and the
// dept panel below can both render groups under the right parent.
const groupsByDept = computed(() => {
  const m = {}
  for (const g of groups.value) {
    if (!m[g.department_id]) m[g.department_id] = []
    m[g.department_id].push(g)
  }
  return m
})

async function loadAll() {
  if (loading.value) return
  loading.value = true
  try {
    const [uRes, dRes, gRes, pRes] = await Promise.all([
      api.listUsers(),
      api.listDepartments(),
      api.listGroups(),
      api.listPendingUsers().catch(() => ({ data: [] })),
    ])
    users.value = uRes.data
    departments.value = dRes.data || []
    groups.value = gRes.data || []
    pendingUsers.value = pRes.data || []
  } catch (e) {
    console.error('加载失败', e)
    ElMessage.error('加载数据失败')
  } finally {
    loading.value = false
  }
}

// ── Pending registration approval ─────────────────────────────────
const pendingUsers = ref([])
const showPending = ref(false)
const showApprove = ref(false)
const approveUser = ref(null)
const approvePerms = ref([])
const approving = ref(false)

async function openApprove(p) {
  approveUser.value = p
  // Default-grant: same product rule as new-user-create — all
  // grantable perms ticked, admin unchecks what they don't want.
  if (!permTree.value.length) {
    try { permTree.value = (await api.getPermissionTree()).data || [] } catch {}
  }
  approvePerms.value = allPermCodes()
  showApprove.value = true
}

function toggleApproveGroup(g, val) {
  const codes = (g.children || []).map(c => c.code)
  if (val) {
    approvePerms.value = [...new Set([...approvePerms.value, ...codes])]
  } else {
    approvePerms.value = approvePerms.value.filter(c => !codes.includes(c))
  }
}
function toggleApprovePerm(code, val) {
  if (val) approvePerms.value = [...approvePerms.value, code]
  else approvePerms.value = approvePerms.value.filter(c => c !== code)
}

async function confirmApprove() {
  if (!approveUser.value) return
  approving.value = true
  try {
    await api.approvePendingUser(approveUser.value.id, { permissions: approvePerms.value })
    ElMessage.success('已通过审核，用户可以登录了')
    showApprove.value = false
    showPending.value = false
    await loadAll()
  } catch (e) {
    if (!e?.response) console.error('审核失败', e)
  } finally {
    approving.value = false
  }
}

async function rejectPending(p) {
  try {
    await ElMessageBox.confirm(
      `确认拒绝 ${p.nickname}（${p.employee_id}）的注册申请？该记录会被删除，邀请码不会恢复。`,
      '拒绝注册', { type: 'warning' }
    )
  } catch { return }
  try {
    await api.rejectPendingUser(p.id)
    ElMessage.success('已拒绝')
    await loadAll()
  } catch (e) {
    if (!e?.response) console.error('拒绝失败', e)
  }
}

async function openAdd() {
  editId.value = null
  form.value = { name:'', email:'', password:'', role:'member', telegram_id:'', department_id:null, group_id:null }
  clearFieldErrors()
  // Product rule: creating a new member defaults to "can do everything
  // except admin-only stuff". The admin then unchecks what they want to
  // restrict. Load the tree synchronously so the checkboxes are already
  // populated by the time the dialog opens — otherwise they'd render
  // empty for a split second and miss the default-check.
  if (!permTree.value.length) {
    try {
      const res = await api.getPermissionTree()
      permTree.value = res.data || []
    } catch (e) { console.error('加载权限树失败', e) }
  }
  initialPerms.value = allPermCodes()
  showDialog.value = true
}
function openEdit(u) {
  editId.value = u.id
  form.value = {
    name: u.name,
    email: u.email,
    role: u.role,
    telegram_id: u.telegram_id || '',
    department_id: u.department_id || null,
    group_id: u.group_id || null,
  }
  clearFieldErrors()
  showDialog.value = true
}

// Reset group when department changes so the cascade doesn't allow
// an invalid (dept, group) pair through the backend validator.
function onDeptChange() {
  form.value.group_id = null
}
// Admins don't have checked permissions (they have all implicitly).
// Clear the inline picker when user flips to admin so we don't silently
// keep stale selections behind a v-if.
function onRoleChange(v) {
  if (v === 'admin') initialPerms.value = []
}

async function saveUser() {
  clearFieldErrors()
  // Send explicit 0 for unassigned so the backend sees "user wants to
  // clear it" rather than "not sent" (pointer-based).
  const payload = {
    ...form.value,
    department_id: form.value.department_id || 0,
    group_id: form.value.group_id || 0,
  }
  // Initial permissions are only meaningful on CREATE and only for
  // non-admin users. Admins get every perm implicitly; skipping the
  // field tells the backend "don't try to apply any".
  if (!editId.value && form.value.role !== 'admin') {
    payload.permissions = [...initialPerms.value]
  }
  // _silent tells the axios interceptor to skip the global toast so we
  // can render field-level errors inline. On unexpected errors (no
  // fields map from the server) we fall back to a single toast.
  const opts = { _silent: true }
  try {
    if (editId.value) await api.updateUser(editId.value, payload, opts)
    else await api.createUser(payload, opts)
    ElMessage.success('保存成功'); showDialog.value = false; loadAll()
  } catch (e) {
    const body = e?.response?.data || {}
    if (body.fields && typeof body.fields === 'object') {
      fieldErrs.value = body.fields
      // If the backend only emitted "_" (non-validation bind error),
      // fall back to a toast so the user sees something.
      if (body.fields._ && Object.keys(body.fields).length === 1) {
        ElMessage.error(body.fields._)
      }
    } else {
      ElMessage.error(body.error || '保存失败')
    }
  }
}

async function deleteUser(u) {
  try {
    await ElMessageBox.confirm(`确认删除用户 ${u.name}?`, '警告', { type: 'warning' })
    await api.deleteUser(u.id); ElMessage.success('删除成功'); loadAll()
  } catch (e) { if (e !== 'cancel') console.error('删除用户失败', e) }
}

// ── Departments ──
const showDeptDialog = ref(false)
const deptEditId = ref(null)
const deptForm = ref({ name: '' })

function openAddDept() {
  deptEditId.value = null
  deptForm.value = { name: '' }
  showDeptDialog.value = true
}
function openEditDept(d) {
  deptEditId.value = d.id
  deptForm.value = { name: d.name }
  showDeptDialog.value = true
}
async function saveDept() {
  if (!deptForm.value.name.trim()) {
    ElMessage.warning('请填写部门名称')
    return
  }
  try {
    if (deptEditId.value) {
      await api.updateDepartment(deptEditId.value, deptForm.value)
    } else {
      await api.createDepartment(deptForm.value)
    }
    ElMessage.success('保存成功')
    showDeptDialog.value = false
    loadAll()
  } catch { /* interceptor */ }
}
async function deleteDept(d) {
  try {
    await ElMessageBox.confirm(
      `确认删除部门「${d.name}」？其下 ${d.group_count} 个组将一并删除，${d.user_count} 个成员会被解除绑定（不会被删除）。`,
      '警告',
      { type: 'warning', confirmButtonText: '删除' },
    )
  } catch { return }
  try {
    await api.deleteDepartment(d.id)
    ElMessage.success('已删除')
    loadAll()
  } catch { /* interceptor */ }
}

// ── Groups ──
const showGroupDialog = ref(false)
const groupEditId = ref(null)
const groupForm = ref({ department_id: null, name: '' })

function openAddGroup() {
  groupEditId.value = null
  groupForm.value = {
    department_id: departments.value[0]?.id || null,
    name: '',
  }
  showGroupDialog.value = true
}
function openEditGroup(g) {
  groupEditId.value = g.id
  groupForm.value = { department_id: g.department_id, name: g.name }
  showGroupDialog.value = true
}
async function saveGroup() {
  if (!groupForm.value.department_id) {
    ElMessage.warning('请选择部门')
    return
  }
  if (!groupForm.value.name.trim()) {
    ElMessage.warning('请填写组名称')
    return
  }
  try {
    if (groupEditId.value) {
      await api.updateGroup(groupEditId.value, groupForm.value)
    } else {
      await api.createGroup(groupForm.value)
    }
    ElMessage.success('保存成功')
    showGroupDialog.value = false
    loadAll()
  } catch { /* interceptor */ }
}
async function deleteGroup(g) {
  try {
    await ElMessageBox.confirm(
      `确认删除组「${g.name}」？其下 ${g.user_count} 个成员会被解除绑定（不会被删除，仍保留在部门里）。`,
      '警告',
      { type: 'warning', confirmButtonText: '删除' },
    )
  } catch { return }
  try {
    await api.deleteGroup(g.id)
    ElMessage.success('已删除')
    loadAll()
  } catch { /* interceptor */ }
}

// ── Platform status (admin monitoring) ──
const showStatus = ref(false)
const statusUser = ref(null)
const statusData = ref(null)

async function openStatus(u) {
  statusUser.value = u
  statusData.value = null
  showStatus.value = true
  try {
    const res = await api.getUserPlatformStatus(u.id)
    statusData.value = res.data
  } catch (e) {
    ElMessage.error('加载失败')
  }
}

// ── Permissions ──
const showPerms = ref(false)
const permUser = ref(null)
const permTree = ref([])
const checkedPerms = ref([])
const savingPerms = ref(false)

async function openPerms(u) {
  permUser.value = u
  checkedPerms.value = []
  try {
    const [treeRes, userRes] = await Promise.all([
      api.getPermissionTree(),
      api.getUserPermissions(u.id),
    ])
    permTree.value = treeRes.data
    checkedPerms.value = userRes.data || []
  } catch (e) { console.error('加载权限失败', e) }
  showPerms.value = true
}

function isGroupChecked(group) {
  return group.children.every(c => checkedPerms.value.includes(c.code))
}
function isGroupIndeterminate(group) {
  const checked = group.children.filter(c => checkedPerms.value.includes(c.code)).length
  return checked > 0 && checked < group.children.length
}
function toggleGroup(group, val) {
  const codes = group.children.map(c => c.code)
  if (val) {
    const set = new Set([...checkedPerms.value, ...codes])
    checkedPerms.value = [...set]
  } else {
    checkedPerms.value = checkedPerms.value.filter(c => !codes.includes(c))
  }
}
function togglePerm(code, val) {
  if (val) { checkedPerms.value = [...checkedPerms.value, code] }
  else { checkedPerms.value = checkedPerms.value.filter(c => c !== code) }
}
function checkAll() {
  // "Select all" = every grantable code. Admin-only leaves are skipped —
  // a member can't actually hold them, so ticking every box would lie
  // to the admin about what will be saved.
  checkedPerms.value = allPermCodes()
}
async function savePerms() {
  savingPerms.value = true
  try {
    await api.saveUserPermissions(permUser.value.id, { permissions: checkedPerms.value })
    ElMessage.success('权限已保存')
    showPerms.value = false
  } catch { ElMessage.error('保存失败') }
  finally { savingPerms.value = false }
}

onMounted(loadAll)
</script>

<style scoped>
.page-top { display:flex; justify-content:space-between; align-items:center; margin-bottom:18px; }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); transform:translateY(-1px); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep),var(--sun)); color:#fff; }
.btn-pending { background:linear-gradient(135deg,#10B981,#059669); color:#fff; border-color:transparent; font-weight:700; padding:8px 16px; box-shadow:0 4px 12px rgba(16,185,129,.25); }
.btn-pending:hover { filter:brightness(1.08); color:#fff; }
.btn-xs { padding:4px 12px; font-size:12px; border-radius:6px; }
.btn-xs:hover { background:var(--sun-glow); border-color:var(--sun); color:var(--sun-deep); }
.btn-danger-xs { color:var(--err); }
.btn-danger-xs:hover { background:var(--err-bg); border-color:var(--err-border); color:var(--err); }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }

.table { width:100%; border-collapse:collapse; }
.table th { padding:10px 14px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.08em; border-bottom:2px solid var(--border); background:var(--surface-3); }
.table td { padding:10px 14px; border-bottom:1px solid var(--border-light); font-size:13px; }
.table tr:hover td { background:var(--sun-light); }

.role-badge { font-size:11px; font-weight:800; padding:2px 10px; border-radius:20px; text-transform:uppercase; }
.role-admin { background:linear-gradient(135deg,#FFF0DE,#FFE4C4); color:var(--sun-deep); border:1px solid var(--peach); }
.role-member { background:var(--surface-3); color:var(--ink-soft); border:1px solid var(--border); }

.action-btns { display:flex; gap:6px; }
.empty-cell { text-align:center; color:var(--ink-hint); padding:30px !important; }

.form-group { margin-bottom:16px; }
.form-group label { display:block; margin-bottom:7px; font-weight:700; color:var(--ink-mid); font-size:13px; }
.form-group input, .form-group select {
  width:100%; padding:10px 13px; border:1.5px solid var(--border); border-radius:9px;
  font-size:14px; font-family:inherit; background:var(--surface); color:var(--ink);
  transition:border-color .18s, box-shadow .18s; outline:none;
}
.form-group input:focus, .form-group select:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.14); }
.form-group input.has-err { border-color:#DC2626; background:#FEF2F2; }
.form-group input.has-err:focus { box-shadow:0 0 0 3px rgba(220,38,38,.14); }
.field-err { margin-top:6px; font-size:12px; color:#DC2626; font-weight:600; }
.btn-perm { background:#EEF2FF; border-color:#C7D2FE; color:#4338CA; }
.btn-perm:hover { background:#E0E7FF; border-color:#A5B4FC; }
.btn-status { background:#F0F9FF; border-color:#BAE6FD; color:#0369A1; }
.btn-status:hover { background:#E0F2FE; border-color:#7DD3FC; }

.status-list { display:flex; flex-direction:column; gap:8px; padding:6px 0; }
.status-row { display:flex; justify-content:space-between; align-items:center; padding:10px 14px; background:var(--surface-3); border-radius:8px; border:1px solid var(--border-light); }
.status-name { font-size:13px; font-weight:600; color:var(--ink); }
.status-tag { font-size:12px; font-weight:700; padding:3px 12px; border-radius:14px; }
.status-tag.ok { background:var(--ok-bg); color:var(--ok); border:1px solid var(--ok-border); }
.status-tag.partial { background:var(--warn-bg); color:var(--warn); border:1px solid var(--warn-border); }
.status-tag.off { background:var(--surface); color:var(--ink-hint); border:1px solid var(--border); }
.status-tag.muted { background:var(--surface); color:var(--ink-mid); border:1px solid var(--border); font-weight:600; }
.perm-hint { font-size:13px; color:var(--ink-hint); margin-bottom:16px; padding:8px 12px; background:#FFFBEB; border:1px solid #FDE68A; border-radius:8px; }
.perm-tree { max-height:55vh; overflow-y:auto; }
.perm-group { margin-bottom:14px; border:1px solid var(--border-light); border-radius:10px; overflow:hidden; }
.perm-group-head { padding:10px 14px; background:var(--surface-3, #f3f4f6); border-bottom:1px solid var(--border-light); }
.perm-group-label { font-weight:700; font-size:14px; }
.perm-children { padding:10px 14px 10px 32px; display:grid; grid-template-columns:1fr 1fr; gap:6px 20px; }
.perm-item { font-size:13px; }

/* ── Inline permission picker (embedded in 添加用户 dialog) ── */
/* Tighter than .perm-tree/.perm-group because it sits inside a 480px
   dialog next to other form fields. Max-height caps growth so the
   dialog doesn't balloon when there are many perms. */
.inline-perm-box {
  border:1.5px solid var(--border-light);
  border-radius:9px;
  background:var(--surface-2, #fafafa);
}
.inline-perm-head {
  display:flex; justify-content:space-between; align-items:center;
  padding:8px 12px;
  border-bottom:1px solid var(--border-light);
  font-size:12px;
  background:var(--surface-3, #f3f4f6);
}
.inline-perm-count { color:var(--ink-mid); font-weight:700; }
.inline-perm-links { display:flex; gap:12px; }
.inline-perm-links a {
  color:var(--sun-deep); font-weight:700; cursor:pointer;
  text-decoration:none; user-select:none;
}
.inline-perm-links a:hover { text-decoration:underline; }

.inline-perm-groups { padding:10px; max-height:280px; overflow-y:auto; }
.inline-perm-group { margin-bottom:10px; }
.inline-perm-group:last-child { margin-bottom:0; }
.ipg-head { padding:4px 6px; background:var(--surface-3, #f3f4f6); border-radius:6px; }
.ipg-label { font-weight:700; font-size:13px; }
.ipg-children {
  padding:6px 6px 4px 24px;
  display:grid; grid-template-columns:1fr 1fr; gap:2px 12px;
}
.ipg-item { font-size:12.5px; }

/* ── Department + group panel (sits above the users table) ── */
.dept-panel { margin-bottom:18px; padding:16px 22px; }
.panel-head { display:flex; align-items:center; justify-content:space-between; margin-bottom:12px; }
.panel-title { font-size:14px; font-weight:800; color:var(--ink); }
.panel-actions { display:flex; gap:8px; }
.btn-sm { padding:6px 14px; font-size:12.5px; border-radius:7px; }

.dept-list { display:grid; grid-template-columns:repeat(auto-fill,minmax(320px,1fr)); gap:12px; }
.dept-card { border:1.5px solid var(--border-light); border-radius:var(--r-sm); padding:12px 14px; background:var(--surface-2); transition:border-color .15s, box-shadow .15s; }
.dept-card:hover { border-color:var(--peach); box-shadow:var(--s1); }
.dept-card-head { display:flex; align-items:center; justify-content:space-between; margin-bottom:10px; gap:10px; }
.dept-name { display:flex; align-items:center; gap:8px; font-size:14px; font-weight:800; color:var(--ink); flex:1; min-width:0; }
.dept-icon { font-size:16px; flex-shrink:0; }
.dept-count { font-size:11px; font-weight:600; color:var(--ink-hint); padding:2px 9px; background:var(--surface); border:1px solid var(--border-light); border-radius:10px; white-space:nowrap; }
.dept-actions { display:flex; gap:4px; flex-shrink:0; }

.group-chip-row { display:flex; flex-wrap:wrap; gap:6px; }
.group-chip { display:inline-flex; align-items:center; gap:5px; padding:4px 4px 4px 10px; background:var(--surface); border:1px solid var(--border-light); border-radius:14px; font-size:12px; transition:border-color .15s; }
.group-chip:hover { border-color:var(--peach); }
.group-chip .group-name { font-weight:700; color:var(--ink); }
.group-chip .group-count { font-size:10.5px; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; }
.chip-btn { background:none; border:none; color:var(--ink-hint); cursor:pointer; padding:2px 5px; border-radius:50%; font-size:10.5px; width:20px; height:20px; display:inline-flex; align-items:center; justify-content:center; }
.chip-btn:hover { background:var(--sun-light); color:var(--sun-deep); }
.chip-danger:hover { background:var(--err-bg); color:var(--err); }
.empty-groups { font-size:12px; color:var(--ink-hint); padding:4px 0; font-style:italic; }

.dept-inline { font-weight:700; color:var(--ink); }
.group-inline { margin-left:4px; color:var(--ink-soft); }

/* Two-column form row used for dept + group side-by-side in the user dialog */
.form-row { display:flex; gap:12px; }
.form-row .form-group { flex:1; }
</style>
