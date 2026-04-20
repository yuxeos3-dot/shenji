<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push('/reports/weekly')" class="back-btn">← 返回周汇报列表</a>
    </div>

    <div v-if="loading && !task" class="panel" style="text-align:center;padding:60px">加载中...</div>

    <template v-else-if="task">
      <div class="panel header-panel">
        <div class="header-row">
          <div class="header-left">
            <div class="title-line">
              <span class="date-badge">{{ task.week_start }} ~ {{ task.week_end }}</span>
              <span :class="['status-badge', 'sb-' + task.status]">
                {{ task.status === 'submitted' ? '✓ 已提交' : '草稿' }}
              </span>
            </div>
            <div class="meta-line">
              <span class="meta-item">
                <span class="meta-label">作者</span>
                {{ task.user_name }}
                <span v-if="task.department_name" class="meta-sub">· {{ task.department_name }}</span>
                <span v-if="task.group_name" class="meta-sub">/ {{ task.group_name }}</span>
              </span>
              <span class="meta-item">
                <span class="meta-label">源日汇报</span>
                {{ sourceCount }} 篇
              </span>
              <span class="meta-item" v-if="task.submitted_at">
                <span class="meta-label">提交时间</span>
                {{ task.submitted_at }}
              </span>
            </div>
          </div>
        </div>

        <!-- Action row: based on status. 'draft' shows edit controls;
             'submitted' shows a locked banner. -->
        <div v-if="!isLocked && isOwner" class="action-row">
          <button class="btn btn-primary" @click="save" :disabled="!dirty || sending">
            {{ sending ? '保存中...' : '💾 保存草稿' }}
          </button>
          <button class="btn btn-polish" @click="polish" :disabled="polishing">
            {{ polishing ? '🤖 润色中...' : '🤖 AI 重新润色' }}
          </button>
          <button class="btn btn-submit" @click="submit" :disabled="submitting">
            {{ submitting ? '提交中...' : '✅ 提交周汇报' }}
          </button>
          <button class="btn btn-danger-soft" @click="remove">🗑 删除草稿</button>
        </div>
        <div v-else-if="isLocked" class="locked-banner">
          🔒 周汇报已提交，内容已锁定（留痕）
        </div>
      </div>

      <!-- Main editor. In draft mode: textarea with monospace font for
           comfortable markdown editing. Submitted mode: read-only
           pre-rendered view. -->
      <div class="panel editor-panel">
        <div class="section-title">
          📝 汇报内容
          <span v-if="dirty && !isLocked" class="dirty-tag">未保存</span>
        </div>
        <textarea
          v-if="!isLocked && isOwner"
          v-model="content"
          class="editor-textarea"
          rows="28"
          placeholder="汇报内容（Markdown）"
          @input="onInput"
        ></textarea>
        <pre v-else class="content-readonly">{{ task.content || '(空)' }}</pre>
      </div>

      <!-- Evaluations panel: admin writes, author reads. Shown in
           both draft and submitted states so managers can leave
           feedback while the draft is still being polished. -->
      <EvaluationsPanel
        v-if="task.id"
        report-type="weekly"
        :report-id="task.id"
      />

      <!-- Compare dialog: shown after AI polish returns a candidate. -->
      <el-dialog v-model="showCompare" title="🤖 AI 润色结果" width="920px" top="4vh">
        <div class="compare-wrap">
          <div class="compare-col">
            <div class="compare-head">当前内容</div>
            <pre class="compare-box">{{ content }}</pre>
          </div>
          <div class="compare-col">
            <div class="compare-head polished">AI 润色后</div>
            <pre class="compare-box polished-box">{{ polishedDraft }}</pre>
          </div>
        </div>
        <template #footer>
          <el-button @click="showCompare = false">取消（保留当前）</el-button>
          <el-button type="primary" @click="applyPolish">应用 AI 版本</el-button>
        </template>
      </el-dialog>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserStore } from '../stores/user'
import api from '../api'
import EvaluationsPanel from '../components/EvaluationsPanel.vue'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const task = ref(null)
const content = ref('')
const dirty = ref(false)
const loading = ref(false)
const sending = ref(false)
const submitting = ref(false)
const polishing = ref(false)

const polishedDraft = ref('')
const showCompare = ref(false)

const isLocked = computed(() => task.value?.status === 'submitted')
const isOwner = computed(() =>
  userStore.user && task.value && task.value.user_id === userStore.user.id
)
const sourceCount = computed(() => {
  try {
    const arr = JSON.parse(task.value?.source_daily_ids || '[]')
    return Array.isArray(arr) ? arr.length : 0
  } catch { return 0 }
})

function onInput() { dirty.value = true }

async function load() {
  loading.value = true
  try {
    const res = await api.getWeeklyReport(route.params.id)
    task.value = res.data
    content.value = task.value?.content || ''
    dirty.value = false
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!dirty.value) return
  sending.value = true
  try {
    await api.updateWeeklyReport(task.value.id, { content: content.value })
    task.value.content = content.value
    dirty.value = false
    ElMessage.success('已保存')
  } catch { /* interceptor */ }
  finally { sending.value = false }
}

async function polish() {
  // Require saving first if dirty — the backend polishes based on the
  // DB row's content. Auto-save then polish for a smoother UX.
  if (dirty.value) {
    sending.value = true
    try {
      await api.updateWeeklyReport(task.value.id, { content: content.value })
      dirty.value = false
    } catch {
      sending.value = false
      return
    }
    sending.value = false
  }
  polishing.value = true
  try {
    const res = await api.polishWeeklyReport(task.value.id)
    polishedDraft.value = res.data?.content || ''
    if (!polishedDraft.value) {
      ElMessage.warning('AI 未返回内容')
      return
    }
    // Show side-by-side compare so the user decides whether to keep.
    showCompare.value = true
  } catch { /* interceptor */ }
  finally { polishing.value = false }
}

function applyPolish() {
  content.value = polishedDraft.value
  dirty.value = true
  showCompare.value = false
  ElMessage.info('已应用 AI 版本，请记得点「保存草稿」或「提交」')
}

async function submit() {
  if (dirty.value) {
    // Auto-save before submit — it's confusing to see a draft with
    // unsaved edits suddenly locked to an older version.
    try {
      await api.updateWeeklyReport(task.value.id, { content: content.value })
      dirty.value = false
    } catch {
      return
    }
  }
  try {
    await ElMessageBox.confirm(
      '提交后将不可再编辑或删除（留痕）。确认提交吗？',
      '提交周汇报',
      { type: 'warning', confirmButtonText: '提交', cancelButtonText: '取消' },
    )
  } catch { return }

  submitting.value = true
  try {
    await api.submitWeeklyReport(task.value.id)
    ElMessage.success('已提交')
    await load()
  } catch { /* interceptor */ }
  finally { submitting.value = false }
}

async function remove() {
  try {
    await ElMessageBox.confirm(
      '确认删除此草稿？删除后不可恢复。',
      '删除草稿',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch { return }
  try {
    await api.deleteWeeklyReport(task.value.id)
    ElMessage.success('已删除')
    router.push('/reports/weekly')
  } catch { /* interceptor */ }
}

onMounted(load)
</script>

<style scoped>
.back-row { margin-bottom:12px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); text-decoration:none; }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:20px 26px; margin-bottom:16px; }

.header-panel { padding:22px 28px; }
.header-row { display:flex; justify-content:space-between; align-items:flex-start; gap:20px; margin-bottom:14px; }
.header-left { flex:1; min-width:0; }
.title-line { display:flex; align-items:center; gap:10px; margin-bottom:10px; flex-wrap:wrap; }
.date-badge { font-size:14px; font-weight:800; color:var(--sun-deep); background:var(--sun-light); padding:5px 14px; border-radius:14px; border:1.5px solid var(--peach); font-family:'JetBrains Mono', monospace; }
.status-badge { font-size:11.5px; font-weight:800; padding:3px 10px; border-radius:12px; }
.sb-draft { background:#FEF3C7; color:#92400E; border:1px solid #FCD34D; }
.sb-submitted { background:var(--ok-bg); color:var(--ok); border:1px solid var(--ok-border); }

.meta-line { display:flex; flex-wrap:wrap; gap:16px; font-size:12.5px; color:var(--ink-mid); }
.meta-item { display:flex; gap:6px; }
.meta-label { font-weight:700; color:var(--ink-hint); text-transform:uppercase; font-size:10.5px; letter-spacing:.05em; display:inline-flex; align-items:center; }
.meta-sub { color:var(--ink-hint); font-size:11.5px; }

.action-row { display:flex; gap:10px; flex-wrap:wrap; padding-top:12px; border-top:1.5px solid var(--border-light); margin-top:14px; }
.locked-banner { padding:14px 18px; background:var(--ok-bg); color:var(--ok); border:1.5px solid var(--ok-border); border-radius:var(--r-sm); font-size:13.5px; font-weight:700; text-align:center; }

.editor-panel { padding:20px 26px; }
.section-title { font-size:14px; font-weight:800; color:var(--ink); margin-bottom:12px; display:flex; align-items:center; gap:10px; }
.dirty-tag { font-size:11px; padding:2px 10px; font-weight:700; color:var(--warn); background:#FEF3C7; border:1px solid #FCD34D; border-radius:10px; }

.editor-textarea {
  width:100%; box-sizing:border-box; padding:16px 18px;
  border:1.5px solid var(--border); border-radius:var(--r-sm);
  font-size:13.5px; font-family:'JetBrains Mono', monospace; line-height:1.75;
  resize:vertical; outline:none; background:#FFFDF9;
  min-height:420px;
}
.editor-textarea:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); }

.content-readonly {
  white-space:pre-wrap; word-break:break-word;
  font-size:13.5px; line-height:1.85; color:var(--ink);
  padding:16px 18px; background:var(--surface-2);
  border-radius:var(--r-sm); border:1.5px solid var(--border-light);
  font-family:inherit; margin:0;
  min-height:420px;
}

.btn { display:inline-flex; align-items:center; gap:6px; padding:10px 18px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn:hover:not(:disabled) { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); }
.btn:disabled { opacity:.55; cursor:not-allowed; }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover:not(:disabled) { background:linear-gradient(135deg,var(--sun-deep),var(--sun)); color:#fff; }
.btn-polish { background:#EEF2FF; color:#4338CA; border-color:#C7D2FE; }
.btn-polish:hover:not(:disabled) { background:#C7D2FE; color:#3730A3; }
.btn-submit { background:linear-gradient(135deg,#10B981,#059669); color:#fff; border-color:transparent; }
.btn-submit:hover:not(:disabled) { filter:brightness(1.08); color:#fff; }
.btn-danger-soft { color:var(--err); border-color:#FFCDD2; background:#FFF5F5; margin-left:auto; }
.btn-danger-soft:hover:not(:disabled) { background:var(--err); color:#fff; border-color:var(--err); }

.compare-wrap { display:grid; grid-template-columns:1fr 1fr; gap:14px; }
.compare-col { display:flex; flex-direction:column; }
.compare-head { font-size:12px; font-weight:800; color:var(--ink-mid); margin-bottom:6px; padding:4px 8px; background:var(--surface-3); border-radius:6px; }
.compare-head.polished { color:#4338CA; background:#EEF2FF; }
.compare-box { white-space:pre-wrap; word-break:break-word; padding:12px 14px; background:var(--surface-2); border:1px solid var(--border-light); border-radius:var(--r-sm); font-size:12.5px; line-height:1.7; max-height:60vh; overflow:auto; margin:0; font-family:'JetBrains Mono', monospace; }
.polished-box { background:#F5F3FF; border-color:#DDD6FE; }
</style>
