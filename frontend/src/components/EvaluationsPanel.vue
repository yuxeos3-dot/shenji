<template>
  <div class="eval-panel">
    <div class="section-title">
      💬 管理员评价
      <span class="count-tag" v-if="evals.length">{{ evals.length }}</span>
    </div>

    <!-- List of existing evaluations. Shown to both admin and owner. -->
    <div v-if="loading" class="eval-loading">加载评价中...</div>
    <div v-else-if="!evals.length && !isAdmin" class="eval-empty">
      暂无评价
    </div>
    <div v-else class="eval-list">
      <div v-for="e in evals" :key="e.id" class="eval-item">
        <div class="eval-head">
          <span class="evaluator">👤 {{ e.evaluator_name || '管理员' }}</span>
          <span class="eval-time">{{ e.created_at }}<span v-if="e.updated_at && e.updated_at !== e.created_at" class="edited-tag"> · 已编辑于 {{ e.updated_at }}</span></span>
          <!-- Only the evaluator themselves sees the retract button. -->
          <button
            v-if="userStore.user && e.evaluator_id === userStore.user.id"
            class="btn-retract"
            @click="retract(e.id)"
            title="撤回这条评价"
          >撤回</button>
        </div>
        <div class="eval-content">{{ e.content }}</div>
      </div>
    </div>

    <!-- Admin-only editor. Pre-fills with the admin's existing
         evaluation if any, so editing feels like "update in place". -->
    <div v-if="isAdmin" class="eval-editor">
      <div class="editor-title">
        {{ myEvalId ? '✏ 编辑我的评价' : '✍ 添加评价' }}
        <span v-if="myEvalId" class="inline-hint">（再次保存会覆盖之前的内容）</span>
      </div>
      <textarea
        v-model="draft"
        rows="4"
        placeholder="写下你对这份汇报的评价..."
        class="eval-textarea"
      ></textarea>
      <div class="editor-actions">
        <button
          class="btn btn-polish"
          :disabled="!canSave || polishing"
          @click="polish"
          title="用 AI 帮你润色草稿"
        >
          {{ polishing ? '🤖 润色中...' : '🤖 AI 润色' }}
        </button>
        <button class="btn btn-primary" :disabled="!canSave || saving" @click="save">
          {{ saving ? '保存中...' : (myEvalId ? '💾 更新评价' : '💾 发表评价') }}
        </button>
      </div>
    </div>

    <!-- Compare dialog: user decides whether to apply AI rewrite. -->
    <el-dialog v-model="showCompare" title="🤖 AI 润色结果" width="760px" top="5vh">
      <div class="compare-wrap">
        <div class="compare-col">
          <div class="compare-head">当前内容</div>
          <pre class="compare-box">{{ draft }}</pre>
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
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserStore } from '../stores/user'
import api from '../api'

const props = defineProps({
  reportType: { type: String, required: true }, // 'daily' | 'weekly' | 'monthly'
  reportId: { type: [Number, String], required: true },
})

const userStore = useUserStore()
const evals = ref([])
const loading = ref(false)
const draft = ref('')
const saving = ref(false)
const polishing = ref(false)
const polishedDraft = ref('')
const showCompare = ref(false)

const isAdmin = computed(() => userStore.isAdmin())

// If the current admin has already written an evaluation for this
// report, we load it into the editor so "write" and "edit" feel like
// the same action.
const myEvalId = computed(() => {
  if (!userStore.user) return null
  const mine = evals.value.find(e => e.evaluator_id === userStore.user.id)
  return mine?.id || null
})

const canSave = computed(() => draft.value.trim().length > 0)

async function load() {
  if (!props.reportId) return
  loading.value = true
  try {
    const res = await api.listEvaluations({
      report_type: props.reportType,
      report_id: props.reportId,
    })
    evals.value = res.data || []

    // Pre-fill the draft with the admin's existing eval so the
    // textarea is "editing" rather than "starting over".
    if (isAdmin.value && userStore.user) {
      const mine = evals.value.find(e => e.evaluator_id === userStore.user.id)
      draft.value = mine?.content || ''
    }
  } catch (e) {
    console.error('加载评价失败', e)
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!canSave.value) return
  saving.value = true
  try {
    await api.createEvaluation({
      report_type: props.reportType,
      report_id: Number(props.reportId),
      content: draft.value.trim(),
    })
    ElMessage.success(myEvalId.value ? '已更新评价' : '已发表评价')
    await load()
  } catch { /* interceptor */ }
  finally { saving.value = false }
}

// AI polish the current draft. Doesn't touch the DB — shows a compare
// dialog so the admin can accept or reject the AI rewrite. The admin
// is still the accountable author of whatever gets saved.
async function polish() {
  if (!canSave.value) return
  polishing.value = true
  try {
    const res = await api.polishEvaluation({
      report_type: props.reportType,
      report_id: Number(props.reportId),
      content: draft.value.trim(),
    })
    polishedDraft.value = res.data?.content || ''
    if (!polishedDraft.value) {
      ElMessage.warning('AI 未返回内容')
      return
    }
    showCompare.value = true
  } catch { /* interceptor */ }
  finally { polishing.value = false }
}

function applyPolish() {
  draft.value = polishedDraft.value
  showCompare.value = false
  ElMessage.info('已应用 AI 版本，记得点「发表评价」或「更新评价」保存')
}

async function retract(id) {
  try {
    await ElMessageBox.confirm(
      '确认撤回这条评价？撤回后不可恢复。',
      '撤回评价',
      { type: 'warning', confirmButtonText: '撤回', cancelButtonText: '取消' },
    )
  } catch { return }
  try {
    await api.deleteEvaluation(id)
    ElMessage.success('已撤回')
    // If I retracted my own, clear the draft so it doesn't look
    // like an uncommitted edit.
    if (userStore.user && evals.value.find(e => e.id === id)?.evaluator_id === userStore.user.id) {
      draft.value = ''
    }
    await load()
  } catch { /* interceptor */ }
}

// Reload when parent re-mounts with a different report id (shouldn't
// happen often, but keep it correct).
watch(() => props.reportId, load)
onMounted(load)
</script>

<style scoped>
.eval-panel {
  background:var(--surface);
  border-radius:var(--r);
  box-shadow:var(--s1);
  border:1.5px solid var(--border-light);
  padding:20px 26px;
  margin-bottom:16px;
}

.section-title {
  font-size:14px; font-weight:800; color:var(--ink);
  margin-bottom:14px;
  display:flex; align-items:center; gap:10px;
}
.count-tag {
  font-size:11px; padding:1px 9px; background:var(--sun-light);
  color:var(--sun-deep); border-radius:10px; font-weight:700;
  border:1px solid var(--peach);
}

.eval-loading, .eval-empty {
  font-size:13px; color:var(--ink-hint);
  padding:16px 0; text-align:center;
}

.eval-list { display:flex; flex-direction:column; gap:10px; margin-bottom:16px; }
.eval-item {
  padding:12px 14px;
  background:#FFF9EC;
  border:1px solid #FDE68A;
  border-left:3px solid var(--sun);
  border-radius:var(--r-sm);
}
.eval-head {
  display:flex; align-items:center; gap:10px;
  font-size:12px; margin-bottom:6px;
}
.evaluator {
  font-weight:800; color:var(--sun-deep);
}
.eval-time {
  color:var(--ink-hint); font-size:11.5px;
  font-family:'JetBrains Mono', monospace;
}
.edited-tag { color:var(--ink-hint); font-style:italic; }
.btn-retract {
  margin-left:auto; background:none; border:1px solid #FFCDD2;
  color:var(--err); padding:2px 10px; border-radius:8px;
  font-size:11px; font-weight:600; cursor:pointer;
  font-family:inherit;
}
.btn-retract:hover { background:var(--err); color:#fff; border-color:var(--err); }

.eval-content {
  font-size:13.5px; line-height:1.75; color:var(--ink);
  white-space:pre-wrap; word-break:break-word;
}

.eval-editor {
  padding-top:14px;
  border-top:1.5px dashed var(--border-light);
}
.editor-title {
  font-size:13px; font-weight:700; color:var(--ink-mid);
  margin-bottom:8px;
}
.inline-hint {
  font-size:11.5px; font-weight:500; color:var(--ink-hint);
  margin-left:8px;
}
.eval-textarea {
  width:100%; box-sizing:border-box;
  padding:12px 14px;
  border:1.5px solid var(--border);
  border-radius:var(--r-sm);
  font-size:13.5px; font-family:inherit; line-height:1.7;
  resize:vertical; outline:none; background:var(--surface);
  min-height:90px;
}
.eval-textarea:focus {
  border-color:var(--sun);
  box-shadow:0 0 0 3px rgba(249,115,22,.12);
}

.editor-actions {
  display:flex; justify-content:flex-end; margin-top:10px;
  gap:8px;
}
.btn {
  display:inline-flex; align-items:center; gap:6px;
  padding:8px 18px; border-radius:8px;
  border:1.5px solid var(--border); background:var(--surface);
  cursor:pointer; font-family:inherit; font-size:13px; font-weight:700;
  transition:all .18s; color:var(--ink-mid);
}
.btn:disabled { opacity:.55; cursor:not-allowed; }
.btn-polish { background:#EEF2FF; color:#4338CA; border-color:#C7D2FE; }
.btn-polish:hover:not(:disabled) { background:#C7D2FE; color:#3730A3; }
.btn-primary {
  background:linear-gradient(135deg,var(--sun),var(--coral));
  color:#fff; border-color:transparent; box-shadow:var(--s-amber);
}
.btn-primary:hover:not(:disabled) {
  background:linear-gradient(135deg,var(--sun-deep),var(--sun));
}

/* Compare dialog: side-by-side draft vs AI candidate */
.compare-wrap { display:grid; grid-template-columns:1fr 1fr; gap:14px; }
.compare-col { display:flex; flex-direction:column; }
.compare-head { font-size:12px; font-weight:800; color:var(--ink-mid); margin-bottom:6px; padding:4px 8px; background:var(--surface-3); border-radius:6px; }
.compare-head.polished { color:#4338CA; background:#EEF2FF; }
.compare-box { white-space:pre-wrap; word-break:break-word; padding:12px 14px; background:var(--surface-2); border:1px solid var(--border-light); border-radius:var(--r-sm); font-size:13px; line-height:1.75; max-height:55vh; overflow:auto; margin:0; font-family:inherit; }
.polished-box { background:#F5F3FF; border-color:#DDD6FE; }
</style>
