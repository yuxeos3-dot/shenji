<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push('/reports')" class="back-btn">← 返回汇报列表</a>
      <span class="back-sep">·</span>
      <span class="back-url">写汇报 · {{ today }}</span>
    </div>

    <div class="panel">
      <div class="form-group">
        <label>📝 工作内容</label>
        <textarea v-model="form.content" rows="6" placeholder="今日工作内容（支持Markdown）"></textarea>
      </div>

      <div class="form-group">
        <label>✅ 完成任务</label>
        <div v-for="(task, idx) in form.tasks_done" :key="'d'+idx" class="task-row">
          <input v-model="form.tasks_done[idx]" placeholder="已完成的任务" />
          <button class="btn-icon" @click="form.tasks_done.splice(idx,1)">✕</button>
        </div>
        <button class="btn btn-xs" @click="form.tasks_done.push('')" style="margin-top:6px">+ 添加</button>
      </div>

      <div class="form-group">
        <label>📌 明日计划</label>
        <div v-for="(task, idx) in form.tasks_todo" :key="'t'+idx" class="task-row">
          <input v-model="form.tasks_todo[idx]" placeholder="明日计划" />
          <button class="btn-icon" @click="form.tasks_todo.splice(idx,1)">✕</button>
        </div>
        <button class="btn btn-xs" @click="form.tasks_todo.push('')" style="margin-top:6px">+ 添加</button>
      </div>

      <div class="form-group">
        <label>⚠️ 问题与风险</label>
        <textarea v-model="form.issues" rows="3" placeholder="遇到的问题或风险（可选）"></textarea>
      </div>

      <div class="form-actions">
        <button class="btn btn-primary" @click="submit" :disabled="submitting">
          {{ submitting ? '提交中...' : '☀ 提交汇报' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'

const router = useRouter()
const today = new Date().toISOString().slice(0, 10)
const submitting = ref(false)
const form = ref({ content: '', tasks_done: [''], tasks_todo: [''], issues: '' })

onMounted(async () => {
  try {
    const res = await api.getReport(today)
    if (res.data) {
      form.value.content = res.data.content
      const d = typeof res.data.tasks_done === 'string' ? JSON.parse(res.data.tasks_done) : res.data.tasks_done
      const t = typeof res.data.tasks_todo === 'string' ? JSON.parse(res.data.tasks_todo) : res.data.tasks_todo
      form.value.tasks_done = d.length ? d : ['']
      form.value.tasks_todo = t.length ? t : ['']
      form.value.issues = res.data.issues
    }
  } catch {}
})

async function submit() {
  if (!form.value.content) { ElMessage.warning('请填写工作内容'); return }
  submitting.value = true
  try {
    await api.submitReport({ content: form.value.content, tasks_done: form.value.tasks_done.filter(t => t.trim()), tasks_todo: form.value.tasks_todo.filter(t => t.trim()), issues: form.value.issues })
    ElMessage.success('汇报提交成功')
    router.push('/reports')
  } catch {} finally { submitting.value = false }
}
</script>

<style scoped>
.back-row { display:flex; align-items:center; gap:8px; margin-bottom:18px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); }
.back-sep { color:var(--ink-hint); }
.back-url { font-size:14px; font-weight:700; }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }

.form-group { margin-bottom:20px; }
.form-group label { display:block; margin-bottom:7px; font-weight:700; color:var(--ink-mid); font-size:13px; }
.form-group textarea, .form-group input {
  width:100%; padding:10px 13px; border:1.5px solid var(--border); border-radius:9px;
  font-size:14px; font-family:inherit; background:var(--surface); color:var(--ink);
  transition:border-color .18s, box-shadow .18s; outline:none; line-height:1.65;
}
.form-group textarea:focus, .form-group input:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.14); background:#FFFDF9; }
.form-group textarea { resize:vertical; min-height:80px; }

.task-row { display:flex; gap:8px; margin-bottom:6px; }
.task-row input { flex:1; }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep),var(--sun)); color:#fff; box-shadow:0 8px 24px rgba(249,115,22,.48); }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; }
.btn-xs { padding:4px 12px; font-size:12px; border-radius:6px; }
.btn-xs:hover { background:var(--sun-glow); border-color:var(--sun); color:var(--sun-deep); }
.btn-icon { background:none; border:none; cursor:pointer; font-size:16px; color:var(--err); padding:4px; transition:opacity .15s; }
.btn-icon:hover { opacity:.7; }
.form-actions { margin-top:22px; }
</style>
