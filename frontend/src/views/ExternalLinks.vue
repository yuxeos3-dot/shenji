<template>
  <div class="extlinks-page">
    <div class="panel selector-bar">
      <span class="selector-label">🔗 外链管理</span>
      <el-select v-model="selectedSiteId" placeholder="选择站点" style="width:240px" @change="onSiteChange">
        <el-option v-for="s in sites" :key="s.id" :label="s.name + ' (' + s.domain + ')'" :value="s.id" />
      </el-select>
      <button class="btn btn-primary btn-sm" @click="showAdd=true" v-if="selectedSiteId">＋ 添加外链</button>
      <el-input v-model="search" placeholder="搜索外链..." style="width:220px;margin-left:auto" clearable v-if="links.length" />
    </div>

    <div v-if="!selectedSiteId" class="empty-hint panel">
      <div class="empty-icon">🔗</div>
      <div>请先选择一个站点管理外链</div>
    </div>

    <template v-else>
      <!-- 统计卡片 -->
      <div class="summary-row" v-if="links.length">
        <div class="summary-card panel">
          <div class="sc-label">外链总数</div>
          <div class="sc-value num-blue">{{ links.length }}</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">DoFollow</div>
          <div class="sc-value num-green">{{ doFollowCount }}</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">NoFollow</div>
          <div class="sc-value num-orange">{{ noFollowCount }}</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">待检测</div>
          <div class="sc-value" style="font-size:22px;font-weight:800;color:var(--ink-soft)">{{ pendingCount }}</div>
        </div>
      </div>

      <!-- 添加表单 -->
      <div class="section-card panel" v-if="showAdd">
        <div class="sc-title">{{ editingId ? '编辑外链' : '添加外链' }}</div>
        <div class="form-grid">
          <div class="form-field full-width">
            <label>外链目标 URL</label>
            <input class="field-input" v-model="form.url" placeholder="https://example.com/page" />
          </div>
          <div class="form-field">
            <label>锚文本</label>
            <input class="field-input" v-model="form.anchor_text" placeholder="点击这里" />
          </div>
          <div class="form-field">
            <label>放置页面 URL</label>
            <input class="field-input" v-model="form.source_url" placeholder="我方站点上哪个页面放了这条外链" />
          </div>
          <div class="form-field">
            <label>rel 属性</label>
            <el-select v-model="form.rel_type" style="width:100%">
              <el-option label="dofollow（传递权重）" value="dofollow" />
              <el-option label="nofollow" value="nofollow" />
              <el-option label="sponsored（赞助/广告）" value="sponsored" />
              <el-option label="ugc（用户生成）" value="ugc" />
            </el-select>
          </div>
          <div class="form-field">
            <label>分类标签</label>
            <input class="field-input" v-model="form.category" placeholder="如：友链、资源站、广告等" />
          </div>
          <div class="form-field">
            <label>备注</label>
            <input class="field-input" v-model="form.note" placeholder="可选备注" />
          </div>
        </div>
        <div class="action-row">
          <button class="btn btn-primary btn-sm" @click="saveLink" :disabled="saving">{{ saving ? '保存中...' : (editingId ? '保存修改' : '确认添加') }}</button>
          <button class="btn btn-sm" @click="cancelAdd">取消</button>
        </div>
      </div>

      <!-- 外链列表 -->
      <div class="panel result-panel" v-if="filteredLinks.length" v-loading="loading">
        <table class="table">
          <thead>
            <tr>
              <th>目标 URL</th>
              <th>锚文本</th>
              <th>放置页面</th>
              <th>属性</th>
              <th>分类</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="l in filteredLinks" :key="l.id">
              <td class="url-cell">
                <a :href="l.url" target="_blank" class="url-link">{{ truncate(l.url, 50) }}</a>
              </td>
              <td><span class="anchor-text">{{ l.anchor_text || '—' }}</span></td>
              <td class="source-cell">{{ truncate(l.source_url, 40) || '—' }}</td>
              <td><span :class="['rel-badge', 'rel-'+l.rel_type]">{{ l.rel_type || 'dofollow' }}</span></td>
              <td><span class="cat-badge" v-if="l.category">{{ l.category }}</span><span v-else style="color:var(--ink-hint)">—</span></td>
              <td>
                <button class="btn btn-xs btn-check" @click="checkLink(l)" :disabled="l.checking">
                  {{ l.checking ? '检测中' : (l.status_code ? l.status_code : '检测') }}
                </button>
              </td>
              <td>
                <div style="display:flex;gap:6px">
                  <button class="btn btn-xs" @click="editLink(l)">编辑</button>
                  <button class="btn btn-xs btn-del" @click="deleteLink(l)">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-else-if="!showAdd" class="empty-hint panel">
        <div class="empty-icon">🔗</div>
        <div>暂无外链记录</div>
        <div style="font-size:12px;color:var(--ink-hint);margin-top:6px">点击「添加外链」开始管理</div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import apiClient from '../api'

const sites = ref([])
const selectedSiteId = ref(null)
const links = ref([])
const loading = ref(false)
const saving = ref(false)
const showAdd = ref(false)
const editingId = ref(null)
const search = ref('')
const form = ref({ url: '', anchor_text: '', source_url: '', rel_type: 'dofollow', category: '', note: '' })

const doFollowCount = computed(() => links.value.filter(l => !l.rel_type || l.rel_type === 'dofollow').length)
const noFollowCount = computed(() => links.value.filter(l => l.rel_type === 'nofollow').length)
const pendingCount = computed(() => links.value.filter(l => !l.status_code).length)

const filteredLinks = computed(() => {
  if (!search.value) return links.value
  const q = search.value.toLowerCase()
  return links.value.filter(l => l.url?.toLowerCase().includes(q) || l.anchor_text?.toLowerCase().includes(q) || l.category?.toLowerCase().includes(q))
})

function truncate(str, len) { return str && str.length > len ? str.substring(0, len) + '...' : str }
function onSiteChange() { links.value = []; loadLinks() }
function cancelAdd() { showAdd.value = false; editingId.value = null; form.value = { url: '', anchor_text: '', source_url: '', rel_type: 'dofollow', category: '', note: '' } }

async function loadLinks() {
  if (!selectedSiteId.value) return
  loading.value = true
  try {
    const res = await apiClient.getExternalLinks(selectedSiteId.value)
    const d = res.data
    links.value = (d.items || d || []).map(l => ({ ...l, checking: false }))
  } catch {}
  loading.value = false
}

async function saveLink() {
  if (!form.value.url) { ElMessage.warning('请填写目标 URL'); return }
  if (!form.value.url.startsWith('http')) { ElMessage.warning('URL 必须以 http:// 或 https:// 开头'); return }
  saving.value = true
  try {
    if (editingId.value) {
      await apiClient.updateExternalLink(editingId.value, { ...form.value, site_id: selectedSiteId.value })
      ElMessage.success('外链已更新')
    } else {
      await apiClient.createExternalLink({ ...form.value, site_id: selectedSiteId.value })
      ElMessage.success('外链已添加')
    }
    cancelAdd(); loadLinks()
  } catch {}
  saving.value = false
}

function editLink(l) {
  editingId.value = l.id
  form.value = { url: l.url, anchor_text: l.anchor_text || '', source_url: l.source_url || '', rel_type: l.rel_type || 'dofollow', category: l.category || '', note: l.note || '' }
  showAdd.value = true
}

async function deleteLink(l) {
  try {
    await ElMessageBox.confirm(`确定删除外链 ${l.url} 吗？`, '确认删除', { type: 'warning', confirmButtonText: '确认', cancelButtonText: '取消' })
    await apiClient.deleteExternalLink(l.id)
    ElMessage.success('已删除'); loadLinks()
  } catch {}
}

async function checkLink(l) {
  l.checking = true
  try {
    const res = await apiClient.checkExternalLink(l.id)
    l.status_code = res.data.status_code
    ElMessage.success(`检测完成：HTTP ${res.data.status_code}`)
  } catch {}
  l.checking = false
}

onMounted(async () => {
  const res = await apiClient.listSites().catch(() => ({ data: [] }))
  sites.value = res.data || []
  if (sites.value.length) { selectedSiteId.value = sites.value[0].id; loadLinks() }
})
</script>

<style scoped>
.extlinks-page { display:flex; flex-direction:column; gap:16px; }
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:16px 22px; }
.selector-bar { display:flex; align-items:center; gap:12px; flex-wrap:wrap; }
.selector-label { font-size:13.5px; font-weight:700; color:var(--ink-mid); white-space:nowrap; }

.summary-row { display:grid; grid-template-columns:repeat(4,1fr); gap:12px; }
.summary-card { padding:14px 18px; }
.sc-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.07em; margin-bottom:6px; }
.sc-value { font-size:26px; font-weight:800; font-family:'JetBrains Mono',monospace; }
.num-blue { color:var(--info); } .num-green { color:var(--ok); } .num-orange { color:var(--warn); }

.section-card { }
.sc-title { font-size:15px; font-weight:700; color:var(--ink); margin-bottom:14px; }
.form-grid { display:grid; grid-template-columns:1fr 1fr; gap:14px; margin-bottom:16px; }
.form-field { display:flex; flex-direction:column; gap:6px; }
.form-field.full-width { grid-column:1/-1; }
.form-field label { font-size:12.5px; font-weight:600; color:var(--ink-mid); }
.field-input { width:100%; padding:9px 12px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13.5px; font-family:inherit; background:var(--surface); color:var(--ink); outline:none; transition:border-color .18s; }
.field-input:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); }
.action-row { display:flex; gap:10px; align-items:center; }

.result-panel { padding:0; overflow:hidden; }
.table { width:100%; border-collapse:collapse; }
.table th { padding:10px 14px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.08em; border-bottom:2px solid var(--border); background:var(--surface-3); }
.table td { padding:10px 14px; border-bottom:1px solid var(--border-light); font-size:12.5px; vertical-align:middle; }
.table tr:hover td { background:var(--sun-light); }

.url-cell { max-width:260px; } .url-link { color:var(--info); font-size:12px; word-break:break-all; }
.anchor-text { font-weight:600; color:var(--ink); }
.source-cell { font-size:11.5px; color:var(--ink-soft); max-width:200px; }

.rel-badge { display:inline-block; padding:2px 8px; border-radius:5px; font-size:11px; font-weight:700; }
.rel-dofollow { background:#DCFCE7; color:#166534; }
.rel-nofollow { background:#FEF9C3; color:#854D0E; }
.rel-sponsored { background:#DBEAFE; color:#1D4ED8; }
.rel-ugc { background:var(--surface-3); color:var(--ink-soft); }
.cat-badge { display:inline-block; padding:2px 8px; background:var(--sun-light); color:var(--sun-deep); border-radius:5px; font-size:11.5px; font-weight:600; }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 18px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; }
.btn-sm { padding:6px 14px; font-size:12.5px; }
.btn-xs { padding:4px 12px; font-size:12px; border-radius:6px; }
.btn-check { background:var(--surface-3); font-family:'JetBrains Mono',monospace; }
.btn-check:disabled { opacity:.6; cursor:not-allowed; }
.btn-del { color:var(--err) !important; border-color:#FFCDD2 !important; background:#FFF5F5 !important; }
.btn-del:hover { background:var(--err) !important; color:#fff !important; }

.empty-hint { text-align:center; padding:50px; color:var(--ink-hint); }
.empty-icon { font-size:44px; margin-bottom:12px; }
</style>
