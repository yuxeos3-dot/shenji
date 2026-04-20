<script setup lang="ts">
defineProps<{
  events: any[]
  isLocked: boolean
}>()

const emit = defineEmits<{
  addComment: [text: string]
}>()

import { ref } from 'vue'

const commentDraft = ref('')
const sending = ref(false)

function fmtTime(s: string) {
  if (!s) return '—'
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function eventLabel(t: string) {
  const m: Record<string, string> = {
    created: '创建', assigned: '分配', submitted: '提交', verified: '验收',
    comment: '备注', status_change: '状态变更', rejected: '驳回',
    ask: '提问', answer: '回答', ping_dev: '催促开发',
    review_approved: '审核通过', review_rejected: '审核拒绝',
    review_started: '开始审核',
  }
  return m[t] || t
}

function statusLabel(s: string) {
  const m: Record<string, string> = {
    pending: '待处理', pending_review: '待审核', in_progress: '进行中',
    awaiting_answer: '等待回答', under_review: '待验收', done: '已完成',
  }
  return m[s] || s
}

async function doAddComment() {
  if (!commentDraft.value.trim()) return
  sending.value = true
  emit('addComment', commentDraft.value.trim())
  commentDraft.value = ''
  sending.value = false
}
</script>

<template>
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
            <span class="tl-actor">
              {{ ev.actor_name || '系统' }}
              <span v-if="ev.actor_dept || ev.actor_group" class="tl-actor-meta">
                ({{ [ev.actor_dept, ev.actor_group].filter(Boolean).join(' · ') }})
              </span>
            </span>
            <span class="tl-time">{{ fmtTime(ev.created_at) }}</span>
          </div>
          <div v-if="ev.from_status !== ev.to_status" class="tl-transition">
            {{ statusLabel(ev.from_status) || '—' }} → {{ statusLabel(ev.to_status) }}
          </div>
          <div v-if="ev.note" class="tl-note">{{ ev.note }}</div>
        </div>
      </div>
    </div>
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

<style scoped>
.panel { background: var(--surface, #fff); border: 1.5px solid var(--border-light, #F3EDE7); border-radius: var(--r, 14px); box-shadow: var(--s1); padding: 20px 24px; margin-bottom: 20px; }
.section-title { font-size: 15px; font-weight: 800; color: var(--ink, #2D1B0E); margin-bottom: 16px; display: flex; align-items: center; gap: 8px; }
.empty-hint { color: var(--ink-hint, #BCA898); text-align: center; padding: 20px; font-size: 13px; }
.timeline { position: relative; padding-left: 24px; }
.tl-item { position: relative; padding-bottom: 20px; padding-left: 16px; border-left: 2px solid var(--border-light, #F3EDE7); }
.tl-item:last-child { border-left-color: transparent; }
.tl-dot { position: absolute; left: -7px; top: 4px; width: 12px; height: 12px; border-radius: 50%; background: var(--border, #E8DDD4); border: 2px solid var(--surface, #fff); }
.tl-created .tl-dot { background: var(--sun, #FF8C00); }
.tl-assigned .tl-dot, .tl-review_started .tl-dot { background: var(--info, #2563EB); }
.tl-submitted .tl-dot { background: var(--gold, #FFB300); }
.tl-verified .tl-dot { background: var(--ok, #16A34A); }
.tl-rejected .tl-dot, .tl-review_rejected .tl-dot { background: var(--err, #DC2626); }
.tl-comment .tl-dot { background: var(--ink-soft, #8D7060); }
.tl-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.tl-type { font-weight: 700; font-size: 13px; color: var(--ink, #2D1B0E); }
.tl-actor { font-size: 12.5px; color: var(--ink-mid, #5D4037); }
.tl-actor-meta { color: var(--ink-hint, #BCA898); font-size: 11.5px; }
.tl-time { font-size: 11.5px; color: var(--ink-hint, #BCA898); margin-left: auto; }
.tl-transition { font-size: 12px; color: var(--ink-soft, #8D7060); margin-top: 4px; }
.tl-note { font-size: 13px; color: var(--ink-mid, #5D4037); margin-top: 6px; line-height: 1.6; white-space: pre-wrap; background: var(--surface-2, #FEFCFA); padding: 8px 12px; border-radius: 8px; }
.comment-row { display: flex; gap: 10px; align-items: flex-start; margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--border-light, #F3EDE7); }
.comment-input { flex: 1; border: 1.5px solid var(--border, #E8DDD4); border-radius: 8px; padding: 8px 12px; font-size: 13px; font-family: inherit; resize: vertical; }
.comment-input:focus { outline: none; border-color: var(--sun, #FF8C00); }
.btn { padding: 8px 16px; border: none; border-radius: 8px; font-weight: 700; font-size: 13px; cursor: pointer; background: linear-gradient(135deg, var(--sun, #FF8C00), var(--coral, #FF9F43)); color: #fff; white-space: nowrap; }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-sm { padding: 6px 14px; font-size: 12px; }
</style>
