<template>
<div class="media-gen-page">
  <div class="page-top"><h1>{{ title }}</h1></div>

  <el-tabs v-model="activeTab" type="border-card">
    <!-- Task list (两种 mode 混合展示, 点新建时再选 mode) -->
    <el-tab-pane :label="`📋 ${kindLabel}任务`" name="list">
      <div class="tab-bar">
        <el-button type="primary" @click="openCreateDialog('text2' + kind)">
          + 新建 文生{{ kindShort }}
        </el-button>
        <el-button type="success" @click="openCreateDialog('img2' + kind)">
          + 新建 图生{{ kindShort }}
        </el-button>
        <el-button link @click="loadList" :loading="loading">🔄 刷新</el-button>
        <span v-if="pollingCount > 0" class="polling-hint">
          · {{ pollingCount }} 个任务生成中，自动刷新
        </span>
      </div>

      <div v-if="!list.length && !loading" class="empty-hint panel">
        <div>还没有任务。点击上方按钮开始生成第一个{{ kindLabel }}。</div>
        <div v-if="!config.has_token" style="margin-top:10px;color:var(--warn)">
          ⚠ 尚未配置 FRW 凭据，请切换到「我的 FRW 配置」填写 Bearer token 和 User ID。
        </div>
      </div>

      <div v-else class="gen-grid">
        <div v-for="g in list" :key="g.id" class="gen-card panel">
          <div class="gen-preview">
            <!-- Image -->
            <template v-if="kind === 'image'">
              <img v-if="g.status === 2 && g.result_url"
                   :src="g.local_path || g.result_url"
                   @click="previewUrl = g.local_path || g.result_url"
                   class="preview-img" />
              <div v-else-if="g.status === -1 || g.status === 4" class="preview-failed">❌ 失败</div>
              <div v-else class="preview-loading">
                <el-progress :percentage="Math.round((g.progress || 0) * 100)" :stroke-width="6" />
                <div class="progress-label">{{ statusLabel(g.status) }}...</div>
              </div>
            </template>

            <!-- Video -->
            <template v-else>
              <video v-if="g.status === 2 && g.result_url"
                     :src="g.local_path || g.result_url"
                     controls preload="metadata"
                     class="preview-video" />
              <div v-else-if="g.status === -1 || g.status === 4" class="preview-failed">❌ 失败</div>
              <div v-else class="preview-loading">
                <el-progress :percentage="Math.round((g.progress || 0) * 100)" :stroke-width="6" />
                <div class="progress-label">{{ statusLabel(g.status) }}...</div>
              </div>
            </template>
          </div>

          <div class="gen-body">
            <div class="gen-mode-tag" :class="`mode-${g.mode}`">{{ modeLabel(g.mode) }}</div>
            <div class="gen-prompt" :title="g.prompt">{{ g.prompt }}</div>
            <div v-if="g.error_msg" class="gen-err">{{ g.error_msg }}</div>

            <div class="gen-meta">
              <span class="gen-time">{{ fmtTime(g.created_at) }}</span>
              <span v-if="g.local_path" class="gen-local-tag">已发布</span>
            </div>

            <div class="gen-actions">
              <el-button v-if="g.status === 2 && !g.local_path" size="small"
                         type="success" @click="publishOne(g)">💾 发布到本地</el-button>
              <el-button v-if="g.status === 2" size="small" @click="copyURL(g)">复制链接</el-button>
              <el-button size="small" type="danger" @click="deleteOne(g)">删除</el-button>
            </div>
          </div>
        </div>
      </div>

      <!-- Simple image lightbox -->
      <el-dialog v-model="previewVisible" title="预览" width="80%">
        <div style="text-align:center">
          <img v-if="previewUrl" :src="previewUrl" style="max-width:100%;max-height:80vh" />
        </div>
      </el-dialog>
    </el-tab-pane>

    <!-- FRW config -->
    <el-tab-pane label="⚙️ 我的 FRW 配置" name="config">
      <div class="panel">
        <h3 style="margin-top:0">FRW AI 生成凭据</h3>
        <p class="help-text">
          FRW 需要你在浏览器登录
          <a href="https://frw-dreamaiai-api2.aiaiartist.com" target="_blank">frw-dreamaiai-api2.aiaiartist.com</a>
          后，打开开发者工具（F12）的「网络」标签，触发一次生成，
          从请求头里复制 <code>Authorization</code> 后面的 Bearer token 和 payload 里的 <code>userID</code>。
          Token 会过期，过期后重复此流程即可。
        </p>

        <el-form label-width="150px" style="max-width:720px;margin-top:20px">
          <el-form-item label="API Key">
            <el-input v-model="configForm.apikey"
                      :placeholder="config.has_apikey ? config.apikey_hint : '留空使用系统默认'"
                      show-password />
            <div class="form-hint">每个用户可以用自己的 apikey；留空则走管理员在系统设置中配置的全局 apikey。</div>
          </el-form-item>

          <el-form-item label="Bearer Token">
            <el-input v-model="configForm.token" type="textarea" :rows="3"
                      :placeholder="config.has_token ? config.token_hint : '从浏览器 devtools 复制 Bearer 后面那串'" />
            <div class="form-hint">粘贴 <code>Bearer</code> 后面的全部内容，不要带 Bearer 前缀。</div>
          </el-form-item>

          <el-form-item label="User ID">
            <el-input v-model="configForm.user_id" :placeholder="config.user_id || 'FRW 用户 ID'" />
            <div class="form-hint">通常是 payload 里的 <code>userID</code> 字段。</div>
          </el-form-item>

          <el-form-item>
            <el-button type="primary" @click="saveConfig" :loading="savingConfig">💾 保存配置</el-button>
            <el-button @click="loadConfig">刷新</el-button>
          </el-form-item>
        </el-form>

        <div class="config-status panel" style="margin-top:20px">
          <div class="cs-item">
            <span class="cs-label">API Key:</span>
            <span :class="config.has_apikey ? 'cs-ok' : 'cs-none'">
              {{ config.has_apikey ? `已配置 (${config.apikey_hint})` : '未配置 (将使用全局默认)' }}
            </span>
          </div>
          <div class="cs-item">
            <span class="cs-label">Bearer Token:</span>
            <span :class="config.has_token ? 'cs-ok' : 'cs-none'">
              {{ config.has_token ? `已配置 (${config.token_hint})` : '未配置' }}
            </span>
          </div>
          <div class="cs-item">
            <span class="cs-label">User ID:</span>
            <span :class="config.user_id ? 'cs-ok' : 'cs-none'">
              {{ config.user_id || '未配置' }}
            </span>
          </div>
        </div>
      </div>
    </el-tab-pane>
  </el-tabs>

  <!-- Create / generate dialog -->
  <el-dialog v-model="showCreate" :title="dialogTitle" width="560px" :close-on-click-modal="false">
    <el-form label-width="100px">
      <el-form-item label="Prompt" required>
        <el-input v-model="createForm.prompt" type="textarea" :rows="5"
                  placeholder="描述你想生成的内容，例如：一只橘猫在窗台晒太阳，柔和光线，写实风格" />
      </el-form-item>

      <el-form-item v-if="createForm.mode === 'img2img' || createForm.mode === 'img2video'"
                    label="参考图 URL" required>
        <el-input v-model="createForm.source_url" placeholder="https://..." />
        <div class="form-hint">用于图生图/图生视频，需要是公网可访问的图片链接。</div>
      </el-form-item>

      <el-form-item label="模式">
        <div class="mode-display">
          <el-tag type="info">{{ modeLabel(createForm.mode) }}</el-tag>
        </div>
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="showCreate = false">取消</el-button>
      <el-button type="primary" @click="submitCreate" :loading="submitting">
        🚀 开始生成
      </el-button>
    </template>
  </el-dialog>
</div>
</template>

<script setup lang="ts">
// Shared media generation page — used by both ImageGen.vue and VideoGen.vue
// via a `kind` prop ('image' | 'video'). Polling logic is identical for
// both; only the preset modes and preview element differ.
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import { copyToClipboard } from '../utils/clipboard'

const props = defineProps({
  kind: { type: String, required: true }, // 'image' | 'video'
})

// Static labels keyed off kind — keep Chinese natural in buttons/titles.
const kindLabel = computed(() => props.kind === 'image' ? '图片' : '视频')
const kindShort = computed(() => props.kind === 'image' ? '图' : '视频')
const title = computed(() => props.kind === 'image' ? '🎨 图片管理' : '🎬 视频管理')

const activeTab = ref('list')
const list = ref([])
const loading = ref(false)

const config = ref({ has_apikey: false, has_token: false, user_id: '', apikey_hint: '', token_hint: '' })
const configForm = reactive({ apikey: '', token: '', user_id: '' })
const savingConfig = ref(false)

const showCreate = ref(false)
const createMode = ref('')
const createForm = reactive({ mode: '', prompt: '', source_url: '' })
const submitting = ref(false)

const previewVisible = ref(false)
const previewUrl = ref('')

// Poll-in-progress list entries every 5 s while any is running.
let pollTimer = null
const pollingCount = computed(() => list.value.filter(x => x.status === 0 || x.status === 1).length)

// ─── Label helpers ───────────────────────────────────────────────────────
function modeLabel(mode) {
  return {
    text2image: '文生图',
    img2img: '图生图',
    text2video: '文生视频',
    img2video: '图生视频',
  }[mode] || mode
}
function statusLabel(s) {
  return { 0: '等待中', 1: '生成中', 2: '完成', [-1]: '失败', 4: '超时' }[s] || '未知'
}
function fmtTime(t) {
  if (!t) return ''
  const d = new Date(t)
  return d.toLocaleString('zh-CN', { hour12: false })
}

// ─── Dialog helpers ──────────────────────────────────────────────────────
const dialogTitle = computed(() => `新建 ${modeLabel(createForm.mode)} 任务`)

function openCreateDialog(modeRaw) {
  // modeRaw is "text2image" / "img2image" / "text2video" / "img2video"
  // but our preset keys use "img2img" for images. Normalize here.
  let mode = modeRaw
  if (mode === 'img2image') mode = 'img2img'
  createMode.value = mode
  createForm.mode = mode
  createForm.prompt = ''
  createForm.source_url = ''
  showCreate.value = true
}

async function submitCreate() {
  if (!createForm.prompt.trim()) { ElMessage.warning('请填写 Prompt'); return }
  if ((createForm.mode === 'img2img' || createForm.mode === 'img2video') && !createForm.source_url.trim()) {
    ElMessage.warning('该模式需要参考图 URL'); return
  }
  submitting.value = true
  try {
    const res = await api.submitFRWGeneration({
      mode: createForm.mode,
      prompt: createForm.prompt.trim(),
      source_url: createForm.source_url.trim(),
    })
    ElMessage.success(`任务已提交，task_id = ${res.data.frw_task_id}`)
    showCreate.value = false
    await loadList()
    startPolling()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '提交失败')
  } finally {
    submitting.value = false
  }
}

// ─── List + polling ──────────────────────────────────────────────────────
async function loadList() {
  loading.value = true
  try {
    const res = await api.listFRWGenerations({ kind: props.kind })
    list.value = res.data || []
  } catch (e) {
    console.error('加载任务失败', e)
  } finally {
    loading.value = false
  }
}

function startPolling() {
  if (pollTimer) return
  pollTimer = setInterval(async () => {
    if (pollingCount.value === 0) { stopPolling(); return }
    // Refresh each pending task individually — cheaper than relisting
    // when there are many finished historical rows.
    const pending = list.value.filter(x => x.status === 0 || x.status === 1)
    for (const row of pending) {
      try {
        const res = await api.getFRWGeneration(row.id)
        Object.assign(row, res.data)
      } catch { /* silent */ }
    }
  }, 5000)
}
function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

// ─── Row actions ─────────────────────────────────────────────────────────
async function publishOne(row) {
  try {
    const res = await api.publishFRWGeneration(row.id)
    row.local_path = res.data.local_path
    ElMessage.success('已下载到本地')
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '发布失败')
  }
}
async function copyURL(row) {
  // navigator.clipboard is only available on HTTPS / localhost; the
  // shared helper does an execCommand fallback so intranet HTTP deploys
  // still get a working "copy" button instead of a silent throw.
  const url = row.local_path ? location.origin + row.local_path : row.result_url
  const ok = await copyToClipboard(url)
  if (ok) {
    ElMessage.success('链接已复制')
  } else {
    ElMessage.error('复制失败，请手动选中链接')
  }
}
async function deleteOne(row) {
  try {
    await ElMessageBox.confirm(`确定删除此任务？`, '确认删除', { type: 'warning' })
    await api.deleteFRWGeneration(row.id)
    list.value = list.value.filter(x => x.id !== row.id)
    ElMessage.success('已删除')
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败')
  }
}

// ─── Config ──────────────────────────────────────────────────────────────
async function loadConfig() {
  try {
    const res = await api.getMyFRWConfig()
    config.value = res.data
  } catch (e) {
    console.error(e)
  }
}
async function saveConfig() {
  savingConfig.value = true
  try {
    await api.saveMyFRWConfig({
      apikey: configForm.apikey,
      token: configForm.token,
      user_id: configForm.user_id,
    })
    ElMessage.success('配置已保存')
    configForm.apikey = ''
    configForm.token = ''
    configForm.user_id = ''
    await loadConfig()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  } finally {
    savingConfig.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadList(), loadConfig()])
  if (pollingCount.value > 0) startPolling()
})
onUnmounted(stopPolling)
</script>

<style scoped>
.media-gen-page { padding: 0; }
.page-top { margin-bottom: 18px; }
.page-top h1 { margin: 0; font-size: 22px; font-weight: 700; color: var(--ink); }

.tab-bar { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
.polling-hint { font-size: 12px; color: var(--ink-hint); }

.empty-hint { padding: 60px 30px; text-align: center; color: var(--ink-hint); font-size: 14px; line-height: 1.8; }

.gen-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 18px; }
.gen-card { padding: 0; overflow: hidden; display: flex; flex-direction: column; border-radius: var(--r, 8px); }

.gen-preview { aspect-ratio: 3/4; background: var(--surface-3, #f7f7f7); display: flex; align-items: center; justify-content: center; overflow: hidden; }
.preview-img { width: 100%; height: 100%; object-fit: cover; cursor: zoom-in; }
.preview-video { width: 100%; height: 100%; object-fit: contain; background: #000; }
.preview-loading { padding: 20px; text-align: center; width: 100%; }
.progress-label { margin-top: 10px; font-size: 12px; color: var(--ink-hint); }
.preview-failed { font-size: 16px; color: var(--err, #E53E3E); font-weight: 600; }

.gen-body { padding: 12px 14px; display: flex; flex-direction: column; gap: 8px; }
.gen-mode-tag { display: inline-block; padding: 2px 10px; border-radius: 10px; font-size: 11.5px; font-weight: 700; width: fit-content; }
.mode-text2image, .mode-text2video { background: #E0E7FF; color: #3730A3; }
.mode-img2img, .mode-img2video { background: #FEF3C7; color: #92400E; }

.gen-prompt { font-size: 13px; line-height: 1.5; color: var(--ink-mid); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.gen-err { font-size: 12px; color: var(--err, #E53E3E); background: #FEF2F2; padding: 4px 8px; border-radius: 4px; }
.gen-meta { display: flex; justify-content: space-between; align-items: center; font-size: 11.5px; color: var(--ink-hint); font-family: 'JetBrains Mono', monospace; }
.gen-local-tag { background: var(--success-light, #D1FAE5); color: var(--success, #059669); padding: 1px 8px; border-radius: 8px; font-size: 10.5px; font-weight: 700; }
.gen-actions { display: flex; gap: 6px; flex-wrap: wrap; margin-top: 4px; }

.help-text { font-size: 13px; line-height: 1.7; color: var(--ink-mid); padding: 12px 16px; background: var(--surface-2, #FFF8E1); border-radius: 8px; }
.help-text code { background: rgba(0,0,0,0.06); padding: 1px 6px; border-radius: 3px; font-family: 'JetBrains Mono', monospace; font-size: 12px; }
.help-text a { color: var(--primary, #F97316); text-decoration: underline; }

.form-hint { font-size: 11.5px; color: var(--ink-hint); line-height: 1.6; margin-top: 4px; }
.form-hint code { background: rgba(0,0,0,0.06); padding: 0 4px; border-radius: 3px; font-family: 'JetBrains Mono', monospace; }

.config-status { padding: 16px 22px; background: var(--surface-2); display: flex; flex-direction: column; gap: 8px; }
.cs-item { font-size: 13px; display: flex; gap: 10px; }
.cs-label { font-weight: 700; color: var(--ink-mid); min-width: 120px; }
.cs-ok { color: var(--success, #059669); }
.cs-none { color: var(--ink-hint); }

.mode-display { padding: 4px 0; }
</style>
