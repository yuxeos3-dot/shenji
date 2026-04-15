<template>
  <div>
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
      <h2 style="margin:0">每日汇报</h2>
      <el-button type="primary" @click="$router.push('/reports/write')">写汇报</el-button>
    </div>

    <el-card style="margin-bottom:16px">
      <el-form :inline="true">
        <el-form-item label="日期">
          <el-date-picker v-model="filterDate" type="date" value-format="YYYY-MM-DD" placeholder="选择日期" @change="loadReports" clearable />
        </el-form-item>
      </el-form>
    </el-card>

    <el-table :data="reports" stripe v-loading="loading">
      <el-table-column prop="user_name" label="提交人" width="120" />
      <el-table-column prop="report_date" label="日期" width="120" />
      <el-table-column prop="content" label="内容摘要" show-overflow-tooltip />
      <el-table-column prop="created_at" label="提交时间" width="180" />
      <el-table-column label="操作" width="100">
        <template #default="{row}">
          <el-button type="primary" size="small" @click="viewReport(row)">查看</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="showDetail" title="汇报详情" width="600px">
      <div v-if="selectedReport">
        <h4>{{ selectedReport.user_name }} - {{ selectedReport.report_date }}</h4>
        <div style="white-space:pre-wrap;margin-bottom:16px">{{ selectedReport.content }}</div>
        <el-divider />
        <h5>今日完成</h5>
        <ul><li v-for="(t, i) in parseTasks(selectedReport.tasks_done)" :key="i">{{ t }}</li></ul>
        <h5>明日计划</h5>
        <ul><li v-for="(t, i) in parseTasks(selectedReport.tasks_todo)" :key="i">{{ t }}</li></ul>
        <div v-if="selectedReport.issues"><h5>问题与风险</h5><p>{{ selectedReport.issues }}</p></div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import api from '../api'

const reports = ref([])
const loading = ref(false)
const filterDate = ref('')
const showDetail = ref(false)
const selectedReport = ref(null)

async function loadReports() {
  loading.value = true
  try {
    const params = {}
    if (filterDate.value) params.date = filterDate.value
    const res = await api.listReports(params)
    reports.value = res.data
  } catch {} finally { loading.value = false }
}

function viewReport(row) { selectedReport.value = row; showDetail.value = true }

function parseTasks(val) {
  if (!val) return []
  try { const arr = typeof val === 'string' ? JSON.parse(val) : val; return Array.isArray(arr) ? arr : [] } catch { return [] }
}

onMounted(loadReports)
</script>
