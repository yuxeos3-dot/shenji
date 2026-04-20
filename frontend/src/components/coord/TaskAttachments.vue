<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../../api'

const props = defineProps<{
  taskId: number | string
  attachments: any[]
  isLocked: boolean
}>()

const emit = defineEmits<{
  refresh: []
}>()

const uploading = ref(false)

function isImage(name: string) {
  return /\.(png|jpe?g|gif|webp|svg|bmp)$/i.test(name || '')
}

async function handleUpload(evt: Event) {
  const input = evt.target as HTMLInputElement
  if (!input.files?.length) return
  uploading.value = true
  const fd = new FormData()
  for (const f of input.files) fd.append('file', f)
  try {
    await api.coordUploadAttachment(props.taskId, fd)
    ElMessage.success('附件已上传')
    emit('refresh')
  } catch (e: any) {
    ElMessage.error(e?.message || '上传失败')
  } finally {
    uploading.value = false
    input.value = ''
  }
}

async function deleteAttachment(att: any) {
  try {
    await ElMessageBox.confirm(`确定删除「${att.original_name}」？`, '删除附件', { type: 'warning' })
    await api.coordDeleteAttachment(att.id)
    ElMessage.success('附件已删除')
    emit('refresh')
  } catch {}
}
</script>

<template>
  <div class="panel attach-panel">
    <div class="section-title">
      附件
      <label v-if="!isLocked" class="upload-btn">
        <input type="file" multiple @change="handleUpload" style="display:none" />
        {{ uploading ? '上传中...' : '📎 上传' }}
      </label>
    </div>
    <div v-if="!attachments.length" class="empty-hint">暂无附件</div>
    <div v-else class="attach-grid">
      <div v-for="att in attachments" :key="att.id" class="attach-card">
        <a
          v-if="isImage(att.original_name)"
          :href="api.coordAttachmentUrl(att.id)"
          target="_blank"
          class="attach-thumb"
        >
          <img :src="api.coordAttachmentUrl(att.id)" :alt="att.original_name" />
        </a>
        <div class="attach-info">
          <a :href="api.coordAttachmentUrl(att.id)" target="_blank" class="attach-name">
            {{ att.original_name }}
          </a>
          <span class="attach-size">{{ att.size_display }}</span>
          <button
            v-if="!isLocked"
            class="attach-del"
            @click="deleteAttachment(att)"
          >🗑️</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.panel { background: var(--surface, #fff); border: 1.5px solid var(--border-light, #F3EDE7); border-radius: var(--r, 14px); box-shadow: var(--s1); padding: 20px 24px; margin-bottom: 20px; }
.section-title { font-size: 15px; font-weight: 800; color: var(--ink, #2D1B0E); margin-bottom: 16px; display: flex; align-items: center; gap: 8px; }
.empty-hint { color: var(--ink-hint, #BCA898); text-align: center; padding: 20px; font-size: 13px; }
.upload-btn { font-size: 12px; font-weight: 700; cursor: pointer; padding: 4px 12px; border-radius: 6px; background: var(--sun-light, #FFF8F0); color: var(--sun-deep, #F57C00); border: 1px solid var(--peach, #FFD6A5); }
.upload-btn:hover { background: var(--sun-glow, #FFF0DE); }
.attach-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 12px; }
.attach-card { border: 1px solid var(--border-light, #F3EDE7); border-radius: 8px; overflow: hidden; }
.attach-thumb { display: block; }
.attach-thumb img { width: 100%; height: 120px; object-fit: cover; }
.attach-info { padding: 8px 10px; display: flex; align-items: center; gap: 6px; }
.attach-name { font-size: 12px; font-weight: 600; color: var(--sun, #FF8C00); text-decoration: none; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.attach-size { font-size: 11px; color: var(--ink-hint, #BCA898); }
.attach-del { background: none; border: none; cursor: pointer; font-size: 14px; opacity: 0.5; }
.attach-del:hover { opacity: 1; }
</style>
