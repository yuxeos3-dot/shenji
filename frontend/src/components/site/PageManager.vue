<template>
  <div class="panel" style="margin-bottom:18px">
    <div class="panel-header">
      <h2>页面管理</h2>
      <div style="display:flex;gap:8px">
        <button class="btn btn-sm btn-fetch-all" @click="$emit('fetch-all')" :disabled="fetchingAll">
          {{ fetchingAll ? '抓取中...' : '一键抓取全部' }}
        </button>
        <button class="btn btn-primary btn-sm" @click="showAddPage=true">+ 添加页面</button>
      </div>
    </div>
    <div class="page-tip">添加需要监控的页面，支持实时抓取 HTML 源码查看</div>

    <div class="add-page-form" v-if="showAddPage">
      <input class="config-input" v-model="newPage.name" placeholder="页面名称（如：首页、详情页）" style="width:180px" />
      <input class="config-input" v-model="newPage.url" placeholder="完整 URL（https://example.com/page）" style="flex:1" />
      <button class="btn btn-primary btn-sm" @click="addPage" :disabled="addingPage">{{ addingPage ? '添加中...' : '确认添加' }}</button>
      <button class="btn btn-sm" @click="showAddPage=false;newPage={name:'',url:''}">取消</button>
    </div>

    <div v-if="pages.length" class="pages-list">
      <div class="page-row" v-for="p in pages" :key="p.id">
        <div class="page-info">
          <span class="page-name">{{ p.name }}</span>
          <a :href="p.url" target="_blank" rel="noopener noreferrer" class="page-url">{{ p.url }}</a>
        </div>
        <div class="page-meta" v-if="p.last_fetched_at">
          <span :class="['page-status', p.status_code===200?'status-ok':'status-err']">{{ p.status_code }}</span>
          <span class="page-ttfb">{{ p.ttfb_ms }}ms</span>
          <span class="page-time">{{ p.last_fetched_at }}</span>
        </div>
        <div class="page-meta" v-else>
          <span class="page-no-fetch">尚未抓取</span>
        </div>
        <div class="page-actions">
          <button class="btn btn-sm btn-fetch" @click="$emit('fetch-page', p)" :disabled="p.fetching">
            {{ p.fetching ? '抓取中...' : '实时抓取' }}
          </button>
          <button class="btn btn-sm btn-source" @click="$emit('view-source', p)" :disabled="!p.last_fetched_at">
            源码查询
          </button>
          <button class="btn btn-sm btn-del-page" @click="$emit('delete-page', p)">删除</button>
        </div>
      </div>
    </div>
    <div v-else class="pages-empty">暂无页面，点击「添加页面」开始监控</div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage } from 'element-plus'

const props = defineProps({
  pages: { type: Array, required: true },
  fetchingAll: { type: Boolean, default: false },
  siteId: { type: [String, Number], required: true },
})
const emit = defineEmits(['fetch-all', 'fetch-page', 'view-source', 'delete-page', 'add-page'])

const showAddPage = ref(false)
const addingPage = ref(false)
const newPage = ref({ name: '', url: '' })

async function addPage() {
  if (!newPage.value.name) { ElMessage.warning('请填写页面名称'); return }
  if (!newPage.value.url || !newPage.value.url.startsWith('http')) {
    ElMessage.warning('请填写完整 URL（以 http:// 或 https:// 开头）'); return
  }
  addingPage.value = true
  try {
    emit('add-page', { ...newPage.value })
    showAddPage.value = false
    newPage.value = { name: '', url: '' }
  } finally {
    addingPage.value = false
  }
}
</script>

<style scoped>
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:18px; }
.panel-header h2 { font-size:15px; font-weight:700; }
.page-tip { font-size:12.5px; color:var(--ink-soft); margin-bottom:16px; }
.add-page-form { display:flex; gap:10px; align-items:center; padding:14px 16px; background:var(--surface-3); border-radius:var(--r-sm); margin-bottom:16px; flex-wrap:wrap; }
.config-input { width:100%; padding:9px 12px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13.5px; font-family:inherit; background:var(--surface); color:var(--ink); outline:none; transition:border-color .18s, box-shadow .18s; }
.config-input:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); background:#FFFDF9; }
.pages-list { display:flex; flex-direction:column; gap:0; border:1.5px solid var(--border-light); border-radius:var(--r-sm); overflow:hidden; }
.page-row { display:flex; align-items:center; gap:14px; padding:12px 16px; border-bottom:1px solid var(--border-light); flex-wrap:wrap; }
.page-row:last-child { border-bottom:none; }
.page-row:hover { background:var(--sun-light); }
.page-info { display:flex; flex-direction:column; gap:3px; flex:1; min-width:200px; }
.page-name { font-weight:700; font-size:14px; color:var(--ink); }
.page-url { font-size:12px; color:var(--info); word-break:break-all; }
.page-url:hover { text-decoration:underline; }
.page-meta { display:flex; align-items:center; gap:10px; flex-shrink:0; }
.page-status { font-family:'JetBrains Mono',monospace; font-size:12px; font-weight:700; padding:2px 8px; border-radius:5px; }
.status-ok { background:var(--ok-bg); color:var(--ok); }
.status-err { background:var(--err-bg); color:var(--err); }
.page-ttfb { font-size:12px; color:var(--ink-soft); font-family:'JetBrains Mono',monospace; }
.page-time { font-size:11.5px; color:var(--ink-hint); }
.page-no-fetch { font-size:12px; color:var(--ink-hint); }
.page-actions { display:flex; gap:6px; flex-shrink:0; }
.pages-empty { text-align:center; padding:36px; color:var(--ink-hint); font-size:13.5px; }
.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-sm { padding:6px 14px !important; font-size:12.5px !important; }
.btn-primary { background:linear-gradient(135deg,var(--sun) 0%,var(--coral) 100%); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep) 0%,var(--sun) 100%); color:#fff; }
.btn-fetch-all { background:linear-gradient(135deg,#F59E0B,#F97316) !important; color:#fff !important; border-color:transparent !important; box-shadow:0 4px 14px rgba(245,158,11,.4) !important; }
.btn-fetch-all:disabled { opacity:.6; cursor:not-allowed; }
.btn-fetch { background:var(--sun-light) !important; border-color:var(--peach) !important; color:var(--sun-deep) !important; }
.btn-fetch:disabled { opacity:.6; cursor:not-allowed; }
.btn-source { background:var(--surface-3) !important; }
.btn-source:disabled { opacity:.4; cursor:not-allowed; }
.btn-del-page { color:var(--err) !important; border-color:#FFCDD2 !important; background:#FFF5F5 !important; }
.btn-del-page:hover { background:var(--err) !important; color:#fff !important; }
</style>
