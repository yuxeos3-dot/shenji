<template>
<div>
  <div class="page-top"><h1>🤖 GEO 管理</h1></div>

  <!-- Site selector -->
  <div class="panel selector-bar">
    <span class="selector-label">🌐 选择站点</span>
    <el-select v-model="siteId" placeholder="请选择站点" style="width:280px" @change="onSiteChange">
      <el-option v-for="s in sites" :key="s.id" :label="s.name+' ('+s.domain+')'" :value="s.id" />
    </el-select>
  </div>

  <div v-if="!siteId" class="empty-hint panel"><div>请先选择站点</div></div>

  <template v-else>
    <!-- Stats -->
    <div class="stats-row">
      <div class="stat-card panel"><div class="sc-num blue">{{ stats.total_articles || 0 }}</div><div class="sc-lbl">总文章</div></div>
      <div class="stat-card panel"><div class="sc-num orange">{{ stats.drafts || 0 }}</div><div class="sc-lbl">草稿</div></div>
      <div class="stat-card panel"><div class="sc-num red">{{ stats.pending_review || 0 }}</div><div class="sc-lbl">待审核</div></div>
      <div class="stat-card panel"><div class="sc-num green">{{ stats.published || 0 }}</div><div class="sc-lbl">已发布</div></div>
    </div>

    <el-tabs v-model="activeTab" type="border-card" @tab-change="onTabChange">
      <!-- ─── Tab 1: Keyword / topic libraries ─── -->
      <el-tab-pane label="📚 关键词 / 话题库" name="libs">
        <!-- Library list view -->
        <div v-if="!curLib">
          <div class="tab-bar">
            <el-button type="primary" @click="showLibForm = true">+ 新建关键词 / 话题库</el-button>
          </div>
          <el-table :data="titleLibs" stripe>
            <el-table-column prop="name" label="名称" />
            <el-table-column prop="description" label="描述" />
            <el-table-column prop="title_count" label="关键词数" width="120" />
            <el-table-column label="操作" width="220">
              <template #default="{ row }">
                <el-button size="small" type="primary" @click="enterLib(row)">进入</el-button>
                <el-button size="small" type="danger" @click="deleteLib(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>

          <el-dialog v-model="showLibForm" title="新建关键词 / 话题库" width="460px">
            <el-form label-width="80px">
              <el-form-item label="名称"><el-input v-model="libForm.name" /></el-form-item>
              <el-form-item label="描述"><el-input v-model="libForm.description" /></el-form-item>
            </el-form>
            <template #footer>
              <el-button @click="showLibForm = false">取消</el-button>
              <el-button type="primary" @click="createLib">创建</el-button>
            </template>
          </el-dialog>
        </div>

        <!-- Keyword list inside a library -->
        <KeywordListView
          v-else
          :lib="curLib"
          :site-id="siteId"
          @back="curLib = null; loadLibs()"
          @open-wizard="openWizard"
        />
      </el-tab-pane>

      <!-- ─── Tab 2: Review queue ─── -->
      <el-tab-pane label="📋 文章审核" name="review">
        <DraftReviewPanel
          ref="reviewPanel"
          :content-type="'geo'"
          :site-id="siteId"
        />
      </el-tab-pane>
    </el-tabs>

    <!-- AI Generate wizard (opened from keyword row) -->
    <AIGenerateWizard
      v-model="wizardVisible"
      :site-id="siteId"
      :content-type="'geo'"
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

onMounted(async () => {
  try {
    const res = await api.listSites()
    sites.value = res.data || []
    if (sites.value.length) {
      siteId.value = sites.value[0].id
      await Promise.all([loadStats(), loadLibs()])
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
    const res = await api.geoStats({ site_id: siteId.value, content_type: 'geo' })
    stats.value = res.data || {}
  } catch (e) { /* non-fatal */ }
}

async function loadLibs() {
  try {
    const res = await api.geoListTitleLibs({ site_id: siteId.value, content_type: 'geo' })
    titleLibs.value = res.data || []
  } catch (e) {
    ElMessage.error('加载关键词库失败')
  }
}

async function createLib() {
  if (!libForm.value.name.trim()) {
    ElMessage.warning('名称必填')
    return
  }
  try {
    await api.geoCreateTitleLib({
      name: libForm.value.name,
      description: libForm.value.description,
      site_id: siteId.value,
      content_type: 'geo',
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
    await api.geoDeleteTitleLib(row.id, 'geo')
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
  // Refresh the current lib view (to reflect is_used flag)
  // Consumers receive @applied and can opt to reload.
  if (curLib.value) {
    // Small nudge: flip curLib to null then back to trigger child onMounted
    // is not ideal. Instead, we rely on the child's watcher, but we should
    // also re-fetch stats. The user will see new drafts in review tab.
  }
  ElMessage.success('已加入审核库，可在「文章审核」tab 处理')
}
</script>

<style scoped>
.page-top { margin-bottom: 14px; }
.page-top h1 { font-size: 18px; font-weight: 800; color: var(--ink); }

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

.empty-hint { text-align: center; color: var(--ink-hint); padding: 40px; font-size: 14px; }

.stats-row { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 14px; }
.stat-card { padding: 18px 22px; text-align: center; }
.sc-num { font-size: 30px; font-weight: 800; letter-spacing: -1px; }
.sc-lbl { font-size: 12px; color: var(--ink-hint); margin-top: 4px; font-weight: 700; text-transform: uppercase; letter-spacing: .06em; }
.blue { color: #2563eb; }
.orange { color: var(--sun-deep); }
.red { color: var(--err); }
.green { color: var(--ok); }

.tab-bar { margin-bottom: 14px; display: flex; gap: 8px; align-items: center; }
</style>
