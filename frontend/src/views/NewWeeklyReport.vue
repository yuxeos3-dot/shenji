<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push('/reports/weekly')" class="back-btn">← 返回周汇报列表</a>
    </div>

    <div class="panel">
      <div class="section-title">📅 选择汇报周</div>
      <div class="hint">周汇报会根据该时间段内所有日汇报 AI 生成草稿，生成后仍可以人工编辑再提交。</div>

      <div class="form-row">
        <div class="form-field">
          <label>周起始</label>
          <el-date-picker
            v-model="form.week_start"
            type="date"
            value-format="YYYY-MM-DD"
            placeholder="选择起始日期"
            style="width:100%"
            @change="loadPreview"
          />
        </div>
        <div class="form-field">
          <label>周结束</label>
          <el-date-picker
            v-model="form.week_end"
            type="date"
            value-format="YYYY-MM-DD"
            placeholder="选择结束日期"
            style="width:100%"
            @change="loadPreview"
          />
        </div>
        <div class="form-field" style="flex:0 0 auto">
          <label>快速选择</label>
          <el-select v-model="preset" placeholder="常用范围" style="width:170px" @change="applyPreset">
            <el-option label="本周（周一～周日）" value="thisWeekMon" />
            <el-option label="上周（周一～周日）" value="lastWeekMon" />
            <el-option label="本周（周日～周六）" value="thisWeekSun" />
            <el-option label="近 7 天" value="last7" />
          </el-select>
        </div>
      </div>

      <!-- Preview box: shows which daily reports we'd pick up if they
           click generate. Helpful for debugging "why is this week empty?"
           without forcing a round-trip to the backend. -->
      <div v-if="form.week_start && form.week_end" class="preview-box">
        <div class="preview-head">
          覆盖日期范围：<span class="mono">{{ form.week_start }} ~ {{ form.week_end }}</span>
          （共 <span class="strong">{{ dayCount }}</span> 天 · 找到 <span class="strong">{{ previewReports.length }}</span> 篇日汇报）
        </div>
        <div v-if="loadingPreview" class="preview-loading">查询中...</div>
        <div v-else-if="!previewReports.length" class="preview-empty">
          ⚠ 所选范围内没有日汇报。生成将失败——请先写日汇报，或扩大范围。
        </div>
        <div v-else class="preview-list">
          <div v-for="r in previewReports" :key="r.id" class="preview-item">
            <span class="date-chip">{{ r.report_date }}</span>
            <span class="content-peek">{{ snippet(r.content) }}</span>
          </div>
        </div>
      </div>

      <div class="action-row">
        <button
          class="btn btn-primary"
          :disabled="!canGenerate || generating"
          @click="generate"
        >
          {{ generating ? '🤖 AI 正在生成中...（最长 2 分钟）' : '🤖 生成周汇报' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'

const router = useRouter()
const form = ref({ week_start: '', week_end: '' })
const preset = ref('')
const previewReports = ref([])
const loadingPreview = ref(false)
const generating = ref(false)

const dayCount = computed(() => {
  if (!form.value.week_start || !form.value.week_end) return 0
  const s = new Date(form.value.week_start + 'T00:00:00')
  const e = new Date(form.value.week_end + 'T00:00:00')
  const diff = Math.round((e - s) / 86400000) + 1
  return diff >= 1 ? diff : 0
})

// Guard: require both dates, a valid positive span, and at least one
// daily report in the window (backend also rejects 0-daily but we
// want to block the button for a clearer UX).
const canGenerate = computed(() =>
  form.value.week_start && form.value.week_end &&
  dayCount.value > 0 && dayCount.value <= 14 &&
  previewReports.value.length > 0
)

function snippet(s) {
  if (!s) return '(空)'
  const one = s.replace(/\s+/g, ' ').trim()
  return one.length > 80 ? one.slice(0, 80) + '…' : one
}

// Apply a named preset. Dates are computed in the browser's local TZ;
// ISO week rules (Mon-Sun) use day index 1..7.
function applyPreset(v) {
  const now = new Date()
  const pad = n => String(n).padStart(2, '0')
  const toYmd = d => `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}`
  if (v === 'thisWeekMon' || v === 'lastWeekMon') {
    // Monday = 1, Sunday = 0 in getDay(). Convert to Mon-based offset
    // so Monday yields 0 and Sunday yields 6.
    const jsDay = now.getDay()
    const offset = jsDay === 0 ? 6 : jsDay - 1
    const monday = new Date(now); monday.setDate(now.getDate() - offset)
    const sunday = new Date(monday); sunday.setDate(monday.getDate() + 6)
    if (v === 'lastWeekMon') {
      monday.setDate(monday.getDate() - 7)
      sunday.setDate(sunday.getDate() - 7)
    }
    form.value.week_start = toYmd(monday)
    form.value.week_end = toYmd(sunday)
  } else if (v === 'thisWeekSun') {
    // US-style: Sunday start, Saturday end.
    const jsDay = now.getDay()
    const sunday = new Date(now); sunday.setDate(now.getDate() - jsDay)
    const saturday = new Date(sunday); saturday.setDate(sunday.getDate() + 6)
    form.value.week_start = toYmd(sunday)
    form.value.week_end = toYmd(saturday)
  } else if (v === 'last7') {
    const end = new Date(now)
    const start = new Date(now); start.setDate(now.getDate() - 6)
    form.value.week_start = toYmd(start)
    form.value.week_end = toYmd(end)
  }
  loadPreview()
}

async function loadPreview() {
  if (!form.value.week_start || !form.value.week_end) {
    previewReports.value = []
    return
  }
  loadingPreview.value = true
  try {
    // Backend's listReports supports a date_range filter via start/end
    // via params — use it here. We don't have a dedicated "by range"
    // endpoint, so we accept that this list is limited to the current
    // user's reports (exactly what the backend generate flow uses too).
    const res = await api.listReports({
      start: form.value.week_start,
      end: form.value.week_end,
    })
    previewReports.value = res.data || []
  } catch {
    previewReports.value = []
  } finally {
    loadingPreview.value = false
  }
}

async function generate() {
  if (!canGenerate.value) return
  generating.value = true
  try {
    const res = await api.generateWeeklyReport({
      week_start: form.value.week_start,
      week_end: form.value.week_end,
    })
    ElMessage.success('已生成周汇报草稿')
    const id = res.data?.id
    if (id) router.push(`/reports/weekly/${id}`)
    else router.push('/reports/weekly')
  } catch (e) {
    // Special case: 409 means a weekly report for this range already
    // exists. Backend returns the existing id in data.id; jump there
    // instead of leaving the user confused.
    const existingId = e?.response?.data?.data?.id
    if (existingId) {
      ElMessage.info('该时间段已有周汇报，直接打开编辑页')
      router.push(`/reports/weekly/${existingId}`)
    }
    // Otherwise the axios interceptor already surfaced the error.
  } finally {
    generating.value = false
  }
}
</script>

<style scoped>
.back-row { margin-bottom:12px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); text-decoration:none; }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:24px 28px; }

.section-title { font-size:15px; font-weight:800; color:var(--ink); margin-bottom:6px; }
.hint { font-size:12.5px; color:var(--ink-soft); margin-bottom:20px; line-height:1.6; padding:10px 14px; background:var(--sun-light); border-left:3px solid var(--sun); border-radius:6px; }

.form-row { display:flex; gap:14px; flex-wrap:wrap; margin-bottom:20px; }
.form-field { flex:1; min-width:200px; }
.form-field label { display:block; font-size:12px; font-weight:700; color:var(--ink-mid); margin-bottom:6px; text-transform:uppercase; letter-spacing:.05em; }

.preview-box { margin-top:10px; padding:16px 18px; background:var(--surface-2); border:1.5px dashed var(--border); border-radius:var(--r-sm); margin-bottom:20px; }
.preview-head { font-size:13px; color:var(--ink-mid); margin-bottom:10px; }
.preview-loading { font-size:13px; color:var(--ink-hint); }
.preview-empty { font-size:13px; color:var(--err); padding:10px 0; }
.preview-list { max-height:240px; overflow-y:auto; display:flex; flex-direction:column; gap:6px; }
.preview-item { display:flex; gap:10px; font-size:12.5px; padding:6px 10px; background:var(--surface); border-radius:6px; border:1px solid var(--border-light); }
.date-chip { flex:0 0 92px; font-family:'JetBrains Mono', monospace; font-weight:700; color:var(--sun-deep); }
.content-peek { color:var(--ink-mid); overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }

.action-row { display:flex; justify-content:flex-end; padding-top:6px; }
.btn { display:inline-flex; align-items:center; gap:6px; padding:11px 24px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:14px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn:disabled { opacity:.55; cursor:not-allowed; }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover:not(:disabled) { background:linear-gradient(135deg,var(--sun-deep),var(--sun)); color:#fff; }

.mono { font-family:'JetBrains Mono', monospace; }
.strong { font-weight:800; color:var(--ink); }
</style>
