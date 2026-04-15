<template>
  <div>
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
      <h2 style="margin:0">用户管理</h2>
      <el-button type="primary" @click="openAdd">添加用户</el-button>
    </div>

    <el-table :data="users" stripe v-loading="loading">
      <el-table-column prop="name" label="姓名" />
      <el-table-column prop="email" label="邮箱" />
      <el-table-column prop="role" label="角色" width="100">
        <template #default="{row}"><el-tag :type="row.role==='admin'?'danger':'info'">{{ row.role }}</el-tag></template>
      </el-table-column>
      <el-table-column prop="telegram_id" label="Telegram ID" />
      <el-table-column prop="last_login" label="最后登录" width="180" />
      <el-table-column label="操作" width="160">
        <template #default="{row}">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button type="danger" size="small" @click="deleteUser(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="showDialog" :title="editId?'编辑用户':'添加用户'" width="450px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="姓名"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="邮箱"><el-input v-model="form.email" /></el-form-item>
        <el-form-item v-if="!editId" label="密码"><el-input v-model="form.password" type="password" show-password /></el-form-item>
        <el-form-item label="角色">
          <el-select v-model="form.role"><el-option label="管理员" value="admin" /><el-option label="成员" value="member" /></el-select>
        </el-form-item>
        <el-form-item label="Telegram ID"><el-input v-model="form.telegram_id" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showDialog=false">取消</el-button>
        <el-button type="primary" @click="saveUser">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const users = ref([])
const loading = ref(false)
const showDialog = ref(false)
const editId = ref(null)
const form = ref({ name:'', email:'', password:'', role:'member', telegram_id:'' })

async function loadUsers() {
  loading.value = true
  try { const res = await api.listUsers(); users.value = res.data } catch {} finally { loading.value = false }
}

function openAdd() {
  editId.value = null
  form.value = { name:'', email:'', password:'', role:'member', telegram_id:'' }
  showDialog.value = true
}

function openEdit(row) {
  editId.value = row.id
  form.value = { name: row.name, email: row.email, role: row.role, telegram_id: row.telegram_id || '' }
  showDialog.value = true
}

async function saveUser() {
  try {
    if (editId.value) { await api.updateUser(editId.value, form.value) }
    else { await api.createUser(form.value) }
    ElMessage.success('保存成功')
    showDialog.value = false
    loadUsers()
  } catch {}
}

async function deleteUser(row) {
  try {
    await ElMessageBox.confirm(`确认删除用户 ${row.name}?`, '警告', { type: 'warning' })
    await api.deleteUser(row.id)
    ElMessage.success('删除成功')
    loadUsers()
  } catch {}
}

onMounted(loadUsers)
</script>
