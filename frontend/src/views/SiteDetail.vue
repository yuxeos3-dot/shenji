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
            <td><span class="badge" :class="a.status==='done'?'badge-ok':a.status==='running'?'badge-warn':'badge-err'">{{ a.status==='done'?'已完成':a.status==='running'?'进行中':a.status==='pending'?'等待中':'失败' }}</span></td>
            <td><div class="progress-bar"><div class="progress-fill" :style="{width:a.progress+'%'}"></div></div><span class="progress-text">{{ a.progress }}%</span></td>
            <td style="font-size:12px;color:var(--ink-soft)">{{ a.created_at }}</td>
            <td style="font-size:12px;color:var(--ink-soft)">{{ a.finished_at || '—' }}</td>
            <td><button class="btn btn-xs" @click="$router.push(`/sites/${$route.params.id}/audit/${a.id}`)">查看详情</button></td>
          </tr>
          <tr v-if="!audits.length"><td colspan="6" style="text-align:center;color:var(--ink-hint);padding:30px">暂无审计记录</td></tr>
        </tbody>
      </table>
    </div>
    <div class="panel" style="margin-top:18px">
      <div class="panel-header"><h2>⏰ 站点独立巡检计划</h2></div>
      <div class="schedule-desc">为此站点设置独立的巡检频率，设置后将覆盖全局调度，仅对本站点生效。</div>

      <div class="sched-field">
        <label class="sched-label">启用独立巡检计划</label>
        <label class="toggle-label" :class="schedEnabled?'toggle-on':'toggle-off'" @click="schedEnabled=!schedEnabled">
          <span class="toggle-switch"></span>
          <span class="toggle-text">{{ schedEnabled ? '已启用' : '未启用（跟随全局）' }}</span>
        </label>
      </div>

      <template v-if="schedEnabled">
        <div class="sched-field">
          <label class="sched-label">巡检模式</label>
          <div class="sched-mode-row">
            <div :class="['sched-mode-card', schedMode==='interval'&&'sched-mode-active']" @click="schedMode='interval'">
              <div class="smc-icon">⏱</div>
              <div class="smc-name">按间隔</div>
              <div class="smc-desc">每隔 N 小时巡查一次</div>
            </div>
            <div :class="['sched-mode-card', schedMode==='daily'&&'sched-mode-active']" @click="schedMode='daily'">
              <div class="smc-icon">🕘</div>
              <div class="smc-name">按时间</div>
              <div class="smc-desc">每天固定时间点巡查</div>
            </div>
          </div>
        </div>

        <div class="sched-field" v-if="schedMode==='interval'">
          <label class="sched-label">巡检间隔</label>
          <div class="num-row">
            <button class="num-btn" @click="intervalHours=Math.max(1,intervalHours-1)">－</button>
            <input class="num-input" type="number" v-model.number="intervalHours" min="1" max="168" />
            <button class="num-btn" @click="intervalHours=Math.min(168,intervalHours+1)">＋</button>
            <span class="num-unit">小时巡查一次</span>
          </div>
        </div>

        <div class="sched-field" v-if="schedMode==='daily'">
          <label class="sched-label">每天巡检时间</label>
          <div class="hour-grid">
            <span
              v-for="h in 24" :key="h-1"
              :class="['hour-chip', dailyHour===(h-1)&&'hour-active']"
              @click="dailyHour=h-1"
            >{{ String(h-1).padStart(2,'0') }}时</span>
          </div>
          <div class="sched-hint">当前选择：每天 {{ String(dailyHour).padStart(2,'0') }}:00 自动巡检</div>
        </div>
      </template>

      <div class="sched-actions">
        <button class="btn btn-primary" @click="saveSchedule" :disabled="savingSched">{{ savingSched ? '保存中...' : '💾 保存巡检计划' }}</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'

const route = useRoute()
const site = ref({})
const audits = ref([])
const auditing = ref(false)

// Per-site schedule
const schedEnabled = ref(false)
const schedMode = ref('daily')
const intervalHours = ref(24)
const dailyHour = ref(9)
const savingSched = ref(false)

async function loadData() {
  try {
    const [siteRes, auditsRes] = await Promise.all([api.getSite(route.params.id), api.listAudits(route.params.id)])
    site.value = siteRes.data
    audits.value = auditsRes.data
    // Load schedule settings
    const s = siteRes.data
    schedEnabled.value = s.audit_schedule_enabled === 'true'
    if (s.audit_interval_hours > 0) {
      schedMode.value = 'interval'
      intervalHours.value = s.audit_interval_hours
    } else {
      schedMode.value = 'daily'
      dailyHour.value = s.audit_daily_hour >= 0 ? s.audit_daily_hour : 9
    }
  } catch {}
}

async function saveSchedule() {
  savingSched.value = true
  try {
    const payload = {
      ...site.value,
      audit_schedule_enabled: schedEnabled.value ? 'true' : 'false',
      audit_interval_hours: schedMode.value === 'interval' ? intervalHours.value : 0,
      audit_daily_hour: schedMode.value === 'daily' ? dailyHour.value : -1,
    }
    await api.updateSite(route.params.id, payload)
    ElMessage.success('巡检计划已保存')
  } catch {}
  finally { savingSched.value = false }
}

async function triggerAudit() {
  auditing.value = true
  try { await api.triggerAudit(route.params.id); ElMessage.success('审计任务已启动'); loadData() } catch {} finally { auditing.value = false }
}

onMounted(loadData)
watch(() => route.params.id, () => { if (route.params.id) loadData() })
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

/* ── 站点独立巡检计划 ── */
.schedule-desc { font-size:13px; color:var(--ink-soft); line-height:1.6; margin-bottom:18px; padding:12px 14px; background:var(--warn-bg); border:1px solid var(--warn-border); border-radius:var(--r-sm); }
.sched-field { display:flex; flex-direction:column; gap:8px; margin-bottom:18px; }
.sched-label { font-size:13px; font-weight:600; color:var(--ink-mid); }
.sched-hint { font-size:12px; color:var(--ink-hint); }

.sched-mode-row { display:grid; grid-template-columns:1fr 1fr; gap:12px; }
.sched-mode-card { border:2px solid var(--border-light); border-radius:var(--r-sm); padding:14px 16px; cursor:pointer; text-align:center; transition:all .18s; background:var(--surface-2); }
.sched-mode-card:hover { border-color:var(--peach); background:var(--sun-light); }
.sched-mode-active { border-color:var(--sun) !important; background:var(--sun-light) !important; box-shadow:0 0 0 3px rgba(249,115,22,.1); }
.smc-icon { font-size:22px; margin-bottom:5px; }
.smc-name { font-weight:700; font-size:14px; color:var(--ink); margin-bottom:3px; }
.smc-desc { font-size:12px; color:var(--ink-soft); }

.num-row { display:flex; align-items:center; gap:10px; }
.num-btn { width:34px; height:34px; border-radius:8px; border:1.5px solid var(--border); background:var(--surface); color:var(--ink); font-size:18px; font-weight:700; cursor:pointer; display:flex; align-items:center; justify-content:center; transition:all .15s; flex-shrink:0; }
.num-btn:hover { background:var(--sun-light); border-color:var(--sun); color:var(--sun); }
.num-input { width:72px; padding:8px 10px; border:1.5px solid var(--border); border-radius:8px; font-size:14px; font-weight:700; text-align:center; font-family:inherit; outline:none; }
.num-input:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); }
.num-unit { font-size:13px; color:var(--ink-soft); white-space:nowrap; }

.hour-grid { display:flex; flex-wrap:wrap; gap:6px; }
.hour-chip { padding:5px 10px; border-radius:7px; border:1.5px solid var(--border-light); background:var(--surface-2); font-size:12px; font-weight:600; color:var(--ink-soft); cursor:pointer; transition:all .15s; font-family:'JetBrains Mono',monospace; }
.hour-chip:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun); }
.hour-active { background:var(--sun) !important; color:#fff !important; border-color:var(--sun) !important; box-shadow:0 2px 8px rgba(249,115,22,.3); }

.sched-actions { padding-top:6px; display:flex; gap:10px; }

.toggle-label { display:inline-flex; align-items:center; gap:9px; cursor:pointer; user-select:none; }
.toggle-switch { width:42px; height:24px; border-radius:12px; position:relative; flex-shrink:0; transition:background .2s; }
.toggle-on .toggle-switch { background:linear-gradient(135deg,#16A34A,#22C55E); box-shadow:0 2px 8px rgba(22,163,74,.3); }
.toggle-off .toggle-switch { background:#D1C4BA; }
.toggle-switch::after { content:''; position:absolute; top:3px; width:18px; height:18px; border-radius:50%; background:#fff; box-shadow:0 1px 4px rgba(0,0,0,.2); transition:left .2s; }
.toggle-on .toggle-switch::after { left:21px; }
.toggle-off .toggle-switch::after { left:3px; }
.toggle-text { font-weight:700; font-size:13px; }
.toggle-on .toggle-text { color:var(--ok); }
.toggle-off .toggle-text { color:var(--ink-soft); }
</style>
