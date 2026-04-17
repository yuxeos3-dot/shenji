<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push(`/sites/${$route.params.id}`)" class="back-btn">← 返回站点</a>
      <span class="back-sep">·</span>
      <span class="back-url">审计详情</span>
    </div>

    <TaskStatusCard
      :task="task"
      :report="v6Report"
      :raw-response="result?.raw_ai_response || ''"
      :downloading="downloading"
      @download="downloadReport"
      @copy-raw="copyRaw"
    />

    <IssueList
      v-if="v6Report"
      :report="v6Report"
      :show-download="task?.status === 'done'"
      @download="downloadReport"
    />

    <!-- 进行中占位 -->
    <div v-if="task && (task.status==='running'||task.status==='pending') && !v6Report" class="panel running-panel">
      <div class="running-icon">🔍</div>
      <div class="running-title">SEO 深度审计进行中</div>
      <div class="running-desc">正在按 v6 工作流逐项检查 28 大类 200+ 检查点</div>
      <div class="progress-bar large-progress">
        <div class="progress-fill" :style="{width:(task.progress||0)+'%'}"></div>
      </div>
      <div class="running-note">{{ task.progress }}%　完成后将自动显示完整报告</div>
    </div>

    <!-- 已完成但报告解析失败 -->
    <div v-if="result && task?.status==='done' && !v6Report" class="panel">
      <div class="summary-bar">报告数据需要重新生成，请点击「立即巡检」重新审计以获取完整报告格式。</div>
      <div v-if="result.summary && !result.summary.startsWith('{')" class="suggestions-box">{{ result.summary }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'
import { copyToClipboard } from '../utils/clipboard'
import TaskStatusCard from '../components/audit/TaskStatusCard.vue'
import IssueList from '../components/audit/IssueList.vue'

const route = useRoute()
const task = ref(null)
const result = ref(null)
const v6Report = ref(null)
const downloading = ref(false)
let pollTimer = null

async function copyRaw() {
  const ok = await copyToClipboard(result.value?.raw_ai_response || '')
  if (ok) ElMessage.success('已复制到剪贴板')
  else ElMessage.error('复制失败，请手动选择文本复制')
}

async function downloadReport() {
  downloading.value = true
  try {
    const { blob, filename } = await api.fetchAuditReportBlob(route.params.taskId)
    const url = URL.createObjectURL(blob)
    const el = document.createElement('a')
    el.href = url
    el.download = filename
    document.body.appendChild(el); el.click(); document.body.removeChild(el)
    URL.revokeObjectURL(url)
    ElMessage.success('报告下载成功')
  } catch (e) { ElMessage.error('下载失败：' + (e?.message || e)) }
  finally { downloading.value = false }
}

function clearPoll() {
  if (pollTimer) { clearTimeout(pollTimer); pollTimer = null }
}

async function loadData() {
  clearPoll()
  try {
    const res = await api.getAudit(route.params.taskId)
    task.value = res.data.task
    result.value = res.data.result
    if (res.data.result?.v6_report) {
      try {
        let raw = res.data.result.v6_report
        raw = raw.trim()
        if (raw.startsWith('```')) {
          const lines = raw.split('\n')
          raw = lines.slice(1).join('\n')
          const lastTick = raw.lastIndexOf('```')
          if (lastTick >= 0) raw = raw.substring(0, lastTick)
          raw = raw.trim()
        }
        const start = raw.indexOf('{')
        if (start >= 0) raw = raw.substring(start)
        const parsed = JSON.parse(raw)
        if (parsed.chapters && parsed.chapters.length > 0) {
          v6Report.value = parsed
        } else {
          ElMessage.warning('审计报告格式异常：缺少章节数据')
        }
      } catch (e) {
        console.warn('v6_report parse error:', e)
        ElMessage.warning('审计报告解析失败，请重新生成')
      }
    }
    if (task.value.status === 'running' || task.value.status === 'pending') {
      pollTimer = setTimeout(loadData, 3000)
    }
  } catch (e) { console.error(e) }
}

onMounted(loadData)
onUnmounted(clearPoll)
</script>

<style scoped>
.back-row { display:flex; align-items:center; gap:8px; margin-bottom:18px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); }
.back-sep { color:var(--ink-hint); }
.back-url { font-size:14px; font-weight:700; color:var(--ink); }
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.running-panel { text-align:center; padding:60px 40px; }
.running-icon { font-size:48px; margin-bottom:12px; }
.running-title { font-size:20px; font-weight:800; color:var(--ink); margin-bottom:8px; }
.running-desc { font-size:14px; color:var(--ink-soft); margin-bottom:24px; }
.large-progress { width:320px; height:10px; margin:0 auto 12px; }
.running-note { font-size:13px; color:var(--ink-hint); }
.progress-bar { display:inline-block; height:6px; background:var(--border-light); border-radius:3px; overflow:hidden; vertical-align:middle; margin-right:6px; }
.progress-fill { height:100%; background:linear-gradient(135deg,var(--sun),var(--coral)); border-radius:3px; transition:width .3s; }
.summary-bar { background:var(--sun-light); border-left:4px solid var(--sun); padding:12px 16px; border-radius:0 var(--r-sm) var(--r-sm) 0; font-size:14px; font-weight:600; color:var(--ink); margin-bottom:16px; }
.suggestions-box { background:var(--surface-2); padding:16px 20px; border-radius:var(--r-sm); font-size:13.5px; white-space:pre-line; color:var(--ink-mid); line-height:1.7; }
</style>
