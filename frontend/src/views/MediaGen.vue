<template>
<div class="media-gen">
  <div class="mg-header">
    <h1 class="mg-title">{{ kind === 'image' ? '🎨 AI 图片工坊' : '🎬 AI 视频工坊' }}</h1>
    <div class="mg-subtitle">{{ kind === 'image' ? '文生图 · 图生图 — AI 一键创作高质量图片' : '文生视频 · 图生视频 — AI 驱动的视频创作引擎' }}</div>
  </div>

  <div class="mg-layout">
    <aside class="mg-sidebar">
      <div class="sidebar-inner">
        <div class="sb-section">
          <label class="sb-label">生成模式</label>
          <div class="mode-btns">
            <button :class="['mode-btn', { active: selectedMode === textMode }]" @click="selectMode(textMode)">
              <span class="mode-icon">{{ kind === 'image' ? '✏️' : '📝' }}</span>
              <span>文生{{ kindShort }}</span>
            </button>
            <button :class="['mode-btn', { active: selectedMode === imgMode }]" @click="selectMode(imgMode)">
              <span class="mode-icon">🖼️</span>
              <span>图生{{ kindShort }}</span>
            </button>
          </div>
        </div>

        <div class="sb-section">
          <label class="sb-label">Prompt 描述</label>
          <div class="prompt-wrap">
            <textarea v-model="promptText" class="prompt-input" :rows="6" :placeholder="promptPlaceholder"></textarea>
            <span class="prompt-count">{{ promptText.length }}</span>
          </div>
        </div>

        <div v-if="needsSourceImage" class="sb-section">
          <label class="sb-label">参考图片 URL</label>
          <input v-model="sourceUrl" class="ref-input" placeholder="https://... 公网可访问的图片链接" />
          <div v-if="sourceUrl" class="ref-preview">
            <img :src="sourceUrl" @error="refImgError = true" @load="refImgError = false" />
            <div v-if="refImgError" class="ref-err">图片加载失败</div>
          </div>
        </div>

        <button class="generate-btn" @click="doSubmit" :disabled="submitting || !promptText.trim()">
          <span v-if="submitting" class="gen-spinner"></span>
          {{ submitting ? '提交中...' : '🚀 开始生成' }}
        </button>

        <div v-if="activeTasks.length" class="active-tasks">
          <div class="sb-label">进行中 ({{ activeTasks.length }})</div>
          <div v-for="t in activeTasks" :key="t.id" class="active-item">
            <div class="active-prompt">{{ t.prompt }}</div>
            <el-progress :percentage="Math.round((t.progress || 0) * 100)" :stroke-width="4" :show-text="false" color="var(--sun)" />
            <div class="active-status">{{ statusLabel(t.status) }}</div>
          </div>
        </div>
      </div>
    </aside>

    <main class="mg-gallery">
      <div class="gallery-top">
        <span class="gallery-count">{{ list.length }} 个作品</span>
        <button class="refresh-btn" @click="loadList" :disabled="loading">{{ loading ? '加载中...' : '🔄 刷新' }}</button>
      </div>

      <div v-if="!list.length && !loading" class="gallery-empty">
        <div class="empty-icon">{{ kind === 'image' ? '🎨' : '🎬' }}</div>
        <div class="empty-text">还没有作品</div>
        <div class="empty-hint">在左侧输入 Prompt，开始你的第一次创作</div>
      </div>

      <div v-else class="gallery-grid" v-loading="loading">
        <div v-for="g in list" :key="g.id" class="gallery-card">
          <div class="card-preview" :class="{ 'is-video': kind === 'video' }">
            <template v-if="g.status === 2 && kind === 'image'">
              <img :src="g.local_path || g.result_url" class="card-img" @click="previewItem = g" />
            </template>
            <template v-else-if="g.status === 2 && kind === 'video'">
              <video :src="g.local_path || g.result_url" class="card-video" controls preload="metadata" />
            </template>
            <template v-else-if="g.status === -1 || g.status === 4">
              <div class="card-failed"><span class="failed-icon">❌</span><span class="failed-text">生成失败</span></div>
            </template>
            <template v-else>
              <div class="card-loading">
                <div class="loading-ring"></div>
                <div class="loading-pct">{{ Math.round((g.progress || 0) * 100) }}%</div>
                <div class="loading-label">{{ statusLabel(g.status) }}</div>
              </div>
            </template>
            <div v-if="g.status === 2" class="card-overlay">
              <button v-if="!g.local_path" class="ov-btn ov-publish" @click.stop="publishOne(g)">💾 发布</button>
              <button class="ov-btn ov-copy" @click.stop="copyURL(g)">📋 复制</button>
              <button class="ov-btn ov-delete" @click.stop="deleteOne(g)">🗑️</button>
            </div>
          </div>
          <div class="card-footer">
            <span class="card-mode" :class="'m-' + g.mode">{{ modeLabel(g.mode) }}</span>
            <div class="card-prompt" :title="g.prompt">{{ g.prompt }}</div>
            <div v-if="g.error_msg" class="card-err">{{ g.error_msg }}</div>
            <div class="card-meta">
              <span class="card-time">{{ fmtTime(g.created_at) }}</span>
              <span v-if="g.local_path" class="card-published">已发布</span>
            </div>
          </div>
        </div>
      </div>
    </main>
  </div>

  <Teleport to="body">
    <div v-if="previewItem" class="lightbox" @click="previewItem = null">
      <div class="lightbox-inner" @click.stop>
        <img :src="previewItem.local_path || previewItem.result_url" class="lightbox-img" />
        <div class="lightbox-info">
          <div class="lightbox-prompt">{{ previewItem.prompt }}</div>
          <div class="lightbox-actions">
            <button v-if="!previewItem.local_path" class="lb-btn" @click="publishOne(previewItem)">💾 发布到本地</button>
            <button class="lb-btn" @click="copyURL(previewItem)">📋 复制链接</button>
            <button class="lb-btn lb-close" @click="previewItem = null">✕ 关闭</button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import { copyToClipboard } from '../utils/clipboard'

const props = defineProps({ kind: { type: String, required: true } })

const kindShort = computed(() => props.kind === 'image' ? '图' : '视频')
const textMode = computed(() => 'text2' + props.kind)
const imgMode = computed(() => props.kind === 'image' ? 'img2img' : 'img2video')
const promptPlaceholder = computed(() => props.kind === 'image'
  ? '描述你想生成的图片，例如：一只橘猫在窗台晒太阳，柔和光线，写实风格'
  : '描述你想生成的视频，例如：海浪拍打礁石的慢镜头，夕阳余晖，电影质感')

const list = ref<any[]>([])
const loading = ref(false)
const submitting = ref(false)
const selectedMode = ref(props.kind === 'image' ? 'text2image' : 'text2video')
const promptText = ref('')
const sourceUrl = ref('')
const refImgError = ref(false)
const previewItem = ref<any>(null)

const needsSourceImage = computed(() => selectedMode.value === 'img2img' || selectedMode.value === 'img2video')
const activeTasks = computed(() => list.value.filter((x: any) => x.status === 0 || x.status === 1))

function selectMode(mode: string) { selectedMode.value = mode; sourceUrl.value = ''; refImgError.value = false }
function modeLabel(m: string) { return ({ text2image: '文生图', img2img: '图生图', text2video: '文生视频', img2video: '图生视频' } as any)[m] || m }
function statusLabel(s: number) { return ({ 0: '排队中', 1: '生成中', 2: '完成', [-1]: '失败', 4: '超时' } as any)[s] || '未知' }
function fmtTime(t: string) { if (!t) return ''; return new Date(t).toLocaleString('zh-CN', { hour12: false }) }

async function loadList() {
  loading.value = true
  try { const res = await api.listFRWGenerations({ kind: props.kind }); list.value = res.data || [] }
  catch (e) { console.error('load failed', e) }
  finally { loading.value = false }
}

async function doSubmit() {
  if (!promptText.value.trim()) return
  if (needsSourceImage.value && !sourceUrl.value.trim()) { ElMessage.warning('该模式需要参考图 URL'); return }
  submitting.value = true
  try {
    await api.submitFRWGeneration({ mode: selectedMode.value, prompt: promptText.value.trim(), source_url: sourceUrl.value.trim() })
    ElMessage.success('任务已提交'); promptText.value = ''; sourceUrl.value = ''
    await loadList(); startPolling()
  } catch (e: any) { ElMessage.error(e?.response?.data?.error || '提交失败') }
  finally { submitting.value = false }
}

async function publishOne(row: any) {
  try { const res = await api.publishFRWGeneration(row.id); row.local_path = res.data.local_path; ElMessage.success('已发布到本地') }
  catch (e: any) { ElMessage.error(e?.response?.data?.error || '发布失败') }
}

async function copyURL(row: any) {
  const url = row.local_path ? location.origin + row.local_path : row.result_url
  if (await copyToClipboard(url)) ElMessage.success('链接已复制'); else ElMessage.error('复制失败')
}

async function deleteOne(row: any) {
  try {
    await ElMessageBox.confirm('确定删除此作品？', '确认删除', { type: 'warning' })
    await api.deleteFRWGeneration(row.id)
    list.value = list.value.filter((x: any) => x.id !== row.id)
    if (previewItem.value?.id === row.id) previewItem.value = null
    ElMessage.success('已删除')
  } catch (e: any) { if (e !== 'cancel') ElMessage.error('删除失败') }
}

let pollTimer: ReturnType<typeof setInterval> | null = null
function startPolling() {
  if (pollTimer) return
  pollTimer = setInterval(async () => {
    if (!activeTasks.value.length) { stopPolling(); return }
    for (const row of activeTasks.value) {
      try { const res = await api.getFRWGeneration(row.id); Object.assign(row, res.data) } catch {}
    }
  }, 5000)
}
function stopPolling() { if (pollTimer) { clearInterval(pollTimer); pollTimer = null } }

onMounted(async () => { await loadList(); if (activeTasks.value.length) startPolling() })
onUnmounted(stopPolling)
</script>

<style scoped>
.media-gen{display:flex;flex-direction:column;height:100%}
.mg-header{margin-bottom:20px}
.mg-title{margin:0;font-size:24px;font-weight:800;color:var(--ink)}
.mg-subtitle{font-size:13px;color:var(--ink-soft);margin-top:4px}
.mg-layout{display:flex;gap:20px;flex:1;min-height:0}
.mg-sidebar{width:340px;min-width:340px;background:var(--surface);border:1.5px solid var(--border-light);border-radius:var(--r);box-shadow:var(--s1);overflow-y:auto}
.sidebar-inner{padding:22px;display:flex;flex-direction:column;gap:20px}
.sb-section{display:flex;flex-direction:column;gap:8px}
.sb-label{font-size:12px;font-weight:800;color:var(--ink-mid);letter-spacing:.04em;text-transform:uppercase}
.mode-btns{display:grid;grid-template-columns:1fr 1fr;gap:8px}
.mode-btn{display:flex;align-items:center;justify-content:center;gap:6px;padding:12px 8px;border:1.5px solid var(--border);border-radius:var(--r-sm);background:var(--surface);color:var(--ink-mid);font-weight:700;font-size:13px;cursor:pointer;transition:all .18s}
.mode-btn:hover{border-color:var(--peach);background:var(--sun-light)}
.mode-btn.active{border-color:var(--sun);background:linear-gradient(135deg,var(--sun-light),var(--sun-glow));color:var(--sun-deep);box-shadow:0 2px 8px rgba(255,140,0,.15)}
.mode-icon{font-size:18px}
.prompt-wrap{position:relative}
.prompt-input{width:100%;border:1.5px solid var(--border);border-radius:var(--r-sm);padding:12px 14px;font-size:14px;font-family:inherit;line-height:1.6;resize:vertical;color:var(--ink);background:var(--surface)}
.prompt-input:focus{outline:none;border-color:var(--sun);box-shadow:0 0 0 3px rgba(255,140,0,.1)}
.prompt-input::placeholder{color:var(--ink-hint)}
.prompt-count{position:absolute;bottom:8px;right:10px;font-size:11px;color:var(--ink-hint);font-family:monospace}
.ref-input{width:100%;border:1.5px solid var(--border);border-radius:var(--r-sm);padding:10px 12px;font-size:13px;color:var(--ink)}
.ref-input:focus{outline:none;border-color:var(--sun)}
.ref-preview{margin-top:8px;border-radius:var(--r-sm);overflow:hidden;border:1px solid var(--border-light)}
.ref-preview img{width:100%;max-height:180px;object-fit:cover;display:block}
.ref-err{padding:8px;font-size:12px;color:var(--err);text-align:center}
.generate-btn{width:100%;padding:14px;border:none;border-radius:var(--r-sm);background:linear-gradient(135deg,var(--sun),var(--coral));color:#fff;font-size:16px;font-weight:800;cursor:pointer;box-shadow:var(--s-amber);transition:all .2s;display:flex;align-items:center;justify-content:center;gap:8px}
.generate-btn:hover:not(:disabled){transform:translateY(-1px);box-shadow:0 6px 24px rgba(255,140,0,.4)}
.generate-btn:disabled{opacity:.5;cursor:not-allowed}
.gen-spinner{width:18px;height:18px;border:2.5px solid rgba(255,255,255,.3);border-top-color:#fff;border-radius:50%;animation:spin .6s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}
.active-tasks{display:flex;flex-direction:column;gap:10px}
.active-item{padding:10px 12px;background:var(--surface-2);border-radius:var(--r-sm);border:1px solid var(--border-light)}
.active-prompt{font-size:12px;color:var(--ink-mid);margin-bottom:6px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.active-status{font-size:11px;color:var(--ink-hint);margin-top:4px}
.mg-gallery{flex:1;min-width:0}
.gallery-top{display:flex;justify-content:space-between;align-items:center;margin-bottom:16px}
.gallery-count{font-size:13px;font-weight:700;color:var(--ink-mid)}
.refresh-btn{padding:6px 14px;border:1.5px solid var(--border);border-radius:var(--r-sm);background:var(--surface);color:var(--ink-mid);font-weight:600;font-size:12px;cursor:pointer}
.refresh-btn:hover{border-color:var(--sun);color:var(--sun-deep)}
.gallery-empty{display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:400px;color:var(--ink-hint);text-align:center}
.empty-icon{font-size:64px;margin-bottom:16px;opacity:.5}
.empty-text{font-size:18px;font-weight:700;margin-bottom:6px}
.empty-hint{font-size:13px}
.gallery-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:16px}
.gallery-card{background:var(--surface);border:1.5px solid var(--border-light);border-radius:var(--r);box-shadow:var(--s1);overflow:hidden;transition:box-shadow .2s,transform .2s}
.gallery-card:hover{box-shadow:var(--s2);transform:translateY(-2px)}
.card-preview{position:relative;aspect-ratio:3/4;background:var(--surface-3);display:flex;align-items:center;justify-content:center;overflow:hidden}
.card-preview.is-video{aspect-ratio:16/9}
.card-img{width:100%;height:100%;object-fit:cover;cursor:zoom-in}
.card-video{width:100%;height:100%;object-fit:contain;background:#000}
.card-failed{display:flex;flex-direction:column;align-items:center;gap:6px}
.failed-icon{font-size:28px}
.failed-text{font-size:13px;color:var(--err);font-weight:600}
.card-loading{display:flex;flex-direction:column;align-items:center;gap:10px}
.loading-ring{width:48px;height:48px;border:3px solid var(--border-light);border-top-color:var(--sun);border-radius:50%;animation:spin 1s linear infinite}
.loading-pct{font-size:20px;font-weight:800;color:var(--sun-deep)}
.loading-label{font-size:12px;color:var(--ink-hint)}
.card-overlay{position:absolute;inset:0;background:rgba(0,0,0,.55);display:flex;align-items:center;justify-content:center;gap:8px;opacity:0;transition:opacity .2s}
.gallery-card:hover .card-overlay{opacity:1}
.ov-btn{padding:8px 14px;border:none;border-radius:8px;font-size:13px;font-weight:700;cursor:pointer;backdrop-filter:blur(4px);transition:transform .15s}
.ov-btn:hover{transform:scale(1.05)}
.ov-publish{background:rgba(22,163,74,.9);color:#fff}
.ov-copy{background:rgba(255,255,255,.9);color:var(--ink)}
.ov-delete{background:rgba(220,38,38,.9);color:#fff}
.card-footer{padding:12px 14px}
.card-mode{display:inline-block;padding:2px 10px;border-radius:10px;font-size:11px;font-weight:700;margin-bottom:6px}
.m-text2image,.m-text2video{background:#E0E7FF;color:#3730A3}
.m-img2img,.m-img2video{background:#FEF3C7;color:#92400E}
.card-prompt{font-size:13px;line-height:1.5;color:var(--ink-mid);display:-webkit-box;-webkit-line-clamp:2;line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.card-err{font-size:11px;color:var(--err);background:var(--err-bg);padding:4px 8px;border-radius:4px;margin-top:4px}
.card-meta{display:flex;justify-content:space-between;align-items:center;margin-top:8px}
.card-time{font-size:11px;color:var(--ink-hint);font-family:monospace}
.card-published{background:var(--ok-bg);color:var(--ok);padding:1px 8px;border-radius:8px;font-size:10px;font-weight:700}
.lightbox{position:fixed;inset:0;z-index:9999;background:rgba(0,0,0,.85);display:flex;align-items:center;justify-content:center;cursor:pointer}
.lightbox-inner{max-width:90vw;max-height:90vh;cursor:default;display:flex;flex-direction:column;align-items:center}
.lightbox-img{max-width:90vw;max-height:78vh;object-fit:contain;border-radius:8px}
.lightbox-info{margin-top:16px;text-align:center}
.lightbox-prompt{color:rgba(255,255,255,.8);font-size:14px;max-width:600px;line-height:1.6;margin-bottom:12px}
.lightbox-actions{display:flex;gap:8px;justify-content:center}
.lb-btn{padding:8px 18px;border:1.5px solid rgba(255,255,255,.3);border-radius:8px;background:rgba(255,255,255,.1);color:#fff;font-weight:600;font-size:13px;cursor:pointer}
.lb-btn:hover{background:rgba(255,255,255,.2)}
@media(max-width:900px){.mg-layout{flex-direction:column}.mg-sidebar{width:100%;min-width:unset}.gallery-grid{grid-template-columns:repeat(auto-fill,minmax(160px,1fr))}}
</style>
