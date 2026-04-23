<template>
  <!-- Keyword/topic list view shown when a user opens a library.
       Each row has 编辑 / 删除 / AI 生成 buttons. Also exposes
       manual-import and GSC-import entry points at the top. -->
  <div>
    <div class="hdr-row">
      <div class="lib-meta">
        <div class="lib-title">📚 {{ lib?.name }}</div>
        <div class="lib-desc">{{ lib?.description || '（无描述）' }} · 共 {{ total }} 条</div>
      </div>
      <div class="hdr-actions">
        <button class="btn" @click="$emit('back')">← 返回库列表</button>
        <button class="btn" @click="showImport = true">✏️ 手动输入</button>
        <button class="btn btn-platforms" @click="doPlatformsImport" :disabled="platformsImporting">
          {{ platformsImporting ? '拉取中...' : '🔄 从4平台拉取' }}
        </button>
      </div>
    </div>

    <table class="table">
      <thead>
        <tr>
          <th style="width:50%">关键词 / 话题</th>
          <th>关联关键词</th>
          <th style="width:90px">已使用</th>
          <th style="width:260px">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="t in items" :key="t.id">
          <td style="font-weight:600">{{ t.title }}</td>
          <td style="color:var(--ink-soft);font-size:12px">{{ t.keyword || '—' }}</td>
          <td>
            <span v-if="t.is_used" class="badge badge-ok">✓ 已生成</span>
            <span v-else class="badge badge-muted">—</span>
          </td>
          <td>
            <div class="action-btns">
              <button class="btn btn-xs" @click="openEdit(t)">编辑</button>
              <button class="btn btn-xs btn-danger-xs" @click="del(t)">删除</button>
              <button class="btn btn-xs btn-primary-xs" @click="genAI(t)">🤖 AI 生成</button>
            </div>
          </td>
        </tr>
        <tr v-if="!items.length">
          <td colspan="4" class="empty-cell">
            {{ loading ? '加载中...' : '这个库里还没有关键词。点右上角「手动输入」或「🔄 从4平台拉取」开始。' }}
          </td>
        </tr>
      </tbody>
    </table>

    <div v-if="total > perPage" class="pag-row">
      <el-pagination
        v-model:current-page="page"
        :page-size="perPage"
        :total="total"
        layout="prev, pager, next, total"
        @current-change="reload"
      />
    </div>

    <!-- Manual import dialog -->
    <el-dialog v-model="showImport" title="✏️ 手动输入关键词 / 话题" width="540px">
      <div class="field">
        <label>一行一条（多行会被自动去重、去空行）</label>
        <el-input
          v-model="importText"
          type="textarea"
          :rows="12"
          placeholder="成人娱乐产业市场规模\n订阅制成人平台\n..."
        />
      </div>
      <template #footer>
        <el-button @click="showImport = false">取消</el-button>
        <el-button type="primary" :loading="importing" @click="doManualImport">导入</el-button>
      </template>
    </el-dialog>

    <!-- Edit keyword dialog -->
    <el-dialog v-model="showEdit" title="✏️ 编辑关键词" width="480px">
      <div class="field">
        <label>标题 / 话题</label>
        <el-input v-model="editRow.title" />
      </div>
      <div class="field">
        <label>关联关键词（可选）</label>
        <el-input v-model="editRow.keyword" placeholder="留空则用标题作为关键词" />
      </div>
      <template #footer>
        <el-button @click="showEdit = false">取消</el-button>
        <el-button type="primary" @click="saveEdit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const props = defineProps({
  lib: { type: Object, required: true },
  siteId: { type: [Number, String], default: 0 },
})
const emit = defineEmits(['back', 'open-wizard'])

const items = ref([])
const total = ref(0)
const page = ref(1)
const perPage = 50
const loading = ref(false)

const showImport = ref(false)
const importText = ref('')
const importing = ref(false)

const showEdit = ref(false)
const editRow = ref({ id: 0, title: '', keyword: '' })

onMounted(reload)
watch(() => props.lib?.id, reload)

async function reload() {
  if (!props.lib?.id) return
  loading.value = true
  try {
    const res = await api.geoListTitles(props.lib.id, { page: page.value, per_page: perPage })
    items.value = res.data.items || []
    total.value = res.data.total || 0
  } catch (e) {
    ElMessage.error('加载关键词列表失败')
  } finally {
    loading.value = false
  }
}

async function doManualImport() {
  if (!importText.value.trim()) {
    ElMessage.warning('请输入关键词')
    return
  }
  importing.value = true
  try {
    const res = await api.geoImportTitles(props.lib.id, { text: importText.value })
    ElMessage.success('导入 ' + (res.data?.imported || 0) + ' 条')
    showImport.value = false
    importText.value = ''
    page.value = 1
    reload()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '导入失败')
  } finally {
    importing.value = false
  }
}

const platformsImporting = ref(false)
async function doPlatformsImport() {
  if (!props.siteId) {
    ElMessage.warning('请先选择站点')
    return
  }
  platformsImporting.value = true
  try {
    const res = await api.geoImportFromPlatforms(props.lib.id, { site_id: props.siteId })
    const d = res.data || {}
    const syncNote = d.auto_synced ? '（已自动触发深度同步）' : ''
    ElMessage.success(
      '✅ 从4平台导入 ' + (d.imported || 0) + ' 条关键词' +
      ' · 来源 ' + (d.source_total || 0) + ' 条（已去重 ' + (d.skipped || 0) + ' 条）' + syncNote
    )
    reload()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '拉取失败 — 请先在「数据管理」同步4大平台')
  } finally {
    platformsImporting.value = false
  }
}

function openEdit(t) {
  editRow.value = { id: t.id, title: t.title, keyword: t.keyword || '' }
  showEdit.value = true
}

async function saveEdit() {
  // Since there's no dedicated update-one endpoint, we re-import over
  // the existing row: delete then add. This is safe per row.
  if (!editRow.value.title?.trim()) {
    ElMessage.warning('标题不能为空')
    return
  }
  try {
    await api.geoDeleteTitle(editRow.value.id)
    await api.geoAddTitles(props.lib.id, {
      titles: [{ title: editRow.value.title, keyword: editRow.value.keyword }],
    })
    ElMessage.success('已更新')
    showEdit.value = false
    reload()
  } catch (e) {
    ElMessage.error('保存失败')
  }
}

async function del(t) {
  try {
    await ElMessageBox.confirm('删除关键词「' + t.title + '」？', '删除确认', { type: 'warning' })
    await api.geoDeleteTitle(t.id)
    ElMessage.success('已删除')
    reload()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败')
  }
}

function genAI(t) {
  emit('open-wizard', t)
}
</script>

<style scoped>
.hdr-row { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 14px; gap: 12px; }
.lib-title { font-size: 16px; font-weight: 800; color: var(--ink); }
.lib-desc { font-size: 12px; color: var(--ink-soft); margin-top: 4px; }
.hdr-actions { display: flex; gap: 8px; flex-shrink: 0; }

.table { width: 100%; border-collapse: collapse; background: var(--surface); border-radius: 10px; overflow: hidden; }
.table th {
  padding: 10px 12px; text-align: left; font-size: 11px; font-weight: 800;
  color: var(--ink-mid); text-transform: uppercase; letter-spacing: .08em;
  border-bottom: 2px solid var(--border); background: var(--surface-3);
}
.table td { padding: 10px 12px; border-bottom: 1px solid var(--border-light); font-size: 13px; }
.table tr:hover td { background: var(--sun-light); }

.badge { display: inline-block; font-size: 11px; font-weight: 800; padding: 2px 10px; border-radius: 20px; }
.badge-ok { background: var(--ok-bg); color: var(--ok); border: 1px solid var(--ok-border); }
.badge-muted { background: var(--surface-3); color: var(--ink-hint); border: 1px solid var(--border); }

.action-btns { display: flex; gap: 4px; flex-wrap: wrap; }
.btn {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 6px 14px; border-radius: 8px; border: 1.5px solid var(--border);
  background: var(--surface); cursor: pointer; font-family: inherit;
  font-size: 13px; font-weight: 700; color: var(--ink-mid); transition: all .18s;
}
.btn:hover { background: var(--sun-light); border-color: var(--peach); color: var(--sun-deep); }
.btn-xs { padding: 3px 10px; font-size: 11px; }
.btn-green { background: linear-gradient(135deg, var(--ok), #10b981); color: #fff; border-color: transparent; }
.btn-green:hover { background: linear-gradient(135deg, #10b981, #059669); color: #fff; }
.btn-primary-xs { color: #fff; background: linear-gradient(135deg, var(--sun), var(--coral)); border-color: transparent; }
.btn-primary-xs:hover { background: linear-gradient(135deg, var(--sun-deep), var(--sun)); color: #fff; }
.btn-danger-xs { color: var(--err); border-color: var(--err-border); background: var(--err-bg); }
.btn-danger-xs:hover { background: var(--err); color: #fff; border-color: var(--err); }

.empty-cell { text-align: center; color: var(--ink-hint); padding: 40px !important; font-size: 13px; }
.pag-row { display: flex; justify-content: center; margin-top: 16px; }
.field { display: flex; flex-direction: column; gap: 4px; margin-bottom: 12px; }
.field label { font-size: 12px; font-weight: 700; color: var(--ink-mid); }

.btn-platforms { background: linear-gradient(135deg, #3b82f6, #8b5cf6); color: #fff; border-color: transparent; }
.btn-platforms:hover { opacity: 0.9; }
.btn-platforms:disabled { opacity: 0.6; cursor: not-allowed; }
</style>
