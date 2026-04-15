<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push('/sites')" class="back-btn">← 返回站点列表</a>
      <span class="back-sep">·</span>
      <span class="back-url">{{ site.name }} ({{ site.domain }})</span>
    </div>

    <div class="panel panel-amber" style="margin-bottom:18px">
      <div class="panel-header">
        <h2>📋 站点信息</h2>
        <button class="btn btn-primary" @click="triggerAudit" :disabled="auditing">
          {{ auditing ? '审计中...' : '🔍 立即审计' }}
        </button>
      </div>
      <div class="info-grid">
        <div class="info-item"><span class="info-label">域名</span><span class="info-val">{{ site.domain }}</span></div>
        <div class="info-item"><span class="info-label">名称</span><span class="info-val">{{ site.name }}</span></div>
        <div class="info-item"><span class="info-label">GSC URL</span><span class="info-val">{{ site.gsc_site_url || '—' }}</span></div>
        <div class="info-item"><span class="info-label">GA Property</span><span class="info-val">{{ site.ga_property_id || '—' }}</span></div>
        <div class="info-item"><span class="info-label">Yandex Host</span><span class="info-val">{{ site.yandex_host_id || '—' }}</span></div>
        <div class="info-item"><span class="info-label">状态</span><span class="info-val"><span class="badge badge-ok">{{ site.status }}</span></span></div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-header"><h2>📊 审计历史</h2></div>
      <table class="table">
        <thead><tr><th>ID</th><th>状态</th><th>进度</th><th>开始时间</th><th>完成时间</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="a in audits" :key="a.id">
            <td style="font-weight:800;color:var(--sun)">#{{ a.id }}</td>
            <td><span class="badge" :class="a.status==='done'?'badge-ok':a.status==='running'?'badge-warn':'badge-err'">{{ a.status }}</span></td>
            <td><div class="progress-bar"><div class="progress-fill" :style="{width:a.progress+'%'}"></div></div><span class="progress-text">{{ a.progress }}%</span></td>
            <td style="font-size:12px;color:var(--ink-soft)">{{ a.created_at }}</td>
            <td style="font-size:12px;color:var(--ink-soft)">{{ a.finished_at || '—' }}</td>
            <td><button class="btn btn-xs" @click="$router.push(`/sites/${$route.params.id}/audit/${a.id}`)">查看详情</button></td>
          </tr>
          <tr v-if="!audits.length"><td colspan="6" style="text-align:center;color:var(--ink-hint);padding:30px">暂无审计记录</td></tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'

const route = useRoute()
const site = ref({})
const audits = ref([])
const auditing = ref(false)

async function loadData() {
  try {
    const [siteRes, auditsRes] = await Promise.all([api.getSite(route.params.id), api.listAudits(route.params.id)])
    site.value = siteRes.data
    audits.value = auditsRes.data
  } catch {}
}

async function triggerAudit() {
  auditing.value = true
  try { await api.triggerAudit(route.params.id); ElMessage.success('审计任务已启动'); loadData() } catch {} finally { auditing.value = false }
}

onMounted(loadData)
</script>

<style scoped>
.back-row { display:flex; align-items:center; gap:8px; margin-bottom:18px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); }
.back-btn:hover { color:var(--sun-deep); }
.back-sep { color:var(--ink-hint); }
.back-url { font-size:14px; font-weight:700; color:var(--ink); }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel + .panel { margin-top:18px; }
.panel-amber { background:linear-gradient(135deg,#FFFDF8 0%,#FFF8EC 60%,#FFF2D8 100%); border-color:var(--peach); }
.panel-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:18px; }
.panel-header h2 { font-size:15px; font-weight:700; }

.info-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:14px; }
.info-item { display:flex; flex-direction:column; gap:4px; }
.info-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.06em; }
.info-val { font-size:14px; font-weight:600; color:var(--ink); }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); transform:translateY(-1px); box-shadow:var(--s1); }
.btn-primary { background:linear-gradient(135deg,var(--sun) 0%,var(--coral) 100%); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep) 0%,var(--sun) 100%); color:#fff; box-shadow:0 8px 24px rgba(249,115,22,.48); }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; transform:none; }
.btn-xs { padding:4px 12px; font-size:12px; border-radius:6px; }
.btn-xs:hover { background:var(--sun-glow); border-color:var(--sun); color:var(--sun-deep); }

.table { width:100%; border-collapse:collapse; }
.table th { padding:10px 14px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.08em; border-bottom:2px solid var(--border); background:var(--surface-3); }
.table td { padding:10px 14px; border-bottom:1px solid var(--border-light); font-size:13px; }
.table tr:hover td { background:var(--sun-light); }

.badge { display:inline-block; font-size:11px; font-weight:800; padding:2px 10px; border-radius:20px; text-transform:uppercase; }
.badge-ok { background:var(--ok-bg); color:var(--ok); border:1px solid var(--ok-border); }
.badge-warn { background:var(--warn-bg); color:var(--warn); border:1px solid var(--warn-border); }
.badge-err { background:var(--err-bg); color:var(--err); border:1px solid var(--err-border); }

.progress-bar { display:inline-block; width:80px; height:6px; background:var(--border-light); border-radius:3px; overflow:hidden; vertical-align:middle; margin-right:6px; }
.progress-fill { height:100%; background:linear-gradient(135deg,var(--sun),var(--coral)); border-radius:3px; transition:width .3s; }
.progress-text { font-size:11px; font-weight:700; color:var(--ink-soft); }
</style>
