<template>
  <div>
    <div class="page-top">
      <div></div>
      <button class="btn btn-primary" @click="openAdd">👤 添加用户</button>
    </div>

    <div class="panel">
      <table class="table">
        <thead><tr><th>姓名</th><th>邮箱</th><th>角色</th><th>Telegram ID</th><th>最后登录</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="u in users" :key="u.id">
            <td style="font-weight:700">{{ u.name }}</td>
            <td style="color:var(--ink-mid)">{{ u.email }}</td>
            <td><span class="role-badge" :class="u.role==='admin'?'role-admin':'role-member'">{{ u.role }}</span></td>
            <td style="font-size:12px;color:var(--ink-soft)">{{ u.telegram_id || '—' }}</td>
            <td style="font-size:12px;color:var(--ink-soft)">{{ u.last_login || '—' }}</td>
            <td>
              <div class="action-btns">
                <button class="btn btn-xs" @click="openEdit(u)">编辑</button>
                <button class="btn btn-xs btn-perm" @click="openPerms(u)" v-if="u.role!=='admin'">权限</button>
                <button class="btn btn-xs btn-danger-xs" @click="deleteUser(u)">删除</button>
              </div>
            </td>
          </tr>
          <tr v-if="!users.length"><td colspan="6" class="empty-cell">暂无用户</td></tr>
        </tbody>
      </table>
    </div>

    <el-dialog v-model="showDialog" :title="editId ? '✏️ 编辑用户' : '👤 添加用户'" width="450px">
      <div class="form-group"><label>姓名</label><input v-model="form.name" @keyup.enter="saveUser" /></div>
      <div class="form-group"><label>邮箱</label><input v-model="form.email" type="email" @keyup.enter="saveUser" /></div>
      <div v-if="!editId" class="form-group"><label>密码 <span style="font-weight:400;color:var(--ink-hint);font-size:12px">（至少 12 位，含大小写字母和数字）</span></label><input v-model="form.password" type="password" @keyup.enter="saveUser" /></div>
      <div class="form-group">
        <label>角色</label>
        <el-select v-model="form.role" style="width:100%">
          <el-option label="管理员" value="admin" />
          <el-option label="成员" value="member" />
        </el-select>
      </div>
      <div class="form-group"><label>Telegram ID</label><input v-model="form.telegram_id" /></div>
      <template #footer>
        <el-button @click="showDialog=false">取消</el-button>
        <el-button type="primary" @click="saveUser">保存</el-button>
      </template>
    </el-dialog>

    <!-- Permissions Dialog -->
    <el-dialog v-model="showPerms" :title="'🔐 权限设置 — ' + (permUser?.name||'')" width="650px" top="5vh">
      <div class="perm-hint">勾选该用户可以访问的功能。管理员拥有所有权限，无需单独设置。</div>
      <div class="perm-tree">
        <div v-for="group in permTree" :key="group.code" class="perm-group">
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
              v-for="child in group.children"
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
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const users = ref([])
const showDialog = ref(false)
const editId = ref(null)
const form = ref({ name:'', email:'', password:'', role:'member', telegram_id:'' })

async function loadUsers() { try { const res = await api.listUsers(); users.value = res.data } catch (e) { console.error('加载用户失败', e); ElMessage.error('加载用户列表失败') } }
function openAdd() { editId.value = null; form.value = { name:'', email:'', password:'', role:'member', telegram_id:'' }; showDialog.value = true }
function openEdit(u) { editId.value = u.id; form.value = { name:u.name, email:u.email, role:u.role, telegram_id:u.telegram_id||'' }; showDialog.value = true }

async function saveUser() {
  try {
    if (editId.value) await api.updateUser(editId.value, form.value)
    else await api.createUser(form.value)
    ElMessage.success('保存成功'); showDialog.value = false; loadUsers()
  } catch (e) { console.error('保存用户失败', e) }
}

async function deleteUser(u) {
  try {
    await ElMessageBox.confirm(`确认删除用户 ${u.name}?`, '警告', { type: 'warning' })
    await api.deleteUser(u.id); ElMessage.success('删除成功'); loadUsers()
  } catch (e) { if (e !== 'cancel') console.error('删除用户失败', e) }
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
  const all = []
  permTree.value.forEach(g => g.children.forEach(c => all.push(c.code)))
  checkedPerms.value = all
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

onMounted(loadUsers)
</script>

<style scoped>
.page-top { display:flex; justify-content:space-between; align-items:center; margin-bottom:18px; }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); transform:translateY(-1px); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep),var(--sun)); color:#fff; }
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
.btn-perm { background:#EEF2FF; border-color:#C7D2FE; color:#4338CA; }
.btn-perm:hover { background:#E0E7FF; border-color:#A5B4FC; }
.perm-hint { font-size:13px; color:var(--ink-hint); margin-bottom:16px; padding:8px 12px; background:#FFFBEB; border:1px solid #FDE68A; border-radius:8px; }
.perm-tree { max-height:55vh; overflow-y:auto; }
.perm-group { margin-bottom:14px; border:1px solid var(--border-light); border-radius:10px; overflow:hidden; }
.perm-group-head { padding:10px 14px; background:var(--surface-3, #f3f4f6); border-bottom:1px solid var(--border-light); }
.perm-group-label { font-weight:700; font-size:14px; }
.perm-children { padding:10px 14px 10px 32px; display:grid; grid-template-columns:1fr 1fr; gap:6px 20px; }
.perm-item { font-size:13px; }
</style>
