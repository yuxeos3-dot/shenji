<template>
  <div class="panel" style="margin-top:18px">
    <div class="panel-header"><h2>站点独立巡检计划</h2></div>
    <div class="schedule-desc">为此站点设置独立的巡检频率，设置后将覆盖全局调度，仅对本站点生效。</div>

    <div class="sched-field">
      <label class="sched-label">启用独立巡检计划</label>
      <label class="toggle-label" :class="enabled?'toggle-on':'toggle-off'" @click="enabled=!enabled">
        <span class="toggle-switch"></span>
        <span class="toggle-text">{{ enabled ? '已启用' : '未启用（跟随全局）' }}</span>
      </label>
    </div>

    <template v-if="enabled">
      <div class="sched-field">
        <label class="sched-label">巡检模式</label>
        <div class="sched-mode-row">
          <div :class="['sched-mode-card', mode==='interval'&&'sched-mode-active']" @click="mode='interval'">
            <div class="smc-name">按间隔</div>
            <div class="smc-desc">每隔 N 小时巡查一次</div>
          </div>
          <div :class="['sched-mode-card', mode==='daily'&&'sched-mode-active']" @click="mode='daily'">
            <div class="smc-name">按时间</div>
            <div class="smc-desc">每天固定时间点巡查</div>
          </div>
        </div>
      </div>

      <div class="sched-field" v-if="mode==='interval'">
        <label class="sched-label">巡检间隔</label>
        <div class="num-row">
          <button class="num-btn" @click="hours=Math.max(1,hours-1)">－</button>
          <input class="num-input" type="number" v-model.number="hours" min="1" max="168" />
          <button class="num-btn" @click="hours=Math.min(168,hours+1)">＋</button>
          <span class="num-unit">小时巡查一次</span>
        </div>
      </div>

      <div class="sched-field" v-if="mode==='daily'">
        <label class="sched-label">每天巡检时间</label>
        <div class="hour-grid">
          <span
            v-for="h in 24" :key="h-1"
            :class="['hour-chip', hour===(h-1)&&'hour-active']"
            @click="hour=h-1"
          >{{ String(h-1).padStart(2,'0') }}时</span>
        </div>
        <div class="sched-hint">当前选择：每天 {{ String(hour).padStart(2,'0') }}:00 自动巡检</div>
      </div>
    </template>

    <div class="sched-actions">
      <button class="btn btn-primary" @click="save" :disabled="saving">{{ saving ? '保存中...' : '保存巡检计划' }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps({
  schedEnabled: { type: Boolean, default: false },
  schedMode: { type: String, default: 'daily' },
  intervalHours: { type: Number, default: 24 },
  dailyHour: { type: Number, default: 9 },
  saving: { type: Boolean, default: false },
})
const emit = defineEmits(['save'])

const enabled = ref(props.schedEnabled)
const mode = ref(props.schedMode)
const hours = ref(props.intervalHours)
const hour = ref(props.dailyHour)

watch(() => props.schedEnabled, v => enabled.value = v)
watch(() => props.schedMode, v => mode.value = v)
watch(() => props.intervalHours, v => hours.value = v)
watch(() => props.dailyHour, v => hour.value = v)

function save() {
  emit('save', {
    enabled: enabled.value,
    mode: mode.value,
    intervalHours: hours.value,
    dailyHour: hour.value,
  })
}
</script>

<style scoped>
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:18px; }
.panel-header h2 { font-size:15px; font-weight:700; }
.schedule-desc { font-size:13px; color:var(--ink-soft); line-height:1.6; margin-bottom:18px; padding:12px 14px; background:var(--warn-bg); border:1px solid var(--warn-border); border-radius:var(--r-sm); }
.sched-field { display:flex; flex-direction:column; gap:8px; margin-bottom:18px; }
.sched-label { font-size:13px; font-weight:600; color:var(--ink-mid); }
.sched-hint { font-size:12px; color:var(--ink-hint); }
.sched-mode-row { display:grid; grid-template-columns:1fr 1fr; gap:12px; }
.sched-mode-card { border:2px solid var(--border-light); border-radius:var(--r-sm); padding:14px 16px; cursor:pointer; text-align:center; transition:all .18s; background:var(--surface-2); }
.sched-mode-card:hover { border-color:var(--peach); background:var(--sun-light); }
.sched-mode-active { border-color:var(--sun) !important; background:var(--sun-light) !important; box-shadow:0 0 0 3px rgba(249,115,22,.1); }
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
.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-primary { background:linear-gradient(135deg,var(--sun) 0%,var(--coral) 100%); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep) 0%,var(--sun) 100%); color:#fff; }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; transform:none; }
</style>
