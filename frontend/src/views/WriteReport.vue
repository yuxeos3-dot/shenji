<template>
  <div>
    <el-page-header @back="$router.push('/reports')">
      <template #content><span>写汇报 - {{ today }}</span></template>
    </el-page-header>

    <el-card style="margin-top:16px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="工作内容">
          <el-input v-model="form.content" type="textarea" :rows="6" placeholder="今日工作内容（支持Markdown）" />
        </el-form-item>
        <el-form-item label="完成任务">
          <div v-for="(task, idx) in form.tasks_done" :key="idx" style="display:flex;margin-bottom:8px;width:100%">
            <el-input v-model="form.tasks_done[idx]" style="flex:1" />
            <el-button type="danger" text @click="form.tasks_done.splice(idx,1)"><el-icon><Delete /></el-icon></el-button>
          </div>
          <el-button type="primary" text @click="form.tasks_done.push('')"><el-icon><Plus /></el-icon> 添加</el-button>
        </el-form-item>
        <el-form-item label="明日计划">
          <div v-for="(task, idx) in form.tasks_todo" :key="idx" style="display:flex;margin-bottom:8px;width:100%">
            <el-input v-model="form.tasks_todo[idx]" style="flex:1" />
            <el-button type="danger" text @click="form.tasks_todo.splice(idx,1)"><el-icon><Delete /></el-icon></el-button>
          </div>
          <el-button type="primary" text @click="form.tasks_todo.push('')"><el-icon><Plus /></el-icon> 添加</el-button>
        </el-form-item>
        <el-form-item label="问题与风险">
          <el-input v-model="form.issues" type="textarea" :rows="3" placeholder="遇到的问题或风险（可选）" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="submit" :loading="submitting">提交汇报</el-button>
        </el-form-item>
      </el-form>
    </el-card>
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
const form = ref({
  content: '',
  tasks_done: [''],
  tasks_todo: [''],
  issues: '',
})

onMounted(async () => {
  try {
    const res = await api.getReport(today)
    if (res.data) {
      form.value.content = res.data.content
      const done = typeof res.data.tasks_done === 'string' ? JSON.parse(res.data.tasks_done) : res.data.tasks_done
      const todo = typeof res.data.tasks_todo === 'string' ? JSON.parse(res.data.tasks_todo) : res.data.tasks_todo
      form.value.tasks_done = done.length ? done : ['']
      form.value.tasks_todo = todo.length ? todo : ['']
      form.value.issues = res.data.issues
    }
  } catch {}
})

async function submit() {
  if (!form.value.content) { ElMessage.warning('请填写工作内容'); return }
  submitting.value = true
  try {
    await api.submitReport({
      content: form.value.content,
      tasks_done: form.value.tasks_done.filter(t => t.trim()),
      tasks_todo: form.value.tasks_todo.filter(t => t.trim()),
      issues: form.value.issues,
    })
    ElMessage.success('汇报提交成功')
    router.push('/reports')
  } catch {} finally { submitting.value = false }
}
</script>
