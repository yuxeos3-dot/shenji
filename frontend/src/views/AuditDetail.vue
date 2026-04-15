<template>
  <div>
    <el-page-header @back="$router.push(`/sites/${$route.params.id}`)">
      <template #content><span>审计详情 #{{ $route.params.taskId }}</span></template>
    </el-page-header>

    <div v-if="task" style="margin-top:16px">
      <el-descriptions :column="3" border style="margin-bottom:16px">
        <el-descriptions-item label="状态">
          <el-tag :type="task.status==='done'?'success':task.status==='running'?'warning':'danger'">{{ task.status }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="进度"><el-progress :percentage="task.progress" /></el-descriptions-item>
        <el-descriptions-item label="时间">{{ task.created_at }}</el-descriptions-item>
      </el-descriptions>

      <el-tabs v-model="activeTab" type="border-card">
        <el-tab-pane label="AI分析报告" name="report">
          <div v-if="result">
            <el-alert v-if="result.summary" :title="result.summary" type="info" :closable="false" style="margin-bottom:16px" />

            <h4>SEO问题</h4>
            <IssueList :issues="parseJSON(result.seo_issues)" color="#409EFF" />

            <h4>代码问题</h4>
            <IssueList :issues="parseJSON(result.code_issues)" color="#67C23A" />

            <h4>安全问题</h4>
            <IssueList :issues="parseJSON(result.security_issues)" color="#F56C6C" />

            <h4>性能问题</h4>
            <IssueList :issues="parseJSON(result.perf_issues)" color="#E6A23C" />

            <h4 v-if="result.suggestions">整改建议</h4>
            <div v-if="result.suggestions" style="background:#f5f7fa;padding:16px;border-radius:4px;white-space:pre-wrap">{{ result.suggestions }}</div>
          </div>
          <el-empty v-else description="暂无分析结果" />
        </el-tab-pane>

        <el-tab-pane label="页面源码" name="pages">
          <el-tabs v-if="pages.length" tab-position="left">
            <el-tab-pane v-for="page in pages" :key="page.id" :label="page.page_type">
              <el-descriptions :column="2" border size="small" style="margin-bottom:12px">
                <el-descriptions-item label="URL">{{ page.url }}</el-descriptions-item>
                <el-descriptions-item label="状态码">
                  <el-tag :type="page.status_code===200?'success':'danger'" size="small">{{ page.status_code }}</el-tag>
                </el-descriptions-item>
                <el-descriptions-item label="TTFB">{{ page.ttfb_ms }}ms</el-descriptions-item>
                <el-descriptions-item label="大小">{{ (page.html_size/1024).toFixed(1) }}KB</el-descriptions-item>
              </el-descriptions>
              <el-input type="textarea" :model-value="page.html" :rows="20" readonly style="font-family:monospace;font-size:12px" />
            </el-tab-pane>
          </el-tabs>
          <el-empty v-else description="暂无页面数据" />
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import api from '../api'
import IssueList from '../components/IssueList.vue'

const route = useRoute()
const task = ref(null)
const result = ref(null)
const pages = ref([])
const activeTab = ref('report')

function parseJSON(str) {
  if (!str) return []
  try {
    const parsed = typeof str === 'string' ? JSON.parse(str) : str
    return Array.isArray(parsed) ? parsed : []
  } catch { return [] }
}

onMounted(async () => {
  try {
    const [auditRes, pagesRes] = await Promise.all([
      api.getAudit(route.params.taskId),
      api.getAuditPages(route.params.taskId),
    ])
    task.value = auditRes.data.task
    result.value = auditRes.data.result
    pages.value = pagesRes.data
  } catch {}
})
</script>
