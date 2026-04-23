<template>
  <div class="media-gen">
    <!-- Header -->
    <div class="mg-header">
      <h2>{{ kindLabel }}管理</h2>
      <div class="mg-header-actions">
        <button class="btn btn-primary" @click="showGenDialog = true">AI生成</button>
        <button class="btn btn-secondary" @click="showUploadDialog = true">上传</button>
        <button class="btn btn-secondary" @click="loadList">刷新</button>
      </div>
    </div>

    <!-- Filter bar -->
    <div class="mg-filter-bar">
      <select v-model="filterMode" @change="loadList">
        <option value="">全部</option>
        <option :value="textModeKey">文生{{ kindShort }}</option>
        <option :value="imgModeKey">图生{{ kindShort }}</option>
        <option value="upload">本地上传</option>
      </select>
      <select v-model="filterStatus" @change="loadList">
        <option value="">全部</option>
        <option value="queued">排队中</option>
        <option value="processing">生成中</option>
        <option value="completed">已完成</option>
        <option value="failed">失败</option>
      </select>
      <input
        v-model="searchText"
        type="text"
        placeholder="搜索提示词..."
        @keyup.enter="loadList"
      />
    </div>

    <!-- Stats line -->
    <div class="mg-stats">
      <span>共 {{ totalCount }} 条</span>
      <span>已发布 {{ publishedCount }} 条</span>
    </div>

    <!-- Table -->
    <div class="mg-table-wrap" v-loading="loading">
      <table class="mg-table">
        <thead>
          <tr>
            <th class="col-check">
              <input type="checkbox" @change="toggleAll($event)" />
            </th>
            <th class="col-thumb">缩略图</th>
            <th class="col-prompt">提示词</th>
            <th class="col-mode">模式</th>
            <th class="col-status">状态</th>
            <th class="col-time">时间</th>
            <th class="col-actions">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in list" :key="item.id">
            <td class="col-check">
              <input type="checkbox" :checked="checked.has(item.id)" @change="toggleCheck(item.id)" />
            </td>
            <td class="col-thumb">
              <img
                v-if="item.result_url || item.output_url"
                :src="item.result_url || item.output_url"
                class="thumb"
                @click="previewItem = item"
              />
              <div v-else class="thumb-placeholder">--</div>
            </td>
            <td class="col-prompt">{{ item.prompt || "--" }}</td>
            <td class="col-mode">
              <span class="tag tag-mode">{{ modeLabel(item.mode) }}</span>
            </td>
            <td class="col-status">
              <div class="status-wrap">
                <span :class="['tag', statusTagClass(item.status)]">{{ statusLabel(item.status) }}</span>
                <div v-if="isProcessing(item.status)" class="progress-bar">
                  <div class="progress-fill" :style="{width: (item.progress || 0) + '%'}"></div>
                  <span class="progress-text">{{ Math.round(item.progress || 0) }}%</span>
                </div>
              </div>
            </td>
            <td class="col-time">{{ fmtTime(item.created_at) }}</td>
            <td class="col-actions">
              <button class="btn-sm" @click="openDetail(item)">详情</button>
              <button class="btn-sm" @click="publishOne(item)" :disabled="!isCompleted(item.status)">发布</button>
              <button class="btn-sm" @click="copyURL(item)" :disabled="!(item.result_url || item.output_url)">复制</button>
              <button class="btn-sm btn-warn" v-if="isFailed(item.status)" @click="retryItem(item)">重试</button>
              <button class="btn-sm btn-danger" @click="deleteOne(item)">删除</button>
            </td>
          </tr>
          <tr v-if="list.length === 0 && !loading">
            <td colspan="7" class="empty-row">暂无数据</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Bottom bar -->
    <div class="mg-bottom-bar">
      <div class="mg-batch" v-if="checked.size > 0">
        <span>已选 {{ checked.size }} 项</span>
        <button class="btn-sm" @click="batchPublish">批量发布</button>
        <button class="btn-sm btn-danger" @click="batchDelete">批量删除</button>
      </div>
      <div class="mg-pagination">
        <button class="btn-sm" :disabled="page <= 1" @click="page--; loadList()">上一页</button>
        <span>第 {{ page }} 页</span>
        <button class="btn-sm" :disabled="list.length < perPage" @click="page++; loadList()">下一页</button>
        <select v-model.number="perPage" @change="page = 1; loadList()">
          <option :value="10">每馄10条</option>
          <option :value="20">每馄20条</option>
          <option :value="50">每馄50条</option>
        </select>
      </div>
    </div>

    <!-- AI Generate Dialog -->
    <el-dialog v-model="showGenDialog" title="AI生成" width="520px" :close-on-click-modal="false">
      <div class="gen-form">
        <div class="gen-field">
          <label>模式</label>
          <select v-model="genMode">
            <option :value="textModeKey">文生{{ kindShort }}</option>
            <option :value="imgModeKey">图生{{ kindShort }}</option>
          </select>
        </div>
        <div class="gen-field">
          <label>提示词</label>
          <textarea v-model="genPrompt" rows="4" placeholder="请输入提示词..."></textarea>
        </div>
        <div class="gen-field" v-if="genMode === imgModeKey">
          <label>源图URL</label>
          <input v-model="genSourceUrl" type="text" placeholder="输入源图片URL" />
        </div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showGenDialog = false">取消</button>
        <button class="btn btn-primary" @click="doGenerate" :disabled="!genPrompt.trim()">提交生成</button>
      </template>
    </el-dialog>

    <!-- Upload Dialog -->
    <el-dialog v-model="showUploadDialog" title="上传文件" width="520px" :close-on-click-modal="false">
      <div class="upload-form">
        <div
          class="upload-drop-zone"
          @dragover.prevent="dragOver = true"
          @dragleave="dragOver = false"
          @drop.prevent="handleDrop"
          :class="{ 'drag-active': dragOver }"
        >
          <p>拖拽文件到此处，或</p>
          <label class="btn btn-secondary upload-btn">
            选择文件
            <input type="file" multiple :accept="kind === 'image' ? 'image/*' : 'video/*'" @change="handleFileSelect" hidden />
          </label>
        </div>
        <ul class="upload-file-list" v-if="uploadFiles.length > 0">
          <li v-for="(f, idx) in uploadFiles" :key="idx">
            <span>{{ f.name }} ({{ formatSize(f.size) }})</span>
            <button class="btn-sm btn-danger" @click="uploadFiles.splice(idx, 1)">移除</button>
          </li>
        </ul>
        <div class="gen-field">
          <label>描述（可选）</label>
          <input v-model="uploadDesc" type="text" placeholder="可选描述" />
        </div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showUploadDialog = false">取消</button>
        <button class="btn btn-primary" @click="doUpload" :disabled="uploadFiles.length === 0">上传</button>
      </template>
    </el-dialog>

    <!-- Detail Dialog -->
    <el-dialog v-model="showDetailDialog" :title="'任务 #' + (detailItem?.id || '')" width="620px">
      <div v-if="detailItem" class="detail-view">
        <div class="detail-preview" v-if="detailItem.result_url || detailItem.output_url">
          <img v-if="detailItem.kind === 'image'" :src="detailItem.result_url || detailItem.output_url" class="detail-media" />
          <video v-else :src="detailItem.result_url || detailItem.output_url" class="detail-media" controls></video>
        </div>
        <div class="detail-preview detail-placeholder" v-else-if="isProcessing(detailItem.status)">
          <div class="detail-loading">
            <div class="spinner"></div>
            <div>生成中 · {{ Math.round(detailItem.progress || 0) }}%</div>
          </div>
        </div>
        <div class="detail-preview detail-placeholder" v-else>
          <div>{{ statusLabel(detailItem.status) }}</div>
        </div>
        <div class="detail-meta">
          <div class="detail-row"><label>提示词</label><div class="detail-val">{{ detailItem.prompt || '--' }}</div></div>
          <div class="detail-row"><label>模式</label><div class="detail-val">{{ modeLabel(detailItem.mode) }}</div></div>
          <div class="detail-row"><label>状态</label><div class="detail-val">
            <span :class="['tag', statusTagClass(detailItem.status)]">{{ statusLabel(detailItem.status) }}</span>
            <span v-if="isProcessing(detailItem.status)" class="detail-prog">{{ Math.round(detailItem.progress || 0) }}%</span>
          </div></div>
          <div class="detail-row" v-if="detailItem.source_url"><label>参考图</label><div class="detail-val"><a :href="detailItem.source_url" target="_blank">{{ detailItem.source_url }}</a></div></div>
          <div class="detail-row"><label>创建时间</label><div class="detail-val">{{ fmtTime(detailItem.created_at) }}</div></div>
          <div class="detail-row" v-if="detailItem.error_msg"><label>错误信息</label><div class="detail-val detail-err">{{ detailItem.error_msg }}</div></div>
          <div class="detail-row" v-if="detailItem.result_url || detailItem.output_url"><label>结果URL</label><div class="detail-val">
            <a :href="detailItem.result_url || detailItem.output_url" target="_blank">{{ detailItem.result_url || detailItem.output_url }}</a>
          </div></div>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showDetailDialog = false">关闭</button>
        <button v-if="detailItem && isFailed(detailItem.status)" class="btn btn-primary" @click="retryItem(detailItem); showDetailDialog = false">重试</button>
        <button v-if="detailItem && isCompleted(detailItem.status)" class="btn btn-primary" @click="publishOne(detailItem)">发布</button>
      </template>
    </el-dialog>

    <!-- Lightbox -->
    <Teleport to="body">
      <div class="lightbox-overlay" v-if="previewItem" @click="previewItem = null">
        <div class="lightbox-content" @click.stop>
          <img v-if="kind === 'image'" :src="previewItem.output_url" />
          <video v-else :src="previewItem.output_url" controls autoplay></video>
          <button class="lightbox-close" @click="previewItem = null">&times;</button>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from "vue"
import { useRouter } from "vue-router"
import { ElMessage, ElMessageBox } from "element-plus"
import api from "../api"
import { copyToClipboard } from "../utils/clipboard"

const router = useRouter()

const props = defineProps<{
  kind: string
}>()

// State
const list = ref<any[]>([])
const loading = ref(false)
const page = ref(1)
const perPage = ref(20)
const totalCount = ref(0)
const publishedCount = ref(0)
const filterMode = ref("")
const filterStatus = ref("")
const searchText = ref("")
const checked = ref<Set<string>>(new Set())

// Dialogs
const showGenDialog = ref(false)
const showUploadDialog = ref(false)
const previewItem = ref<any>(null)

// Generate form
const genPrompt = ref("")
const genSourceUrl = ref("")

// Upload form
const uploadFiles = ref<File[]>([])
const uploadDesc = ref("")
const dragOver = ref(false)

// Computed
const textModeKey = computed(() => "text2" + props.kind)
const imgModeKey = computed(() => props.kind === "image" ? "img2img" : "img2video")
const kindLabel = computed(() => props.kind === "image" ? "图片" : "视频")
const kindShort = computed(() => props.kind === "image" ? "图" : "视频")
const genMode = ref("")

watch(() => props.kind, () => {
  genMode.value = textModeKey.value
}, { immediate: true })

// Polling
let pollTimer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  loadList()
  pollTimer = setInterval(() => {
    const hasActive = list.value.some(
      (item) => isProcessing(item.status)
    )
    if (hasActive) {
      loadList()
    }
  }, 3000)
})

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})

// Load list
async function loadList() {
  loading.value = true
  try {
    const params: any = {
      kind: props.kind,
      page: page.value,
      per_page: perPage.value,
    }
    if (filterMode.value) params.mode = filterMode.value
    if (filterStatus.value) params.status = filterStatus.value
    if (searchText.value.trim()) params.search = searchText.value.trim()

    const res = await api.listFRWGenerations(params)
    if (res.code === 0 && res.data) {
      list.value = res.data.items || []
      totalCount.value = res.data.total || 0
      publishedCount.value = res.data.published || 0
    }
  } catch (e) {
    console.error("loadList error:", e)
  } finally {
    loading.value = false
  }
}

// Generate
async function doGenerate() {
  const p = genPrompt.value.trim()
  if (!p) {
    ElMessage.warning("请输入提示词")
    return
  }
  if (genMode.value === imgModeKey.value && !genSourceUrl.value.trim()) {
    ElMessage.warning("图生模式需要参考图 URL")
    return
  }
  try {
    const payload: any = {
      kind: props.kind,
      mode: genMode.value,
      prompt: p,
    }
    if (genMode.value === imgModeKey.value && genSourceUrl.value.trim()) {
      payload.source_url = genSourceUrl.value.trim()
    }
    await api.submitFRWGeneration(payload)
    ElMessage.success("任务已提交，生成中...")
    showGenDialog.value = false
    genPrompt.value = ""
    genSourceUrl.value = ""
    loadList()
  } catch (e: any) {
    console.error("doGenerate error:", e)
    const msg = e?.response?.data?.error || e?.message || "提交失败"
    if (msg.includes("FRW") && (msg.includes("未配置") || msg.includes("API Key") || msg.includes("JWT") || msg.includes("Bearer"))) {
      ElMessageBox.confirm(
        `${msg}\n\n请到「系统设置」粘贴你的 FRW API Key 后再试。`,
        "FRW 配置问题",
        { confirmButtonText: "去配置", cancelButtonText: "取消", type: "warning" }
      ).then(() => {
        router.push("/settings")
      }).catch(() => {})
    } else {
      ElMessage({ message: msg, type: "error", duration: 6000, showClose: true })
    }
  }
}

// Retry a failed task — submit again with same params
async function retryItem(item: any) {
  try {
    const payload: any = {
      kind: item.kind,
      mode: item.mode,
      prompt: item.prompt,
    }
    if (item.source_url) payload.source_url = item.source_url
    await api.submitFRWGeneration(payload)
    ElMessage.success("已重新提交")
    loadList()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || "重试失败")
  }
}

// Detail dialog state
const detailItem = ref<any>(null)
const showDetailDialog = ref(false)

function openDetail(item: any) {
  detailItem.value = item
  showDetailDialog.value = true
}

// Upload
async function doUpload() {
  try {
    for (const file of uploadFiles.value) {
      const formData = new FormData()
      formData.append("file", file)
      formData.append("kind", props.kind)
      if (uploadDesc.value.trim()) {
        formData.append("description", uploadDesc.value.trim())
      }
      await api.uploadFRWMedia(formData)
    }
    showUploadDialog.value = false
    uploadFiles.value = []
    uploadDesc.value = ""
    loadList()
    ElMessage.success("上传成功")
  } catch (e: any) {
    console.error("doUpload error:", e)
    ElMessage.error(e?.response?.data?.error || "上传失败")
  }
}

function handleDrop(e: DragEvent) {
  dragOver.value = false
  if (e.dataTransfer?.files) {
    uploadFiles.value = [...uploadFiles.value, ...Array.from(e.dataTransfer.files)]
  }
}

function handleFileSelect(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files) {
    uploadFiles.value = [...uploadFiles.value, ...Array.from(input.files)]
  }
}

// Actions
async function publishOne(item: any) {
  try {
    await api.publishFRWGeneration(item.id)
    loadList()
  } catch (e) {
    console.error("publishOne error:", e)
  }
}

function copyURL(item: any) {
  if (item.output_url) {
    copyToClipboard(item.output_url)
  }
}

async function deleteOne(item: any) {
  if (!confirm("确认删除？")) return
  try {
    await api.deleteFRWGeneration(item.id)
    checked.value.delete(item.id)
    loadList()
  } catch (e) {
    console.error("deleteOne error:", e)
  }
}

async function batchPublish() {
  for (const id of checked.value) {
    await api.publishFRWGeneration(id)
  }
  checked.value.clear()
  loadList()
}

async function batchDelete() {
  if (!confirm(`确认删除 ${checked.value.size} 项？`)) return
  for (const id of checked.value) {
    await api.deleteFRWGeneration(id)
  }
  checked.value.clear()
  loadList()
}

// Checkbox helpers
function toggleCheck(id: string) {
  const s = new Set(checked.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  checked.value = s
}

function toggleAll(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.checked) {
    checked.value = new Set(list.value.map((item) => item.id))
  } else {
    checked.value = new Set()
  }
}

// Helpers
function modeLabel(mode: string): string {
  if (mode === "text2image") return "文生图"
  if (mode === "text2video") return "文生视频"
  if (mode === "img2img") return "图生图"
  if (mode === "img2video") return "图生视频"
  if (mode === "upload") return "上传"
  return mode || "--"
}

function statusLabel(status: any): string {
  const s = typeof status === "number" ? status : Number(status)
  if (!isNaN(s)) {
    if (s === 0) return "排队中"
    if (s === 1) return "生成中"
    if (s === 2) return "已完成"
    if (s === -1) return "失败"
    if (s === 4) return "超时"
  }
  if (status === "queued") return "排队中"
  if (status === "processing") return "生成中"
  if (status === "completed") return "已完成"
  if (status === "failed") return "失败"
  return "待处理"
}

function statusTagClass(status: any): string {
  const s = typeof status === "number" ? status : Number(status)
  if (!isNaN(s)) {
    if (s === 0) return "tag-status-queued"
    if (s === 1) return "tag-status-processing"
    if (s === 2) return "tag-status-completed"
    if (s === -1 || s === 4) return "tag-status-failed"
  }
  return "tag-status-" + status
}

function isProcessing(status: any): boolean {
  const s = typeof status === "number" ? status : Number(status)
  return s === 0 || s === 1
}

function isCompleted(status: any): boolean {
  const s = typeof status === "number" ? status : Number(status)
  return s === 2 || status === "completed"
}

function isFailed(status: any): boolean {
  const s = typeof status === "number" ? status : Number(status)
  return s === -1 || s === 4 || status === "failed"
}

function fmtTime(ts: string): string {
  if (!ts) return "--"
  const d = new Date(ts)
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + " B"
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB"
  return (bytes / (1024 * 1024)).toFixed(1) + " MB"
}
</script>

<style scoped>
.media-gen {
  padding: 20px;
  color: var(--ink);
}

.mg-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.mg-header h2 {
  margin: 0;
  font-size: 20px;
  color: var(--ink);
}

.mg-header-actions {
  display: flex;
  gap: 8px;
}

.btn {
  padding: 8px 16px;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  transition: opacity 0.2s;
}

.btn:hover { opacity: 0.85; }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-primary { background: var(--sun); color: #fff; }
.btn-secondary { background: var(--border-light); color: var(--ink); }

.btn-sm {
  padding: 4px 10px;
  border: 1px solid var(--border-light);
  border-radius: 4px;
  background: #fff;
  cursor: pointer;
  font-size: 12px;
}
.btn-sm:hover { background: var(--border-light); }
.btn-sm:disabled { opacity: 0.4; cursor: not-allowed; }
.btn-danger { color: #e53e3e; border-color: #e53e3e; }

.mg-filter-bar {
  display: flex;
  gap: 10px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.mg-filter-bar select, .mg-filter-bar input {
  padding: 6px 12px;
  border: 1px solid var(--border-light);
  border-radius: 6px;
  font-size: 13px;
  outline: none;
}
.mg-filter-bar input { flex: 1; min-width: 160px; }

.mg-stats {
  font-size: 13px;
  color: #666;
  margin-bottom: 12px;
  display: flex;
  gap: 16px;
}

.mg-table-wrap {
  overflow-x: auto;
  border: 1px solid var(--border-light);
  border-radius: 8px;
  margin-bottom: 12px;
}
.mg-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.mg-table th, .mg-table td { padding: 10px 12px; text-align: left; border-bottom: 1px solid var(--border-light); }
.mg-table th { background: #fafafa; font-weight: 600; white-space: nowrap; }
.mg-table tbody tr:hover { background: #f9f9f9; }

.col-check { width: 40px; text-align: center; }
.col-thumb { width: 72px; }
.thumb { width: 60px; height: 60px; object-fit: cover; border-radius: 4px; cursor: pointer; transition: transform 0.2s; }
.thumb:hover { transform: scale(1.05); }
.thumb-placeholder { width: 60px; height: 60px; display: flex; align-items: center; justify-content: center; background: #f0f0f0; border-radius: 4px; color: #999; font-size: 12px; }
.col-prompt { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.col-mode { width: 80px; }
.col-status { width: 80px; }
.col-time { width: 140px; white-space: nowrap; }
.col-actions { width: 160px; white-space: nowrap; }
.col-actions .btn-sm { margin-right: 4px; }

.tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 12px; }
.tag-mode { background: #e8f4fd; color: #1890ff; }
.tag-status-queued { background: #fff7e6; color: #fa8c16; }
.tag-status-processing { background: #e6f7ff; color: #1890ff; }
.tag-status-completed { background: #f6ffed; color: #52c41a; }
.tag-status-failed { background: #fff1f0; color: #f5222d; }

.empty-row { text-align: center; color: #999; padding: 40px 0; }

.mg-bottom-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}
.mg-batch { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.mg-pagination { display: flex; align-items: center; gap: 8px; font-size: 13px; margin-left: auto; }
.mg-pagination select { padding: 4px 8px; border: 1px solid var(--border-light); border-radius: 4px; font-size: 12px; }

/* Dialog form styles */
.gen-form, .upload-form { display: flex; flex-direction: column; gap: 14px; }
.gen-field { display: flex; flex-direction: column; gap: 6px; }
.gen-field label { font-size: 13px; font-weight: 500; color: var(--ink); }
.gen-field select, .gen-field input, .gen-field textarea { padding: 8px 12px; border: 1px solid var(--border-light); border-radius: 6px; font-size: 13px; outline: none; resize: vertical; }
.gen-field textarea { min-height: 80px; }

.upload-drop-zone { border: 2px dashed var(--border-light); border-radius: 8px; padding: 32px; text-align: center; transition: border-color 0.2s, background 0.2s; }
.upload-drop-zone.drag-active { border-color: var(--sun); background: #fffbe6; }
.upload-drop-zone p { margin: 0 0 10px; color: #999; font-size: 14px; }
.upload-btn { display: inline-block; }
.upload-file-list { list-style: none; padding: 0; margin: 0; }
.upload-file-list li { display: flex; justify-content: space-between; align-items: center; padding: 6px 0; font-size: 13px; border-bottom: 1px solid var(--border-light); }

/* Lightbox */
.lightbox-overlay { position: fixed; inset: 0; background: rgba(0, 0, 0, 0.75); display: flex; align-items: center; justify-content: center; z-index: 9999; }
.lightbox-content { position: relative; max-width: 90vw; max-height: 90vh; }
.lightbox-content img, .lightbox-content video { max-width: 100%; max-height: 85vh; border-radius: 8px; }
.lightbox-close { position: absolute; top: -12px; right: -12px; width: 32px; height: 32px; border-radius: 50%; border: none; background: #fff; font-size: 20px; cursor: pointer; display: flex; align-items: center; justify-content: center; box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2); }

/* Responsive */
@media (max-width: 768px) {
  .mg-header { flex-direction: column; align-items: flex-start; gap: 10px; }
  .mg-filter-bar { flex-direction: column; }
  .mg-filter-bar input { min-width: unset; }
  .mg-bottom-bar { flex-direction: column; align-items: flex-start; }
  .mg-pagination { margin-left: 0; }
  .col-prompt { max-width: 120px; }
}

.status-wrap { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; }
.progress-bar { position: relative; width: 100px; height: 6px; background: #e5e7eb; border-radius: 3px; overflow: hidden; }
.progress-fill { position: absolute; top: 0; left: 0; height: 100%; background: linear-gradient(90deg, #1890ff, #52c41a); transition: width .3s; }
.progress-text { position: absolute; right: -32px; top: -5px; font-size: 11px; color: #666; font-weight: 600; }
.btn-warn { background: #fa8c16; color: #fff; border-color: #fa8c16; }
.btn-warn:hover { background: #d46b08; }
.detail-view { display: flex; flex-direction: column; gap: 16px; }
.detail-preview { width: 100%; background: #f5f5f5; border-radius: 8px; display: flex; align-items: center; justify-content: center; min-height: 240px; }
.detail-placeholder { color: #999; font-size: 14px; }
.detail-media { max-width: 100%; max-height: 400px; border-radius: 8px; }
.detail-loading { display: flex; flex-direction: column; align-items: center; gap: 12px; }
.spinner { width: 40px; height: 40px; border: 3px solid #e5e7eb; border-top-color: #1890ff; border-radius: 50%; animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.detail-meta { display: flex; flex-direction: column; gap: 10px; }
.detail-row { display: flex; gap: 10px; align-items: flex-start; font-size: 13px; }
.detail-row label { min-width: 72px; font-weight: 600; color: #666; }
.detail-val { flex: 1; color: #333; word-break: break-all; }
.detail-val a { color: #1890ff; }
.detail-err { color: #f5222d; }
.detail-prog { margin-left: 8px; font-weight: 600; color: #1890ff; }
</style>
