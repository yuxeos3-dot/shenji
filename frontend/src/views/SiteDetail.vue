<template>
  <div>
    <el-page-header @back="$router.push('/sites')">
      <template #content><span>{{ site.name }} ({{ site.domain }})</span></template>
    </el-page-header>

    <el-card style="margin-top:16px">
      <template #header>
        <div style="display:flex;justify-content:space-between;align-items:center">
          <span>站点信息</span>
          <el-button type="primary" @click="triggerAudit" :loading="auditing">立即审计</el-button>
        </div>
      </template>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="域名">{{ site.domain }}</el-descriptions-item>
        <el-descriptions-item label="名称">{{ site.name }}</el-descriptions-item>
        <el-descriptions-item label="GSC URL">{{ site.gsc_site_url || '-' }}</el-descriptions-item>
        <el-descriptions-item label="GA Property">{{ site.ga_property_id || '-' }}</el-descriptions-item>
        <el-descriptions-item label="Yandex Host">{{ site.yandex_host_id || '-' }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ site.status }}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-card style="margin-top:16px">
      <template #header><span>审计历史</span></template>
      <el-table :data="audits" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="status" label="状态" width="120">
          <template #default="{row}">
            <el-tag :type="row.status==='done'?'success':row.status==='running'?'warning':'danger'">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="progress" label="进度" width="120">
          <template #default="{row}"><el-progress :percentage="row.progress" :status="row.status==='done'?'success':row.status==='error'?'exception':''" /></template>
        </el-table-column>
        <el-table-column prop="created_at" label="开始时间" />
        <el-table-column prop="finished_at" label="完成时间" />
        <el-table-column label="操作" width="120">
          <template #default="{row}">
            <el-button type="primary" size="small" @click="$router.push(`/sites/${$route.params.id}/audit/${row.id}`)">查看</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'

const route = useRoute()
const site = ref({})
const audits = ref([])
const auditing = ref(false)

async function loadData() {
  try {
    const [siteRes, auditsRes] = await Promise.all([
      api.getSite(route.params.id),
      api.listAudits(route.params.id),
    ])
    site.value = siteRes.data
    audits.value = auditsRes.data
  } catch {}
}

async function triggerAudit() {
  auditing.value = true
  try {
    await api.triggerAudit(route.params.id)
    ElMessage.success('审计任务已启动')
    loadData()
  } catch {} finally { auditing.value = false }
}

onMounted(loadData)
</script>
