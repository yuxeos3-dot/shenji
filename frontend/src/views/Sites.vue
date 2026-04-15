<template>
  <div class="sites-page">
    <!-- Page Header -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">Sites</h1>
        <span class="page-subtitle">Manage your websites and trigger audits</span>
      </div>
      <el-button class="btn-sunrise" @click="openAddDialog">
        <span class="btn-icon">+</span> Add Site
      </el-button>
    </div>

    <!-- Stats Row -->
    <div class="stats-row">
      <div class="stat-card panel">
        <div class="stat-emoji">🌐</div>
        <div class="stat-info">
          <div class="stat-value">{{ sites.length }}</div>
          <div class="stat-label">Total Sites</div>
        </div>
      </div>
      <div class="stat-card panel">
        <div class="stat-emoji">✅</div>
        <div class="stat-info">
          <div class="stat-value">{{ activeSites }}</div>
          <div class="stat-label">Active</div>
        </div>
      </div>
      <div class="stat-card panel">
        <div class="stat-emoji">🔍</div>
        <div class="stat-info">
          <div class="stat-value">{{ auditedSites }}</div>
          <div class="stat-label">Audited</div>
        </div>
      </div>
    </div>

    <!-- Sites Table -->
    <div class="panel table-panel">
      <el-table
        :data="sites"
        v-loading="loading"
        class="sunrise-table"
        :header-cell-style="headerCellStyle"
        :row-class-name="tableRowClassName"
      >
        <el-table-column prop="name" label="SITE NAME" min-width="160">
          <template #default="{ row }">
            <div class="site-name-cell">
              <span class="site-icon">🌐</span>
              <span class="site-name-text">{{ row.name || row.domain }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="domain" label="DOMAIN" min-width="180">
          <template #default="{ row }">
            <span class="domain-text">{{ row.domain }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="STATUS" width="140">
          <template #default="{ row }">
            <span class="status-badge" :class="'status-' + row.status">
              {{ row.status === 'active' ? '🟢' : '⚪' }}
              {{ row.status === 'active' ? 'Active' : 'Inactive' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="last_audit_status" label="LAST AUDIT" width="160">
          <template #default="{ row }">
            <span v-if="row.last_audit_status" class="audit-badge" :class="'audit-' + row.last_audit_status">
              {{ auditEmoji(row.last_audit_status) }}
              {{ row.last_audit_status }}
            </span>
            <span v-else class="no-audit">--</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="CREATED" width="180">
          <template #default="{ row }">
            <span class="date-text">{{ row.created_at }}</span>
          </template>
        </el-table-column>
        <el-table-column label="ACTIONS" width="320" fixed="right">
          <template #default="{ row }">
            <div class="action-btns">
              <el-button class="btn-action btn-audit" size="small" @click="triggerAudit(row)">
                🔍 Audit
              </el-button>
              <el-button class="btn-action btn-detail" size="small" @click="$router.push(`/sites/${row.id}`)">
                📋 Details
              </el-button>
              <el-button class="btn-action btn-data" size="small" @click="$router.push(`/sites/${row.id}/data`)">
                📊 Data
              </el-button>
              <el-button class="btn-action btn-delete" size="small" @click="deleteSite(row)">
                🗑️
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- Add/Edit Dialog -->
    <el-dialog
      v-model="showDialog"
      :title="editId ? '✏️ Edit Site' : '🌐 Add New Site'"
      width="620px"
      class="sunrise-dialog"
      :close-on-click-modal="false"
    >
      <el-form :model="form" label-width="120px" class="sunrise-form">
        <div class="form-section-title">Basic Information</div>
        <el-form-item label="Domain">
          <el-input v-model="form.domain" placeholder="example.com" />
        </el-form-item>
        <el-form-item label="Site Name">
          <el-input v-model="form.name" placeholder="My Website" />
        </el-form-item>

        <div class="form-section-title">Analytics Integration</div>
        <el-form-item label="GSC URL">
          <el-input v-model="form.gsc_site_url" placeholder="sc-domain:example.com" />
        </el-form-item>
        <el-form-item label="GA Property">
          <el-input v-model="form.ga_property_id" placeholder="123456789" />
        </el-form-item>
        <el-form-item label="Yandex Host">
          <el-input v-model="form.yandex_host_id" placeholder="Host ID" />
        </el-form-item>

        <div class="form-section-title">Page URL Configuration <span class="optional-tag">Optional</span></div>
        <el-form-item label="Category">
          <el-input v-model="form.url_category" placeholder="/category/movies" />
        </el-form-item>
        <el-form-item label="Detail">
          <el-input v-model="form.url_detail" placeholder="/movie/123" />
        </el-form-item>
        <el-form-item label="News List">
          <el-input v-model="form.url_news_list" placeholder="/news" />
        </el-form-item>
        <el-form-item label="News Detail">
          <el-input v-model="form.url_news_detail" placeholder="/news/123" />
        </el-form-item>
        <el-form-item label="Search">
          <el-input v-model="form.url_search" placeholder="/search?q=test" />
        </el-form-item>
        <el-form-item label="Static Page">
          <el-input v-model="form.url_static" placeholder="/about" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button class="btn-cancel" @click="showDialog = false">Cancel</el-button>
          <el-button class="btn-sunrise" @click="saveSite">
            {{ editId ? 'Update Site' : 'Add Site' }}
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
  fontSize: '11px',
  letterSpacing: '0.08em',
  textTransform: 'uppercase',
  border: 'none',
  padding: '14px 12px',
  fontFamily: "'Plus Jakarta Sans', sans-serif"
})

function tableRowClassName({ rowIndex }) {
  return rowIndex % 2 === 0 ? 'row-even' : 'row-odd'
}

function auditEmoji(status) {
  if (status === 'done') return '✅'
  if (status === 'running') return '⏳'
  if (status === 'failed') return '❌'
  return '⚪'
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
  } catch { /* handled */ }
  finally { loading.value = false }
}

async function saveSite() {
  try {
    if (editId.value) {
      await api.updateSite(editId.value, form.value)
    } else {
      await api.createSite(form.value)
    }
    ElMessage.success('Site saved successfully')
    showDialog.value = false
    loadSites()
  } catch { /* handled */ }
}

async function deleteSite(row) {
  try {
    await ElMessageBox.confirm(
      `Are you sure you want to delete "${row.name || row.domain}"?`,
      'Delete Site',
      { type: 'warning', confirmButtonText: 'Delete', cancelButtonText: 'Cancel' }
    )
    await api.deleteSite(row.id)
    ElMessage.success('Site deleted')
    loadSites()
  } catch { /* handled */ }
}

async function triggerAudit(row) {
  try {
    const res = await api.triggerAudit(row.id)
    ElMessage.success(`Audit started (ID: ${res.data.task_id})`)
    loadSites()
  } catch { /* handled */ }
}

onMounted(loadSites)
</script>

<style scoped>
.sites-page {
  padding: 0;
  font-family: 'Plus Jakarta Sans', sans-serif;
}

/* ---- Page Header ---- */
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 28px;
}
.header-left {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.page-title {
  margin: 0;
  font-size: 28px;
  font-weight: 800;
  color: var(--ink);
  letter-spacing: -0.02em;
}
.page-subtitle {
  font-size: 14px;
  color: var(--ink-soft);
  font-weight: 500;
}

/* ---- Stats Row ---- */
.stats-row {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 18px;
  margin-bottom: 24px;
}
.stat-card {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px 24px;
}
.stat-emoji {
  font-size: 32px;
  line-height: 1;
}
.stat-info {
  display: flex;
  flex-direction: column;
}
.stat-value {
  font-size: 26px;
  font-weight: 800;
  color: var(--ink);
}
.stat-label {
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--ink-soft);
  margin-top: 2px;
}

/* ---- Panel (shared) ---- */
.panel {
  background: var(--surface);
  border: 1.5px solid var(--border-light);
  border-radius: var(--r);
  box-shadow: var(--s1);
  transition: box-shadow 0.2s ease;
}
.panel:hover {
  box-shadow: var(--s2);
}

/* ---- Table Panel ---- */
.table-panel {
  overflow: hidden;
  padding: 0;
}

/* ---- Sunrise Table ---- */
.sunrise-table {
  --el-table-border-color: var(--border-light);
  --el-table-row-hover-bg-color: var(--sun-glow);
  --el-table-bg-color: transparent;
  --el-table-tr-bg-color: transparent;
  --el-table-header-bg-color: transparent;
  font-family: 'Plus Jakarta Sans', sans-serif;
  width: 100%;
}

:deep(.sunrise-table .el-table__header-wrapper) {
  border-radius: var(--r) var(--r) 0 0;
}

:deep(.sunrise-table td.el-table__cell) {
  padding: 14px 12px;
  border-bottom: 1px solid var(--border-light);
  font-size: 13.5px;
  color: var(--ink);
}

:deep(.sunrise-table .row-even td) {
  background: var(--surface);
}
:deep(.sunrise-table .row-odd td) {
  background: var(--surface-2);
}

:deep(.sunrise-table .el-table__body tr:hover > td) {
  background: var(--sun-glow) !important;
}

/* ---- Cell Styles ---- */
.site-name-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}
.site-icon {
  font-size: 18px;
}
.site-name-text {
  font-weight: 700;
  color: var(--ink);
}
.domain-text {
  color: var(--ink-mid);
  font-weight: 500;
  font-size: 13px;
}
.date-text {
  color: var(--ink-soft);
  font-size: 12.5px;
  font-weight: 500;
}

/* ---- Status Badge ---- */
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 14px;
  border-radius: 20px;
  font-size: 12.5px;
  font-weight: 700;
}
.status-active {
  background: #E8F5E9;
  color: var(--ok);
}
.status-inactive,
.status-undefined {
  background: var(--surface-3);
  color: var(--ink-hint);
}

/* ---- Audit Badge ---- */
.audit-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 700;
  text-transform: capitalize;
}
.audit-done {
  background: #E8F5E9;
  color: var(--ok);
}
.audit-running {
  background: #FFF8E1;
  color: var(--warn);
}
.audit-failed {
  background: #FFEBEE;
  color: var(--err);
}
.no-audit {
  color: var(--ink-hint);
  font-size: 13px;
}

/* ---- Action Buttons ---- */
.action-btns {
  display: flex;
  gap: 6px;
  align-items: center;
}
.btn-action {
  border-radius: var(--r-xs) !important;
  font-weight: 700 !important;
  font-size: 12px !important;
  padding: 5px 12px !important;
  border: 1.5px solid var(--border-light) !important;
  background: var(--surface) !important;
  color: var(--ink-mid) !important;
  transition: all 0.2s ease !important;
  font-family: 'Plus Jakarta Sans', sans-serif !important;
}
.btn-action:hover {
  border-color: var(--sun) !important;
  color: var(--sun-deep) !important;
  background: var(--sun-light) !important;
  box-shadow: var(--s-amber) !important;
}
.btn-audit {
  background: var(--sun-light) !important;
  border-color: var(--peach) !important;
  color: var(--sun-deep) !important;
}
.btn-audit:hover {
  background: linear-gradient(135deg, var(--sun), var(--coral)) !important;
  color: #fff !important;
  border-color: var(--sun) !important;
}
.btn-delete {
  border-color: #FFCDD2 !important;
  color: var(--err) !important;
  background: #FFF5F5 !important;
}
.btn-delete:hover {
  background: var(--err) !important;
  color: #fff !important;
  border-color: var(--err) !important;
}

/* ---- Primary Sunrise Button ---- */
.btn-sunrise {
  background: linear-gradient(135deg, var(--sun), var(--coral)) !important;
  border: none !important;
  color: #fff !important;
  font-weight: 700 !important;
  font-size: 14px !important;
  padding: 10px 24px !important;
  border-radius: var(--r-sm) !important;
  box-shadow: var(--s-amber) !important;
  transition: all 0.2s ease !important;
  font-family: 'Plus Jakarta Sans', sans-serif !important;
  letter-spacing: 0.01em;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.btn-sunrise:hover {
  filter: brightness(1.06);
  transform: translateY(-1px);
  box-shadow: 0 6px 20px rgba(255, 140, 0, 0.35) !important;
}
.btn-icon {
  font-size: 18px;
  font-weight: 800;
  line-height: 1;
}

/* ---- Cancel Button ---- */
.btn-cancel {
  border: 1.5px solid var(--border) !important;
  background: var(--surface) !important;
  color: var(--ink-mid) !important;
  font-weight: 600 !important;
  border-radius: var(--r-sm) !important;
  padding: 10px 24px !important;
  font-family: 'Plus Jakarta Sans', sans-serif !important;
}
.btn-cancel:hover {
  border-color: var(--ink-hint) !important;
  color: var(--ink) !important;
}

/* ---- Dialog ---- */
.sunrise-dialog {
  font-family: 'Plus Jakarta Sans', sans-serif;
}
:deep(.sunrise-dialog .el-dialog) {
  border-radius: var(--r) !important;
  border: 1.5px solid var(--border-light);
  box-shadow: 0 20px 60px rgba(45, 27, 14, 0.12);
  overflow: hidden;
}
:deep(.sunrise-dialog .el-dialog__header) {
  background: linear-gradient(135deg, var(--sun), var(--coral));
  padding: 20px 24px;
  margin: 0;
}
:deep(.sunrise-dialog .el-dialog__title) {
  color: #fff;
  font-weight: 800;
  font-size: 18px;
  font-family: 'Plus Jakarta Sans', sans-serif;
}
:deep(.sunrise-dialog .el-dialog__headerbtn .el-dialog__close) {
  color: #fff;
  font-size: 18px;
}
:deep(.sunrise-dialog .el-dialog__body) {
  padding: 24px;
  background: var(--surface);
}
:deep(.sunrise-dialog .el-dialog__footer) {
  padding: 16px 24px;
  background: var(--surface-2);
  border-top: 1px solid var(--border-light);
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

/* ---- Form ---- */
.form-section-title {
  font-size: 13px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--sun-deep);
  margin: 20px 0 14px 0;
  padding-bottom: 8px;
  border-bottom: 2px solid var(--sun-glow);
  display: flex;
  align-items: center;
  gap: 8px;
}
.form-section-title:first-child {
  margin-top: 0;
}
.optional-tag {
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  background: var(--sun-glow);
  color: var(--sun-deep);
  padding: 2px 8px;
  border-radius: 10px;
  letter-spacing: 0.04em;
}

:deep(.sunrise-form .el-form-item__label) {
  font-weight: 600;
  color: var(--ink-mid);
  font-size: 13px;
  font-family: 'Plus Jakarta Sans', sans-serif;
}
:deep(.sunrise-form .el-input__wrapper) {
  border-radius: var(--r-xs);
  border: 1.5px solid var(--border);
  box-shadow: none !important;
  padding: 4px 12px;
  transition: border-color 0.2s ease;
}
:deep(.sunrise-form .el-input__wrapper:hover) {
  border-color: var(--peach);
}
:deep(.sunrise-form .el-input__wrapper.is-focus) {
  border-color: var(--sun) !important;
  box-shadow: 0 0 0 3px rgba(255, 140, 0, 0.12) !important;
}
:deep(.sunrise-form .el-input__inner) {
  font-family: 'Plus Jakarta Sans', sans-serif;
  font-size: 13.5px;
  color: var(--ink);
}
:deep(.sunrise-form .el-input__inner::placeholder) {
  color: var(--ink-hint);
}
</style>
