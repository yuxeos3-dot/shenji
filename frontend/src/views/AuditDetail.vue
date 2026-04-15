<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push(`/sites/${$route.params.id}`)" class="back-btn">← 返回站点</a>
      <span class="back-sep">·</span>
      <span class="back-url">审计 #{{ $route.params.taskId }}</span>
    </div>

    <div v-if="task" class="panel panel-amber" style="margin-bottom:18px">
      <div class="info-grid-3">
        <div class="info-item"><span class="info-label">状态</span><span class="badge" :class="task.status==='done'?'badge-ok':task.status==='running'?'badge-warn':'badge-err'">{{ task.status==='done'?'已完成':task.status==='running'?'进行中':task.status==='pending'?'等待中':'失败' }}</span></div>
        <div class="info-item"><span class="info-label">进度</span><div style="display:flex;align-items:center;gap:8px"><div class="progress-bar" style="width:120px"><div class="progress-fill" :style="{width:task.progress+'%'}"></div></div><span class="progress-text">{{ task.progress }}%</span></div></div>
        <div class="info-item"><span class="info-label">时间</span><span style="font-size:13px;color:var(--ink-mid)">{{ task.created_at }}</span></div>
      </div>
    </div>

    <div class="tab-bar">
      <button :class="['tab-btn', activeTab==='report'&&'active']" @click="activeTab='report'">📊 AI分析报告</button>
      <button :class="['tab-btn', activeTab==='pages'&&'active']" @click="activeTab='pages'">📄 页面源码</button>
    </div>

    <!-- Report Tab -->
    <div v-show="activeTab==='report'" class="panel" style="margin-top:0;border-top-left-radius:0;border-top-right-radius:0">
      <div v-if="result">
        <div v-if="result.summary" class="summary-bar">💡 {{ result.summary }}</div>

        <div class="issue-section">
          <h3>🔍 SEO问题</h3>
          <IssueList :issues="parseJSON(result.seo_issues)" />
        </div>
        <div class="issue-section">
          <h3>💻 代码问题</h3>
          <IssueList :issues="parseJSON(result.code_issues)" />
        </div>
        <div class="issue-section">
          <h3>🔒 安全问题</h3>
          <IssueList :issues="parseJSON(result.security_issues)" />
        </div>
        <div class="issue-section">
          <h3>⚡ 性能问题</h3>
          <IssueList :issues="parseJSON(result.perf_issues)" />
        </div>

        <div v-if="result.suggestions" class="issue-section">
          <h3>📝 整改建议</h3>
          <div class="suggestions-box">{{ result.suggestions }}</div>
        </div>
      </div>
      <div v-else style="text-align:center;color:var(--ink-hint);padding:40px">暂无分析结果</div>
    </div>

    <!-- Pages Tab -->
    <div v-show="activeTab==='pages'" class="panel" style="margin-top:0;border-top-left-radius:0;border-top-right-radius:0">
      <div v-if="pages.length" class="page-tabs">
        <button v-for="p in pages" :key="p.id" :class="['page-tab', activePage===p.page_type&&'active']" @click="activePage=p.page_type">{{ p.page_type }}</button>
      </div>
      <div v-for="p in pages" :key="p.id" v-show="activePage===p.page_type">
        <div class="info-grid-4" style="margin:14px 0">
          <div class="info-item"><span class="info-label">URL</span><span style="font-size:12px;word-break:break-all">{{ p.url }}</span></div>
          <div class="info-item"><span class="info-label">状态码</span><span class="badge" :class="p.status_code===200?'badge-ok':'badge-err'">{{ p.status_code }}</span></div>
          <div class="info-item"><span class="info-label">TTFB</span><span style="font-weight:700;color:var(--sun)">{{ p.ttfb_ms }}ms</span></div>
          <div class="info-item"><span class="info-label">大小</span><span style="font-weight:700">{{ (p.html_size/1024).toFixed(1) }}KB</span></div>
        </div>
        <textarea class="code-area" readonly :value="p.html" rows="20"></textarea>
      </div>
      <div v-if="!pages.length" style="text-align:center;color:var(--ink-hint);padding:40px">暂无页面数据</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import api from '../api'
import IssueList from '../components/IssueList.vue'

const route = useRoute()
const task = ref(null)
const result = ref(null)
const pages = ref([])
const activeTab = ref('report')
const activePage = ref('')

function parseJSON(str) {
  if (!str) return []
  try { const p = typeof str === 'string' ? JSON.parse(str) : str; return Array.isArray(p) ? p : [] } catch { return [] }
}

async function loadData() {
  try {
    const [auditRes, pagesRes] = await Promise.all([api.getAudit(route.params.taskId), api.getAuditPages(route.params.taskId)])
    task.value = auditRes.data.task
    result.value = auditRes.data.result
    pages.value = pagesRes.data
    if (pages.value.length) activePage.value = pages.value[0].page_type
  } catch {}
}

onMounted(loadData)
watch(() => route.params.taskId, () => { if (route.params.taskId) loadData() })
</script>

<style scoped>
.back-row { display:flex; align-items:center; gap:8px; margin-bottom:18px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); }
.back-sep { color:var(--ink-hint); }
.back-url { font-size:14px; font-weight:700; color:var(--ink); }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel-amber { background:linear-gradient(135deg,#FFFDF8 0%,#FFF8EC 60%,#FFF2D8 100%); border-color:var(--peach); }

.info-grid-3 { display:grid; grid-template-columns:repeat(3,1fr); gap:14px; }
.info-grid-4 { display:grid; grid-template-columns:2fr 1fr 1fr 1fr; gap:14px; }
.info-item { display:flex; flex-direction:column; gap:4px; }
.info-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.06em; }

.badge { display:inline-block; font-size:11px; font-weight:800; padding:2px 10px; border-radius:20px; }
.badge-ok { background:#F0FDF4; color:#16A34A; border:1px solid #BBF7D0; }
.badge-warn { background:#FFFBEB; color:#D97706; border:1px solid #FDE68A; }
.badge-err { background:#FFF5F5; color:#DC2626; border:1px solid #FECACA; }

.progress-bar { display:inline-block; height:6px; background:var(--border-light); border-radius:3px; overflow:hidden; }
.progress-fill { height:100%; background:linear-gradient(135deg,var(--sun),var(--coral)); border-radius:3px; }
.progress-text { font-size:12px; font-weight:700; color:var(--ink-soft); }

.tab-bar { display:flex; gap:0; margin-top:18px; }
.tab-btn { padding:10px 22px; font-size:13px; font-weight:700; border:1.5px solid var(--border-light); border-bottom:none; background:var(--surface-2); color:var(--ink-soft); cursor:pointer; transition:all .15s; border-radius:var(--r-sm) var(--r-sm) 0 0; font-family:inherit; }
.tab-btn.active { background:var(--surface); color:var(--sun-deep); border-color:var(--border-light); box-shadow:0 -2px 8px rgba(150,60,0,.05); }
.tab-btn:hover:not(.active) { background:var(--sun-light); color:var(--sun); }

.summary-bar { background:linear-gradient(135deg,#FFFDF8,#FFF8EC); border:1.5px solid var(--peach); border-radius:var(--r-sm); padding:14px 18px; font-size:14px; font-weight:600; color:var(--ink); margin-bottom:20px; }

.issue-section { margin-bottom:20px; }
.issue-section h3 { font-size:14px; font-weight:700; color:var(--ink); margin-bottom:10px; }

.suggestions-box { background:var(--surface-2); border:1.5px solid var(--border-light); border-radius:var(--r-sm); padding:16px; white-space:pre-wrap; font-size:13px; color:var(--ink-mid); line-height:1.8; }

.page-tabs { display:flex; gap:6px; margin-bottom:14px; flex-wrap:wrap; }
.page-tab { padding:5px 14px; font-size:12px; font-weight:700; border:1.5px solid var(--border); border-radius:20px; background:var(--surface); color:var(--ink-soft); cursor:pointer; transition:all .15s; font-family:inherit; }
.page-tab.active { background:var(--sun); border-color:var(--sun); color:#fff; box-shadow:var(--s-amber); }
.page-tab:hover:not(.active) { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); }

.code-area { width:100%; font-family:'JetBrains Mono',monospace; font-size:12px; border:1.5px solid var(--border); border-radius:var(--r-sm); padding:12px; color:var(--ink); background:var(--surface-2); resize:vertical; line-height:1.6; }
</style>
