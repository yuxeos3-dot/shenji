<template>
  <div class="sites-page">
    <!-- 页头 -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">站点管理</h1>
        <span class="page-subtitle">管理所有监控站点，触发巡检任务</span>
      </div>
      <el-button class="btn-sunrise" @click="openAddDialog">
        <span class="btn-icon">+</span> 添加站点
      </el-button>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card panel">
        <div class="stat-emoji">🌐</div>
        <div class="stat-info">
          <div class="stat-value">{{ sites.length }}</div>
          <div class="stat-label">站点总数</div>
        </div>
      </div>
      <div class="stat-card panel">
        <div class="stat-emoji">✅</div>
        <div class="stat-info">
          <div class="stat-value">{{ activeSites }}</div>
          <div class="stat-label">启用中</div>
        </div>
      </div>
      <div class="stat-card panel">
        <div class="stat-emoji">🔍</div>
        <div class="stat-info">
          <div class="stat-value">{{ auditedSites }}</div>
          <div class="stat-label">已审计</div>
        </div>
      </div>
    </div>

    <!-- 站点列表 -->
    <div class="panel table-panel">
      <el-table
        :data="sites"
        v-loading="loading"
        class="sunrise-table"
        :header-cell-style="headerCellStyle"
        :row-class-name="tableRowClassName"
      >
        <el-table-column prop="name" label="站点名称" min-width="160">
          <template #default="{ row }">
            <div class="site-name-cell">
              <span class="site-icon">🌐</span>
              <span class="site-name-text">{{ row.name || row.domain }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="domain" label="域名" min-width="180">
          <template #default="{ row }">
            <span class="domain-text">{{ row.domain }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="120">
          <template #default="{ row }">
            <span class="status-badge" :class="'status-' + row.status">
              {{ row.status === 'active' ? '🟢 启用' : '⚪ 停用' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="last_audit_status" label="最近巡检" width="140">
          <template #default="{ row }">
            <span v-if="row.last_audit_status" class="audit-badge" :class="'audit-' + row.last_audit_status">
              {{ auditLabel(row.last_audit_status) }}
            </span>
            <span v-else class="no-audit">暂未巡检</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="添加时间" width="180">
          <template #default="{ row }">
            <span class="date-text">{{ row.created_at }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="300" fixed="right">
          <template #default="{ row }">
            <div class="action-btns">
              <el-button class="btn-action btn-audit" size="small" @click="triggerAudit(row)">🔍 立即巡检</el-button>
              <el-button class="btn-action btn-detail" size="small" @click="$router.push(`/sites/${row.id}`)">📋 详情</el-button>
              <el-button class="btn-action btn-data" size="small" @click="$router.push(`/sites/${row.id}/data`)">📊 数据</el-button>
              <el-button class="btn-action btn-delete" size="small" @click="deleteSite(row)">🗑️</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 添加/编辑弹窗 -->
    <el-dialog
      v-model="showDialog"
      :title="editId ? '✏️ 编辑站点' : '🌐 添加站点'"
      width="580px"
      class="sunrise-dialog"
      :close-on-click-modal="false"
    >
      <el-form :model="form" label-width="110px" class="sunrise-form">
        <div class="form-section-title">基本信息</div>
        <el-form-item label="域名">
          <el-input v-model="form.domain" placeholder="https://example.com（必须包含 https:// 或 http://）" />
        </el-form-item>
        <el-form-item label="站点名称">
          <el-input v-model="form.name" placeholder="我的网站" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button class="btn-cancel" @click="showDialog = false">取 消</el-button>
          <el-button class="btn-sunrise" @click="saveSite">
            {{ editId ? '保存修改' : '确认添加' }}
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const sites = ref([])
const loading = ref(false)
const showDialog = ref(false)
const editId = ref(null)
const form = ref({
  domain: '', name: '', gsc_site_url: '', ga_property_id: '',
  yandex_host_id: '', url_category: '', url_detail: '',
  url_news_list: '', url_news_detail: '', url_search: '', url_static: ''
})

const activeSites = computed(() => sites.value.filter(s => s.status === 'active').length)
const auditedSites = computed(() => sites.value.filter(s => s.last_audit_status === 'done').length)

const headerCellStyle = () => ({
  background: 'linear-gradient(135deg, var(--sun), var(--coral))',
  color: '#FFFFFF',
  fontWeight: '800',
  fontSize: '12px',
  letterSpacing: '0.04em',
  border: 'none',
  padding: '14px 12px',
})

function tableRowClassName({ rowIndex }) {
  return rowIndex % 2 === 0 ? 'row-even' : 'row-odd'
}

function auditLabel(status) {
  if (status === 'done') return '✅ 已完成'
  if (status === 'running') return '⏳ 进行中'
  if (status === 'pending') return '🕐 等待中'
  if (status === 'error') return '❌ 失败'
  return status
}

function openAddDialog() {
  editId.value = null
  form.value = {
    domain: '', name: '', gsc_site_url: '', ga_property_id: '',
    yandex_host_id: '', url_category: '', url_detail: '',
    url_news_list: '', url_news_detail: '', url_search: '', url_static: ''
  }
  showDialog.value = true
}

async function loadSites() {
  loading.value = true
  try {
    const res = await api.listSites()
    sites.value = res.data
  } catch {}
  finally { loading.value = false }
}

async function saveSite() {
  if (!form.value.domain) { ElMessage.warning('请填写域名'); return }
  if (!form.value.domain.startsWith('http://') && !form.value.domain.startsWith('https://')) {
    ElMessage.warning('域名必须以 https:// 或 http:// 开头'); return
  }
  if (!form.value.name) { ElMessage.warning('请填写站点名称'); return }
  try {
    if (editId.value) {
      await api.updateSite(editId.value, form.value)
      ElMessage.success('站点信息已更新')
    } else {
      await api.createSite(form.value)
      ElMessage.success('站点添加成功')
    }
    showDialog.value = false
    loadSites()
  } catch {}
}

async function deleteSite(row) {
  try {
    await ElMessageBox.confirm(
      `确定要删除站点「${row.name || row.domain}」吗？删除后不可恢复。`,
      '确认删除',
      { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' }
    )
    await api.deleteSite(row.id)
    ElMessage.success('站点已删除')
    loadSites()
  } catch {}
}

async function triggerAudit(row) {
  try {
    const res = await api.triggerAudit(row.id)
    ElMessage.success(`巡检任务已启动（任务ID：${res.data.task_id}）`)
    loadSites()
  } catch {}
}

onMounted(loadSites)
</script>

<style scoped>
.sites-page { padding: 0; }

.page-header { display:flex; justify-content:space-between; align-items:center; margin-bottom:28px; }
.header-left { display:flex; flex-direction:column; gap:4px; }
.page-title { margin:0; font-size:24px; font-weight:800; color:var(--ink); letter-spacing:-0.02em; }
.page-subtitle { font-size:13px; color:var(--ink-soft); font-weight:500; }

.stats-row { display:grid; grid-template-columns:repeat(3,1fr); gap:18px; margin-bottom:24px; }
.stat-card { display:flex; align-items:center; gap:16px; padding:20px 24px; }
.stat-emoji { font-size:32px; line-height:1; }
.stat-info { display:flex; flex-direction:column; }
.stat-value { font-size:26px; font-weight:800; color:var(--ink); }
.stat-label { font-size:12px; font-weight:600; letter-spacing:0.04em; color:var(--ink-soft); margin-top:2px; }

.panel { background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r); box-shadow:var(--s1); transition:box-shadow .2s; }
.panel:hover { box-shadow:var(--s2); }
.table-panel { overflow:hidden; padding:0; }

.sunrise-table {
  --el-table-border-color: var(--border-light);
  --el-table-row-hover-bg-color: var(--sun-glow);
  --el-table-bg-color: transparent;
  width:100%;
}
:deep(.sunrise-table td.el-table__cell) { padding:13px 12px; border-bottom:1px solid var(--border-light); font-size:13.5px; }
:deep(.sunrise-table .row-even td) { background:var(--surface); }
:deep(.sunrise-table .row-odd td) { background:var(--surface-2); }
:deep(.sunrise-table .el-table__body tr:hover > td) { background:var(--sun-glow) !important; }

.site-name-cell { display:flex; align-items:center; gap:10px; }
.site-icon { font-size:18px; }
.site-name-text { font-weight:700; color:var(--ink); }
.domain-text { color:var(--ink-mid); font-weight:500; font-size:13px; }
.date-text { color:var(--ink-soft); font-size:12.5px; }

.status-badge { display:inline-flex; align-items:center; gap:4px; padding:4px 12px; border-radius:20px; font-size:12.5px; font-weight:700; }
.status-active { background:#E8F5E9; color:var(--ok); }
.status-inactive { background:var(--surface-3); color:var(--ink-hint); }

.audit-badge { display:inline-flex; align-items:center; gap:4px; padding:4px 10px; border-radius:20px; font-size:12px; font-weight:700; }
.audit-done { background:#E8F5E9; color:var(--ok); }
.audit-running { background:#FFF8E1; color:var(--warn); }
.audit-error { background:#FFEBEE; color:var(--err); }
.no-audit { color:var(--ink-hint); font-size:13px; }

.action-btns { display:flex; gap:6px; align-items:center; }
.btn-action { border-radius:var(--r-xs) !important; font-weight:700 !important; font-size:12px !important; padding:5px 10px !important; border:1.5px solid var(--border-light) !important; background:var(--surface) !important; color:var(--ink-mid) !important; transition:all .2s !important; }
.btn-action:hover { border-color:var(--sun) !important; color:var(--sun-deep) !important; background:var(--sun-light) !important; }
.btn-audit { background:var(--sun-light) !important; border-color:var(--peach) !important; color:var(--sun-deep) !important; }
.btn-audit:hover { background:linear-gradient(135deg,var(--sun),var(--coral)) !important; color:#fff !important; border-color:var(--sun) !important; }
.btn-delete { border-color:#FFCDD2 !important; color:var(--err) !important; background:#FFF5F5 !important; }
.btn-delete:hover { background:var(--err) !important; color:#fff !important; border-color:var(--err) !important; }

.btn-sunrise { background:linear-gradient(135deg,var(--sun),var(--coral)) !important; border:none !important; color:#fff !important; font-weight:700 !important; font-size:14px !important; padding:10px 24px !important; border-radius:var(--r-sm) !important; box-shadow:var(--s-amber) !important; transition:all .2s !important; display:inline-flex; align-items:center; gap:6px; }
.btn-sunrise:hover { filter:brightness(1.06); transform:translateY(-1px); box-shadow:0 6px 20px rgba(255,140,0,.35) !important; }
.btn-icon { font-size:18px; font-weight:800; line-height:1; }
.btn-cancel { border:1.5px solid var(--border) !important; background:var(--surface) !important; color:var(--ink-mid) !important; font-weight:600 !important; border-radius:var(--r-sm) !important; padding:10px 24px !important; }
.btn-cancel:hover { border-color:var(--ink-hint) !important; color:var(--ink) !important; }

:deep(.sunrise-dialog .el-dialog) { border-radius:var(--r) !important; overflow:hidden; box-shadow:0 20px 60px rgba(45,27,14,.12); }
:deep(.sunrise-dialog .el-dialog__header) { background:linear-gradient(135deg,var(--sun),var(--coral)); padding:20px 24px; margin:0; }
:deep(.sunrise-dialog .el-dialog__title) { color:#fff; font-weight:800; font-size:17px; }
:deep(.sunrise-dialog .el-dialog__headerbtn .el-dialog__close) { color:#fff; font-size:18px; }
:deep(.sunrise-dialog .el-dialog__body) { padding:24px; background:var(--surface); }
:deep(.sunrise-dialog .el-dialog__footer) { padding:16px 24px; background:var(--surface-2); border-top:1px solid var(--border-light); }

.dialog-footer { display:flex; justify-content:flex-end; gap:10px; }

.form-section-title { font-size:12px; font-weight:800; letter-spacing:0.06em; color:var(--sun-deep); margin:20px 0 14px; padding-bottom:8px; border-bottom:2px solid var(--sun-glow); display:flex; align-items:center; gap:8px; }
.form-section-title:first-child { margin-top:0; }
.optional-tag { font-size:10px; font-weight:600; background:var(--sun-glow); color:var(--sun-deep); padding:2px 8px; border-radius:10px; }
.url-tip { font-size:12px; color:var(--ink-soft); margin:-8px 0 12px 0; line-height:1.5; }

:deep(.sunrise-form .el-form-item__label) { font-weight:600; color:var(--ink-mid); font-size:13px; }
:deep(.sunrise-form .el-input__wrapper) { border-radius:var(--r-xs); border:1.5px solid var(--border); box-shadow:none !important; padding:4px 12px; transition:border-color .2s; }
:deep(.sunrise-form .el-input__wrapper:hover) { border-color:var(--peach); }
:deep(.sunrise-form .el-input__wrapper.is-focus) { border-color:var(--sun) !important; box-shadow:0 0 0 3px rgba(255,140,0,.12) !important; }
:deep(.sunrise-form .el-input__inner) { font-size:13.5px; color:var(--ink); }
:deep(.sunrise-form .el-input__inner::placeholder) { color:var(--ink-hint); }
</style>
