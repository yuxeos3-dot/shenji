<template>
  <div v-if="issues.length" class="issue-list">
    <div v-for="(issue, idx) in issues" :key="idx" class="issue-item" @click="toggle(idx)">
      <div class="issue-header">
        <span class="issue-level" :class="'level-'+issue.level">{{ issue.level }}</span>
        <span class="issue-title">{{ issue.title }}</span>
        <span class="issue-toggle">{{ expanded[idx] ? '▾' : '▸' }}</span>
      </div>
      <div v-show="expanded[idx]" class="issue-body">
        <p v-if="issue.desc"><strong>问题描述：</strong>{{ issue.desc }}</p>
        <p v-if="issue.fix"><strong>修复建议：</strong>{{ issue.fix }}</p>
      </div>
    </div>
  </div>
  <div v-else class="no-issues">暂无问题 ✓</div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
const props = defineProps({ issues: { type: Array, default: () => [] } })
const expanded = ref({})
function toggle(idx) { expanded.value[idx] = !expanded.value[idx] }
</script>

<style scoped>
.issue-list { display:flex; flex-direction:column; gap:6px; }
.issue-item { border:1.5px solid var(--border-light); border-radius:9px; overflow:hidden; transition:all .15s; cursor:pointer; }
.issue-item:hover { border-color:var(--peach); box-shadow:var(--s1); }
.issue-header { display:flex; align-items:center; gap:10px; padding:10px 14px; }
.issue-level { font-size:11px; font-weight:800; padding:2px 10px; border-radius:20px; flex-shrink:0; }
.level-P0 { background:#FFF5F5; color:#DC2626; border:1px solid #FECACA; }
.level-P1 { background:#FFFBEB; color:#D97706; border:1px solid #FDE68A; }
.level-P2 { background:var(--surface-3); color:var(--ink-soft); border:1px solid var(--border); }
.issue-title { font-size:13px; font-weight:600; color:var(--ink); flex:1; }
.issue-toggle { font-size:12px; color:var(--ink-hint); }
.issue-body { padding:0 14px 14px; font-size:13px; color:var(--ink-mid); line-height:1.8; border-top:1px solid var(--border-light); margin-top:0; padding-top:12px; }
.issue-body strong { font-weight:700; color:var(--ink); }
.no-issues { color:var(--ok); font-size:13px; font-weight:600; padding:8px 0; }
</style>
