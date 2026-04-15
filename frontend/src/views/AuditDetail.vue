<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push(`/sites/${$route.params.id}`)" class="back-btn">← 返回站点</a>
      <span class="back-sep">·</span>
      <span class="back-url">审计 #{{ $route.params.taskId }}</span>
    </div>

    <!-- 任务状态卡片 -->
    <div v-if="task" class="panel panel-amber" style="margin-bottom:18px">
      <div class="task-header">
        <div class="info-grid-3">
          <div class="info-item">
            <span class="info-label">状态</span>
            <span class="badge" :class="task.status==='done'?'badge-ok':task.status==='running'?'badge-warn':'badge-err'">
              {{ task.status==='done'?'✅ 已完成':task.status==='running'?'⏳ 进行中':task.status==='pending'?'🕐 等待中':'❌ 失败' }}
            </span>
          </div>
          <div class="info-item">
            <span class="info-label">进度</span>
            <div style="display:flex;align-items:center;gap:8px">
              <div class="progress-bar" style="width:120px"><div class="progress-fill" :style="{width:task.progress+'%'}"></div></div>
              <span class="progress-text">{{ task.progress }}%</span>
            </div>
          </div>
          <div class="info-item">
            <span class="info-label">完成时间</span>
            <span style="font-size:13px;color:var(--ink-mid)">{{ task.finished_at || '—' }}</span>
          </div>
        </div>
        <!-- 下载按钮 -->
        <a v-if="task.status==='done' && v6Report" :href="downloadUrl" class="btn btn-download" target="_blank">
          📥 下载审计报告（Word）
        </a>
      </div>

      <!-- 报告概览 -->
      <div v-if="v6Report" class="report-overview">
        <div class="overview-stat" :class="v6Report.fatal>0?'ov-fatal':''">
          <div class="ov-num">{{ v6Report.fatal }}</div>
          <div class="ov-label">致命</div>
        </div>
        <div class="overview-stat" :class="v6Report.serious>0?'ov-serious':''">
          <div class="ov-num">{{ v6Report.serious }}</div>
          <div class="ov-label">严重</div>
        </div>
        <div class="overview-stat" :class="v6Report.medium>0?'ov-medium':''">
          <div class="ov-num">{{ v6Report.medium }}</div>
          <div class="ov-label">中等</div>
        </div>
        <div class="overview-stat" :class="v6Report.low>0?'ov-low':''">
          <div class="ov-num">{{ v6Report.low }}</div>
          <div class="ov-label">低</div>
        </div>
        <div class="overview-stat ov-total">
          <div class="ov-num">{{ v6Report.total_issues }}</div>
          <div class="ov-label">总计</div>
        </div>
        <div class="ov-summary" v-if="v6Report.summary">
          <span class="ov-summary-label">总结：</span>{{ v6Report.summary }}
        </div>
      </div>

      <div v-if="task.status==='running'" class="running-hint">
        <span class="spin">⏳</span> 正在深度审计中，使用 SEO 深度审计工作流 v6，覆盖 24 大类 180+ 检查点，请耐心等待（约 2-5 分钟）...
      </div>
    </div>

    <!-- V6 报告主体 -->
    <div v-if="v6Report && v6Report.chapters && v6Report.chapters.length">
      <!-- 章节导航 -->
      <div class="chapter-nav">
        <span class="chapter-nav-label">快速导航：</span>
        <a v-for="(ch, i) in v6Report.chapters" :key="i" :href="'#chapter-'+i" class="chapter-nav-item">
          {{ ch.title.substring(0, ch.title.indexOf('、')+3 || 8) }}
        </a>
      </div>

      <!-- 各章节 -->
      <div v-for="(chapter, ci) in v6Report.chapters" :key="ci" class="chapter-block" :id="'chapter-'+ci">
        <div class="chapter-title">{{ chapter.title }}</div>
        <div class="issue-count-badge">{{ chapter.issues.length }} 个问题</div>

        <div v-for="issue in chapter.issues" :key="issue.id" :class="['issue-card', 'issue-'+severityClass(issue.severity)]">
          <!-- 问题标题行 -->
          <div class="issue-header">
            <span :class="['sev-badge', 'sev-'+severityClass(issue.severity)]">{{ issue.severity }}</span>
            <span v-if="issue.module" class="mod-badge">{{ issue.module }}</span>
            <span class="issue-id">#{{ issue.id }}</span>
            <span class="issue-title">{{ issue.title }}</span>
          </div>

          <!-- 影响范围 -->
          <div v-if="issue.scope" class="issue-scope">📍 影响范围：{{ issue.scope }}</div>

          <!-- 证据 -->
          <div v-if="issue.evidence_label" class="evidence-label">📄 {{ issue.evidence_label }}</div>
          <pre v-if="issue.evidence_code" class="evidence-code">{{ issue.evidence_code }}</pre>

          <!-- 现状 -->
          <div v-if="issue.current_status" class="issue-section">
            <div class="section-label current-label">⚠️ 目前现状</div>
            <div class="section-body">{{ issue.current_status }}</div>
          </div>

          <!-- 修改建议 -->
          <div v-if="issue.fix_suggestion" class="issue-section">
            <div class="section-label fix-label">✅ 修改建议</div>
            <div class="section-body fix-body">{{ issue.fix_suggestion }}</div>
          </div>
        </div>
      </div>
    </div>

    <!-- 旧版报告（无v6数据时显示） -->
    <div v-else-if="result && task && task.status==='done'" class="panel">
      <div class="panel-header"><h2>📊 审计摘要</h2></div>
      <div v-if="result.summary" class="summary-bar">💡 {{ result.summary }}</div>
      <div v-if="result.suggestions" class="suggestions-box">{{ result.suggestions }}</div>
    </div>

    <!-- 进行中占位 -->
    <div v-else-if="task && task.status==='running'" class="panel running-panel">
      <div class="running-icon">🔍</div>
      <div class="running-title">SEO 深度审计进行中</div>
      <div class="running-desc">正在按 v6 工作流逐项检查 24 大类 180+ 检查点</div>
      <div class="progress-bar large-progress">
        <div class="progress-fill" :style="{width:task.progress+'%'}"></div>
      </div>
      <div class="running-note">{{ task.progress }}%　完成后将自动显示完整报告</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import api from '../api'

const route = useRoute()
const task = ref(null)
const result = ref(null)
const v6Report = ref(null)
let pollTimer = null

const downloadUrl = computed(() => {
  return api.downloadAuditReport(route.params.taskId)
})

function severityClass(sev) {
  if (sev === '致命') return 'fatal'
  if (sev === '严重') return 'serious'
  if (sev === '中等') return 'medium'
  return 'low'
}

async function loadData() {
  try {
    const res = await api.getAudit(route.params.taskId)
    task.value = res.data.task
    result.value = res.data.result

    // Parse v6_report
    if (res.data.result && res.data.result.v6_report) {
      try {
        v6Report.value = JSON.parse(res.data.result.v6_report)
      } catch {}
    }

    // Poll while running
    if (task.value.status === 'running' || task.value.status === 'pending') {
      pollTimer = setTimeout(loadData, 5000)
    } else if (pollTimer) {
      clearTimeout(pollTimer)
    }
  } catch {}
}

onMounted(loadData)
onUnmounted(() => { if (pollTimer) clearTimeout(pollTimer) })
</script>

<style scoped>
.back-row { display:flex; align-items:center; gap:8px; margin-bottom:18px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); }
.back-sep { color:var(--ink-hint); }
.back-url { font-size:14px; font-weight:700; color:var(--ink); }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel + .panel { margin-top:18px; }
.panel-amber { background:linear-gradient(135deg,#FFFDF8 0%,#FFF8EC 60%,#FFF2D8 100%); border-color:var(--peach); }

/* Task header */
.task-header { display:flex; align-items:flex-start; justify-content:space-between; margin-bottom:16px; flex-wrap:wrap; gap:12px; }
.info-grid-3 { display:grid; grid-template-columns:repeat(3,1fr); gap:14px; flex:1; }
.info-item { display:flex; flex-direction:column; gap:4px; }
.info-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.06em; }

.btn-download { display:inline-flex; align-items:center; gap:7px; padding:10px 20px; background:linear-gradient(135deg,#2563EB,#3B82F6); color:#fff; border-radius:var(--r-sm); font-weight:700; font-size:13.5px; text-decoration:none; box-shadow:0 4px 14px rgba(37,99,235,.3); transition:all .18s; white-space:nowrap; }
.btn-download:hover { filter:brightness(1.1); transform:translateY(-1px); }

/* Report overview */
.report-overview { display:flex; align-items:center; gap:12px; flex-wrap:wrap; padding-top:14px; border-top:1.5px solid var(--border-light); margin-top:4px; }
.overview-stat { display:flex; flex-direction:column; align-items:center; padding:10px 16px; border-radius:var(--r-sm); background:var(--surface-3); min-width:64px; }
.ov-num { font-size:24px; font-weight:900; font-family:'JetBrains Mono',monospace; }
.ov-label { font-size:11px; font-weight:700; color:var(--ink-soft); margin-top:2px; }
.ov-fatal { background:#FFF0F0; } .ov-fatal .ov-num { color:#C0392B; }
.ov-serious { background:#FFF4E6; } .ov-serious .ov-num { color:#E67E22; }
.ov-medium { background:#FFFBE6; } .ov-medium .ov-num { color:#F39C12; }
.ov-low { background:#F0FFF4; } .ov-low .ov-num { color:#27AE60; }
.ov-total { background:var(--sun-light); } .ov-total .ov-num { color:var(--sun-deep); }
.ov-summary { flex:1; font-size:13px; color:var(--ink-mid); min-width:200px; }
.ov-summary-label { font-weight:700; color:var(--ink); }

.running-hint { margin-top:14px; font-size:13px; color:var(--warn); display:flex; align-items:center; gap:8px; }
.spin { animation:spin 1.5s linear infinite; display:inline-block; }
@keyframes spin { to { transform:rotate(360deg); } }

/* Chapter nav */
.chapter-nav { background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r-sm); padding:12px 16px; margin-bottom:16px; display:flex; flex-wrap:wrap; gap:8px; align-items:center; }
.chapter-nav-label { font-size:12px; font-weight:700; color:var(--ink-hint); }
.chapter-nav-item { font-size:12px; font-weight:600; color:var(--info); padding:3px 10px; background:var(--surface-3); border-radius:5px; text-decoration:none; }
.chapter-nav-item:hover { background:var(--sun-light); color:var(--sun-deep); }

/* Chapter block */
.chapter-block { margin-bottom:28px; }
.chapter-title { font-size:18px; font-weight:800; color:#2C3E50; padding:14px 0 6px; border-bottom:3px solid var(--sun); display:inline-block; margin-bottom:4px; }
.issue-count-badge { font-size:12px; font-weight:600; color:var(--ink-soft); margin-bottom:14px; }

/* Issue card */
.issue-card { background:var(--surface); border-radius:var(--r-sm); border:1.5px solid var(--border-light); padding:18px 20px; margin-bottom:14px; box-shadow:var(--s1); }
.issue-fatal { border-left:4px solid #C0392B; }
.issue-serious { border-left:4px solid #E67E22; }
.issue-medium { border-left:4px solid #F39C12; }
.issue-low { border-left:4px solid #27AE60; }

.issue-header { display:flex; align-items:center; gap:8px; flex-wrap:wrap; margin-bottom:10px; }
.sev-badge { display:inline-block; padding:2px 9px; border-radius:4px; font-size:11px; font-weight:800; color:#fff; }
.sev-fatal { background:#C0392B; }
.sev-serious { background:#E67E22; }
.sev-medium { background:#F39C12; }
.sev-low { background:#27AE60; }
.mod-badge { display:inline-block; padding:2px 8px; border-radius:4px; font-size:11px; font-weight:700; background:#34495E; color:#ECF0F1; }
.issue-id { font-size:11px; font-weight:700; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; }
.issue-title { font-size:15px; font-weight:700; color:var(--ink); }

.issue-scope { font-size:12.5px; color:var(--ink-soft); margin-bottom:10px; }

.evidence-label { font-size:12px; font-weight:700; color:#34495E; margin-bottom:6px; }
.evidence-code { background:#2C3E50; color:#ECF0F1; padding:12px 16px; border-radius:var(--r-sm); font-family:'JetBrains Mono',monospace; font-size:12px; line-height:1.6; overflow-x:auto; white-space:pre-wrap; word-break:break-all; margin-bottom:10px; }

.issue-section { margin-top:10px; }
.section-label { font-size:13px; font-weight:700; margin-bottom:6px; }
.current-label { color:#C0392B; }
.fix-label { color:#27AE60; }
.section-body { font-size:13px; color:var(--ink-mid); line-height:1.7; white-space:pre-line; }
.fix-body { background:#F0FFF4; padding:12px 14px; border-radius:var(--r-sm); border-left:3px solid #27AE60; }

/* Running state */
.running-panel { text-align:center; padding:60px 40px; }
.running-icon { font-size:48px; margin-bottom:12px; }
.running-title { font-size:20px; font-weight:800; color:var(--ink); margin-bottom:8px; }
.running-desc { font-size:14px; color:var(--ink-soft); margin-bottom:24px; }
.large-progress { width:320px; height:10px; margin:0 auto 12px; }
.running-note { font-size:13px; color:var(--ink-hint); }

.badge { display:inline-block; font-size:11px; font-weight:800; padding:3px 10px; border-radius:20px; }
.badge-ok { background:var(--ok-bg); color:var(--ok); border:1px solid var(--ok-border); }
.badge-warn { background:var(--warn-bg); color:var(--warn); border:1px solid var(--warn-border); }
.badge-err { background:var(--err-bg); color:var(--err); border:1px solid var(--err-border); }

.progress-bar { display:inline-block; height:6px; background:var(--border-light); border-radius:3px; overflow:hidden; vertical-align:middle; margin-right:6px; }
.progress-fill { height:100%; background:linear-gradient(135deg,var(--sun),var(--coral)); border-radius:3px; transition:width .3s; }
.progress-text { font-size:11px; font-weight:700; color:var(--ink-soft); }

.summary-bar { background:var(--sun-light); border-left:4px solid var(--sun); padding:12px 16px; border-radius:0 var(--r-sm) var(--r-sm) 0; font-size:14px; font-weight:600; color:var(--ink); margin-bottom:16px; }
.suggestions-box { background:var(--surface-2); padding:16px 20px; border-radius:var(--r-sm); font-size:13.5px; white-space:pre-line; color:var(--ink-mid); line-height:1.7; }
.panel-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:18px; }
.panel-header h2 { font-size:15px; font-weight:700; }
</style>
