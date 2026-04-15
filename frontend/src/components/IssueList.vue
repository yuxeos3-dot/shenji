<template>
  <div v-if="issues.length">
    <el-collapse>
      <el-collapse-item v-for="(issue, idx) in issues" :key="idx" :name="idx">
        <template #title>
          <el-tag :type="levelType(issue.level)" size="small" style="margin-right:8px">{{ issue.level }}</el-tag>
          <span>{{ issue.title }}</span>
        </template>
        <div style="padding:0 16px">
          <p v-if="issue.desc"><strong>问题描述：</strong>{{ issue.desc }}</p>
          <p v-if="issue.fix"><strong>修复建议：</strong>{{ issue.fix }}</p>
        </div>
      </el-collapse-item>
    </el-collapse>
  </div>
  <div v-else style="color:#909399;padding:12px">暂无问题</div>
</template>

<script setup>
defineProps({ issues: { type: Array, default: () => [] }, color: String })

function levelType(level) {
  if (level === 'P0') return 'danger'
  if (level === 'P1') return 'warning'
  return 'info'
}
</script>
