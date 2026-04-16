<template>
<div>
  <div class="page-top"><h1>📡 数据管理</h1></div>

  <div class="panel selector-bar">
    <span class="selector-label">🌐 选择站点</span>
    <el-select v-model="siteId" placeholder="请选择站点" style="width:280px" @change="loadData">
      <el-option v-for="s in sites" :key="s.id" :label="s.name+' ('+s.domain+')'" :value="s.id" />
    </el-select>
    <el-button type="primary" @click="syncAll" :loading="syncing" v-if="siteId">↻ 同步全部数据</el-button>
  </div>

  <div v-if="!siteId" class="empty-hint panel"><div>请先选择站点查看数据</div></div>

  <template v-else>
    <el-tabs v-model="activeTab" type="border-card">
      <!-- GSC -->
      <el-tab-pane label="🔍 Google Search Console" name="gsc">
        <div class="tab-bar">
          <el-button size="small" @click="syncGSC" :loading="syncingGSC">↻ 同步 GSC</el-button>
          <el-select v-model="gscDate" placeholder="选择日期" style="width:160px" @change="parseGSC" v-if="gscDates.length">
            <el-option v-for="d in gscDates" :key="d" :label="d" :value="d" />
          </el-select>
          <el-input v-model="gscSearch" placeholder="搜索关键词" style="width:180px" clearable />
        </div>
        <div class="stats-row" v-if="gscRow">
          <div class="stat-card panel"><div class="sc-num blue">{{ gscRow.clicks }}</div><div class="sc-lbl">点击</div></div>
          <div class="stat-card panel"><div class="sc-num orange">{{ gscRow.impressions }}</div><div class="sc-lbl">展示</div></div>
          <div class="stat-card panel"><div class="sc-num green">{{ (gscRow.ctr*100).toFixed(2) }}%</div><div class="sc-lbl">CTR</div></div>
          <div class="stat-card panel"><div class="sc-num purple">{{ gscRow.position?.toFixed(1) }}</div><div class="sc-lbl">平均排名</div></div>
        </div>
        <el-table :data="gscFiltered.slice((gscPage-1)*50,gscPage*50)" stripe max-height="500" v-loading="loading">
          <el-table-column type="index" label="#" width="50" />
          <el-table-column prop="query" label="关键词" show-overflow-tooltip />
          <el-table-column prop="clicks" label="点击" width="80" sortable />
          <el-table-column prop="impressions" label="展示" width="80" sortable />
          <el-table-column label="CTR" width="80"><template #default="{row}">{{ row.ctr?(row.ctr*100).toFixed(2)+'%':'-' }}</template></el-table-column>
          <el-table-column label="排名" width="80"><template #default="{row}">{{ row.position?.toFixed(1) }}</template></el-table-column>
        </el-table>
        <div style="margin-top:10px;text-align:center" v-if="gscFiltered.length>50">
          <el-pagination v-model:current-page="gscPage" :page-size="50" :total="gscFiltered.length" layout="prev,pager,next,total" background />
        </div>
        <div v-if="!gscData.length && !loading" class="empty-cell">暂无 GSC 数据，请先点「同步 GSC」</div>
      </el-tab-pane>

      <!-- GA -->
      <el-tab-pane label="📊 Google Analytics" name="ga">
        <div class="tab-bar">
          <el-button size="small" @click="syncGA" :loading="syncingGA">↻ 同步 GA</el-button>
        </div>
        <div class="stats-row" v-if="gaRow">
          <div class="stat-card panel"><div class="sc-num blue">{{ gaRow.sessions }}</div><div class="sc-lbl">会话</div></div>
          <div class="stat-card panel"><div class="sc-num green">{{ gaRow.users }}</div><div class="sc-lbl">用户</div></div>
          <div class="stat-card panel"><div class="sc-num orange">{{ gaRow.pageviews }}</div><div class="sc-lbl">页面浏览</div></div>
          <div class="stat-card panel"><div class="sc-num purple">{{ gaRow.bounce_rate?.toFixed(1) }}%</div><div class="sc-lbl">跳出率</div></div>
        </div>
        <el-table :data="gaData" stripe v-loading="loading">
          <el-table-column prop="date" label="日期" width="120" />
          <el-table-column prop="sessions" label="会话" />
          <el-table-column prop="users" label="用户" />
          <el-table-column prop="pageviews" label="浏览量" />
          <el-table-column label="跳出率"><template #default="{row}">{{ row.bounce_rate?.toFixed(1) }}%</template></el-table-column>
        </el-table>
        <div v-if="!gaData.length && !loading" class="empty-cell">暂无 GA 数据</div>
      </el-tab-pane>

      <!-- Yandex -->
      <el-tab-pane label="🟡 Yandex" name="yandex">
        <div class="tab-bar">
          <el-button size="small" @click="syncYandex" :loading="syncingYandex">↻ 同步 Yandex</el-button>
        </div>
        <div class="stats-row" v-if="yandexRow">
          <div class="stat-card panel"><div class="sc-num blue">{{ yandexRow.clicks }}</div><div class="sc-lbl">点击</div></div>
          <div class="stat-card panel"><div class="sc-num orange">{{ yandexRow.impressions }}</div><div class="sc-lbl">展示</div></div>
          <div class="stat-card panel"><div class="sc-num green">{{ yandexRow.index_count }}</div><div class="sc-lbl">收录数</div></div>
        </div>
        <el-table :data="yandexData" stripe v-loading="loading">
          <el-table-column prop="date" label="日期" width="120" />
          <el-table-column prop="clicks" label="点击" />
          <el-table-column prop="impressions" label="展示" />
          <el-table-column prop="index_count" label="收录数" />
        </el-table>
        <div v-if="!yandexData.length && !loading" class="empty-cell">暂无 Yandex 数据</div>
      </el-tab-pane>
    </el-tabs>
  </template>
</div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const sites = ref([])
const siteId = ref(null)
const activeTab = ref('gsc')
const loading = ref(false)
const syncing = ref(false)
const syncingGSC = ref(false)
const syncingGA = ref(false)
const syncingYandex = ref(false)

// GSC
const gscData = ref([])
const gscDate = ref(null)
const gscDates = ref([])
const gscKeywords = ref([])
const gscRow = ref(null)
const gscSearch = ref('')
const gscPage = ref(1)
const gscFiltered = computed(() => {
  let kws = [...gscKeywords.value]
  if (gscSearch.value) { const q = gscSearch.value.toLowerCase(); kws = kws.filter(k => k.query?.toLowerCase().includes(q)) }
  return kws.sort((a,b) => (b.clicks||0)-(a.clicks||0))
})

// GA
const gaData = ref([])
const gaRow = computed(() => gaData.value[0] || null)

// Yandex
const yandexData = ref([])
const yandexRow = computed(() => yandexData.value[0] || null)

async function loadData() {
  if (!siteId.value) return
  loading.value = true
  try {
    const [gsc, ga, yx] = await Promise.all([
      api.getGSCData(siteId.value, 30).catch(() => ({data:[]})),
      api.getGAData(siteId.value).catch(() => ({data:[]})),
      api.getYandexData(siteId.value).catch(() => ({data:[]})),
    ])
    gscData.value = gsc.data || []
    gaData.value = ga.data || []
    yandexData.value = yx.data || []
    gscDates.value = gscData.value.map(d => d.date)
    if (gscDates.value.length) { gscDate.value = gscDates.value[0]; parseGSC() }
  } catch {}
  loading.value = false
}

function parseGSC() {
  const row = gscData.value.find(d => d.date === gscDate.value)
  gscRow.value = row || null
  if (row?.keywords_json) {
    try { gscKeywords.value = typeof row.keywords_json === 'string' ? JSON.parse(row.keywords_json) : row.keywords_json } catch { gscKeywords.value = [] }
  } else { gscKeywords.value = [] }
  gscPage.value = 1
}

async function syncGSC() { syncingGSC.value=true; try { await api.syncGSCData(siteId.value); ElMessage.success('GSC 同步成功'); loadData() } catch(e){ElMessage.error(e?.response?.data?.error||'同步失败')} finally{syncingGSC.value=false} }
async function syncGA() { syncingGA.value=true; try { await api.syncGA(siteId.value); ElMessage.success('GA 同步成功'); loadData() } catch(e){ElMessage.error(e?.response?.data?.error||'同步失败')} finally{syncingGA.value=false} }
async function syncYandex() { syncingYandex.value=true; try { await api.syncYandex(siteId.value); ElMessage.success('Yandex 同步成功'); loadData() } catch(e){ElMessage.error(e?.response?.data?.error||'同步失败')} finally{syncingYandex.value=false} }
async function syncAll() { syncing.value=true; await Promise.allSettled([syncGSC(),syncGA(),syncYandex()]); syncing.value=false; ElMessage.success('同步完成') }

onMounted(async () => { const res = await api.listSites().catch(()=>({data:[]})); sites.value = res.data||[]; if(sites.value.length){siteId.value=sites.value[0].id;loadData()} })
</script>

<style scoped>
.page-top{margin-bottom:18px} .page-top h1{font-size:20px;font-weight:800}
.selector-bar{display:flex;align-items:center;gap:12px;margin-bottom:18px}
.selector-label{font-weight:700;color:var(--ink-mid);font-size:13px}
.empty-hint{text-align:center;padding:40px;color:var(--ink-hint)}
.empty-cell{text-align:center;padding:30px;color:var(--ink-hint)}
.stats-row{display:flex;gap:14px;margin-bottom:14px} .stat-card{flex:1;text-align:center;padding:14px 10px}
.sc-num{font-size:24px;font-weight:800} .sc-lbl{font-size:12px;color:var(--ink-hint);margin-top:4px}
.blue{color:#2563EB} .orange{color:#F59E0B} .green{color:#16A34A} .purple{color:#7C3AED}
.tab-bar{margin-bottom:14px;display:flex;gap:10px;align-items:center}
.panel{background:var(--surface);border-radius:var(--r);box-shadow:var(--s1);border:1.5px solid var(--border-light);padding:16px 20px}
</style>
