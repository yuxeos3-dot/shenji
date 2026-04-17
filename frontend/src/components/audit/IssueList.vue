<template>
  <template v-if="allIssues.length">
    <!-- 筛选栏 -->
    <div class="filter-bar">
      <span class="filter-label">筛选严重级别：</span>
      <button :class="['filter-btn', filterSev===''&&'filter-active']" @click="filterSev='';page=1">全部 ({{ allIssues.length }})</button>
      <button :class="['filter-btn', 'sev-btn-fatal', filterSev==='致命'&&'filter-active']" @click="filterSev='致命';page=1">致命 ({{ countBySev('致命') }})</button>
      <button :class="['filter-btn', 'sev-btn-serious', filterSev==='严重'&&'filter-active']" @click="filterSev='严重';page=1">严重 ({{ countBySev('严重') }})</button>
      <button :class="['filter-btn', 'sev-btn-medium', filterSev==='中等'&&'filter-active']" @click="filterSev='中等';page=1">中等 ({{ countBySev('中等') }})</button>
      <button :class="['filter-btn', 'sev-btn-low', filterSev==='低'&&'filter-active']" @click="filterSev='低';page=1">低 ({{ countBySev('低') }})</button>
      <button v-if="showDownload" class="btn-dl-inline" @click="$emit('download')">下载报告</button>
    </div>

    <!-- 问题卡片 -->
    <div v-for="issue in pagedIssues" :key="issue.id" :class="['issue-card', 'issue-'+severityClass(issue.severity)]">
      <div class="issue-header">
        <span :class="['sev-badge', 'sev-'+severityClass(issue.severity)]">{{ issue.severity }}</span>
        <span v-if="issue.module" class="mod-badge">{{ issue.module }}</span>
        <span class="issue-id">#{{ issue.id }}</span>
        <span class="issue-title">{{ issue.title }}</span>
        <span v-if="issue._chapter" class="chapter-tag">{{ issue._chapter }}</span>
        <button
          v-if="enableCoord"
          class="coord-btn"
          :disabled="coordAddedIds.has(String(issue.id))"
          @click="$emit('add-to-coord', issue)"
          :title="coordAddedIds.has(String(issue.id)) ? '已加入协同管理' : '加入协同管理交给开发修复'"
        >
          {{ coordAddedIds.has(String(issue.id)) ? '✓ 已加入' : '➕ 加入协同' }}
        </button>
      </div>
      <div v-if="issue.scope" class="issue-scope">{{ issue.scope }}</div>
      <div v-if="issue.evidence_label" class="evidence-label">{{ issue.evidence_label }}</div>
      <pre v-if="issue.evidence_code" class="evidence-code">{{ issue.evidence_code }}</pre>
      <div v-if="issue.current_status" class="issue-section">
        <div class="section-label current-label">目前现状</div>
        <div class="section-body">{{ issue.current_status }}</div>
      </div>
      <div v-if="issue.fix_suggestion" class="issue-section">
        <div class="section-label fix-label">修改建议</div>
        <div class="section-body fix-body">{{ issue.fix_suggestion }}</div>
      </div>
    </div>

    <!-- 分页 -->
    <div class="pagination-bar">
      <span class="page-info">共 {{ filteredIssues.length }} 个问题，第 {{ page }} / {{ totalPages }} 页</span>
      <div class="page-btns">
        <button class="page-btn" @click="page=1" :disabled="page===1">«</button>
        <button class="page-btn" @click="page--" :disabled="page===1">‹</button>
        <button
          v-for="p in pageRange" :key="p"
          :class="['page-btn', p===page&&'page-active']"
          @click="page=p"
        >{{ p }}</button>
        <button class="page-btn" @click="page++" :disabled="page===totalPages">›</button>
        <button class="page-btn" @click="page=totalPages" :disabled="page===totalPages">»</button>
      </div>
    </div>
  </template>
</template>

<script setup>
import { ref, computed } from 'vue'

const props = defineProps({
  report: { type: Object, default: null },
  showDownload: { type: Boolean, default: false },
  // Enables the "加入协同" button on each issue card. Parent listens for
  // `add-to-coord` and pushes the snapshot into /v1/coord/tasks.
  enableCoord: { type: Boolean, default: false },
  // Set of issue-ids (as strings) already in the coordination queue for
  // this audit — used to disable the button and show a checkmark so the
  // user doesn't queue the same issue twice accidentally.
  coordAddedIds: { type: Set, default: () => new Set() },
})
defineEmits(['download', 'add-to-coord'])

const filterSev = ref('')
const page = ref(1)
const PAGE_SIZE = 20

function severityClass(sev) {
  if (sev === '致命') return 'fatal'
  if (sev === '严重') return 'serious'
  if (sev === '中等') return 'medium'
  return 'low'
}

const allIssues = computed(() => {
  if (!props.report?.chapters) return []
  const list = []
  for (const ch of props.report.chapters) {
    for (const issue of ch.issues || []) {
      list.push({ ...issue, _chapter: ch.title })
    }
  }
  return list
})

const filteredIssues = computed(() => {
  if (!filterSev.value) return allIssues.value
  return allIssues.value.filter(i => i.severity === filterSev.value)
})

const totalPages = computed(() => Math.max(1, Math.ceil(filteredIssues.value.length / PAGE_SIZE)))

const pagedIssues = computed(() => {
  const start = (page.value - 1) * PAGE_SIZE
  return filteredIssues.value.slice(start, start + PAGE_SIZE)
})

const pageRange = computed(() => {
  const total = totalPages.value
  const cur = page.value
  const delta = 2
  const range = []
  for (let i = Math.max(1, cur - delta); i <= Math.min(total, cur + delta); i++) range.push(i)
  return range
})

function countBySev(sev) {
  return allIssues.value.filter(i => i.severity === sev).length
}
</script>

<style scoped>
.filter-bar { display:flex; align-items:center; gap:8px; flex-wrap:wrap; background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r-sm); padding:12px 16px; margin-bottom:16px; }
.filter-label { font-size:12.5px; font-weight:700; color:var(--ink-mid); white-space:nowrap; }
.filter-btn { padding:5px 14px; border-radius:20px; border:1.5px solid var(--border-light); background:var(--surface-2); font-size:12px; font-weight:600; color:var(--ink-soft); cursor:pointer; font-family:inherit; transition:all .15s; }
.filter-btn:hover { border-color:var(--peach); color:var(--sun-deep); background:var(--sun-light); }
.filter-active { background:var(--sun) !important; color:#fff !important; border-color:var(--sun) !important; }
.sev-btn-fatal.filter-active { background:#C0392B !important; border-color:#C0392B !important; }
.sev-btn-serious.filter-active { background:#E67E22 !important; border-color:#E67E22 !important; }
.sev-btn-medium.filter-active { background:#F39C12 !important; border-color:#F39C12 !important; }
.sev-btn-low.filter-active { background:#27AE60 !important; border-color:#27AE60 !important; }
.btn-dl-inline { margin-left:auto; display:inline-flex; align-items:center; gap:6px; padding:6px 14px; background:#EFF6FF; border:1.5px solid #BFDBFE; color:#1D4ED8; border-radius:var(--r-sm); font-size:12.5px; font-weight:700; transition:all .15s; white-space:nowrap; }
.btn-dl-inline:hover { background:#2563EB; color:#fff; border-color:#2563EB; }
.issue-card { background:var(--surface); border-radius:var(--r-sm); border:1.5px solid var(--border-light); padding:18px 20px; margin-bottom:12px; box-shadow:var(--s1); }
.issue-fatal { border-left:4px solid #C0392B; }
.issue-serious { border-left:4px solid #E67E22; }
.issue-medium { border-left:4px solid #F39C12; }
.issue-low { border-left:4px solid #27AE60; }
.issue-header { display:flex; align-items:center; gap:8px; flex-wrap:wrap; margin-bottom:10px; }
.sev-badge { display:inline-block; padding:2px 9px; border-radius:4px; font-size:11px; font-weight:800; color:#fff; }
.sev-fatal { background:#C0392B; } .sev-serious { background:#E67E22; } .sev-medium { background:#F39C12; } .sev-low { background:#27AE60; }
.mod-badge { display:inline-block; padding:2px 8px; border-radius:4px; font-size:11px; font-weight:700; background:#34495E; color:#ECF0F1; }
.issue-id { font-size:11px; font-weight:700; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; }
.issue-title { font-size:15px; font-weight:700; color:var(--ink); flex:1; }
.chapter-tag { font-size:11px; color:var(--ink-hint); padding:2px 8px; background:var(--surface-3); border-radius:4px; white-space:nowrap; }
.coord-btn { padding:4px 12px; border-radius:6px; font-size:11.5px; font-weight:700; cursor:pointer; background:var(--sun-light); color:var(--sun-deep); border:1.5px solid var(--peach); transition:all .15s; font-family:inherit; white-space:nowrap; }
.coord-btn:hover:not(:disabled) { background:var(--sun); color:#fff; border-color:var(--sun); box-shadow:0 2px 8px rgba(249,115,22,.3); }
.coord-btn:disabled { background:var(--ok-bg); color:var(--ok); border-color:var(--ok-border); cursor:not-allowed; opacity:.85; }
.issue-scope { font-size:12.5px; color:var(--ink-soft); margin-bottom:10px; }
.evidence-label { font-size:12px; font-weight:700; color:#34495E; margin-bottom:6px; }
.evidence-code { background:#2C3E50; color:#ECF0F1; padding:12px 16px; border-radius:var(--r-sm); font-family:'JetBrains Mono',monospace; font-size:12px; line-height:1.6; overflow-x:auto; white-space:pre-wrap; word-break:break-all; margin-bottom:10px; }
.issue-section { margin-top:10px; }
.section-label { font-size:13px; font-weight:700; margin-bottom:6px; }
.current-label { color:#C0392B; } .fix-label { color:#27AE60; }
.section-body { font-size:13px; color:var(--ink-mid); line-height:1.7; white-space:pre-line; }
.fix-body { background:#F0FFF4; padding:12px 14px; border-radius:var(--r-sm); border-left:3px solid #27AE60; }
.pagination-bar { display:flex; align-items:center; justify-content:space-between; background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r-sm); padding:12px 16px; margin-top:8px; flex-wrap:wrap; gap:10px; }
.page-info { font-size:13px; color:var(--ink-soft); }
.page-btns { display:flex; gap:4px; }
.page-btn { min-width:34px; height:34px; border-radius:7px; border:1.5px solid var(--border-light); background:var(--surface); color:var(--ink-mid); font-size:13px; font-weight:600; cursor:pointer; font-family:inherit; transition:all .15s; display:flex; align-items:center; justify-content:center; }
.page-btn:hover:not(:disabled) { border-color:var(--sun); color:var(--sun-deep); background:var(--sun-light); }
.page-btn:disabled { opacity:.4; cursor:not-allowed; }
.page-active { background:var(--sun) !important; color:#fff !important; border-color:var(--sun) !important; }
</style>
