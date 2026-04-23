<template>
  <!-- 4-step AI generation wizard used by both GEO and 外链 flows.
       Props.contentType is "geo" | "extlink" — same backend, just
       different content_type on the workflow job. -->
  <el-dialog
    v-model="visible"
    :title="wizardTitle"
    width="880px"
    top="3vh"
    :close-on-click-modal="false"
    :close-on-press-escape="phase !== 'running'"
    @close="onClose"
  >
    <!-- Step indicator -->
    <div class="step-bar">
      <div
        v-for="(s, i) in stepLabels"
        :key="s"
        class="step-pill"
        :class="{ active: step === i + 1, done: step > i + 1 }"
      >
        <span class="step-num">{{ i + 1 }}</span>
        <span class="step-name">{{ s }}</span>
      </div>
    </div>

    <!-- ─── Step 1: 基础信息 ─── -->
    <div v-show="step === 1" class="wiz-body">
      <div class="kw-banner">
        <div class="kw-label">关键词 / 话题</div>
        <div class="kw-text">{{ form.keyword }}</div>
      </div>
      <div class="form-grid">
        <div class="form-field">
          <label>写作语言</label>
          <el-select v-model="form.language" style="width:100%">
            <el-option v-for="o in defaults.languages" :key="o.value" :value="o.value" :label="o.label" />
          </el-select>
        </div>
        <div class="form-field">
          <label>内容类型</label>
          <el-select v-model="form.detail_type" style="width:100%">
            <el-option v-for="o in defaults.detail_types" :key="o.value" :value="o.value" :label="o.label" />
          </el-select>
        </div>
        <div class="form-field full">
          <label>建议标题（可选）</label>
          <el-input v-model="form.title_text" placeholder="留空则由 AI 基于关键词生成" />
        </div>
        <div class="form-field full">
          <label>参考链接（可选）</label>
          <el-input v-model="form.reference_url" placeholder="https://... — 作为灵感来源，AI 不会整段复制" />
        </div>
      </div>
    </div>

    <!-- ─── Step 2: 生成要求 ─── -->
    <div v-show="step === 2" class="wiz-body">
      <div class="form-grid">
        <div class="form-field">
          <label>目标字数</label>
          <el-input-number v-model="form.word_count" :min="800" :max="5000" :step="100" style="width:100%" />
        </div>
        <div class="form-field">
          <label>行文风格</label>
          <el-select v-model="form.style" style="width:100%">
            <el-option v-for="o in defaults.styles" :key="o.value" :value="o.value" :label="o.label" />
          </el-select>
        </div>
        <div class="form-field">
          <label>输出格式</label>
          <el-select v-model="form.output_format" style="width:100%">
            <el-option v-for="o in defaults.formats" :key="o.value" :value="o.value" :label="o.label" />
          </el-select>
        </div>
        <div class="form-field full">
          <label>段落结构建议（可选）</label>
          <el-input
            v-model="form.paragraph_struct"
            type="textarea"
            :rows="2"
            placeholder="例如：先定义 → 再讲历史 → 然后给数据 → 最后 FAQ"
          />
        </div>
        <div class="form-field full">
          <label>附加要求（可选）</label>
          <el-input
            v-model="form.extra_requirements"
            type="textarea"
            :rows="3"
            placeholder="例如：插入至少一张表格、引用来源标注年份、结尾必须有 8 条 FAQ"
          />
        </div>
      </div>
    </div>

    <!-- ─── Step 3: Prompt ─── -->
    <div v-show="step === 3" class="wiz-body">
      <div v-if="promptNotice" class="prompt-notice" style="background:#fef3c7;border:1.5px solid #f59e0b;color:#92400e;padding:12px 16px;border-radius:8px;margin-bottom:12px;font-size:13px;line-height:1.6">
        {{ promptNotice }}
      </div>
      <div v-else class="prompt-hint">
        下面是系统内置的默认 Prompt，你可以直接使用，也可以整段修改。
        可用变量：
        <code v-for="v in promptVars" :key="v.name">{{ v.name }}</code>
      </div>
      <el-input
        v-model="form.custom_prompt"
        :disabled="promptReadonly"
        type="textarea"
        :rows="16"
        placeholder="加载中..."
      />
      <div class="prompt-actions" v-if="!promptReadonly">
        <el-button size="small" @click="resetPrompt">↺ 恢复为默认</el-button>
        <el-button size="small" @click="form.custom_prompt = ''">清空（生成时走默认）</el-button>
      </div>
    </div>

    <!-- ─── Step 4: 生成预览 ─── -->
    <div v-show="step === 4" class="wiz-body">
      <div v-if="phase === 'ready'" class="ready-block">
        <div class="ready-summary">
          <div><b>关键词：</b>{{ form.keyword }}</div>
          <div><b>字数：</b>约 {{ form.word_count }} 字</div>
          <div><b>风格：</b>{{ labelOf(defaults.styles, form.style) }} · <b>格式：</b>{{ labelOf(defaults.formats, form.output_format) }}</div>
          <div><b>语言：</b>{{ labelOf(defaults.languages, form.language) }} · <b>类型：</b>{{ labelOf(defaults.detail_types, form.detail_type) }}</div>
          <div v-if="form.custom_prompt"><b>Prompt：</b>已自定义</div>
          <div v-else><b>Prompt：</b>使用内置默认</div>
        </div>
        <div class="ready-cta">
          <el-button type="primary" size="large" @click="startGenerate">🚀 开始生成</el-button>
        </div>
      </div>

      <div v-else-if="phase === 'running'" class="running-block">
        <el-progress type="circle" :percentage="progressPct" :width="90" />
        <div class="running-label">{{ stepLabel || '⏳ 正在准备...' }}</div>
        <div class="running-sub">大约需要 30 秒 – 2 分钟。关掉弹窗不会中断任务，稍后在「文章审核」里还能找到草稿。</div>
      </div>

      <div v-else-if="phase === 'done'" class="done-block">
        <div class="meta-row">
          <div class="meta-field">
            <label>Meta Title</label>
            <el-input v-model="result.meta_title" />
          </div>
          <div class="meta-field">
            <label>Meta Description</label>
            <el-input v-model="result.meta_desc" type="textarea" :rows="2" />
          </div>
        </div>
        <div class="meta-field">
          <label>最终标题</label>
          <el-input v-model="result.title" />
        </div>
        <div class="meta-field">
          <label>正文预览（可以在这里直接改）</label>
          <el-input v-model="result.content" type="textarea" :rows="14" />
        </div>
        <div class="done-actions">
          <el-button type="primary" size="large" @click="applyToReview">✅ 加入审核库</el-button>
          <el-button size="large" @click="restart">🔄 重新生成</el-button>
          <el-button size="large" type="danger" plain @click="discard">🗑 放弃</el-button>
        </div>
      </div>

      <div v-else-if="phase === 'error'" class="err-block">
        <div class="err-title">❌ 生成失败</div>
        <div class="err-msg">{{ errorMsg }}</div>
        <div class="done-actions">
          <el-button type="primary" @click="restart">🔄 重试</el-button>
          <el-button @click="discard">关闭</el-button>
        </div>
      </div>
    </div>

    <!-- Footer nav -->
    <template #footer>
      <div class="wiz-footer">
        <el-button
          v-if="step > 1 && phase !== 'running' && phase !== 'done'"
          @click="step -= 1"
        >← 上一步</el-button>
        <span style="flex:1"></span>
        <el-button v-if="phase !== 'done' && phase !== 'running'" @click="onClose">取消</el-button>
        <el-button
          v-if="step < 4"
          type="primary"
          @click="next"
        >下一步 →</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const props = defineProps({
  // v-model:visible
  modelValue: { type: Boolean, default: false },
  siteId: { type: [Number, String], default: 0 },
  contentType: { type: String, default: 'geo' }, // 'geo' | 'extlink'
  titleRow: { type: Object, default: () => null }, // { id, title, keyword } — from lib
})
const emit = defineEmits(['update:modelValue', 'applied'])

const visible = computed({
  get: () => props.modelValue,
  set: v => emit('update:modelValue', v),
})

const wizardTitle = computed(() =>
  (props.contentType === 'extlink' ? '外链 AI 生成' : 'GEO AI 生成') + ' · ' + (props.titleRow?.title || ''))
const stepLabels = ['基础信息', '生成要求', 'Prompt 模板', '生成 & 预览']

const step = ref(1)
const phase = ref('ready') // ready → running → done | error
const form = ref({
  keyword: '',
  title_text: '',
  language: 'zh',
  detail_type: 'article',
  style: 'professional',
  output_format: 'markdown',
  word_count: 1500,
  paragraph_struct: '',
  extra_requirements: '',
  reference_url: '',
  custom_prompt: '',
})
const defaults = ref({
  languages: [], detail_types: [], styles: [], formats: [],
  prompt: '', default_word_count: 1500,
})
const defaultPrompt = ref('')
const promptReadonly = ref(false)
const promptNotice = ref('')
// Static so the template can render braces without Vue trying to
// interpret {{ }} inside the mustaches as nested interpolation.
const promptVars = [
  { name: '{{keyword}}' }, { name: '{{title}}' }, { name: '{{wordcount}}' },
  { name: '{{style}}' }, { name: '{{format}}' }, { name: '{{lang}}' },
  { name: '{{type}}' }, { name: '{{extra}}' }, { name: '{{paragraph}}' },
  { name: '{{reference}}' }, { name: '{{knowledge}}' }, { name: '{{pages}}' },
  { name: '{{year}}' },
]
const jobId = ref(null)
const stepLabel = ref('')
const progressPct = ref(0)
const pollTimer = ref(null)
const errorMsg = ref('')
const result = ref({ title: '', content: '', meta_title: '', meta_desc: '' })

// ─── Lifecycle ─────────────────────────────────────────────────────────

watch(() => props.modelValue, async v => {
  if (!v) return
  await resetState()
})

async function resetState() {
  step.value = 1
  phase.value = 'ready'
  jobId.value = null
  stepLabel.value = ''
  progressPct.value = 0
  errorMsg.value = ''
  result.value = { title: '', content: '', meta_title: '', meta_desc: '' }
  form.value = {
    keyword: props.titleRow?.keyword || props.titleRow?.title || '',
    title_text: props.titleRow?.title || '',
    language: 'zh',
    detail_type: 'article',
    style: 'professional',
    output_format: 'markdown',
    word_count: 1500,
    paragraph_struct: '',
    extra_requirements: '',
    reference_url: '',
    custom_prompt: '',
  }
  await loadDefaults()
}

// loadDefaults fetches wizard defaults (prompt template + options) from the server.
// Extracted so it can be called both on initial mount AND when user changes
// detail_type (news type has a different prompt than article).
async function loadDefaults() {
  try {
    const res = await api.geoGetDefaults(props.contentType, form.value.detail_type)
    defaults.value = res.data
    defaultPrompt.value = res.data.prompt || ''
    promptReadonly.value = !!res.data.prompt_readonly
    promptNotice.value = res.data.prompt_notice || ''
    form.value.custom_prompt = defaultPrompt.value
    if (res.data.default_word_count) form.value.word_count = res.data.default_word_count
  } catch (e) {
    console.error('load defaults', e)
    ElMessage.error('加载默认 Prompt 失败')
  }
}

// Re-fetch defaults when detail_type changes — news type has a different prompt template
watch(() => form.value.detail_type, (newType, oldType) => {
  if (oldType !== undefined && newType !== oldType) {
    loadDefaults()
  }
})

onBeforeUnmount(stopPoll)

function labelOf(list, v) {
  const o = (list || []).find(x => x.value === v)
  return o?.label || v
}

function resetPrompt() {
  form.value.custom_prompt = defaultPrompt.value
  ElMessage.success('已恢复默认 Prompt')
}

function next() {
  if (step.value < 4) step.value += 1
}

// ─── Generation ────────────────────────────────────────────────────────

async function startGenerate() {
  if (!props.siteId) {
    ElMessage.warning('请先选择站点')
    return
  }
  phase.value = 'running'
  stepLabel.value = '⏳ 提交任务...'
  progressPct.value = 5
  try {
    // If the textarea still matches the default verbatim, send empty
    // custom_prompt so server uses its structured wizard prompt with
    // live style/format/language mixing. User edits sent as-is.
    const custom = (form.value.custom_prompt || '').trim() === (defaultPrompt.value || '').trim()
      ? ''
      : form.value.custom_prompt
    const res = await api.geoStartKeywordWorkflow({
      site_id: Number(props.siteId),
      content_type: props.contentType,
      title_id: props.titleRow?.id || 0,
      keyword: form.value.keyword,
      title_text: form.value.title_text || form.value.keyword,
      word_count: form.value.word_count,
      writing_angle: '', // retained for backend compat; new style/detail_type drive the prompt now
      knowledge_id: 0,
      detail_type: form.value.detail_type,
      language: form.value.language,
      style: form.value.style,
      output_format: form.value.output_format,
      paragraph_struct: form.value.paragraph_struct,
      extra_requirements: form.value.extra_requirements,
      reference_url: form.value.reference_url,
      custom_prompt: custom,
    })
    jobId.value = res.data.job_id
    startPoll()
  } catch (e) {
    phase.value = 'error'
    errorMsg.value = e?.response?.data?.error || e.message || '提交失败'
  }
}

function startPoll() {
  stopPoll()
  pollTimer.value = setInterval(pollOnce, 2500)
  pollOnce()
}

function stopPoll() {
  if (pollTimer.value) {
    clearInterval(pollTimer.value)
    pollTimer.value = null
  }
}

async function pollOnce() {
  if (!jobId.value) return
  try {
    const res = await api.geoGetWorkflowJob(jobId.value)
    const j = res.data
    stepLabel.value = j.step_label || ''
    // rough progress heuristic: step 1 ≈ 55%, step 2 ≈ 90%, done = 100
    progressPct.value = j.current_step === 3 ? 92 : (j.current_step === 2 ? 78 : (j.current_step === 1 ? 55 : 15))
    if (j.status === 'done') {
      stopPoll()
      progressPct.value = 100
      phase.value = 'done'
      result.value = {
        title: j.title_text || form.value.keyword,
        content: j.final_content || '',
        meta_title: j.final_meta_title || '',
        meta_desc: j.final_meta_desc || '',
      }
    } else if (j.status === 'failed') {
      stopPoll()
      phase.value = 'error'
      errorMsg.value = j.error_msg || '生成失败'
    }
  } catch (e) {
    console.error('poll', e)
  }
}

async function applyToReview() {
  if (!jobId.value) return
  try {
    await api.geoApplyWorkflowJob(jobId.value, {
      title: result.value.title,
      content: result.value.content,
      meta_title: result.value.meta_title,
      meta_description: result.value.meta_desc,
    })
    ElMessage.success('✅ 已加入审核库，请到「文章审核」tab 查看')
    emit('applied')
    visible.value = false
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '加入审核失败')
  }
}

async function restart() {
  if (jobId.value) {
    try { await api.geoDiscardWorkflowJob(jobId.value) } catch (e) { /* best effort */ }
  }
  jobId.value = null
  phase.value = 'ready'
  step.value = 2 // back to "生成要求" so user can tweak before regenerating
}

async function discard() {
  if (jobId.value) {
    try { await api.geoDiscardWorkflowJob(jobId.value) } catch (e) { /* best effort */ }
  }
  onClose()
}

function onClose() {
  stopPoll()
  visible.value = false
}
</script>

<style scoped>
.step-bar {
  display: flex;
  gap: 8px;
  margin: 0 -18px 16px;
  padding: 8px 18px 16px;
  border-bottom: 1.5px solid var(--border-light);
}
.step-pill {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  border: 1.5px solid var(--border);
  background: var(--surface-3);
  border-radius: 10px;
  font-size: 12px;
  font-weight: 700;
  color: var(--ink-hint);
}
.step-pill.active {
  background: linear-gradient(135deg, var(--sun-light), var(--surface));
  border-color: var(--sun);
  color: var(--sun-deep);
  box-shadow: 0 2px 6px rgba(249, 115, 22, 0.12);
}
.step-pill.done {
  background: var(--ok-bg);
  border-color: var(--ok-border);
  color: var(--ok);
}
.step-num {
  width: 22px; height: 22px;
  border-radius: 50%;
  background: var(--surface);
  border: 1.5px solid currentColor;
  display: inline-flex; align-items: center; justify-content: center;
  font-size: 11px;
}
.wiz-body { min-height: 360px; padding: 4px 2px; }
.kw-banner {
  background: linear-gradient(135deg, var(--sun-light), var(--surface));
  border: 1.5px solid var(--peach);
  border-radius: 10px;
  padding: 14px 18px;
  margin-bottom: 16px;
  display: flex;
  align-items: baseline;
  gap: 14px;
}
.kw-label { font-size: 11px; font-weight: 800; color: var(--sun-deep); text-transform: uppercase; letter-spacing: .08em; }
.kw-text { font-size: 18px; font-weight: 800; color: var(--ink); }
.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px 16px;
}
.form-field { display: flex; flex-direction: column; gap: 6px; }
.form-field.full { grid-column: span 2; }
.form-field label { font-size: 12px; font-weight: 700; color: var(--ink-mid); }
.prompt-hint {
  font-size: 12px; color: var(--ink-soft); line-height: 1.7;
  margin-bottom: 8px;
}
.prompt-hint code {
  background: var(--sun-light);
  padding: 1px 6px; border-radius: 4px;
  color: var(--sun-deep); font-size: 11px;
  margin: 0 2px;
}
.prompt-actions { margin-top: 8px; display: flex; gap: 8px; }
.ready-block {
  background: var(--surface-3);
  border: 1.5px dashed var(--border);
  border-radius: 12px;
  padding: 22px;
}
.ready-summary > div { padding: 4px 0; color: var(--ink-mid); font-size: 13px; }
.ready-cta { display: flex; justify-content: center; margin-top: 16px; }
.running-block { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 40px 0; }
.running-label { font-size: 14px; font-weight: 700; color: var(--ink); }
.running-sub { font-size: 12px; color: var(--ink-hint); text-align: center; max-width: 420px; }
.done-block { display: flex; flex-direction: column; gap: 12px; }
.meta-row { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.meta-field { display: flex; flex-direction: column; gap: 4px; }
.meta-field label { font-size: 11px; font-weight: 800; color: var(--ink-mid); text-transform: uppercase; letter-spacing: .06em; }
.done-actions { display: flex; gap: 10px; margin-top: 8px; justify-content: flex-end; }
.err-block { padding: 30px; text-align: center; }
.err-title { font-size: 18px; font-weight: 700; color: var(--err); margin-bottom: 10px; }
.err-msg { font-size: 13px; color: var(--ink-mid); margin-bottom: 18px; white-space: pre-wrap; }
.wiz-footer { display: flex; align-items: center; gap: 8px; }
</style>
