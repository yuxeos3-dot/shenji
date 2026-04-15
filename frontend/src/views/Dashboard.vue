<template>
  <div>
    <h2 style="margin-top:0">仪表盘</h2>
    <el-row :gutter="20" style="margin-bottom:20px">
      <el-col :span="6" v-for="stat in stats" :key="stat.label">
        <el-card shadow="hover">
          <div style="display:flex;align-items:center;justify-content:space-between">
            <div>
              <div style="color:#909399;font-size:14px">{{ stat.label }}</div>
              <div style="font-size:28px;font-weight:bold;margin-top:8px">{{ stat.value }}</div>
            </div>
            <el-icon :size="40" :color="stat.color"><component :is="stat.icon" /></el-icon>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20">
      <el-col :span="16">
        <el-card>
          <template #header><span>最近审计记录</span></template>
          <el-table :data="recentAudits" stripe>
            <el-table-column prop="site_name" label="站点" />
            <el-table-column prop="status" label="状态">
              <template #default="{row}">
                <el-tag :type="row.status==='done'?'success':row.status==='running'?'warning':'danger'">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="created_at" label="时间" />
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card>
          <template #header><span>站点健康状态</span></template>
          <div v-for="site in sites" :key="site.id" style="display:flex;align-items:center;justify-content:space-between;padding:8px 0;border-bottom:1px solid #eee">
            <span>{{ site.name }}</span>
            <el-tag :type="site.last_audit_status==='done'?'success':'info'" size="small">
              {{ site.last_audit_status || '未审计' }}
            </el-tag>
          </div>
          <div v-if="!sites.length" style="text-align:center;color:#909399;padding:20px">暂无站点</div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import api from '../api'

const sites = ref([])
const recentAudits = ref([])

const stats = computed(() => [
  { label: '站点总数', value: sites.value.length, icon: 'Monitor', color: '#409EFF' },
  { label: '健康站点', value: sites.value.filter(s => s.last_audit_status === 'done').length, icon: 'CircleCheck', color: '#67C23A' },
  { label: '待审计', value: sites.value.filter(s => !s.last_audit_status).length, icon: 'Clock', color: '#E6A23C' },
  { label: '异常站点', value: sites.value.filter(s => s.last_audit_status === 'error').length, icon: 'Warning', color: '#F56C6C' },
])

onMounted(async () => {
  try {
    const res = await api.listSites()
    sites.value = res.data
  } catch {}
})
</script>
