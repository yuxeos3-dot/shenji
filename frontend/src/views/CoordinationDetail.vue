<template>
  <div class="coord-detail">
    <div class="back-row">
      <a href="#" @click.prevent="$router.push('/coordination')" class="back-btn">← 返回协同列表</a>
      <span class="back-sep">·</span>
      <span class="back-url">任务 #{{ task?.id || route.params.id }}</span>
    </div>

    <div v-if="loading && !task" class="panel" style="text-align:center;padding:60px">加载中...</div>

    <template v-else-if="task">
      <!-- Header with status + key timestamps -->
      <div class="panel header-panel">
        <div class="header-top">
          <span :class="['sev-tag', sevClass(task.issue_severity)]">{{ task.issue_severity || '—' }}</span>
          <span v-if="task.issue_module" class="mod-chip">{{ task.issue_module }}</span>
          <h1 class="issue-title">{{ task.issue_title }}</h1>
          <span :class="['status-tag', 'st-'+task.status]">{{ statusLabel(task.status) }}</span>
        </div>

        <div class="meta-row">
          <div class="meta-item">
            <div class="meta-label">站点</div>
            <div class="meta-val">{{ task.site_name }}<span class="meta-sub">{{ task.site_domain }}</span></div>
          </div>
          <div class="meta-item">
            <div class="meta-label">创建人</div>
            <div class="meta-val">{{ task.creator_name || '—' }}</div>
          </div>
          <div class="meta-item">
            <div class="meta-label">开发</div>
            <div class="meta-val">{{ task.assignee_name || '（未分配）' }}</div>
          </div>
          <div class="meta-item">
            <div class="meta-label">来源审计</div>
            <div class="meta-val">
              <a href="#" @click.prevent="$router.push(`/sites/${task.site_id}/audit/${task.audit_task_id}`)" class="audit-link">
                #{{ task.audit_task_id }} →
              </a>
            </div>
          </div>
        </div>

        <!-- 4 milestone timestamps -->
        <div class="milestones">
          <div class="mst" :class="{done: !!task.created_at}">
            <div class="mst-dot"></div>
            <div class="mst-label">创建</div>
            <div class="mst-time">{{ fmtTime(task.created_at) }}</div>
          </div>
          <div class="mst-line" :class="{done: !!task.started_at}"></div>
          <div class="mst" :class="{done: !!task.started_at}">
            <div class="mst-dot"></div>
            <div class="mst-label">开始开发</div>
            <div class="mst-time">{{ task.started_at ? fmtTime(task.started_at) : '—' }}</div>
          </div>
          <div class="mst-line" :class="{done: !!task.submitted_at}"></div>
          <div class="mst" :class="{done: !!task.submitted_at}">
            <div class="mst-dot"></div>
            <div class="mst-label">修改完毕</div>
            <div class="mst-time">{{ task.submitted_at ? fmtTime(task.submitted_at) : '—' }}</div>
          </div>
          <div class="mst-line" :class="{done: !!task.verified_at}"></div>
          <div class="mst" :class="{done: !!task.verified_at}">
            <div class="mst-dot"></div>
            <div class="mst-label">验收完成</div>
            <div class="mst-time">{{ task.verified_at ? fmtTime(task.verified_at) : '—' }}</div>
          </div>
        </div>

        <!-- State-transition buttons. Which one shows is decided by status. -->
        <div class="action-row" v-if="!isLocked">
          <template v-if="task.status === 'pending' || task.status === 'in_progress'">
            <button class="btn btn-primary" @click="showAssignDialog = true">
              {{ task.status === 'pending' ? '🛠 分配给开发（开始）' : '🔁 重新分配' }}
            </button>
          </template>
          <button
            v-if="task.status === 'in_progress'"
            class="btn btn-submit"
            @click="doSubmit"
          >✅ 标记修改完毕</button>
          <button
            v-if="task.status === 'under_review'"
            class="btn btn-verify"
            @click="doVerify"
          >🎯 验收通过 · 关闭任务</button>
          <button
            v-if="task.status === 'pending'"
            class="btn btn-danger-soft"
            @click="doDelete"
          >🗑 删除任务</button>
        </div>
        <div v-else class="locked-banner">
          🔒 任务已完成并关闭，所有操作已锁定（留痕）
        </div>
      </div>

      <!-- Issue snapshot (read-only, even while status is active) -->
      <div class="panel snapshot-panel" v-if="issueSnapshotParsed">
        <div class="section-title">原始问题快照（创建时）</div>
        <div v-if="issueSnapshotParsed.scope" class="issue-scope">{{ issueSnapshotParsed.scope }}</div>
        <pre v-if="issueSnapshotParsed.evidence_code" class="evidence-code">{{ issueSnapshotParsed.evidence_code }}</pre>
        <div v-if="issueSnapshotParsed.current_status" class="issue-section">
          <div class="section-label current-label">目前现状</div>
          <div class="section-body">{{ issueSnapshotParsed.current_status }}</div>
        </div>
        <div v-if="issueSnapshotParsed.fix_suggestion" class="issue-section">
          <div class="section-label fix-label">修改建议</div>
          <div class="section-body fix-body">{{ issueSnapshotParsed.fix_suggestion }}</div>
        </div>
      </div>

      <!-- Editable note + new comment -->
      <div class="panel note-panel" v-if="!isLocked">
        <div class="section-title">
          任务备注
          <button class="btn-xs" v-if="!editingNote" @click="startEditNote">✏ 编辑</button>
          <template v-else>
            <button class="btn-xs" @click="saveNote">保存</button>
            <button class="btn-xs btn-xs-ghost" @click="editingNote = false; noteDraft = task.note">取消</button>
          </template>
        </div>
        <textarea
          v-if="editingNote"
          v-model="noteDraft"
          class="note-textarea"
          rows="4"
          placeholder="描述需求、附加上下文、验收标准等"
        ></textarea>
        <div v-else class="note-display">{{ task.note || '（暂无备注）' }}</div>
      </div>

      <!-- Timeline -->
      <div class="panel timeline-panel">
        <div class="section-title">流程时间线</div>
        <div v-if="!events.length" class="empty-hint">暂无事件</div>
        <div v-else class="timeline">
          <div
            v-for="ev in events"
            :key="ev.id"
            :class="['tl-item', 'tl-'+ev.event_type]"
          >
            <div class="tl-dot"></div>
            <div class="tl-body">
              <div class="tl-head">
                <span class="tl-type">{{ eventLabel(ev.event_type) }}</span>
                <span class="tl-actor">{{ ev.actor_name || '系统' }}</span>
                <span class="tl-time">{{ fmtTime(ev.created_at) }}</span>
              </div>
              <div v-if="ev.from_status !== ev.to_status" class="tl-transition">
                {{ statusLabel(ev.from_status) || '—' }} → {{ statusLabel(ev.to_status) }}
              </div>
              <div v-if="ev.note" class="tl-note">{{ ev.note }}</div>
            </div>
          </div>
        </div>

        <!-- Add comment row -->
        <div v-if="!isLocked" class="comment-row">
          <textarea
            v-model="commentDraft"
            class="comment-input"
            rows="2"
            placeholder="添加备注（@提及、附链接、描述卡点）"
          ></textarea>
          <button class="btn btn-sm" @click="doAddComment" :disabled="!commentDraft.trim() || sending">
            {{ sending ? '发送中...' : '💬 添加备注' }}
          </button>
        </div>
      </div>
    </template>

    <!-- Assign dialog -->
    <el-dialog v-model="showAssignDialog" title="分配给开发" width="440px">
      <div class="assign-field">
        <label>选择开发</label>
        <el-select v-model="assignForm.assignee_id" placeholder="选择开发账号" style="width:100%" filterable>
          <el-option
            v-for="u in developers"
            :key="u.id"
            :label="`${u.name} (${u.email})`"
            :value="u.id"
          />
        </el-select>
      </div>
      <div class="assign-field">
        <label>备注（可选）</label>
        <textarea v-model="assignForm.note" rows="3" class="assign-textarea" placeholder="给开发的说明"></textarea>
      </div>
      <template #footer>
        <el-button @click="showAssignDialog = false">取消</el-button>
        <el-button type="primary" @click="doAssign" :loading="sending">确认分配</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const route = useRoute()
const router = useRouter()

const task = ref(null)
const events = ref([])
const developers = ref([])
const loading = ref(false)
const sending = ref(false)

const editingNote = ref(false)
const noteDraft = ref('')
const commentDraft = ref('')

const showAssignDialog = ref(false)
const assignForm = ref({ assignee_id: null, note: '' })

// Parse the snapshot once so template access is cheap.
const issueSnapshotParsed = computed(() => {
  if (!task.value?.issue_snapshot) return null
  try { return JSON.parse(task.value.issue_snapshot) } catch { return null }
})

// Locked means: the workflow is complete, nothing can be edited per the
// 工作留痕 (audit trail) requirement. Backend also enforces this with 409
// Conflict on any write; the frontend just hides the buttons.
const isLocked = computed(() => task.value?.status === 'done')

function statusLabel(s) {
  return {
    '': '—',
    pending: '待分配',
    in_progress: '开发中',
    under_review: '待验收',
    done: '已完成',
  }[s] || s
}
function sevClass(sev) {
  if (sev === '致命') return 'sev-fatal'
  if (sev === '严重') return 'sev-serious'
  if (sev === '中等') return 'sev-medium'
  if (sev === '低') return 'sev-low'
  return 'sev-none'
}
function eventLabel(t) {
  return {
    create: '🆕 创建任务',
    assign: '👤 分配开发',
    start: '▶ 开始处理',
    submit: '✅ 修改完毕',
    verify: '🎯 审计通过',
    comment: '💬 备注',
  }[t] || t
}
function fmtTime(s) {
  if (!s) return '—'
  try {
    const d = new Date(s.includes('T') ? s : s.replace(' ', 'T') + 'Z')
    if (Number.isNaN(d.getTime())) return s
    const pad = (n) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  } catch { return s }
}

async function load() {
  loading.value = true
  try {
    const res = await api.coordGetTask(route.params.id)
    task.value = res.data?.task || null
    events.value = res.data?.events || []
    noteDraft.value = task.value?.note || ''
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

async function loadDevelopers() {
  try {
    const res = await api.listUsers()
    // Show everyone who could plausibly be assigned: admins + members. We
    // don't filter by role specifically because sometimes a team lead is
    // an admin who still does dev work. The picker is searchable (filterable)
    // so long lists are fine.
    developers.value = res.data || []
  } catch {
    developers.value = []
  }
}

function startEditNote() {
  noteDraft.value = task.value?.note || ''
  editingNote.value = true
}

async function saveNote() {
  sending.value = true
  try {
    await api.coordUpdateTask(task.value.id, { note: noteDraft.value })
    task.value.note = noteDraft.value
    editingNote.value = false
    ElMessage.success('已保存')
  } catch { /* interceptor showed */ }
  finally { sending.value = false }
}

async function doAssign() {
  if (!assignForm.value.assignee_id) {
    ElMessage.warning('请选择开发')
    return
  }
  sending.value = true
  try {
    await api.coordAssignTask(task.value.id, {
      assignee_id: assignForm.value.assignee_id,
      note: assignForm.value.note || '',
    })
    ElMessage.success('已分配，任务进入「开发中」')
    showAssignDialog.value = false
    assignForm.value = { assignee_id: null, note: '' }
    await load()
  } catch { /* interceptor showed */ }
  finally { sending.value = false }
}

async function doSubmit() {
  try {
    const { value } = await ElMessageBox.prompt('请填写修改说明（可选）', '标记修改完毕', {
      confirmButtonText: '提交',
      cancelButtonText: '取消',
      inputPlaceholder: '例如：已按建议增加首页 hero 区文案 300 字',
    })
    sending.value = true
    await api.coordSubmitTask(task.value.id, { note: value || '' })
    ElMessage.success('已提交，等待审计验收')
    await load()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
  } finally { sending.value = false }
}

async function doVerify() {
  try {
    await ElMessageBox.confirm('确认已经重新审计并通过吗？任务将进入「已完成」状态，之后无法修改或删除（工作留痕）。', '验收通过', {
      type: 'warning',
      confirmButtonText: '确认验收',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    const { value } = await ElMessageBox.prompt('验收备注（可选）', '完成任务', {
      confirmButtonText: '完成',
      cancelButtonText: '取消',
      inputPlaceholder: '例如：首页已符合 ≥200 字要求',
    })
    sending.value = true
    await api.coordVerifyTask(task.value.id, { note: value || '' })
    ElMessage.success('任务已完成')
    await load()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
  } finally { sending.value = false }
}

async function doAddComment() {
  const text = commentDraft.value.trim()
  if (!text) return
  sending.value = true
  try {
    await api.coordAddComment(task.value.id, { note: text })
    commentDraft.value = ''
    await load()
    ElMessage.success('已添加')
  } catch { /* interceptor showed */ }
  finally { sending.value = false }
}

async function doDelete() {
  try {
    await ElMessageBox.confirm('确认删除此任务？只有待分配状态可删除，删除后不可恢复。', '删除任务', {
      type: 'warning',
      confirmButtonText: '确认删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await api.coordDeleteTask(task.value.id)
    ElMessage.success('已删除')
    router.push('/coordination')
  } catch { /* interceptor */ }
}

onMounted(async () => {
  await Promise.all([load(), loadDevelopers()])
})
</script>

<style scoped>
.coord-detail { display:flex; flex-direction:column; gap:16px; }
.back-row { display:flex; align-items:center; gap:8px; margin-bottom:4px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); }
.back-sep { color:var(--ink-hint); }
.back-url { font-size:14px; font-weight:700; color:var(--ink); }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:20px 26px; }

.header-panel { padding:24px 28px; }
.header-top { display:flex; align-items:center; gap:10px; flex-wrap:wrap; margin-bottom:18px; }
.issue-title { font-size:20px; font-weight:800; color:var(--ink); flex:1; margin:0; line-height:1.35; }
.sev-tag { display:inline-block; padding:3px 10px; border-radius:4px; font-size:12px; font-weight:800; color:#fff; }
.sev-fatal { background:#C0392B; } .sev-serious { background:#E67E22; } .sev-medium { background:#F39C12; } .sev-low { background:#27AE60; } .sev-none { background:#94A3B8; }
.mod-chip { padding:2px 9px; background:#34495E; color:#ECF0F1; border-radius:3px; font-size:11px; font-weight:700; }

.status-tag { display:inline-block; padding:5px 14px; border-radius:20px; font-size:12.5px; font-weight:800; }
.st-pending { background:#DBEAFE; color:#1D4ED8; }
.st-in_progress { background:#FEF3C7; color:#92400E; }
.st-under_review { background:#EDE9FE; color:#6D28D9; }
.st-done { background:var(--ok-bg); color:var(--ok); }

.meta-row { display:grid; grid-template-columns:repeat(4,1fr); gap:16px; padding:16px 0; border-top:1.5px solid var(--border-light); border-bottom:1.5px solid var(--border-light); margin-bottom:20px; }
.meta-item .meta-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.07em; margin-bottom:4px; }
.meta-item .meta-val { font-size:13.5px; font-weight:700; color:var(--ink); }
.meta-sub { font-size:11.5px; font-weight:500; color:var(--ink-hint); margin-left:6px; font-family:'JetBrains Mono',monospace; }
.audit-link { color:var(--info); font-weight:700; }
.audit-link:hover { text-decoration:underline; }

.milestones { display:flex; align-items:flex-start; justify-content:space-between; margin-bottom:20px; gap:8px; }
.mst { display:flex; flex-direction:column; align-items:center; min-width:110px; text-align:center; flex:0 0 auto; }
.mst-dot { width:14px; height:14px; border-radius:50%; background:var(--border-light); border:2px solid #FFF; box-shadow:0 0 0 2px var(--border-light); }
.mst.done .mst-dot { background:var(--sun); box-shadow:0 0 0 2px var(--sun); }
.mst-label { font-size:12px; font-weight:700; color:var(--ink-hint); margin-top:8px; }
.mst.done .mst-label { color:var(--ink); }
.mst-time { font-size:11px; color:var(--ink-soft); font-family:'JetBrains Mono',monospace; margin-top:2px; min-height:14px; }
.mst-line { flex:1; height:2px; background:var(--border-light); margin-top:7px; }
.mst-line.done { background:var(--sun); }

.action-row { display:flex; gap:10px; flex-wrap:wrap; padding-top:6px; }
.locked-banner { padding:14px 18px; background:var(--ok-bg); color:var(--ok); border:1.5px solid var(--ok-border); border-radius:var(--r-sm); font-size:13.5px; font-weight:700; text-align:center; }

.snapshot-panel { background:linear-gradient(135deg,#FFFDF8,#FFF8EC); border-color:var(--peach); }
.section-title { font-size:14px; font-weight:800; color:var(--ink); margin-bottom:14px; display:flex; align-items:center; gap:10px; }
.issue-scope { font-size:13px; color:var(--ink-soft); margin-bottom:12px; }
.evidence-code { background:#2C3E50; color:#ECF0F1; padding:12px 16px; border-radius:var(--r-sm); font-family:'JetBrains Mono',monospace; font-size:12px; line-height:1.6; overflow-x:auto; white-space:pre-wrap; word-break:break-all; margin-bottom:12px; }
.issue-section { margin-top:10px; }
.section-label { font-size:12.5px; font-weight:700; margin-bottom:6px; }
.current-label { color:#C0392B; } .fix-label { color:#27AE60; }
.section-body { font-size:13px; color:var(--ink-mid); line-height:1.7; white-space:pre-line; }
.fix-body { background:#F0FFF4; padding:12px 14px; border-radius:var(--r-sm); border-left:3px solid #27AE60; }

.note-panel { padding:20px 26px; }
.note-display { font-size:13.5px; color:var(--ink-mid); line-height:1.7; padding:12px 16px; background:var(--surface-2); border-radius:var(--r-sm); white-space:pre-wrap; min-height:40px; }
.note-textarea { width:100%; padding:12px 14px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13.5px; font-family:inherit; line-height:1.7; resize:vertical; outline:none; background:#FFFDF9; }
.note-textarea:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); }

.timeline { position:relative; padding-left:22px; }
.timeline::before { content:''; position:absolute; left:6px; top:6px; bottom:6px; width:2px; background:var(--border-light); }
.tl-item { position:relative; padding:12px 0 12px 12px; }
.tl-item .tl-dot { position:absolute; left:-22px; top:16px; width:14px; height:14px; border-radius:50%; background:var(--sun-light); border:2px solid var(--sun); }
.tl-item.tl-verify .tl-dot { background:var(--ok); border-color:var(--ok); }
.tl-item.tl-submit .tl-dot { background:#8B5CF6; border-color:#8B5CF6; }
.tl-item.tl-comment .tl-dot { background:var(--surface-2); border-color:var(--ink-hint); }
.tl-head { display:flex; gap:10px; align-items:center; font-size:13px; flex-wrap:wrap; }
.tl-type { font-weight:800; color:var(--ink); }
.tl-actor { font-weight:700; color:var(--sun-deep); font-size:12px; padding:2px 8px; background:var(--sun-light); border-radius:10px; }
.tl-time { font-size:11.5px; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; margin-left:auto; }
.tl-transition { font-size:11.5px; color:var(--ink-soft); margin-top:4px; font-family:'JetBrains Mono',monospace; }
.tl-note { font-size:13px; color:var(--ink-mid); margin-top:6px; padding:8px 12px; background:var(--surface-2); border-radius:6px; white-space:pre-wrap; line-height:1.6; }

.comment-row { margin-top:16px; padding-top:16px; border-top:1.5px dashed var(--border-light); display:flex; gap:10px; align-items:flex-start; }
.comment-input { flex:1; padding:10px 12px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13px; font-family:inherit; resize:vertical; outline:none; }
.comment-input:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); }

.assign-field { margin-bottom:14px; }
.assign-field label { display:block; font-size:12.5px; font-weight:700; color:var(--ink-mid); margin-bottom:6px; }
.assign-textarea { width:100%; padding:9px 12px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13px; font-family:inherit; resize:vertical; outline:none; box-sizing:border-box; }
.assign-textarea:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); }

.empty-hint { text-align:center; padding:30px; color:var(--ink-hint); font-size:13px; }

.btn { display:inline-flex; align-items:center; gap:6px; padding:10px 18px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; color:var(--ink-mid); transition:all .15s; }
.btn:hover:not(:disabled) { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); }
.btn:disabled { opacity:.6; cursor:not-allowed; }
.btn-sm { padding:8px 14px; font-size:12.5px; }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover:not(:disabled) { background:linear-gradient(135deg,var(--sun-deep),var(--sun)); color:#fff; }
.btn-submit { background:linear-gradient(135deg,#8B5CF6,#7C3AED); color:#fff; border-color:transparent; }
.btn-submit:hover:not(:disabled) { filter:brightness(1.1); color:#fff; }
.btn-verify { background:linear-gradient(135deg,#10B981,#059669); color:#fff; border-color:transparent; }
.btn-verify:hover:not(:disabled) { filter:brightness(1.1); color:#fff; }
.btn-danger-soft { color:var(--err); border-color:#FFCDD2; background:#FFF5F5; }
.btn-danger-soft:hover:not(:disabled) { background:var(--err); color:#fff; border-color:var(--err); }
.btn-xs { padding:3px 10px; font-size:11.5px; border-radius:5px; border:1px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-weight:700; color:var(--ink-mid); margin-left:8px; }
.btn-xs:hover { background:var(--sun-light); color:var(--sun-deep); }
.btn-xs-ghost { color:var(--ink-soft); background:transparent; }
</style>
