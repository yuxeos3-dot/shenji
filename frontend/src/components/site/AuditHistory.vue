<template>
  <div class="panel">
    <div class="panel-header"><h2>审计历史</h2></div>
    <table class="table">
      <thead><tr><th>状态</th><th>进度</th><th>开始时间</th><th>完成时间</th><th>操作</th></tr></thead>
      <tbody>
        <tr v-for="a in audits" :key="a.id">
          <td><span class="badge" :class="a.status==='done'?'badge-ok':a.status==='running'?'badge-warn':'badge-err'">{{ a.status==='done'?'已完成':a.status==='running'?'进行中':a.status==='pending'?'等待中':'失败' }}</span></td>
          <td><div class="progress-bar"><div class="progress-fill" :style="{width:a.progress+'%'}"></div></div><span class="progress-text">{{ a.progress }}%</span></td>
          <td style="font-size:12px;color:var(--ink-soft)">{{ a.created_at }}</td>
          <td style="font-size:12px;color:var(--ink-soft)">{{ a.finished_at || '—' }}</td>
          <td style="display:flex;gap:6px;align-items:center">
            <button class="btn btn-xs" @click="$emit('view-audit', a.id)">查看详情</button>
            <button v-if="a.status==='done'" class="btn btn-xs btn-dl" @click="$emit('download-report', a.id)" :disabled="a.downloading">
              {{ a.downloading ? '生成中...' : '下载报告' }}
            </button>
            <button class="btn btn-xs btn-del-audit" @click="$emit('delete-audit', a)" :disabled="a.status==='running'">删除</button>
          </td>
        </tr>
        <tr v-if="!audits.length"><td colspan="5" style="text-align:center;color:var(--ink-hint);padding:30px">暂无审计记录</td></tr>
      </tbody>
    </table>
  </div>
</template>

<script setup>
defineProps({
  audits: { type: Array, required: true },
})
defineEmits(['view-audit', 'download-report', 'delete-audit'])
</script>

<style scoped>
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:18px; }
.panel-header h2 { font-size:15px; font-weight:700; }
.table { width:100%; border-collapse:collapse; }
.table th { padding:10px 14px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.08em; border-bottom:2px solid var(--border); background:var(--surface-3); }
.table td { padding:10px 14px; border-bottom:1px solid var(--border-light); font-size:13px; }
.table tr:hover td { background:var(--sun-light); }
.badge { display:inline-block; font-size:11px; font-weight:800; padding:2px 10px; border-radius:20px; }
.badge-ok { background:var(--ok-bg); color:var(--ok); border:1px solid var(--ok-border); }
.badge-warn { background:var(--warn-bg); color:var(--warn); border:1px solid var(--warn-border); }
.badge-err { background:var(--err-bg); color:var(--err); border:1px solid var(--err-border); }
.progress-bar { display:inline-block; width:80px; height:6px; background:var(--border-light); border-radius:3px; overflow:hidden; vertical-align:middle; margin-right:6px; }
.progress-fill { height:100%; background:linear-gradient(135deg,var(--sun),var(--coral)); border-radius:3px; transition:width .3s; }
.progress-text { font-size:11px; font-weight:700; color:var(--ink-soft); }
.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-xs:hover { background:var(--sun-glow); border-color:var(--sun); color:var(--sun-deep); }
.btn-dl { background:#EFF6FF !important; border-color:#BFDBFE !important; color:#1D4ED8 !important; }
.btn-dl:hover { background:#2563EB !important; color:#fff !important; border-color:#2563EB !important; }
.btn-del-audit { color:var(--err) !important; border-color:#FFCDD2 !important; background:#FFF5F5 !important; }
.btn-del-audit:hover { background:var(--err) !important; color:#fff !important; border-color:var(--err) !important; }
.btn-del-audit:disabled { opacity:.4; cursor:not-allowed; }
</style>
