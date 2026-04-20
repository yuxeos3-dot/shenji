<template>
  <div v-if="task" class="panel panel-amber" style="margin-bottom:18px">
    <div class="task-header">
      <div class="info-grid-3">
        <div class="info-item">
          <span class="info-label">状态</span>
          <span class="badge" :class="task.status==='done'?'badge-ok':task.status==='running'?'badge-warn':'badge-err'">
            {{ task.status==='done'?'已完成':task.status==='running'?'进行中':task.status==='pending'?'等待中':'失败' }}
          </span>
        </div>
        <div class="info-item">
          <span class="info-label">进度</span>
          <div style="display:flex;align-items:center;gap:8px">
            <div class="progress-bar" style="width:140px"><div class="progress-fill" :style="{width:task.progress+'%'}"></div></div>
            <span class="progress-text">{{ task.progress }}%</span>
          </div>
        </div>
        <div class="info-item">
          <span class="info-label">完成时间</span>
          <span style="font-size:13px;color:var(--ink-mid)">{{ task.finished_at || '—' }}</span>
        </div>
      </div>
      <button v-if="task.status==='done'" class="btn btn-download" @click="$emit('download')" :disabled="downloading">
        {{ downloading ? '生成中...' : '下载 Word 报告' }}
      </button>
    </div>

    <div v-if="task.status==='error'" class="error-bar">
      <div class="err-title">审计失败原因</div>
      <!-- error_msg may span multiple lines (e.g. "AI 调用失败：余额不足..." +
           raw 原始错误 payload). Preserve newlines so the user can actually
           read the hint rather than seeing one crammed line. -->
      <pre class="err-body">{{ task.error_msg || 'AI 接口超时或配置错误，请检查「个人 AI 设置」并重新触发巡检' }}</pre>
    </div>

    <div v-if="task.status==='error' && rawResponse" class="raw-ai-box">
      <div class="raw-ai-head">
        <span>AI 原始输出（{{ rawResponse.length }} 字符）</span>
        <div>
          <button class="btn btn-sm" @click="showRaw = !showRaw">{{ showRaw ? '收起' : '展开' }}</button>
          <button class="btn btn-sm" style="margin-left:6px" @click="$emit('copy-raw')">复制</button>
        </div>
      </div>
      <pre v-if="showRaw" class="raw-ai-pre">{{ rawResponse }}</pre>
      <div v-else class="raw-ai-preview">{{ rawResponse.slice(0, 400) }}...</div>
    </div>

    <div v-if="report" class="report-overview">
      <div class="overview-stat ov-fatal"><div class="ov-num">{{ report.fatal }}</div><div class="ov-label">致命</div></div>
      <div class="overview-stat ov-serious"><div class="ov-num">{{ report.serious }}</div><div class="ov-label">严重</div></div>
      <div class="overview-stat ov-medium"><div class="ov-num">{{ report.medium }}</div><div class="ov-label">中等</div></div>
      <div class="overview-stat ov-low"><div class="ov-num">{{ report.low }}</div><div class="ov-label">低</div></div>
      <div class="overview-stat ov-total"><div class="ov-num">{{ report.total_issues }}</div><div class="ov-label">总计</div></div>
      <div class="ov-summary" v-if="report.summary"><span class="ov-summary-label">总结：</span>{{ report.summary }}</div>
    </div>

    <div v-if="task.status==='running'" class="running-hint">
      <span class="spin">⏳</span> 正在深度审计中，使用 SEO 深度审计工作流 v6，覆盖 28 大类 200+ 检查点，请耐心等待…
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'

defineProps({
  task: { type: Object, default: null },
  report: { type: Object, default: null },
  rawResponse: { type: String, default: '' },
  downloading: { type: Boolean, default: false },
})
defineEmits(['download', 'copy-raw'])

const showRaw = ref(false)
</script>

<style scoped>
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel-amber { background:linear-gradient(135deg,#FFFDF8 0%,#FFF8EC 60%,#FFF2D8 100%); border-color:var(--peach); }
.task-header { display:flex; align-items:flex-start; justify-content:space-between; margin-bottom:16px; flex-wrap:wrap; gap:12px; }
.info-grid-3 { display:grid; grid-template-columns:repeat(3,1fr); gap:14px; flex:1; }
.info-item { display:flex; flex-direction:column; gap:4px; }
.info-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.06em; }
.btn-download { display:inline-flex; align-items:center; gap:7px; padding:10px 20px; background:linear-gradient(135deg,#2563EB,#3B82F6); color:#fff; border-radius:var(--r-sm); font-weight:700; font-size:13.5px; box-shadow:0 4px 14px rgba(37,99,235,.3); transition:all .18s; white-space:nowrap; }
.btn-download:hover { filter:brightness(1.1); transform:translateY(-1px); }
.report-overview { display:flex; align-items:center; gap:12px; flex-wrap:wrap; padding-top:14px; border-top:1.5px solid var(--border-light); margin-top:4px; }
.overview-stat { display:flex; flex-direction:column; align-items:center; padding:10px 16px; border-radius:var(--r-sm); min-width:64px; }
.ov-num { font-size:24px; font-weight:900; font-family:'JetBrains Mono',monospace; }
.ov-label { font-size:11px; font-weight:700; color:var(--ink-soft); margin-top:2px; }
.ov-fatal { background:#FFF0F0; } .ov-fatal .ov-num { color:#C0392B; }
.ov-serious { background:#FFF4E6; } .ov-serious .ov-num { color:#E67E22; }
.ov-medium { background:#FFFBE6; } .ov-medium .ov-num { color:#F39C12; }
.ov-low { background:#F0FFF4; } .ov-low .ov-num { color:#27AE60; }
.ov-total { background:var(--sun-light); } .ov-total .ov-num { color:var(--sun-deep); }
.ov-summary { flex:1; font-size:13px; color:var(--ink-mid); min-width:200px; }
.ov-summary-label { font-weight:700; color:var(--ink); }
.error-bar { background:var(--err-bg); border:1.5px solid var(--err-border); border-radius:var(--r-sm); padding:12px 16px; margin-top:12px; }
.error-bar .err-title { font-size:13.5px; font-weight:800; color:var(--err); margin-bottom:6px; }
.error-bar .err-body { font-size:13px; color:var(--err); line-height:1.65; white-space:pre-wrap; word-break:break-word; margin:0; font-family:inherit; }
.raw-ai-box { background:#fafafa; border:1px solid #ddd; border-radius:var(--r-sm); padding:10px 14px; margin-top:12px; font-size:13px; }
.raw-ai-head { display:flex; justify-content:space-between; align-items:center; font-weight:600; color:#555; margin-bottom:6px; }
.raw-ai-pre { max-height:520px; overflow:auto; background:#1e1e1e; color:#d4d4d4; padding:12px; border-radius:4px; font-family:Consolas,Menlo,monospace; font-size:12px; white-space:pre-wrap; word-break:break-all; margin:0; }
.raw-ai-preview { color:#888; font-family:Consolas,Menlo,monospace; font-size:12px; line-height:1.5; white-space:pre-wrap; word-break:break-all; max-height:80px; overflow:hidden; }
.btn-sm { padding:3px 10px; font-size:12px; border-radius:4px; border:1px solid #ccc; background:white; cursor:pointer; }
.running-hint { margin-top:14px; font-size:13px; color:var(--warn); display:flex; align-items:center; gap:8px; }
.spin { animation:spin 1.5s linear infinite; display:inline-block; }
@keyframes spin { to { transform:rotate(360deg); } }
.badge { display:inline-block; font-size:11px; font-weight:800; padding:3px 10px; border-radius:20px; }
.badge-ok { background:var(--ok-bg); color:var(--ok); border:1px solid var(--ok-border); }
.badge-warn { background:var(--warn-bg); color:var(--warn); border:1px solid var(--warn-border); }
.badge-err { background:var(--err-bg); color:var(--err); border:1px solid var(--err-border); }
.progress-bar { display:inline-block; height:6px; background:var(--border-light); border-radius:3px; overflow:hidden; vertical-align:middle; margin-right:6px; }
.progress-fill { height:100%; background:linear-gradient(135deg,var(--sun),var(--coral)); border-radius:3px; transition:width .3s; }
.progress-text { font-size:11px; font-weight:700; color:var(--ink-soft); }
</style>
