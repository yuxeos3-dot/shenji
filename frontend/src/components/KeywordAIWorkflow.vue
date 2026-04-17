<template>
  <!-- Shared component used by GeoContent.vue and ExternalLinks.vue.
       Opens as a full-screen dialog. Three phases:
       1. CONFIG    — pick word_count, writing_angle, knowledge, then start
       2. RUNNING   — poll every 2s, show step_label + spinner
       3. REVIEW    — editor for final content + meta, then apply -->
  <el-dialog
    v-model="visible"
    :title="title"
    width="960px"
    top="3vh"
    :close-on-click-modal="false"
    :close-on-press-escape="phase !== 'running'"
    @close="onClose"
  >
    <!-- PHASE: config -->
    <div v-if="phase === 'config'" class="cfg">
      <div class="kw-banner">
        <div class="kw-label">关键词</div>
        <div class="kw-text">{{ keyword }}</div>
      </div>

      <div class="form-row">
        <div class="form-field">
          <label>目标字数</label>
          <el-select v-model="form.word_count" style="width:100%">
            <el-option :value="1200" label="短文（约 1200 字）" />
            <el-option :value="1800" label="标准（约 1800 字）" />
            <el-option :value="2500" label="长文（约 2500 字）" />
            <el-option :value="3500" label="深度（约 3500 字）" />
          </el-select>
        </div>
        <div class="form-field">
          <label>写作视角</label>
          <el-select v-model="form.writing_angle" style="width:100%" clearable placeholder="（不指定）">
            <el-option value="评测" label="评测 · 第三方客观分析" />
            <el-option value="攻略" label="攻略 · 实操步骤避坑" />
            <el-option value="对比" label="对比 · 横向 2-3 家同类" />
            <el-option value="新手" label="新手入门 · 白话化讲解" />
          </el-select>
        </div>
        <div class="form-field" v-if="knowledgeOptions.length">
          <label>参考知识（可选）</label>
          <el-select v-model="form.knowledge_id" style="width:100%" clearable placeholder="（不使用）">
            <el-option v-for="k in knowledgeOptions" :key="k.id" :label="k.name" :value="k.id" />
          </el-select>
        </div>
      </div>

      <div class="hint-block">
        <div class="hint-title">🎯 本次生成的核心约束</div>
        <ul class="hint-list">
          <li>✅ 中性表达 — 即使关键词偏敏感/成人话题，也只输出信息性、行业层面的中性内容，不得露骨</li>
          <li>✅ 去 AI 化 — 禁用"首先/其次/综上所述"等模板连接词，句长交替，至少一处第二人称对话</li>
          <li>✅ 自然内链 — 从该站点已配置的页面 URL 中自动挑选 2-4 条嵌入（若无页面则跳过）</li>
        </ul>
        <div v-if="!sitePageCount" class="warn-line">
          ⚠ 当前站点还没添加页面，本次不会自动插入站内链接。建议去「站点详情 → 页面」添加主要栏目页后再来。
        </div>
      </div>

      <div class="actions">
        <el-button @click="close">取消</el-button>
        <el-button type="primary" @click="start" :loading="starting">🤖 开始 AI 写作</el-button>
      </div>
    </div>

    <!-- PHASE: running -->
    <div v-else-if="phase === 'running'" class="running">
      <div class="kw-banner">
        <div class="kw-label">正在写：</div>
        <div class="kw-text">{{ keyword }}</div>
      </div>

      <div class="progress-big">
        <div class="spinner">⏳</div>
        <div class="step-label">{{ job?.step_label || '初始化...' }}</div>
        <div class="step-sub">步骤 {{ job?.current_step || 0 }} / 2</div>
      </div>

      <div class="elapsed">已耗时 {{ elapsedSec }}s · 通常 30~90s · 最长 5 分钟超时</div>

      <div class="actions">
        <el-button @click="close">后台运行并关闭</el-button>
      </div>
      <div class="bg-hint">关闭对话框不会中断任务，你可以稍后在「我的写作任务」继续查看。</div>
    </div>

    <!-- PHASE: review -->
    <div v-else-if="phase === 'review'" class="review">
      <div class="kw-banner kw-banner-done">
        <div class="kw-label">✅ 生成完成</div>
        <div class="kw-text">{{ keyword }}</div>
        <div class="kw-meta">
          {{ wordCountOf(edit.content) }} 字 · 模型 {{ job?.ai_model || '—' }}
        </div>
      </div>

      <el-tabs v-model="reviewTab" type="border-card" class="review-tabs">
        <el-tab-pane label="📝 正文" name="content">
          <el-input
            v-model="edit.title"
            size="large"
            placeholder="文章标题"
            class="title-input"
          />
          <el-input
            v-model="edit.content"
            type="textarea"
            :rows="22"
            placeholder="正文（Markdown）"
            class="content-textarea"
          />
          <div class="stat-row">
            <span>字数 {{ wordCountOf(edit.content) }}</span>
            <span>·</span>
            <span>段落 {{ paraCountOf(edit.content) }}</span>
          </div>
        </el-tab-pane>

        <el-tab-pane label="🔎 SEO 元数据" name="meta">
          <div class="form-field">
            <label>Meta Title（50 字内）</label>
            <el-input v-model="edit.meta_title" maxlength="60" show-word-limit />
          </div>
          <div class="form-field">
            <label>Meta Description（140-160 字）</label>
            <el-input v-model="edit.meta_description" type="textarea" :rows="3" maxlength="200" show-word-limit />
          </div>
        </el-tab-pane>

        <el-tab-pane :label="'❓ FAQ (' + faqList.length + ')'" name="faq">
          <div v-if="!faqList.length" class="empty-hint">AI 未生成 FAQ</div>
          <div v-for="(f, i) in faqList" :key="i" class="faq-item">
            <div class="faq-q">Q: {{ f.q }}</div>
            <div class="faq-a">A: {{ f.a }}</div>
          </div>
        </el-tab-pane>
      </el-tabs>

      <div class="actions">
        <el-button @click="close">关闭（不应用）</el-button>
        <el-button type="primary" @click="apply" :loading="applying">✅ 应用到文章库</el-button>
      </div>
    </div>

    <!-- PHASE: failed -->
    <div v-else-if="phase === 'failed'" class="failed">
      <div class="err-big">❌ 生成失败</div>
      <pre class="err-msg">{{ job?.error_msg || '未知错误' }}</pre>
      <div class="actions">
        <el-button @click="close">关闭</el-button>
        <el-button type="primary" @click="phase = 'config'">重新配置</el-button>
      </div>
    </div>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  siteId: { type: [Number, String], required: true },
  contentType: { type: String, default: 'geo' }, // 'geo' | 'extlink'
  titleId: { type: [Number, String], default: 0 },
  keyword: { type: String, default: '' },
  titleText: { type: String, default: '' },
  knowledgeOptions: { type: Array, default: () => [] },
  sitePageCount: { type: Number, default: 0 },
})

const emit = defineEmits(['update:modelValue', 'applied'])

const visible = computed({
  get: () => props.modelValue,
  set: v => emit('update:modelValue', v),
})

const title = computed(() =>
  props.contentType === 'extlink' ? '🤖 AI 生成外链文章' : '🤖 AI 生成 GEO 文章'
)

const phase = ref('config')   // config | running | review | failed
const form = ref({ word_count: 1800, writing_angle: '', knowledge_id: null })
const starting = ref(false)
const applying = ref(false)

// Polling state
const job = ref(null)
const jobStartTs = ref(0)
const elapsedSec = ref(0)
let pollHandle = null
let elapsedHandle = null

// Review phase editable fields
const reviewTab = ref('content')
const edit = ref({ title: '', content: '', meta_title: '', meta_description: '' })

const faqList = computed(() => {
  try {
    const arr = JSON.parse(job.value?.final_faq || '[]')
    return Array.isArray(arr) ? arr : []
  } catch { return [] }
})

// Reset phase every time the dialog opens so previous runs don't leak.
watch(visible, v => {
  if (v) {
    phase.value = 'config'
    job.value = null
    form.value = { word_count: 1800, writing_angle: '', knowledge_id: null }
    edit.value = { title: '', content: '', meta_title: '', meta_description: '' }
    stopPolling()
  } else {
    stopPolling()
  }
})

function wordCountOf(s) {
  if (!s) return 0
  // Approximate: every CJK char = 1 word, every run of ASCII = 1 word.
  // Good enough for "how long is this article" at a glance.
  const cjk = (s.match(/[\u4e00-\u9fff\u3040-\u30ff]/g) || []).length
  const ascii = (s.match(/[A-Za-z0-9]+/g) || []).length
  return cjk + ascii
}
function paraCountOf(s) {
  if (!s) return 0
  return s.split(/\n\s*\n/).filter(x => x.trim().length > 0).length
}

async function start() {
  starting.value = true
  try {
    const res = await api.geoStartKeywordWorkflow({
      site_id: Number(props.siteId),
      content_type: props.contentType,
      title_id: Number(props.titleId) || 0,
      keyword: props.keyword,
      title_text: props.titleText || props.keyword,
      word_count: form.value.word_count,
      writing_angle: form.value.writing_angle,
      knowledge_id: form.value.knowledge_id || 0,
    })
    const jobId = res.data?.job_id
    if (!jobId) {
      ElMessage.error('启动失败：未获取到任务 ID')
      return
    }
    phase.value = 'running'
    jobStartTs.value = Date.now()
    elapsedSec.value = 0
    startPolling(jobId)
    startElapsedTimer()
  } catch { /* interceptor */ }
  finally { starting.value = false }
}

function startElapsedTimer() {
  stopElapsedTimer()
  elapsedHandle = setInterval(() => {
    elapsedSec.value = Math.round((Date.now() - jobStartTs.value) / 1000)
  }, 1000)
}
function stopElapsedTimer() {
  if (elapsedHandle) { clearInterval(elapsedHandle); elapsedHandle = null }
}

function startPolling(jobId) {
  stopPolling()
  const poll = async () => {
    try {
      const res = await api.geoGetWorkflowJob(jobId)
      job.value = res.data
      if (job.value.status === 'done') {
        stopPolling()
        stopElapsedTimer()
        edit.value = {
          title: job.value.title_text || props.keyword,
          content: job.value.final_content || '',
          meta_title: job.value.final_meta_title || '',
          meta_description: job.value.final_meta_desc || '',
        }
        phase.value = 'review'
      } else if (job.value.status === 'failed') {
        stopPolling()
        stopElapsedTimer()
        phase.value = 'failed'
      }
      // 5-minute hard timeout on the frontend side. Backend may still
      // be churning, but we stop showing the spinner — user can check
      // the "我的任务" list later.
      if (elapsedSec.value > 300) {
        stopPolling()
        stopElapsedTimer()
        phase.value = 'failed'
        if (job.value) job.value.error_msg = '超时（5 分钟）—— 任务可能仍在后台，请到「最近任务」查看'
      }
    } catch (e) {
      console.error('poll failed', e)
    }
  }
  pollHandle = setInterval(poll, 2000)
  poll() // fire immediately so the first step_label shows right away
}
function stopPolling() {
  if (pollHandle) { clearInterval(pollHandle); pollHandle = null }
}

async function apply() {
  applying.value = true
  try {
    const payload = {
      title: edit.value.title,
      content: edit.value.content,
      meta_title: edit.value.meta_title,
      meta_description: edit.value.meta_description,
    }
    const res = await api.geoApplyWorkflowJob(job.value.id, payload)
    ElMessage.success('已应用到文章库')
    emit('applied', { article_id: res.data?.article_id, content_type: props.contentType })
    close()
  } catch { /* interceptor */ }
  finally { applying.value = false }
}

function close() {
  emit('update:modelValue', false)
}
function onClose() {
  stopPolling()
  stopElapsedTimer()
}

onBeforeUnmount(() => {
  stopPolling()
  stopElapsedTimer()
})
</script>

<style scoped>
.cfg, .running, .review, .failed { padding:6px 2px; }

.kw-banner {
  display:flex; align-items:center; gap:14px;
  padding:14px 18px;
  background:var(--sun-light);
  border:1.5px solid var(--peach);
  border-radius:var(--r-sm);
  margin-bottom:18px;
}
.kw-banner-done { background:var(--ok-bg); border-color:var(--ok-border); }
.kw-label { font-size:11.5px; font-weight:800; color:var(--sun-deep); text-transform:uppercase; letter-spacing:.05em; }
.kw-banner-done .kw-label { color:var(--ok); }
.kw-text { font-size:17px; font-weight:800; color:var(--ink); flex:1; min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.kw-meta { font-size:12px; color:var(--ink-hint); font-family:'JetBrains Mono', monospace; }

.form-row { display:flex; gap:12px; flex-wrap:wrap; margin-bottom:18px; }
.form-field { flex:1; min-width:200px; }
.form-field label { display:block; font-size:11.5px; font-weight:700; color:var(--ink-mid); margin-bottom:6px; text-transform:uppercase; letter-spacing:.05em; }

.hint-block { padding:14px 18px; background:#F5F3FF; border:1.5px solid #DDD6FE; border-radius:var(--r-sm); margin-bottom:16px; }
.hint-title { font-size:12.5px; font-weight:800; color:#4338CA; margin-bottom:8px; }
.hint-list { margin:0; padding-left:20px; font-size:12.5px; line-height:1.8; color:var(--ink-mid); }
.warn-line { margin-top:10px; padding:8px 12px; font-size:12px; color:#92400E; background:#FEF3C7; border:1px solid #FCD34D; border-radius:6px; }

.actions { display:flex; justify-content:flex-end; gap:10px; margin-top:18px; }

.running { text-align:center; padding:20px; }
.progress-big { padding:40px 20px; }
.spinner { font-size:52px; animation:spin 2s linear infinite; display:inline-block; }
@keyframes spin { from { transform:rotate(0deg); } to { transform:rotate(360deg); } }
.step-label { font-size:16px; font-weight:800; color:var(--ink); margin-top:14px; }
.step-sub { font-size:12px; color:var(--ink-hint); margin-top:4px; font-family:'JetBrains Mono', monospace; }
.elapsed { font-size:13px; color:var(--ink-hint); margin:14px 0 0; font-family:'JetBrains Mono', monospace; }
.bg-hint { text-align:center; font-size:12px; color:var(--ink-hint); padding-top:10px; font-style:italic; }

.review-tabs { margin-bottom:16px; }
.title-input { margin-bottom:10px; }
.content-textarea { font-family:'JetBrains Mono', monospace; font-size:13px; }
.content-textarea :deep(textarea) { line-height:1.75; background:#FFFDF9; }
.stat-row { display:flex; gap:10px; font-size:11.5px; color:var(--ink-hint); padding-top:8px; font-family:'JetBrains Mono', monospace; }

.faq-item { padding:12px 14px; margin-bottom:10px; background:var(--surface-2); border-left:3px solid var(--sun); border-radius:var(--r-sm); }
.faq-q { font-weight:800; font-size:13.5px; color:var(--ink); margin-bottom:6px; }
.faq-a { font-size:13px; color:var(--ink-mid); line-height:1.7; }
.empty-hint { padding:20px; text-align:center; color:var(--ink-hint); }

.failed { text-align:center; padding:20px; }
.err-big { font-size:20px; font-weight:800; color:var(--err); margin-bottom:16px; }
.err-msg { padding:14px 16px; background:#FEF2F2; border:1px solid #FECACA; border-radius:var(--r-sm); font-family:'JetBrains Mono', monospace; font-size:12.5px; color:#7F1D1D; white-space:pre-wrap; text-align:left; max-height:200px; overflow:auto; }
</style>
