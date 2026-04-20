<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push('/reports/monthly')" class="back-btn">← 返回月汇报列表</a>
    </div>

    <div class="panel">
      <div class="section-title">📅 选择汇报月份</div>
      <div class="hint">
        月汇报基于该月份内<strong>已提交</strong>的周汇报自动生成；草稿周汇报不会计入。
        如果本月还有周汇报是草稿状态，请先提交后再来生成月汇报。
      </div>

      <div class="form-row">
        <div class="form-field">
          <label>月份</label>
          <el-date-picker
            v-model="pickedMonth"
            type="month"
            value-format="YYYY-MM"
            placeholder="选择月份"
            style="width:100%"
            @change="onMonthChange"
          />
        </div>
        <div class="form-field" style="flex:0 0 auto">
          <label>快速选择</label>
          <el-select v-model="preset" placeholder="常用月份" style="width:160px" @change="applyPreset">
            <el-option label="本月" value="thisMonth" />
            <el-option label="上月" value="lastMonth" />
          </el-select>
        </div>
      </div>

      <div v-if="form.year && form.month" class="preview-box">
        <div class="preview-head">
          <span class="mono">{{ form.year }} 年 {{ form.month }} 月</span>
          （找到 <span class="strong">{{ previewReports.length }}</span> 篇<strong>已提交</strong>的周汇报）
        </div>
        <div v-if="loadingPreview" class="preview-loading">查询中...</div>
        <div v-else-if="!previewReports.length" class="preview-empty">
          ⚠ 本月没有已提交的周汇报。生成将失败——请先确认有周汇报处于「已提交」状态。
        </div>
        <div v-else class="preview-list">
          <div v-for="r in previewReports" :key="r.id" class="preview-item">
            <span class="date-chip">{{ r.week_start }} ~ {{ r.week_end }}</span>
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
          {{ generating ? '🤖 AI 正在生成中...（最长 3 分钟）' : '🤖 生成月汇报' }}
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
const pickedMonth = ref('') // "YYYY-MM" from el-date-picker
const form = ref({ year: 0, month: 0 })
const preset = ref('')
const previewReports = ref([])
const loadingPreview = ref(false)
const generating = ref(false)

const canGenerate = computed(() =>
  form.value.year > 0 && form.value.month > 0 && previewReports.value.length > 0
)

function snippet(s) {
  if (!s) return '(空)'
  const one = s.replace(/\s+/g, ' ').trim()
  return one.length > 80 ? one.slice(0, 80) + '…' : one
}

function onMonthChange(v) {
  if (!v) {
    form.value = { year: 0, month: 0 }
    previewReports.value = []
    return
  }
  // v is "YYYY-MM"
  const [y, m] = v.split('-').map(n => parseInt(n, 10))
  form.value = { year: y, month: m }
  loadPreview()
}

function applyPreset(v) {
  const now = new Date()
  let y = now.getFullYear()
  let m = now.getMonth() + 1
  if (v === 'lastMonth') {
    m -= 1
    if (m === 0) { m = 12; y -= 1 }
  }
  const pad = n => String(n).padStart(2, '0')
  pickedMonth.value = `${y}-${pad(m)}`
  form.value = { year: y, month: m }
  loadPreview()
}

async function loadPreview() {
  if (!form.value.year || !form.value.month) {
    previewReports.value = []
    return
  }
  // Compute month date range client-side so we can reuse the weekly
  // list endpoint. Then filter to 'submitted' status on the client.
  const pad = n => String(n).padStart(2, '0')
  const y = form.value.year
  const m = form.value.month
  const startStr = `${y}-${pad(m)}-01`
  // Last day of month: roll forward one month then subtract a day.
  const endDate = new Date(y, m, 0) // m is 1-based in our form, Date() month is 0-based → gives last day
  const endStr = `${endDate.getFullYear()}-${pad(endDate.getMonth() + 1)}-${pad(endDate.getDate())}`

  loadingPreview.value = true
  try {
    const res = await api.listWeeklyReports()
    const all = res.data || []
    // Filter: status=submitted AND week_start in [startStr, endStr]
    previewReports.value = all.filter(w =>
      w.status === 'submitted' &&
      w.week_start >= startStr && w.week_start <= endStr
    )
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
    const res = await api.generateMonthlyReport({
      year: form.value.year,
      month: form.value.month,
    })
    ElMessage.success('已生成月汇报草稿')
    const id = res.data?.id
    if (id) router.push(`/reports/monthly/${id}`)
    else router.push('/reports/monthly')
  } catch (e) {
    const existingId = e?.response?.data?.data?.id
    if (existingId) {
      ElMessage.info('该月份已有月汇报，直接打开编辑页')
      router.push(`/reports/monthly/${existingId}`)
    }
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
.hint strong { color:var(--sun-deep); }

.form-row { display:flex; gap:14px; flex-wrap:wrap; margin-bottom:20px; }
.form-field { flex:1; min-width:200px; }
.form-field label { display:block; font-size:12px; font-weight:700; color:var(--ink-mid); margin-bottom:6px; text-transform:uppercase; letter-spacing:.05em; }

.preview-box { margin-top:10px; padding:16px 18px; background:var(--surface-2); border:1.5px dashed var(--border); border-radius:var(--r-sm); margin-bottom:20px; }
.preview-head { font-size:13px; color:var(--ink-mid); margin-bottom:10px; }
.preview-loading { font-size:13px; color:var(--ink-hint); }
.preview-empty { font-size:13px; color:var(--err); padding:10px 0; }
.preview-list { max-height:240px; overflow-y:auto; display:flex; flex-direction:column; gap:6px; }
.preview-item { display:flex; gap:10px; font-size:12.5px; padding:6px 10px; background:var(--surface); border-radius:6px; border:1px solid var(--border-light); }
.date-chip { flex:0 0 180px; font-family:'JetBrains Mono', monospace; font-weight:700; color:var(--sun-deep); }
.content-peek { color:var(--ink-mid); overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }

.action-row { display:flex; justify-content:flex-end; padding-top:6px; }
.btn { display:inline-flex; align-items:center; gap:6px; padding:11px 24px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:14px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn:disabled { opacity:.55; cursor:not-allowed; }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover:not(:disabled) { background:linear-gradient(135deg,var(--sun-deep),var(--sun)); color:#fff; }

.mono { font-family:'JetBrains Mono', monospace; }
.strong { font-weight:800; color:var(--ink); }
</style>
