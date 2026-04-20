<template>
  <div class="push-mgmt">
    <!-- Top bar: site selector -->
    <div class="panel selector-bar">
      <span class="selector-label">📤 选择站点</span>
      <el-select v-model="selectedSiteId" placeholder="请选择站点" style="width:280px" @change="onSiteChange">
        <el-option v-for="s in sites" :key="s.id" :label="s.name + ' (' + s.domain + ')'" :value="s.id" />
      </el-select>
    </div>

    <div v-if="!selectedSiteId" class="empty-hint panel">
      <div class="empty-icon">📤</div>
      <div>请先选择一个站点</div>
    </div>

    <template v-else>
      <el-tabs v-model="activeTab" class="push-tabs">
        <!-- Tab 1: Sitemap 推送 -->
        <el-tab-pane label="Sitemap 推送" name="sitemap">
          <div class="panel tab-body">
            <div class="form-group">
              <label>Sitemap URL</label>
              <el-input v-model="sitemapUrl" placeholder="https://example.com/sitemap.xml" clearable />
            </div>
            <div class="form-group">
              <label>推送平台</label>
              <div class="platform-checks">
                <el-checkbox v-model="sitemapPlatforms.gsc"><span class="plat-icon">🔍</span> Google Search Console</el-checkbox>
                <el-checkbox v-model="sitemapPlatforms.bing"><span class="plat-icon">🅱️</span> Bing Webmaster</el-checkbox>
                <el-checkbox v-model="sitemapPlatforms.yandex"><span class="plat-icon">🔴</span> Yandex Webmaster</el-checkbox>
              </div>
            </div>
            <div class="btn-row">
              <button class="btn btn-primary" @click="submitSitemap" :disabled="sitemapLoading || !sitemapUrl">
                {{ sitemapLoading ? '推送中...' : '📤 推送 Sitemap' }}
              </button>
              <button class="btn btn-secondary" @click="submitSitemapAll" :disabled="sitemapAllLoading">
                {{ sitemapAllLoading ? '推送中...' : '🌐 一键推送所有站点' }}
              </button>
            </div>
            <div v-if="sitemapResult" class="result-box" :class="sitemapResult.success ? 'result-ok' : 'result-err'">
              <div class="result-title">{{ sitemapResult.success ? '✅ 推送成功' : '❌ 推送失败' }}</div>
              <div v-if="sitemapResult.details" class="result-details">
                <div v-for="(d, i) in sitemapResult.details" :key="i" class="result-item">
                  <span :class="d.ok ? 'dot-ok' : 'dot-err'">{{ d.ok ? '✓' : '✗' }}</span>
                  <span class="plat-name">{{ d.platform }}</span>
                  <span v-if="d.error" class="err-msg">{{ d.error }}</span>
                </div>
              </div>
              <div v-if="sitemapResult.message" class="result-msg">{{ sitemapResult.message }}</div>
            </div>
          </div>
        </el-tab-pane>

        <!-- Tab 2: URL 推送 -->
        <el-tab-pane label="URL 推送" name="urls">
          <div class="panel tab-body">
            <div class="form-group">
              <label>推送平台</label>
              <el-radio-group v-model="urlPlatform">
                <el-radio value="gsc"><span class="plat-icon">🔍</span> GSC</el-radio>
                <el-radio value="bing"><span class="plat-icon">🅱️</span> Bing</el-radio>
                <el-radio value="yandex"><span class="plat-icon">🔴</span> Yandex</el-radio>
              </el-radio-group>
            </div>
            <div class="form-group">
              <label>URL 列表 <span class="url-count">({{ urlCount }} 个 URL)</span></label>
              <el-input v-model="urlText" type="textarea" :rows="8" placeholder="每行一个 URL" />
            </div>
            <div class="limit-note">
              <strong>每日限额：</strong>
              <span class="limit-item">🔍 GSC: 200/天</span>
              <span class="limit-item">🅱️ Bing: 100/天, 最多500个/次</span>
              <span class="limit-item">🔴 Yandex: 20/天</span>
            </div>
            <div class="btn-row">
              <button class="btn btn-primary" @click="submitUrls" :disabled="urlLoading || urlCount === 0">
                {{ urlLoading ? '提交中...' : '📤 提交 URL' }}
              </button>
            </div>
            <div v-if="urlResult" class="result-box" :class="urlResult.success ? 'result-ok' : 'result-err'">
              <div class="result-title">{{ urlResult.success ? '✅ 提交成功' : '❌ 提交失败' }}</div>
              <div v-if="urlResult.message" class="result-msg">{{ urlResult.message }}</div>
            </div>
          </div>
        </el-tab-pane>

        <!-- Tab 3: 定时推送 -->
        <el-tab-pane label="定时推送" name="schedules">
          <div class="panel tab-body">
            <div class="btn-row" style="margin-bottom:16px">
              <button class="btn btn-primary" @click="openScheduleDialog">➕ 添加定时推送</button>
              <button class="btn" @click="loadSchedules" :disabled="scheduleLoading">↻ 刷新</button>
            </div>
            <el-table :data="schedules" v-loading="scheduleLoading" class="sunrise-table" :header-cell-style="headerStyle">
              <el-table-column prop="site_name" label="站点" min-width="120" />
              <el-table-column prop="platform" label="平台" width="100">
                <template #default="{ row }">
                  <span class="plat-tag">{{ platformLabel(row.platform) }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="sitemap_url" label="Sitemap URL" min-width="200" show-overflow-tooltip />
              <el-table-column prop="cron" label="Cron 表达式" width="140">
                <template #default="{ row }"><code class="cron-code">{{ row.cron }}</code></template>
              </el-table-column>
              <el-table-column label="启用" width="80" align="center">
                <template #default="{ row }">
                  <el-switch v-model="row.enabled" @change="toggleSchedule(row)" />
                </template>
              </el-table-column>
              <el-table-column label="操作" width="80" align="center">
                <template #default="{ row }">
                  <el-button type="danger" text size="small" @click="deleteSchedule(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>

        <!-- Tab 4: 推送记录 -->
        <el-tab-pane label="推送记录" name="logs">
          <div class="panel tab-body">
            <div class="filter-bar">
              <el-select v-model="logFilter.platform" placeholder="平台" clearable style="width:130px" @change="loadLogs">
                <el-option label="全部" value="" />
                <el-option label="GSC" value="gsc" />
                <el-option label="Bing" value="bing" />
                <el-option label="Yandex" value="yandex" />
              </el-select>
              <el-select v-model="logFilter.status" placeholder="状态" clearable style="width:120px" @change="loadLogs">
                <el-option label="全部" value="" />
                <el-option label="成功" value="success" />
                <el-option label="失败" value="failed" />
                <el-option label="进行中" value="pending" />
              </el-select>
              <el-date-picker
                v-model="logFilter.dateRange"
                type="daterange"
                range-separator="~"
                start-placeholder="开始日期"
                end-placeholder="结束日期"
                format="YYYY-MM-DD"
                value-format="YYYY-MM-DD"
                size="default"
                @change="loadLogs"
              />
              <button class="btn btn-sm" @click="loadLogs">↻ 刷新</button>
            </div>
            <el-table :data="logs" v-loading="logLoading" class="sunrise-table" :header-cell-style="headerStyle" style="margin-top:12px">
              <el-table-column prop="created_at" label="时间" width="170">
                <template #default="{ row }"><span class="time-cell">{{ row.created_at }}</span></template>
              </el-table-column>
              <el-table-column prop="platform" label="平台" width="100">
                <template #default="{ row }">
                  <span class="plat-tag">{{ platformLabel(row.platform) }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="push_type" label="类型" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.push_type === 'sitemap' ? '' : 'warning'">{{ row.push_type === 'sitemap' ? 'Sitemap' : 'URL' }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="target_url" label="目标 URL" min-width="200" show-overflow-tooltip />
              <el-table-column prop="status" label="状态" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="error_message" label="错误信息" min-width="160" show-overflow-tooltip>
                <template #default="{ row }">
                  <span class="err-msg">{{ row.error_message || '-' }}</span>
                </template>
              </el-table-column>
            </el-table>
            <div style="margin-top:14px;text-align:center" v-if="logTotal > logPerPage">
              <el-pagination
                v-model:current-page="logPage"
                :page-size="logPerPage"
                :total="logTotal"
                layout="prev,pager,next,total"
                background
                @current-change="loadLogs"
              />
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </template>

    <!-- Schedule Dialog -->
    <el-dialog v-model="scheduleDialogVisible" title="添加定时推送" width="520px">
      <div class="form-group">
        <label>推送平台</label>
        <el-select v-model="scheduleForm.platform" placeholder="选择平台" style="width:100%">
          <el-option label="全部平台" value="all" />
          <el-option label="Google Search Console" value="gsc" />
          <el-option label="Bing Webmaster" value="bing" />
          <el-option label="Yandex Webmaster" value="yandex" />
        </el-select>
      </div>
      <div class="form-group">
        <label>Sitemap URL</label>
        <el-input v-model="scheduleForm.sitemap_url" placeholder="https://example.com/sitemap.xml" />
      </div>
      <div class="form-group">
        <label>Cron 表达式</label>
        <el-input v-model="scheduleForm.cron" placeholder="0 9 * * *" />
        <div class="cron-presets">
          <span class="cron-preset" @click="scheduleForm.cron = '0 9 * * *'">每天9点</span>
          <span class="cron-preset" @click="scheduleForm.cron = '0 9 * * 1'">每周一9点</span>
          <span class="cron-preset" @click="scheduleForm.cron = '0 */12 * * *'">每12小时</span>
        </div>
      </div>
      <template #footer>
        <el-button @click="scheduleDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveSchedule" :loading="scheduleSaving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import apiClient from '../api'

// ─── Site selector ───────────────────────────────────────────────────────
const sites = ref([])
const selectedSiteId = ref(null)
const activeTab = ref('sitemap')

const selectedSite = computed(() => sites.value.find(s => s.id === selectedSiteId.value))

function onSiteChange() {
  if (selectedSite.value) {
    sitemapUrl.value = `https://${selectedSite.value.domain}/sitemap.xml`
  }
  sitemapResult.value = null
  urlResult.value = null
  loadSchedules()
  loadLogs()
}

// ─── Tab 1: Sitemap Push ─────────────────────────────────────────────────
const sitemapUrl = ref('')
const sitemapPlatforms = ref({ gsc: true, bing: true, yandex: true })
const sitemapLoading = ref(false)
const sitemapAllLoading = ref(false)
const sitemapResult = ref(null)

function getSelectedPlatforms() {
  const p = []
  if (sitemapPlatforms.value.gsc) p.push('gsc')
  if (sitemapPlatforms.value.bing) p.push('bing')
  if (sitemapPlatforms.value.yandex) p.push('yandex')
  return p
}

async function submitSitemap() {
  const platforms = getSelectedPlatforms()
  if (!platforms.length) { ElMessage.warning('请至少选择一个平台'); return }
  sitemapLoading.value = true
  sitemapResult.value = null
  try {
    const res = await apiClient.submitSitemap(selectedSiteId.value, { sitemap_url: sitemapUrl.value, platforms })
    sitemapResult.value = { success: true, details: res.data?.results, message: res.data?.message || '推送成功' }
  } catch (e) {
    sitemapResult.value = { success: false, message: e?.response?.data?.error || e?.message || '推送失败' }
  }
  sitemapLoading.value = false
}

async function submitSitemapAll() {
  const platforms = getSelectedPlatforms()
  if (!platforms.length) { ElMessage.warning('请至少选择一个平台'); return }
  sitemapAllLoading.value = true
  sitemapResult.value = null
  try {
    const suffix = sitemapUrl.value ? sitemapUrl.value.replace(/^https?:\/\/[^/]+/, '') : '/sitemap.xml'
    const res = await apiClient.submitSitemapAll({ sitemap_url_suffix: suffix, platforms })
    sitemapResult.value = { success: true, details: res.data?.results, message: res.data?.message || '全站推送成功' }
  } catch (e) {
    sitemapResult.value = { success: false, message: e?.response?.data?.error || e?.message || '推送失败' }
  }
  sitemapAllLoading.value = false
}

// ─── Tab 2: URL Push ─────────────────────────────────────────────────────
const urlPlatform = ref('gsc')
const urlText = ref('')
const urlLoading = ref(false)
const urlResult = ref(null)

const urlCount = computed(() => {
  if (!urlText.value.trim()) return 0
  return urlText.value.trim().split('\n').filter(l => l.trim()).length
})

async function submitUrls() {
  const urls = urlText.value.trim().split('\n').map(l => l.trim()).filter(Boolean)
  if (!urls.length) return
  urlLoading.value = true
  urlResult.value = null
  try {
    const res = await apiClient.submitURLs(selectedSiteId.value, { platform: urlPlatform.value, urls })
    urlResult.value = { success: true, message: res.data?.message || `成功提交 ${urls.length} 个 URL` }
  } catch (e) {
    urlResult.value = { success: false, message: e?.response?.data?.error || e?.message || '提交失败' }
  }
  urlLoading.value = false
}

// ─── Tab 3: Schedules ────────────────────────────────────────────────────
const schedules = ref([])
const scheduleLoading = ref(false)
const scheduleDialogVisible = ref(false)
const scheduleSaving = ref(false)
const scheduleForm = ref({ platform: 'all', sitemap_url: '', cron: '0 9 * * *' })

async function loadSchedules() {
  if (!selectedSiteId.value) return
  scheduleLoading.value = true
  try {
    const res = await apiClient.listPushSchedules(selectedSiteId.value)
    schedules.value = res.data || []
  } catch { schedules.value = [] }
  scheduleLoading.value = false
}

function openScheduleDialog() {
  scheduleForm.value = {
    platform: 'all',
    sitemap_url: sitemapUrl.value || (selectedSite.value ? `https://${selectedSite.value.domain}/sitemap.xml` : ''),
    cron: '0 9 * * *',
  }
  scheduleDialogVisible.value = true
}

async function saveSchedule() {
  if (!scheduleForm.value.cron) { ElMessage.warning('请填写 Cron 表达式'); return }
  scheduleSaving.value = true
  try {
    await apiClient.savePushSchedule(selectedSiteId.value, scheduleForm.value)
    ElMessage.success('定时推送已保存')
    scheduleDialogVisible.value = false
    loadSchedules()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  }
  scheduleSaving.value = false
}

async function toggleSchedule(row) {
  try {
    await apiClient.togglePushSchedule(row.id)
  } catch (e) {
    row.enabled = !row.enabled
    ElMessage.error('切换失败')
  }
}

async function deleteSchedule(row) {
  try {
    await ElMessageBox.confirm('确定删除该定时推送？', '提示', { type: 'warning' })
    await apiClient.deletePushSchedule(row.id)
    ElMessage.success('已删除')
    loadSchedules()
  } catch {}
}

// ─── Tab 4: Push Logs ────────────────────────────────────────────────────
const logs = ref([])
const logLoading = ref(false)
const logPage = ref(1)
const logPerPage = 50
const logTotal = ref(0)
const logFilter = ref({ platform: '', status: '', dateRange: null })

async function loadLogs() {
  if (!selectedSiteId.value) return
  logLoading.value = true
  try {
    const params = { page: logPage.value, per_page: logPerPage }
    if (logFilter.value.platform) params.platform = logFilter.value.platform
    if (logFilter.value.status) params.status = logFilter.value.status
    if (logFilter.value.dateRange && logFilter.value.dateRange[0]) {
      params.start_date = logFilter.value.dateRange[0]
      params.end_date = logFilter.value.dateRange[1]
    }
    const res = await apiClient.listPushLogs(selectedSiteId.value, params)
    logs.value = res.data?.items || res.data || []
    logTotal.value = res.data?.total || logs.value.length
  } catch { logs.value = [] }
  logLoading.value = false
}

// ─── Helpers ─────────────────────────────────────────────────────────────
function platformLabel(code) {
  const map = { gsc: 'GSC', bing: 'Bing', yandex: 'Yandex', all: '全部' }
  return map[code] || code
}
function statusTagType(s) {
  if (s === 'success') return 'success'
  if (s === 'failed') return 'danger'
  return 'info'
}
function statusLabel(s) {
  const map = { success: '成功', failed: '失败', pending: '进行中' }
  return map[s] || s
}
const headerStyle = { background: 'var(--surface-3)', color: 'var(--ink-mid)', fontWeight: 800, fontSize: '11px', textTransform: 'uppercase', letterSpacing: '.08em' }

// ─── Mount ───────────────────────────────────────────────────────────────
onMounted(async () => {
  const res = await apiClient.listSites().catch(() => ({ data: [] }))
  sites.value = res.data || []
  if (sites.value.length) {
    selectedSiteId.value = sites.value[0].id
    onSiteChange()
  }
})
</script>

<style scoped>
.push-mgmt { display: flex; flex-direction: column; gap: 16px; }
.panel { background: var(--surface); border-radius: var(--r); box-shadow: var(--s1); border: 1.5px solid var(--border-light); padding: 16px 22px; }
.selector-bar { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.selector-label { font-size: 13.5px; font-weight: 700; color: var(--ink-mid); white-space: nowrap; }

.push-tabs { margin-top: 0; }
.push-tabs :deep(.el-tabs__header) { margin-bottom: 0; }
.push-tabs :deep(.el-tabs__nav-wrap::after) { display: none; }
.push-tabs :deep(.el-tabs__item) { font-weight: 700; font-size: 14px; }
.push-tabs :deep(.el-tabs__item.is-active) { color: var(--sun-deep); }
.push-tabs :deep(.el-tabs__active-bar) { background: linear-gradient(135deg, var(--sun), var(--coral)); }

.tab-body { margin-top: 12px; }

.form-group { margin-bottom: 18px; }
.form-group label { display: block; margin-bottom: 7px; font-weight: 700; color: var(--ink-mid); font-size: 13px; }

.platform-checks { display: flex; gap: 20px; flex-wrap: wrap; }
.plat-icon { margin-right: 4px; }

.btn-row { display: flex; gap: 10px; margin-top: 18px; flex-wrap: wrap; }
.btn { display: inline-flex; align-items: center; gap: 6px; padding: 9px 18px; border-radius: 9px; border: 1.5px solid var(--border); background: var(--surface); cursor: pointer; font-family: inherit; font-size: 13.5px; font-weight: 700; transition: all .18s; color: var(--ink-mid); }
.btn:hover { background: var(--sun-light); border-color: var(--peach); color: var(--sun-deep); }
.btn:disabled { opacity: .6; cursor: not-allowed; }
.btn-primary { background: linear-gradient(135deg, var(--sun), var(--coral)); color: #fff; border-color: transparent; box-shadow: var(--s-amber); }
.btn-primary:hover { opacity: .92; }
.btn-secondary { background: var(--sun-light); color: var(--sun-deep); border-color: var(--peach); }
.btn-sm { padding: 6px 14px; font-size: 12.5px; }

.result-box { margin-top: 18px; padding: 14px 18px; border-radius: 10px; border: 1.5px solid; }
.result-ok { background: var(--ok-bg, #F0FFF4); border-color: var(--ok, #48BB78); }
.result-err { background: var(--err-bg, #FFF5F5); border-color: var(--err, #F56565); }
.result-title { font-weight: 800; font-size: 14px; margin-bottom: 8px; }
.result-details { display: flex; flex-direction: column; gap: 4px; }
.result-item { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.dot-ok { color: var(--ok, #48BB78); font-weight: 800; }
.dot-err { color: var(--err, #F56565); font-weight: 800; }
.plat-name { font-weight: 600; }
.err-msg { color: var(--err, #F56565); font-size: 12px; }
.result-msg { font-size: 13px; color: var(--ink-soft); margin-top: 6px; }

.url-count { font-weight: 400; color: var(--ink-hint); font-size: 12px; }
.limit-note { margin-top: 12px; padding: 10px 14px; border-radius: 8px; background: var(--surface-3, #F7FAFC); font-size: 12.5px; color: var(--ink-soft); display: flex; gap: 16px; flex-wrap: wrap; align-items: center; }
.limit-item { white-space: nowrap; }

.filter-bar { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }

.plat-tag { font-size: 12px; font-weight: 700; color: var(--ink-mid); }
.cron-code { font-family: 'JetBrains Mono', monospace; font-size: 12px; background: var(--surface-3); padding: 2px 8px; border-radius: 4px; }
.time-cell { font-size: 12px; color: var(--ink-hint); white-space: nowrap; }

.cron-presets { display: flex; gap: 8px; margin-top: 8px; }
.cron-preset { padding: 4px 12px; border-radius: 16px; background: var(--sun-light); color: var(--sun-deep); font-size: 12px; font-weight: 600; cursor: pointer; border: 1px solid var(--peach); transition: all .15s; }
.cron-preset:hover { background: var(--sun); color: #fff; }

.empty-hint { text-align: center; padding: 50px; color: var(--ink-hint); }
.empty-icon { font-size: 44px; margin-bottom: 12px; }

.sunrise-table { width: 100%; }
</style>
