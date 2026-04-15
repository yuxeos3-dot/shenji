<template>
  <div>
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
      <h2 style="margin:0">站点管理</h2>
      <el-button type="primary" @click="showDialog=true">添加站点</el-button>
    </div>

    <el-table :data="sites" stripe v-loading="loading">
      <el-table-column prop="name" label="站点名称" />
      <el-table-column prop="domain" label="域名" />
      <el-table-column prop="status" label="状态">
        <template #default="{row}"><el-tag :type="row.status==='active'?'success':'info'">{{ row.status }}</el-tag></template>
      </el-table-column>
      <el-table-column prop="last_audit_status" label="最近审计">
        <template #default="{row}">
          <el-tag v-if="row.last_audit_status" :type="row.last_audit_status==='done'?'success':row.last_audit_status==='running'?'warning':'danger'" size="small">{{ row.last_audit_status }}</el-tag>
          <span v-else style="color:#909399">-</span>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" width="180" />
      <el-table-column label="操作" width="300">
        <template #default="{row}">
          <el-button type="primary" size="small" @click="triggerAudit(row)">审计</el-button>
          <el-button size="small" @click="$router.push(`/sites/${row.id}`)">详情</el-button>
          <el-button size="small" @click="$router.push(`/sites/${row.id}/data`)">数据</el-button>
          <el-button type="danger" size="small" @click="deleteSite(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="showDialog" :title="editId?'编辑站点':'添加站点'" width="600px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="域名"><el-input v-model="form.domain" placeholder="example.com" /></el-form-item>
        <el-form-item label="站点名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="GSC URL"><el-input v-model="form.gsc_site_url" placeholder="sc-domain:example.com" /></el-form-item>
        <el-form-item label="GA Property"><el-input v-model="form.ga_property_id" placeholder="123456789" /></el-form-item>
        <el-form-item label="Yandex Host"><el-input v-model="form.yandex_host_id" /></el-form-item>
        <el-divider>页面URL配置（可选）</el-divider>
        <el-form-item label="分类页"><el-input v-model="form.url_category" placeholder="/category/movies" /></el-form-item>
        <el-form-item label="详情页"><el-input v-model="form.url_detail" placeholder="/movie/123" /></el-form-item>
        <el-form-item label="资讯列表"><el-input v-model="form.url_news_list" placeholder="/news" /></el-form-item>
        <el-form-item label="资讯详情"><el-input v-model="form.url_news_detail" placeholder="/news/123" /></el-form-item>
        <el-form-item label="搜索页"><el-input v-model="form.url_search" placeholder="/search?q=test" /></el-form-item>
        <el-form-item label="辅助页"><el-input v-model="form.url_static" placeholder="/about" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showDialog=false">取消</el-button>
        <el-button type="primary" @click="saveSite">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const sites = ref([])
const loading = ref(false)
const showDialog = ref(false)
const editId = ref(null)
const form = ref({ domain:'', name:'', gsc_site_url:'', ga_property_id:'', yandex_host_id:'', url_category:'', url_detail:'', url_news_list:'', url_news_detail:'', url_search:'', url_static:'' })

async function loadSites() {
  loading.value = true
  try { const res = await api.listSites(); sites.value = res.data } catch {} finally { loading.value = false }
}

async function saveSite() {
  try {
    if (editId.value) { await api.updateSite(editId.value, form.value) }
    else { await api.createSite(form.value) }
    ElMessage.success('保存成功')
    showDialog.value = false
    loadSites()
  } catch {}
}

async function deleteSite(row) {
  try {
    await ElMessageBox.confirm(`确认删除站点 ${row.name}?`, '警告', { type: 'warning' })
    await api.deleteSite(row.id)
    ElMessage.success('删除成功')
    loadSites()
  } catch {}
}

async function triggerAudit(row) {
  try {
    const res = await api.triggerAudit(row.id)
    ElMessage.success(`审计任务已启动 (ID: ${res.data.task_id})`)
    loadSites()
  } catch {}
}

onMounted(loadSites)
</script>
