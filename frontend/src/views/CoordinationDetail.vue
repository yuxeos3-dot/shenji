<template>
  <div class="coord-detail">
    <div class="back-row">
      <a href="#" @click.prevent="$router.push('/coordination')" class="back-btn">← 返回协同列表</a>
      <span class="back-sep">·</span>
      <span class="back-url">任务 #{{ task?.id || route.params.id }}</span>
    </div>

    <div v-if="loading && !task" class="panel" style="text-align:center;padding:60px">加载中...</div>

    <template v-else-if="task">
      <!-- Header: stats + issue title + status -->
      <div class="panel header-panel">
        <div class="header-top">
          <span :class="['sev-tag', sevClass(task.issue_severity)]">{{ task.issue_severity || '—' }}</span>
          <span v-if="task.issue_module" class="mod-chip">{{ task.issue_module }}</span>
          <h1 v-if="!editingNote" class="issue-title">{{ task.issue_title }}</h1>
          <input
            v-else
            v-model="titleDraft"
            class="issue-title-input"
            placeholder="问题标题"
            maxlength="200"
          />
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
          <!-- Edit mode: show save/cancel pair and hide other actions so
               the user can't accidentally assign while they have unsaved
               title/note changes. -->
          <template v-if="editingNote">
            <button class="btn btn-primary" @click="saveNote" :disabled="sending">💾 保存问题编辑</button>
            <button class="btn btn-edit" @click="cancelEditNote">取消</button>
          </template>
          <template v-else>
            <template v-if="task.status === 'pending' || task.status === 'in_progress'">
              <button class="btn btn-primary" @click="showAssignDialog = true">
                {{ task.status === 'pending' ? '🛠 分配给开发（开始）' : '🔁 重新分配' }}
              </button>
            </template>
            <!-- Developer: ask a question. Only shown while actively developing;
                 pauses the task until the creator answers. -->
            <button
              v-if="task.status === 'in_progress'"
              class="btn btn-ask"
              @click="doAsk"
            >❓ 提问（转给发起人）</button>
            <button
              v-if="task.status === 'in_progress' || task.status === 'awaiting_answer'"
              class="btn btn-submit"
              @click="doSubmit"
            >✅ 开发完毕</button>
            <button
              v-if="task.status === 'under_review'"
              class="btn btn-verify"
              @click="doVerify"
            >🎯 验收通过 · 关闭任务</button>
            <button
              v-if="task.status === 'under_review'"
              class="btn btn-danger-soft"
              @click="doReject"
            >🔁 打回修改</button>
            <button
              v-if="task.status === 'pending'"
              class="btn btn-edit"
              @click="startEditNote"
            >✏ 编辑问题</button>
            <button
              v-if="task.status === 'pending'"
              class="btn btn-danger-soft"
              @click="doDelete"
            >🗑 删除任务</button>
          </template>
        </div>
        <div v-else class="locked-banner">
          🔒 任务已完成并关闭，所有操作已锁定（留痕）
        </div>

        <!-- When waiting for the creator to answer, surface the question
             and a prominent reply input. This is the single most
             important action for the creator, so it goes in the header. -->
        <div v-if="task.status === 'awaiting_answer' && latestAsk" class="pending-answer-box">
          <div class="pa-title">💬 开发 {{ latestAsk.actor_name }} 提了一个问题，需要你的回复：</div>
          <div class="pa-question">{{ latestAsk.note }}</div>
          <textarea
            v-model="answerDraft"
            class="pa-input"
            rows="3"
            placeholder="回复开发的问题..."
          ></textarea>
          <button class="btn btn-primary" @click="doAnswer" :disabled="!answerDraft.trim() || sending">
            {{ sending ? '发送中...' : '✉ 回复并继续开发' }}
          </button>
        </div>
      </div>

      <!-- Pre-audit panel: shown while task is under_review (待验收).
           Admin attaches test-server URLs that were modified by dev,
           fetches their HTML, asks AI for a verdict (fixed/not_fixed/
           unclear), then decides 验收通过 or 打回. -->
      <div v-if="task.status === 'under_review'" class="panel preaudit-panel">
        <div class="section-title">
          🧪 测试服验收
          <span class="pa-subtitle">开发已提交修复，请添加测试服页面、抓取源码、运行 AI 预审核后决定是否通过</span>
        </div>

        <!-- Existing AI verdict (if any) -->
        <div v-if="task.pre_audit_verdict" :class="['verdict-box', 'vb-' + task.pre_audit_verdict]">
          <div class="vb-head">
            <span class="vb-badge">
              {{ task.pre_audit_verdict === 'fixed' ? '✅ AI 判断：已修复'
                : task.pre_audit_verdict === 'not_fixed' ? '❌ AI 判断：未修复'
                : '⚠️ AI 判断：无法确定' }}
            </span>
            <span v-if="task.pre_audit_at" class="vb-time">{{ task.pre_audit_at }}</span>
          </div>
          <div class="vb-reasoning">{{ task.pre_audit_result }}</div>
          <div class="vb-hint">⚠️ AI 判断仅供参考，最终由人工审核决定</div>
        </div>

        <!-- Test pages list -->
        <div class="test-pages-head">
          <div class="tp-title">测试服页面（{{ testPages.length }}）</div>
          <div class="tp-actions">
            <button class="btn btn-xs btn-primary-soft" @click="openAddTestPage">+ 添加测试页</button>
            <button
              class="btn btn-xs btn-polish"
              :disabled="!hasFetchedPage || preAuditing"
              @click="doPreAudit"
              :title="hasFetchedPage ? '运行 AI 预审核' : '请先添加并抓取至少一个页面'"
            >{{ preAuditing ? '🤖 AI 审核中...' : '🤖 AI 预审核' }}</button>
          </div>
        </div>

        <table v-if="testPages.length" class="tp-table">
          <thead>
            <tr>
              <th>名称</th>
              <th>URL</th>
              <th>状态</th>
              <th>抓取时间</th>
              <th style="width:220px">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in testPages" :key="p.id">
              <td><span class="tp-name">{{ p.name }}</span></td>
              <td class="tp-url"><a :href="p.url" target="_blank" rel="noopener">{{ p.url }}</a></td>
              <td>
                <span v-if="!p.last_fetched_at" class="tp-status tp-none">未抓取</span>
                <span v-else-if="p.status_code >= 200 && p.status_code < 400" class="tp-status tp-ok">HTTP {{ p.status_code }}</span>
                <span v-else class="tp-status tp-err">HTTP {{ p.status_code || '-' }}</span>
              </td>
              <td class="tp-time">{{ p.last_fetched_at || '—' }}</td>
              <td>
                <button class="btn btn-xs" @click="doFetchPage(p)" :disabled="fetchingPageIds.includes(p.id)">
                  {{ fetchingPageIds.includes(p.id) ? '抓取中...' : '🔄 抓取' }}
                </button>
                <button class="btn btn-xs" v-if="p.last_fetched_at" @click="openViewSource(p)">👁 源码</button>
                <button class="btn btn-xs btn-danger-soft-xs" @click="doDeletePage(p)">删</button>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-else class="tp-empty">还没有测试页。点击「+ 添加测试页」开始。</div>
      </div>

      <!-- Add test page dialog -->
      <el-dialog v-model="showAddTestPage" title="添加测试服页面" width="520px">
        <div class="form-group">
          <label>名称</label>
          <input v-model="testPageForm.name" placeholder="例如：文章详情页 测试服" @keyup.enter="saveTestPage" />
        </div>
        <div class="form-group">
          <label>URL</label>
          <input v-model="testPageForm.url" placeholder="https://test.example.com/..." @keyup.enter="saveTestPage" />
        </div>
        <template #footer>
          <el-button @click="showAddTestPage = false">取消</el-button>
          <el-button type="primary" @click="saveTestPage">保存</el-button>
        </template>
      </el-dialog>

      <!-- View source dialog -->
      <el-dialog v-model="showSource" :title="'页面源码：' + (viewingPage?.name || '')" width="860px" top="5vh">
        <div v-if="viewingPage" class="src-meta">
          <span>URL: <a :href="viewingPage.url" target="_blank" rel="noopener">{{ viewingPage.url }}</a></span>
          <span>抓取时间: {{ viewingPage.last_fetched_at || '—' }}</span>
        </div>
        <pre class="src-html">{{ viewingHtml }}</pre>
      </el-dialog>

      <!-- Reject dialog -->
      <el-dialog v-model="showReject" title="打回修改" width="540px">
        <p class="reject-hint">打回后任务会回到「开发中」状态，以下备注会推送给开发者：</p>
        <textarea
          v-model="rejectNote"
          rows="5"
          placeholder="请说明哪里还没修好、希望开发做什么修改（必填）"
          class="reject-textarea"
        ></textarea>
        <template #footer>
          <el-button @click="showReject = false">取消</el-button>
          <el-button type="danger" @click="confirmReject" :disabled="!rejectNote.trim()">确认打回</el-button>
        </template>
      </el-dialog>

      <!-- Issue snapshot. Read-only in every status except pending when
           the user is in edit mode — then all 4 fields become editable
           textareas so the creator can rewrite scope / current_status /
           fix_suggestion / evidence_code before pushing to dev. -->
      <div class="panel snapshot-panel" v-if="issueSnapshotParsed || editingNote">
        <div class="section-title">
          原始问题快照{{ editingNote ? '（编辑中）' : '（创建时）' }}
        </div>

        <!-- Read-only view -->
        <template v-if="!editingNote">
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
        </template>

        <!-- Edit view — 4 editable fields. We keep the key names aligned
             with the audit's v6 report schema so changes round-trip
             through `issue_snapshot` as valid JSON. -->
        <template v-else>
          <div class="edit-field">
            <div class="edit-field-label">范围 (scope)</div>
            <input v-model="snapshotDraft.scope" class="snap-input" placeholder="例如：全站 / 首页 / 文章详情页" />
          </div>
          <div class="edit-field">
            <div class="edit-field-label">证据代码 (evidence_code)</div>
            <textarea
              v-model="snapshotDraft.evidence_code"
              class="snap-textarea mono"
              rows="3"
              placeholder="相关 HTML / JS / 配置片段"
            ></textarea>
          </div>
          <div class="edit-field">
            <div class="edit-field-label current-label">目前现状 (current_status)</div>
            <textarea
              v-model="snapshotDraft.current_status"
              class="snap-textarea"
              rows="3"
              placeholder="描述目前的状况、存在的问题"
            ></textarea>
          </div>
          <div class="edit-field">
            <div class="edit-field-label fix-label">修改建议 (fix_suggestion)</div>
            <textarea
              v-model="snapshotDraft.fix_suggestion"
              class="snap-textarea"
              rows="4"
              placeholder="给开发的具体修改建议、步骤、验收标准"
            ></textarea>
          </div>
        </template>
      </div>

      <!-- Editable note — enters edit mode from the header button when
           status === pending. Non-pending states show read-only with a
           lock hint (the problem is already contracted with the dev). -->
      <div class="panel note-panel" v-if="!isLocked" ref="noteAnchor">
        <div class="section-title">
          任务备注
          <span v-if="editingNote" class="editing-badge">编辑中</span>
          <span v-else-if="task.status !== 'pending'" class="lock-hint">
            🔒 任务已推给开发，标题/备注不可再改（如需补充请在下方添加备注）
          </span>
        </div>
        <textarea
          v-if="editingNote"
          v-model="noteDraft"
          class="note-textarea"
          rows="5"
          placeholder="描述需求、附加上下文、验收标准等"
        ></textarea>
        <div v-else class="note-display">{{ task.note || '（暂无备注）' }}</div>
      </div>

      <!-- Attachment panel: image grid + file list + upload button.
           Visible even when the task is done (we show attachments for
           archival purposes) but the action buttons are hidden. -->
      <div class="panel attach-panel">
        <div class="section-title">
          📎 附件（截图/文档 {{ attachments.length }} 个）
          <button
            v-if="!isLocked"
            class="btn-xs btn-xs-primary"
            :disabled="uploading"
            @click="pickFile"
          >
            {{ uploading ? '上传中...' : '+ 上传截图/文件' }}
          </button>
          <input
            ref="fileInputRef"
            type="file"
            multiple
            accept=".png,.jpg,.jpeg,.gif,.webp,.pdf,.doc,.docx,.xls,.xlsx,.txt,.csv,.md"
            style="display:none"
            @change="onFilePicked"
          />
        </div>

        <div v-if="!attachments.length" class="empty-hint">
          暂无附件。点击「上传截图/文件」附加证据或补充材料。
          <div class="hint-sub">支持图片 / PDF / Office 文档，单个 10 MB 内。</div>
        </div>

        <!-- Image grid -->
        <div v-if="imageAttachments.length" class="img-grid">
          <div
            v-for="a in imageAttachments"
            :key="a.id"
            class="img-cell"
          >
            <a :href="attachmentUrl(a.id)" target="_blank" rel="noopener">
              <img :src="attachmentUrl(a.id)" :alt="a.filename" loading="lazy" />
            </a>
            <div class="img-meta">
              <span class="img-name" :title="a.filename">{{ a.filename }}</span>
              <button
                v-if="!isLocked"
                class="img-del"
                @click="deleteAttachment(a)"
                title="删除附件"
              >✕</button>
            </div>
          </div>
        </div>

        <!-- Non-image file list -->
        <div v-if="fileAttachments.length" class="file-list">
          <div v-for="a in fileAttachments" :key="a.id" class="file-row">
            <span class="file-icon">{{ fileIcon(a.mime) }}</span>
            <a :href="attachmentUrl(a.id)" target="_blank" rel="noopener" class="file-name">{{ a.filename }}</a>
            <span class="file-size">{{ fmtSize(a.size) }}</span>
            <span class="file-time">{{ fmtTime(a.uploaded_at) }}</span>
            <button
              v-if="!isLocked"
              class="btn-xs btn-xs-ghost"
              @click="deleteAttachment(a)"
            >删除</button>
          </div>
        </div>
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
import { ref, computed, onMounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const route = useRoute()
const router = useRouter()

const task = ref(null)
const events = ref([])
const attachments = ref([])
const developers = ref([])
const loading = ref(false)
const sending = ref(false)

const editingNote = ref(false)
const noteDraft = ref('')
const titleDraft = ref('')
// When editing, we edit individual fields of the snapshot rather than
// raw JSON — reassembled on save. Keys match the v6 report shape.
const snapshotDraft = ref({
  scope: '',
  evidence_code: '',
  current_status: '',
  fix_suggestion: '',
})
const noteAnchor = ref(null) // DOM ref for smooth scroll target
const commentDraft = ref('')
const answerDraft = ref('')
const uploading = ref(false)
const fileInputRef = ref(null)

const showAssignDialog = ref(false)
const assignForm = ref({ assignee_id: null, note: '' })

// ── Pre-audit (待验收) state ─────────────────────────────────────────────
// All pre-audit refs are only meaningful when task.status === 'under_review'.
// They're declared at module scope so the template can bind them without
// v-if guards, and we guard data loading via the status check in load().
const testPages = ref([])
const preAuditing = ref(false)
const fetchingPageIds = ref([])     // page ids currently being crawled
const showAddTestPage = ref(false)
const testPageForm = ref({ name: '', url: '' })
const showSource = ref(false)
const viewingPage = ref(null)
const viewingHtml = ref('')
const showReject = ref(false)
const rejectNote = ref('')

// True when at least one test page has been successfully fetched. The
// AI pre-audit button stays disabled until this is true — asking AI to
// inspect empty HTML would be wasted tokens.
const hasFetchedPage = computed(() =>
  testPages.value.some(p => p.last_fetched_at)
)

// Parse the snapshot once so template access is cheap.
const issueSnapshotParsed = computed(() => {
  if (!task.value?.issue_snapshot) return null
  try { return JSON.parse(task.value.issue_snapshot) } catch { return null }
})

// Locked means: the workflow is complete, nothing can be edited per the
// 工作留痕 (audit trail) requirement. Backend also enforces this with 409
// Conflict on any write; the frontend just hides the buttons.
const isLocked = computed(() => task.value?.status === 'done')

// Find the most recent ask event so the creator can see what the dev
// asked without scrolling the whole timeline. Paired with the answer
// input box in the header when status === awaiting_answer.
const latestAsk = computed(() => {
  for (let i = events.value.length - 1; i >= 0; i--) {
    if (events.value[i].event_type === 'ask') return events.value[i]
  }
  return null
})

// Split attachments by kind so we can render images as thumbnails
// and non-images as a classic file list with icon + filename.
const imageAttachments = computed(() =>
  attachments.value.filter(a => (a.mime || '').startsWith('image/'))
)
const fileAttachments = computed(() =>
  attachments.value.filter(a => !(a.mime || '').startsWith('image/'))
)

function statusLabel(s) {
  return {
    '': '—',
    pending: '待分配',
    in_progress: '开发中',
    awaiting_answer: '待发起人回复',
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
    edit: '✏ 编辑问题',
    assign: '👤 分配开发',
    start: '▶ 开始处理',
    ask: '❓ 开发提问',
    answer: '✉ 发起人回复',
    submit: '✅ 开发完毕',
    verify: '🎯 审计通过',
    comment: '💬 备注',
    upload: '📎 上传附件',
    attach_delete: '🗑 删除附件',
    pre_audit: '🧪 AI 预审核',
    reject: '🔁 打回修改',
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
    attachments.value = res.data?.attachments || []
    noteDraft.value = task.value?.note || ''
    // Load test pages only while under_review; other states don't need
    // them and one extra request is enough noise to skip.
    if (task.value?.status === 'under_review') {
      await loadTestPages()
    } else {
      testPages.value = []
    }
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

// ── Pre-audit handlers ─────────────────────────────────────────────────

async function loadTestPages() {
  try {
    const res = await api.coordListTestPages(task.value.id)
    testPages.value = res.data || []
  } catch {
    testPages.value = []
  }
}

function openAddTestPage() {
  testPageForm.value = { name: '', url: '' }
  showAddTestPage.value = true
}

async function saveTestPage() {
  const name = testPageForm.value.name.trim()
  const url = testPageForm.value.url.trim()
  if (!name) { ElMessage.warning('请填写页面名称'); return }
  if (!url) { ElMessage.warning('请填写页面 URL'); return }
  if (!/^https?:\/\//i.test(url)) {
    ElMessage.warning('URL 必须以 http:// 或 https:// 开头')
    return
  }
  try {
    await api.coordAddTestPage(task.value.id, { name, url })
    ElMessage.success('已添加')
    showAddTestPage.value = false
    await loadTestPages()
  } catch { /* interceptor */ }
}

// Crawl a test page. We track fetching ids in a list rather than a
// boolean so multiple pages can be fetched concurrently (though in
// practice users tend to click them one by one).
async function doFetchPage(page) {
  fetchingPageIds.value = [...fetchingPageIds.value, page.id]
  try {
    const res = await api.coordFetchTestPage(task.value.id, page.id)
    const err = res.data?.error
    if (err) {
      ElMessage.warning('抓取完成但有错误：' + err)
    } else {
      ElMessage.success('抓取成功（HTTP ' + (res.data?.status_code || '-') + '）')
    }
    await loadTestPages()
  } catch { /* interceptor */ }
  finally {
    fetchingPageIds.value = fetchingPageIds.value.filter(id => id !== page.id)
  }
}

async function openViewSource(page) {
  viewingPage.value = page
  viewingHtml.value = '加载中...'
  showSource.value = true
  try {
    const res = await api.coordGetTestPageSource(task.value.id, page.id)
    viewingHtml.value = res.data?.html || '(源码为空)'
  } catch {
    viewingHtml.value = '(加载失败)'
  }
}

async function doDeletePage(page) {
  try {
    await ElMessageBox.confirm(`确认删除测试页「${page.name}」？`, '删除', { type: 'warning' })
  } catch { return }
  try {
    await api.coordDeleteTestPage(task.value.id, page.id)
    ElMessage.success('已删除')
    await loadTestPages()
  } catch { /* interceptor */ }
}

async function doPreAudit() {
  if (!hasFetchedPage.value) {
    ElMessage.warning('请先添加并抓取至少一个测试页')
    return
  }
  preAuditing.value = true
  try {
    const res = await api.coordPreAudit(task.value.id)
    const verdict = res.data?.verdict
    const label = verdict === 'fixed' ? '✅ 已修复'
      : verdict === 'not_fixed' ? '❌ 未修复'
      : '⚠️ 无法确定'
    ElMessage.info('AI 审核完成：' + label)
    await load() // reload so the verdict_box on the page shows the fresh result
  } catch { /* interceptor */ }
  finally { preAuditing.value = false }
}

function doReject() {
  rejectNote.value = ''
  // If AI gave a "not_fixed" reasoning, pre-fill the reject note with
  // it — saves the admin retyping the reason.
  if (task.value?.pre_audit_verdict === 'not_fixed' && task.value?.pre_audit_result) {
    rejectNote.value = task.value.pre_audit_result
  }
  showReject.value = true
}

async function confirmReject() {
  const note = rejectNote.value.trim()
  if (!note) {
    ElMessage.warning('请填写打回原因')
    return
  }
  try {
    await api.coordRejectTask(task.value.id, { note })
    ElMessage.success('已打回，任务回到「开发中」')
    showReject.value = false
    await load()
  } catch { /* interceptor */ }
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
  titleDraft.value = task.value?.issue_title || ''
  // Hydrate snapshot draft from the currently-displayed snapshot JSON.
  // Missing fields default to empty strings so the bound inputs don't
  // show `undefined`.
  const snap = issueSnapshotParsed.value || {}
  snapshotDraft.value = {
    scope: snap.scope || '',
    evidence_code: snap.evidence_code || '',
    current_status: snap.current_status || '',
    fix_suggestion: snap.fix_suggestion || '',
  }
  editingNote.value = true
  // Smooth scroll to the editor so the user sees the textarea. Without
  // this the note panel is below the snapshot panel and a user clicking
  // the header "edit" button might not realize anything happened.
  // nextTick waits for `note-textarea` to be in the DOM before we
  // try to scroll/focus.
  nextTick(() => {
    if (noteAnchor.value) {
      noteAnchor.value.scrollIntoView({ behavior: 'smooth', block: 'center' })
      const ta = noteAnchor.value.querySelector('textarea')
      if (ta) ta.focus()
    }
  })
}

function cancelEditNote() {
  editingNote.value = false
  noteDraft.value = task.value?.note || ''
  titleDraft.value = task.value?.issue_title || ''
  // snapshotDraft will be re-hydrated on next enter; don't need to reset.
}

async function saveNote() {
  // Rebuild the snapshot JSON from the draft. Preserve any fields
  // that the AI put in the original but we don't expose for editing
  // (e.g. module, severity_reason) — merge them back in.
  const originalSnap = issueSnapshotParsed.value || {}
  const newSnap = {
    ...originalSnap,
    scope: snapshotDraft.value.scope,
    evidence_code: snapshotDraft.value.evidence_code,
    current_status: snapshotDraft.value.current_status,
    fix_suggestion: snapshotDraft.value.fix_suggestion,
  }
  const newSnapJson = JSON.stringify(newSnap)

  const titleChanged = titleDraft.value.trim() && titleDraft.value.trim() !== task.value.issue_title
  const noteChanged = noteDraft.value !== task.value.note
  const snapChanged = newSnapJson !== (task.value.issue_snapshot || '')
  if (!titleChanged && !noteChanged && !snapChanged) {
    editingNote.value = false
    return
  }
  if (!titleDraft.value.trim()) {
    ElMessage.warning('标题不能为空')
    return
  }
  sending.value = true
  try {
    await api.coordUpdateTask(task.value.id, {
      issue_title: titleDraft.value.trim(),
      note: noteDraft.value,
      issue_snapshot: snapChanged ? newSnapJson : '',
    })
    task.value.issue_title = titleDraft.value.trim()
    task.value.note = noteDraft.value
    if (snapChanged) task.value.issue_snapshot = newSnapJson
    editingNote.value = false
    ElMessage.success('已保存')
    // Refresh events so the "edit" timeline entry shows up.
    await load()
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

// Developer raises a question. Required input; empty prompt cancels.
async function doAsk() {
  try {
    const { value } = await ElMessageBox.prompt('描述你的问题（将推送给任务发起人）', '提问', {
      confirmButtonText: '转给发起人',
      cancelButtonText: '取消',
      inputType: 'textarea',
      inputPlaceholder: '例如：首页 hero 区的文案字数要求是 200 字正文还是含标题？',
      inputValidator: v => !!(v && v.trim()) || '请填写你的问题',
    })
    sending.value = true
    await api.coordAskTask(task.value.id, { note: value })
    ElMessage.success('已转给发起人')
    await load()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
  } finally { sending.value = false }
}

// Creator answers a developer's question. Uses the always-visible
// `answerDraft` in the header (not a prompt box, since the answer is
// often long and the user wants to reference the question above it).
async function doAnswer() {
  const text = answerDraft.value.trim()
  if (!text) {
    ElMessage.warning('请填写回复内容')
    return
  }
  sending.value = true
  try {
    await api.coordAnswerTask(task.value.id, { note: text })
    answerDraft.value = ''
    await load()
    ElMessage.success('已回复，任务重新回到开发中')
  } catch { /* interceptor showed */ }
  finally { sending.value = false }
}

// Open the hidden file picker. We use a plain <input type="file"> so the
// user sees the native OS picker (which supports drag, preview, multi-
// select filtering, etc. depending on the browser).
function pickFile() {
  if (fileInputRef.value) fileInputRef.value.click()
}

async function onFilePicked(ev) {
  const files = Array.from(ev.target?.files || [])
  ev.target.value = '' // reset so picking the same file twice still triggers
  if (!files.length) return

  uploading.value = true
  try {
    // Upload one at a time so we can show per-file errors. Could be
    // parallelised, but sequential keeps the UI obvious.
    for (const f of files) {
      const fd = new FormData()
      fd.append('file', f)
      try {
        await api.coordUploadAttachment(task.value.id, fd)
      } catch (e) {
        // Single-file failure shouldn't abort the batch; surface a
        // message pointing at which file failed.
        ElMessage.error(`${f.name} 上传失败：${e?.response?.data?.error || '未知错误'}`)
      }
    }
    await load()
    ElMessage.success('上传完成')
  } finally {
    uploading.value = false
  }
}

async function deleteAttachment(att) {
  try {
    await ElMessageBox.confirm(`确认删除附件「${att.filename}」？`, '删除附件', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await api.coordDeleteAttachment(att.id)
    await load()
    ElMessage.success('已删除')
  } catch { /* interceptor */ }
}

function attachmentUrl(id) {
  return api.coordAttachmentUrl(id)
}

function fmtSize(bytes) {
  if (!bytes) return ''
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1024 / 1024).toFixed(1) + ' MB'
}

function fileIcon(mime) {
  if (!mime) return '📄'
  if (mime === 'application/pdf') return '📕'
  if (mime.includes('word')) return '📘'
  if (mime.includes('sheet') || mime.includes('excel')) return '📗'
  if (mime.startsWith('text/')) return '📝'
  return '📎'
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
.issue-title-input { flex:1; font-size:20px; font-weight:800; color:var(--ink); padding:6px 12px; border:2px solid var(--sun); border-radius:var(--r-sm); outline:none; background:#FFFDF8; font-family:inherit; line-height:1.35; }
.issue-title-input:focus { box-shadow:0 0 0 3px rgba(249,115,22,.15); }
.editing-badge { display:inline-block; margin-left:10px; padding:2px 10px; font-size:11px; font-weight:800; color:var(--sun-deep); background:var(--sun-light); border:1px solid var(--peach); border-radius:10px; animation: blink 2s ease-in-out infinite; }
.sev-tag { display:inline-block; padding:3px 10px; border-radius:4px; font-size:12px; font-weight:800; color:#fff; }
.sev-fatal { background:#C0392B; } .sev-serious { background:#E67E22; } .sev-medium { background:#F39C12; } .sev-low { background:#27AE60; } .sev-none { background:#94A3B8; }
.mod-chip { padding:2px 9px; background:#34495E; color:#ECF0F1; border-radius:3px; font-size:11px; font-weight:700; }

.status-tag { display:inline-block; padding:5px 14px; border-radius:20px; font-size:12.5px; font-weight:800; }
.st-pending { background:#DBEAFE; color:#1D4ED8; }
.st-in_progress { background:#FEF3C7; color:#92400E; }
.st-awaiting_answer { background:#FEE2E2; color:#B91C1C; animation: blink 2s ease-in-out infinite; }
.st-under_review { background:#EDE9FE; color:#6D28D9; }
.st-done { background:var(--ok-bg); color:var(--ok); }
@keyframes blink { 0%,100% { opacity: 1; } 50% { opacity: .55; } }

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

/* Snapshot edit mode: labeled fields with focused borders */
.edit-field { margin-bottom:14px; }
.edit-field-label { font-size:12.5px; font-weight:700; color:var(--ink-mid); margin-bottom:6px; }
.edit-field-label.current-label { color:#C0392B; }
.edit-field-label.fix-label { color:#27AE60; }
.snap-input { width:100%; box-sizing:border-box; padding:9px 12px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13.5px; font-family:inherit; outline:none; background:#FFFDF9; }
.snap-input:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); }
.snap-textarea { width:100%; box-sizing:border-box; padding:10px 12px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13px; font-family:inherit; line-height:1.7; resize:vertical; outline:none; background:#FFFDF9; }
.snap-textarea:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); }
.snap-textarea.mono { font-family:'JetBrains Mono', monospace; font-size:12px; background:#F8F9FA; }

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
.btn-xs-primary { background:var(--sun); color:#fff; border-color:var(--sun); }
.btn-xs-primary:hover:not(:disabled) { background:var(--sun-deep); border-color:var(--sun-deep); color:#fff; }
.btn-xs:disabled { opacity:.55; cursor:not-allowed; }

.btn-ask { background:linear-gradient(135deg,#F59E0B,#EA580C); color:#fff; border-color:transparent; }
.btn-ask:hover:not(:disabled) { filter:brightness(1.08); color:#fff; }
.btn-edit { background:#FFFDF8; color:var(--sun-deep); border-color:var(--peach); }
.btn-edit:hover:not(:disabled) { background:var(--sun-light); }

.lock-hint { font-size:11.5px; font-weight:600; color:var(--ink-hint); margin-left:10px; }

/* Q&A pending-answer prominent box in the header */
.pending-answer-box { margin-top:18px; padding:16px 18px; background:linear-gradient(135deg,#FFF1F2,#FFF); border:2px solid #FCA5A5; border-radius:var(--r-sm); }
.pending-answer-box .pa-title { font-size:13px; font-weight:800; color:#B91C1C; margin-bottom:10px; }
.pending-answer-box .pa-question { font-size:14px; color:var(--ink); line-height:1.65; margin-bottom:14px; padding:12px 14px; background:#FFF; border-left:3px solid #DC2626; border-radius:6px; white-space:pre-wrap; }
.pending-answer-box .pa-input { width:100%; box-sizing:border-box; padding:10px 12px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13.5px; font-family:inherit; resize:vertical; outline:none; margin-bottom:10px; }
.pending-answer-box .pa-input:focus { border-color:#DC2626; box-shadow:0 0 0 3px rgba(220,38,38,.12); }

/* Attachment panel */
.attach-panel { padding:20px 26px; }
.hint-sub { font-size:11.5px; color:var(--ink-hint); margin-top:4px; font-weight:500; }

.img-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(180px,1fr)); gap:12px; margin-top:8px; }
.img-cell { border:1.5px solid var(--border-light); border-radius:var(--r-sm); overflow:hidden; background:var(--surface-2); }
.img-cell a { display:block; width:100%; aspect-ratio:4/3; overflow:hidden; background:#F3F4F6; }
.img-cell img { width:100%; height:100%; object-fit:cover; display:block; transition:transform .2s; }
.img-cell a:hover img { transform:scale(1.04); }
.img-meta { display:flex; align-items:center; justify-content:space-between; padding:6px 10px; gap:6px; border-top:1px solid var(--border-light); background:var(--surface); }
.img-name { font-size:11.5px; color:var(--ink-mid); overflow:hidden; text-overflow:ellipsis; white-space:nowrap; flex:1; }
.img-del { background:none; border:none; color:var(--ink-hint); cursor:pointer; font-size:13px; font-weight:700; padding:2px 6px; border-radius:4px; }
.img-del:hover { background:var(--err); color:#fff; }

.file-list { display:flex; flex-direction:column; gap:6px; margin-top:12px; }
.file-row { display:flex; align-items:center; gap:10px; padding:8px 12px; background:var(--surface-2); border:1px solid var(--border-light); border-radius:var(--r-sm); font-size:13px; }
.file-icon { font-size:18px; }
.file-name { flex:1; color:var(--info); font-weight:600; text-decoration:none; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.file-name:hover { text-decoration:underline; }
.file-size { font-size:11.5px; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; min-width:60px; text-align:right; }
.file-time { font-size:11.5px; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; }

/* Additional timeline dot colors for new event types */
.tl-item.tl-ask .tl-dot { background:#F59E0B; border-color:#F59E0B; }
.tl-item.tl-answer .tl-dot { background:#3B82F6; border-color:#3B82F6; }
.tl-item.tl-upload .tl-dot { background:#10B981; border-color:#10B981; }
.tl-item.tl-edit .tl-dot { background:var(--info); border-color:var(--info); }
.tl-item.tl-attach_delete .tl-dot { background:var(--err); border-color:var(--err); }
.tl-item.tl-pre_audit .tl-dot { background:#8B5CF6; border-color:#8B5CF6; }
.tl-item.tl-reject .tl-dot { background:var(--err); border-color:var(--err); }

/* ── Pre-audit (待验收) panel ─────────────────────────────────────────── */
.preaudit-panel { padding:20px 26px; margin-bottom:16px; border:1.5px solid #C7D2FE; background:linear-gradient(180deg, #F5F3FF 0%, var(--surface) 55%); }
.pa-subtitle { font-size:12px; font-weight:500; color:var(--ink-hint); margin-left:10px; }

/* AI verdict box — color-coded by verdict value */
.verdict-box { margin:14px 0 18px; padding:14px 18px; border-radius:var(--r-sm); border:1.5px solid; }
.verdict-box.vb-fixed { background:#ECFDF5; border-color:#A7F3D0; }
.verdict-box.vb-not_fixed { background:#FEF2F2; border-color:#FECACA; }
.verdict-box.vb-unclear { background:#FFFBEB; border-color:#FCD34D; }
.vb-head { display:flex; align-items:center; gap:14px; margin-bottom:8px; }
.vb-badge { font-size:14px; font-weight:800; }
.vb-fixed .vb-badge { color:#047857; }
.vb-not_fixed .vb-badge { color:#B91C1C; }
.vb-unclear .vb-badge { color:#92400E; }
.vb-time { font-size:11.5px; color:var(--ink-hint); font-family:'JetBrains Mono', monospace; margin-left:auto; }
.vb-reasoning { font-size:13.5px; line-height:1.7; color:var(--ink); white-space:pre-wrap; word-break:break-word; }
.vb-hint { margin-top:8px; font-size:11.5px; color:var(--ink-hint); font-style:italic; }

/* Test pages header bar */
.test-pages-head { display:flex; align-items:center; justify-content:space-between; margin:14px 0 10px; }
.tp-title { font-size:13.5px; font-weight:800; color:var(--ink-mid); }
.tp-actions { display:flex; gap:8px; }
.btn-primary-soft { background:#EEF2FF; color:#4338CA; border-color:#C7D2FE; }
.btn-primary-soft:hover:not(:disabled) { background:#C7D2FE; color:#3730A3; }
.btn-polish { background:linear-gradient(135deg, #8B5CF6, #6366F1); color:#fff; border-color:transparent; }
.btn-polish:hover:not(:disabled) { filter:brightness(1.08); color:#fff; }
.btn-danger-soft-xs { color:var(--err); border-color:#FFCDD2; background:#FFF5F5; padding:4px 10px; font-size:11.5px; }
.btn-danger-soft-xs:hover:not(:disabled) { background:var(--err); color:#fff; }

/* Test pages table */
.tp-table { width:100%; border-collapse:collapse; background:var(--surface); border-radius:var(--r-sm); overflow:hidden; border:1px solid var(--border-light); }
.tp-table th { padding:8px 12px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.06em; background:var(--surface-3); border-bottom:1.5px solid var(--border); }
.tp-table td { padding:8px 12px; border-bottom:1px solid var(--border-light); font-size:13px; vertical-align:middle; }
.tp-table tr:last-child td { border-bottom:none; }
.tp-name { font-weight:700; color:var(--ink); }
.tp-url { font-family:'JetBrains Mono', monospace; font-size:11.5px; max-width:260px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.tp-url a { color:var(--sun-deep); text-decoration:none; }
.tp-url a:hover { text-decoration:underline; }
.tp-status { font-size:11px; font-weight:700; padding:2px 9px; border-radius:10px; font-family:'JetBrains Mono', monospace; }
.tp-status.tp-ok { background:var(--ok-bg); color:var(--ok); border:1px solid var(--ok-border); }
.tp-status.tp-err { background:#FEE2E2; color:#B91C1C; border:1px solid #FECACA; }
.tp-status.tp-none { background:var(--surface-3); color:var(--ink-hint); border:1px solid var(--border); }
.tp-time { font-size:11.5px; color:var(--ink-hint); font-family:'JetBrains Mono', monospace; }
.tp-empty { padding:24px; text-align:center; color:var(--ink-hint); font-size:13px; font-style:italic; background:var(--surface-2); border:1.5px dashed var(--border); border-radius:var(--r-sm); }

/* Source view dialog */
.src-meta { display:flex; gap:18px; font-size:12px; color:var(--ink-mid); padding:8px 12px; background:var(--surface-3); border-radius:var(--r-sm); margin-bottom:10px; }
.src-meta a { color:var(--sun-deep); font-family:'JetBrains Mono', monospace; }
.src-html { white-space:pre-wrap; word-break:break-all; padding:12px 14px; background:#1E293B; color:#E2E8F0; font-family:'JetBrains Mono', monospace; font-size:11.5px; line-height:1.6; max-height:60vh; overflow:auto; border-radius:var(--r-sm); margin:0; }

/* Reject dialog */
.reject-hint { font-size:13px; color:var(--ink-mid); margin-bottom:10px; padding:8px 12px; background:#FEF2F2; border-left:3px solid var(--err); border-radius:6px; line-height:1.6; }
.reject-textarea { width:100%; box-sizing:border-box; padding:10px 12px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13.5px; font-family:inherit; line-height:1.7; resize:vertical; outline:none; background:var(--surface); min-height:110px; }
.reject-textarea:focus { border-color:var(--err); box-shadow:0 0 0 3px rgba(239,68,68,.12); }
</style>
