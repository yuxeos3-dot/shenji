<template>
  <div class="geo-page">
    <!-- Site Selector -->
    <div class="panel selector-bar">
      <span class="selector-label">🌍 选择站点</span>
      <el-select v-model="selectedSiteId" placeholder="请选择站点" style="width:260px" @change="onSiteChange">
        <el-option v-for="s in sites" :key="s.id" :label="s.name + ' (' + s.domain + ')'" :value="s.id" />
      </el-select>
      <el-select v-model="selectedDate" placeholder="选择日期" style="width:160px" @change="onDateChange" v-if="dateOptions.length">
        <el-option v-for="d in dateOptions" :key="d" :label="d" :value="d" />
      </el-select>
      <el-input v-model="search" placeholder="搜索关键词..." style="width:200px" clearable>
        <template #prefix>🔍</template>
      </el-input>
      <button class="btn btn-primary btn-sm" @click="syncGSC" :disabled="syncing" v-if="selectedSiteId">
        {{ syncing ? '同步中...' : '↻ 同步GSC' }}
      </button>
    </div>

    <div v-if="!selectedSiteId" class="empty-hint panel">
      <div class="empty-icon">🌍</div>
      <div>请先选择一个站点查看关键词数据</div>
    </div>

    <template v-else>
      <!-- Summary Cards -->
      <div class="summary-row" v-if="keywords.length">
        <div class="summary-card panel">
          <div class="sc-label">关键词总数</div>
          <div class="sc-value num-blue">{{ keywords.length }}</div>
          <div class="sc-sub">{{ selectedDate }}</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">总点击数</div>
          <div class="sc-value num-green">{{ totalClicks }}</div>
          <div class="sc-sub">来自 GSC</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">总展示数</div>
          <div class="sc-value num-orange">{{ totalImpressions }}</div>
          <div class="sc-sub">来自 GSC</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">平均排名</div>
          <div class="sc-value num-purple">{{ avgPosition }}</div>
          <div class="sc-sub">Top 关键词</div>
        </div>
      </div>

      <!-- Keywords Table -->
      <div class="panel" v-loading="loading">
        <div class="panel-header">
          <h2>关键词排名数据 <span class="badge" v-if="filteredKeywords.length">{{ filteredKeywords.length }} 条</span></h2>
          <div class="sort-btns">
            <button :class="['sort-btn', sortBy==='clicks'&&'active']" @click="sortBy='clicks'">按点击排</button>
            <button :class="['sort-btn', sortBy==='impressions'&&'active']" @click="sortBy='impressions'">按展示排</button>
            <button :class="['sort-btn', sortBy==='position'&&'active']" @click="sortBy='position'">按排名排</button>
          </div>
        </div>

        <div v-if="!keywords.length && !loading" class="empty-cell-big">
          暂无关键词数据，请先同步GSC数据
        </div>

        <table class="table" v-else>
          <thead>
            <tr>
              <th>#</th>
              <th>关键词</th>
              <th>点击</th>
              <th>展示</th>
              <th>CTR</th>
              <th>平均排名</th>
              <th>排名级别</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(kw, idx) in paginatedKeywords" :key="kw.query">
              <td class="idx-cell">{{ (currentPage-1)*pageSize + idx + 1 }}</td>
              <td class="kw-cell">
                <span class="kw-text">{{ kw.query }}</span>
              </td>
              <td class="num-blue">{{ kw.clicks }}</td>
              <td class="num-green">{{ kw.impressions }}</td>
              <td class="num-orange">{{ kw.ctr ? (kw.ctr*100).toFixed(2)+'%' : '-' }}</td>
              <td class="num-purple">{{ kw.position?.toFixed(1) }}</td>
              <td>
                <span :class="['rank-badge', rankClass(kw.position)]">{{ rankLabel(kw.position) }}</span>
              </td>
            </tr>
          </tbody>
        </table>

        <div class="pagination" v-if="filteredKeywords.length > pageSize">
          <el-pagination
            v-model:current-page="currentPage"
            :page-size="pageSize"
            :total="filteredKeywords.length"
            layout="prev, pager, next, total"
            background
          />
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import apiClient from '../api'

const sites = ref([])
const selectedSiteId = ref(null)
const selectedDate = ref(null)
const dateOptions = ref([])
const rawGscData = ref([])
const loading = ref(false)
const syncing = ref(false)
const search = ref('')
const sortBy = ref('clicks')
const currentPage = ref(1)
const pageSize = 50

const keywords = computed(() => {
  if (!selectedDate.value) return []
  const row = rawGscData.value.find(d => d.date === selectedDate.value)
  if (!row || !row.keywords_json) return []
  try {
    const kws = typeof row.keywords_json === 'string' ? JSON.parse(row.keywords_json) : row.keywords_json
    return Array.isArray(kws) ? kws : []
  } catch { return [] }
})

const filteredKeywords = computed(() => {
  let kws = [...keywords.value]
  if (search.value) {
    const q = search.value.toLowerCase()
    kws = kws.filter(k => k.query?.toLowerCase().includes(q))
  }
  kws.sort((a, b) => {
    if (sortBy.value === 'position') return (a.position || 999) - (b.position || 999)
    return (b[sortBy.value] || 0) - (a[sortBy.value] || 0)
  })
  return kws
})

const paginatedKeywords = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return filteredKeywords.value.slice(start, start + pageSize)
})

const totalClicks = computed(() => keywords.value.reduce((s, k) => s + (k.clicks || 0), 0))
const totalImpressions = computed(() => keywords.value.reduce((s, k) => s + (k.impressions || 0), 0))
const avgPosition = computed(() => {
  const kws = keywords.value.filter(k => k.position)
  if (!kws.length) return '-'
  return (kws.reduce((s, k) => s + k.position, 0) / kws.length).toFixed(1)
})

function rankClass(pos) {
  if (!pos) return 'rank-none'
  if (pos <= 3) return 'rank-top3'
  if (pos <= 10) return 'rank-top10'
  if (pos <= 30) return 'rank-top30'
  return 'rank-low'
}
function rankLabel(pos) {
  if (!pos) return '-'
  if (pos <= 3) return '🥇 Top 3'
  if (pos <= 10) return '✅ Top 10'
  if (pos <= 30) return '📈 Top 30'
  return '📉 30+'
}

onMounted(async () => {
  const res = await apiClient.listSites().catch(() => ({ data: [] }))
  sites.value = res.data || []
  if (sites.value.length) {
    selectedSiteId.value = sites.value[0].id
    loadData()
  }
})

function onSiteChange() { currentPage.value = 1; loadData() }
function onDateChange() { currentPage.value = 1 }

watch(search, () => { currentPage.value = 1 })
watch(sortBy, () => { currentPage.value = 1 })

async function loadData() {
  if (!selectedSiteId.value) return
  loading.value = true
  try {
    const res = await apiClient.getGSCData(selectedSiteId.value, 30)
    rawGscData.value = res.data || []
    dateOptions.value = rawGscData.value.map(d => d.date)
    if (dateOptions.value.length) selectedDate.value = dateOptions.value[0]
    else selectedDate.value = null
  } catch {}
  loading.value = false
}

async function syncGSC() {
  syncing.value = true
  try { await apiClient.syncGSC(selectedSiteId.value); ElMessage.success('GSC同步成功'); loadData() } catch {}
  finally { syncing.value = false }
}
</script>

<style scoped>
.geo-page { display: flex; flex-direction: column; gap: 18px; }

.panel { background: var(--surface); border-radius: var(--r); box-shadow: var(--s1); border: 1.5px solid var(--border-light); padding: 18px 24px; }

.selector-bar { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.selector-label { font-size: 13.5px; font-weight: 700; color: var(--ink-mid); white-space: nowrap; }

.empty-hint { text-align: center; padding: 60px 24px; color: var(--ink-hint); }
.empty-icon { font-size: 48px; margin-bottom: 12px; }

.summary-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 14px; }
.summary-card { padding: 18px 20px; }
.sc-label { font-size: 11px; font-weight: 700; color: var(--ink-hint); text-transform: uppercase; letter-spacing: .07em; margin-bottom: 8px; }
.sc-value { font-size: 28px; font-weight: 800; font-family: 'JetBrains Mono', monospace; line-height: 1; margin-bottom: 6px; }
.sc-sub { font-size: 11.5px; color: var(--ink-soft); }

.panel-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; flex-wrap: wrap; gap: 10px; }
.panel-header h2 { font-size: 15px; font-weight: 700; display: flex; align-items: center; gap: 8px; }
.badge { background: var(--sun-light); color: var(--sun-deep); font-size: 11px; font-weight: 700; padding: 2px 8px; border-radius: 10px; border: 1px solid var(--peach); }

.sort-btns { display: flex; gap: 6px; }
.sort-btn { padding: 5px 12px; font-size: 12px; font-weight: 600; border: 1.5px solid var(--border); border-radius: 7px; background: var(--surface-2); color: var(--ink-soft); cursor: pointer; font-family: inherit; transition: all .15s; }
.sort-btn.active { background: var(--sun-light); color: var(--sun-deep); border-color: var(--peach); }
.sort-btn:hover:not(.active) { background: var(--sun-light); color: var(--sun); }

.table { width: 100%; border-collapse: collapse; }
.table th { padding: 10px 14px; text-align: left; font-size: 11px; font-weight: 800; color: var(--ink-mid); text-transform: uppercase; letter-spacing: .08em; border-bottom: 2px solid var(--border); background: var(--surface-3); }
.table td { padding: 9px 14px; border-bottom: 1px solid var(--border-light); font-size: 13px; }
.table tr:hover td { background: var(--sun-light); }

.idx-cell { color: var(--ink-hint); font-size: 11px; width: 40px; }
.kw-cell { max-width: 320px; }
.kw-text { font-weight: 600; color: var(--ink); word-break: break-all; }

.num-blue { color: var(--info); font-weight: 700; font-family: 'JetBrains Mono', monospace; font-size: 12px; }
.num-green { color: var(--ok); font-weight: 700; font-family: 'JetBrains Mono', monospace; font-size: 12px; }
.num-orange { color: var(--warn); font-weight: 700; font-family: 'JetBrains Mono', monospace; font-size: 12px; }
.num-purple { color: var(--purple); font-weight: 700; font-family: 'JetBrains Mono', monospace; font-size: 12px; }

.rank-badge { display: inline-block; padding: 2px 8px; border-radius: 6px; font-size: 11px; font-weight: 700; }
.rank-top3 { background: #FFF7E0; color: #B45309; }
.rank-top10 { background: #DCFCE7; color: #166534; }
.rank-top30 { background: #DBEAFE; color: #1D4ED8; }
.rank-low { background: var(--surface-3); color: var(--ink-soft); }
.rank-none { background: var(--surface-3); color: var(--ink-hint); }

.empty-cell-big { text-align: center; color: var(--ink-hint); padding: 50px; font-size: 14px; }
.empty-cell { text-align: center; color: var(--ink-hint); padding: 30px !important; }

.pagination { margin-top: 18px; display: flex; justify-content: center; }

.btn { display: inline-flex; align-items: center; gap: 6px; padding: 9px 20px; border-radius: 9px; border: 1.5px solid var(--border); background: var(--surface); cursor: pointer; font-family: inherit; font-size: 13.5px; font-weight: 700; transition: all .18s; color: var(--ink-mid); }
.btn-primary { background: linear-gradient(135deg,var(--sun),var(--coral)); color: #fff; border-color: transparent; box-shadow: var(--s-amber); }
.btn-primary:hover { background: linear-gradient(135deg,var(--sun-deep),var(--sun)); }
.btn-primary:disabled { opacity: .6; cursor: not-allowed; }
.btn-sm { padding: 6px 14px; font-size: 12.5px; }
</style>
