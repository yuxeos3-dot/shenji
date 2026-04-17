<template>
  <div class="extlinks-page">

    <!-- Site selector -->
    <div class="panel selector-bar">
      <span class="selector-label">🔗 外链管理</span>
      <el-select v-model="siteId" placeholder="选择站点" style="width:260px" @change="onSiteChange">
        <el-option v-for="s in sites" :key="s.id" :label="s.name + ' (' + s.domain + ')'" :value="s.id" />
      </el-select>
    </div>

    <div v-if="!siteId" class="empty-hint panel">
      <div class="empty-icon">🔗</div>
      <div>请先选择站点</div>
    </div>

    <template v-else>
      <!-- Stats -->
      <div class="stats-row">
        <div class="stat-card panel"><div class="sc-num blue">{{ stats.total_articles || 0 }}</div><div class="sc-lbl">总文章</div></div>
        <div class="stat-card panel"><div class="sc-num orange">{{ stats.drafts || 0 }}</div><div class="sc-lbl">草稿</div></div>
        <div class="stat-card panel"><div class="sc-num red">{{ stats.pending_review || 0 }}</div><div class="sc-lbl">待审核</div></div>
        <div class="stat-card panel"><div class="sc-num green">{{ stats.published || 0 }}</div><div class="sc-lbl">已发布</div></div>
      </div>

      <!-- 2-tab panel -->
      <div class="panel tab-panel">
        <el-tabs v-model="activeTab" @tab-change="onTabChange">
          <!-- Tab 1: Keyword / topic libraries -->
          <el-tab-pane label="📚 关键词 / 话题库" name="libs">
            <div v-if="!curLib">
              <div class="tab-bar">
                <el-button type="primary" @click="showLibForm = true">+ 新建关键词 / 话题库</el-button>
              </div>
              <el-table :data="titleLibs" stripe>
                <el-table-column prop="name" label="库名" />
                <el-table-column prop="description" label="描述" show-overflow-tooltip />
                <el-table-column prop="title_count" label="关键词数" width="120" />
                <el-table-column label="操作" width="220">
                  <template #default="{ row }">
                    <el-button size="small" type="primary" @click="enterLib(row)">进入</el-button>
                    <el-button size="small" type="danger" @click="deleteLib(row)">删除</el-button>
                  </template>
                </el-table-column>
              </el-table>

              <el-dialog v-model="showLibForm" title="新建关键词 / 话题库" width="420px">
                <el-form label-width="70px">
                  <el-form-item label="库名"><el-input v-model="libForm.name" /></el-form-item>
                  <el-form-item label="描述"><el-input v-model="libForm.description" /></el-form-item>
                </el-form>
                <template #footer>
                  <el-button @click="showLibForm = false">取消</el-button>
                  <el-button type="primary" @click="createLib">创建</el-button>
                </template>
              </el-dialog>
            </div>

            <KeywordListView
              v-else
              :lib="curLib"
              :site-id="siteId"
              @back="curLib = null; loadLibs()"
              @open-wizard="openWizard"
            />
          </el-tab-pane>

          <!-- Tab 2: Review queue -->
          <el-tab-pane label="📋 文章审核" name="review">
            <DraftReviewPanel
              ref="reviewPanel"
              :content-type="'extlink'"
              :site-id="siteId"
            />
          </el-tab-pane>
        </el-tabs>
      </div>

      <!-- Shared public backlink pool (this is unique to 外链) -->
      <div class="panel pool-panel">
        <div class="pool-head">
          <h2>📦 公共外链库 <span style="font-size:12px;font-weight:400;color:var(--ink-hint);margin-left:8px">所有用户共享</span></h2>
          <button class="btn btn-primary btn-sm" @click="showPoolImport = true">＋ 批量添加</button>
        </div>
        <el-table :data="pool" stripe max-height="400" size="small">
          <el-table-column type="index" label="#" width="50" />
          <el-table-column prop="name" label="网站名称" width="200" />
          <el-table-column prop="url" label="URL" show-overflow-tooltip />
          <el-table-column prop="created_at" label="添加时间" width="160" />
          <el-table-column label="操作" width="80">
            <template #default="{ row }">
              <el-button size="small" type="danger" text @click="delPoolItem(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div v-if="!pool.length" style="text-align:center;padding:30px;color:var(--ink-hint);font-size:13px">
          暂无外链库数据，点击「批量添加」导入
        </div>
      </div>

      <el-dialog v-model="showPoolImport" title="批量添加外链到公共库" width="560px">
        <p style="font-size:12px;color:#888;margin-bottom:8px">
          每行一条，格式：<code>URL|网站名称</code> 或只填 URL（自动去重）
        </p>
        <el-input v-model="poolImportText" type="textarea" :rows="10"
          placeholder="https://example.com|示例站
https://blog.xxx.com|XXX博客" />
        <template #footer>
          <el-button @click="showPoolImport = false">取消</el-button>
          <el-button type="primary" @click="doPoolImport" :loading="poolImporting">导入</el-button>
        </template>
      </el-dialog>

      <!-- AI Generate wizard -->
      <AIGenerateWizard
        v-model="wizardVisible"
        :site-id="siteId"
        :content-type="'extlink'"
        :title-row="wizardRow"
        @applied="onWizardApplied"
      />
    </template>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import AIGenerateWizard from '../components/AIGenerateWizard.vue'
import DraftReviewPanel from '../components/DraftReviewPanel.vue'
import KeywordListView from '../components/KeywordListView.vue'

const sites = ref([])
const siteId = ref(null)
const activeTab = ref('libs')
const stats = ref({ total_articles: 0, drafts: 0, pending_review: 0, published: 0 })

const titleLibs = ref([])
const curLib = ref(null)

const showLibForm = ref(false)
const libForm = ref({ name: '', description: '' })

const wizardVisible = ref(false)
const wizardRow = ref(null)
const reviewPanel = ref(null)

// Public backlink pool (unique to extlink)
const pool = ref([])
const showPoolImport = ref(false)
const poolImportText = ref('')
const poolImporting = ref(false)

onMounted(async () => {
  try {
    const res = await api.listSites()
    sites.value = res.data || []
    if (sites.value.length) {
      siteId.value = sites.value[0].id
      await Promise.all([loadStats(), loadLibs(), loadPool()])
    }
  } catch (e) { console.error('load sites', e) }
})

async function onSiteChange() {
  curLib.value = null
  await Promise.all([loadStats(), loadLibs()])
  if (activeTab.value === 'review' && reviewPanel.value?.reload) {
    reviewPanel.value.reload()
  }
}

function onTabChange() {
  if (activeTab.value === 'review' && reviewPanel.value?.reload) {
    reviewPanel.value.reload()
  }
}

async function loadStats() {
  try {
    const res = await api.geoStats({ site_id: siteId.value, content_type: 'extlink' })
    stats.value = res.data || {}
  } catch (e) { /* non-fatal */ }
}

async function loadLibs() {
  try {
    const res = await api.geoListTitleLibs({ site_id: siteId.value, content_type: 'extlink' })
    titleLibs.value = res.data || []
  } catch (e) {
    ElMessage.error('加载关键词库失败')
  }
}

async function createLib() {
  if (!libForm.value.name.trim()) {
    ElMessage.warning('库名必填')
    return
  }
  try {
    await api.geoCreateTitleLib({
      name: libForm.value.name,
      description: libForm.value.description,
      site_id: siteId.value,
      content_type: 'extlink',
    })
    showLibForm.value = false
    libForm.value = { name: '', description: '' }
    ElMessage.success('创建成功')
    loadLibs()
  } catch (e) {
    ElMessage.error('创建失败')
  }
}

async function deleteLib(row) {
  try {
    await ElMessageBox.confirm(`删除「${row.name}」及其所有关键词？`, '删除确认', { type: 'warning' })
    await api.geoDeleteTitleLib(row.id, 'extlink')
    ElMessage.success('已删除')
    loadLibs()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败')
  }
}

function enterLib(row) {
  curLib.value = row
}

function openWizard(titleRow) {
  wizardRow.value = titleRow
  wizardVisible.value = true
}

function onWizardApplied() {
  loadStats()
  ElMessage.success('已加入审核库，可在「文章审核」tab 处理')
}

// ── Public backlink pool ──
async function loadPool() {
  try {
    const res = await api.listExtlinkPool()
    pool.value = res.data || []
  } catch (e) { /* silent */ }
}

async function doPoolImport() {
  if (!poolImportText.value.trim()) {
    ElMessage.warning('请输入 URL')
    return
  }
  poolImporting.value = true
  try {
    const res = await api.importExtlinkPool({ text: poolImportText.value })
    ElMessage.success('导入 ' + (res.data?.imported || 0) + ' 条')
    showPoolImport.value = false
    poolImportText.value = ''
    loadPool()
  } catch (e) {
    ElMessage.error('导入失败')
  } finally {
    poolImporting.value = false
  }
}

async function delPoolItem(r) {
  try {
    await ElMessageBox.confirm('删除「' + (r.name || r.url) + '」？')
    await api.deleteExtlinkPoolItem(r.id)
    loadPool()
  } catch (e) { /* cancel */ }
}
</script>

<style scoped>
.extlinks-page { max-width: 1400px; }

.panel {
  background: var(--surface);
  border-radius: var(--r);
  box-shadow: var(--s1);
  border: 1.5px solid var(--border-light);
  padding: 18px 22px;
  margin-bottom: 14px;
}
.selector-bar { display: flex; align-items: center; gap: 12px; }
.selector-label { font-size: 13px; font-weight: 700; color: var(--ink-mid); }
.empty-hint { text-align: center; color: var(--ink-hint); padding: 60px; font-size: 14px; }
.empty-icon { font-size: 32px; margin-bottom: 8px; }

.stats-row { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 14px; }
.stat-card { padding: 18px 22px; text-align: center; }
.sc-num { font-size: 30px; font-weight: 800; letter-spacing: -1px; }
.sc-lbl { font-size: 12px; color: var(--ink-hint); margin-top: 4px; font-weight: 700; text-transform: uppercase; letter-spacing: .06em; }
.blue { color: #2563eb; }
.orange { color: var(--sun-deep); }
.red { color: var(--err); }
.green { color: var(--ok); }

.tab-panel { padding: 14px 18px; }
.tab-bar { margin-bottom: 14px; display: flex; gap: 8px; align-items: center; }

.pool-panel { margin-top: 20px; }
.pool-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; }
.pool-head h2 { font-size: 15px; font-weight: 800; color: var(--ink); }

.btn {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 8px 16px; border-radius: 8px; border: 1.5px solid var(--border);
  background: var(--surface); cursor: pointer; font-family: inherit;
  font-size: 13px; font-weight: 700; color: var(--ink-mid); transition: all .18s;
}
.btn:hover { background: var(--sun-light); border-color: var(--peach); color: var(--sun-deep); }
.btn-sm { padding: 6px 14px; font-size: 12px; }
.btn-primary { background: linear-gradient(135deg, var(--sun), var(--coral)); color: #fff; border-color: transparent; }
.btn-primary:hover { background: linear-gradient(135deg, var(--sun-deep), var(--sun)); color: #fff; }
</style>
