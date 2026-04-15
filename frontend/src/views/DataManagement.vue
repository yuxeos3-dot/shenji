<template>
  <div class="dm-page">
    <!-- Site Selector -->
    <div class="panel selector-bar">
      <span class="selector-label">📡 选择站点</span>
      <el-select v-model="selectedSiteId" placeholder="请选择站点" style="width:260px" @change="onSiteChange">
        <el-option v-for="s in sites" :key="s.id" :label="s.name + ' (' + s.domain + ')'" :value="s.id" />
      </el-select>
      <div class="sync-btns" v-if="selectedSiteId">
        <button class="btn btn-primary btn-sm" @click="syncGSC" :disabled="syncing.gsc">{{ syncing.gsc ? '同步中...' : '↻ GSC' }}</button>
        <button class="btn btn-primary btn-sm" @click="syncGA" :disabled="syncing.ga">{{ syncing.ga ? '同步中...' : '↻ GA' }}</button>
        <button class="btn btn-primary btn-sm" @click="syncYandex" :disabled="syncing.yandex">{{ syncing.yandex ? '同步中...' : '↻ Yandex' }}</button>
      </div>
    </div>

    <div v-if="!selectedSiteId" class="empty-hint panel">
      <div class="empty-icon">📊</div>
      <div>请先选择一个站点查看数据</div>
    </div>

    <template v-else>
      <!-- Summary Cards -->
      <div class="summary-row" v-if="latestGSC">
        <div class="summary-card panel">
          <div class="sc-label">GSC 点击（最新）</div>
          <div class="sc-value num-blue">{{ latestGSC.clicks }}</div>
          <div class="sc-sub">{{ latestGSC.date }}</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">GSC 展示（最新）</div>
          <div class="sc-value num-green">{{ latestGSC.impressions }}</div>
          <div class="sc-sub">CTR {{ (latestGSC.ctr*100).toFixed(2) }}%</div>
        </div>
        <div class="summary-card panel">
          <div class="sc-label">GSC 平均排名</div>
          <div class="sc-value num-orange">{{ latestGSC.position?.toFixed(1) }}</div>
          <div class="sc-sub">{{ latestGSC.date }}</div>
        </div>
        <div class="summary-card panel" v-if="latestGA">
          <div class="sc-label">GA 会话（最新）</div>
          <div class="sc-value num-purple">{{ latestGA.sessions }}</div>
          <div class="sc-sub">用户 {{ latestGA.users }}</div>
        </div>
      </div>

      <!-- Tabs -->
      <div class="tab-bar">
        <button :class="['tab-btn', activeTab==='gsc'&&'active']" @click="activeTab='gsc'">📊 Google Search Console</button>
        <button :class="['tab-btn', activeTab==='ga'&&'active']" @click="activeTab='ga'">📈 Google Analytics</button>
        <button :class="['tab-btn', activeTab==='yandex'&&'active']" @click="activeTab='yandex'">🔍 Yandex Webmaster</button>
      </div>

      <div class="panel tab-panel" v-loading="loading">
        <!-- GSC Tab -->
        <div v-show="activeTab==='gsc'">
          <table class="table">
            <thead><tr><th>日期</th><th>点击</th><th>展示</th><th>CTR</th><th>平均排名</th></tr></thead>
            <tbody>
              <tr v-for="d in gscData" :key="d.date">
                <td style="font-weight:600">{{ d.date }}</td>
                <td class="num-blue">{{ d.clicks }}</td>
                <td class="num-green">{{ d.impressions }}</td>
                <td class="num-orange">{{ (d.ctr*100).toFixed(2) }}%</td>
                <td class="num-purple">{{ d.position?.toFixed(1) }}</td>
              </tr>
              <tr v-if="!gscData.length"><td colspan="5" class="empty-cell">暂无数据，请先同步GSC</td></tr>
            </tbody>
          </table>
        </div>

        <!-- GA Tab -->
        <div v-show="activeTab==='ga'">
          <table class="table">
            <thead><tr><th>日期</th><th>会话</th><th>用户</th><th>页面浏览</th><th>跳出率</th><th>平均时长(s)</th></tr></thead>
            <tbody>
              <tr v-for="d in gaData" :key="d.date">
                <td style="font-weight:600">{{ d.date }}</td>
                <td class="num-blue">{{ d.sessions }}</td>
                <td class="num-green">{{ d.users }}</td>
                <td class="num-orange">{{ d.pageviews }}</td>
                <td class="num-purple">{{ (d.bounce_rate*100).toFixed(1) }}%</td>
                <td class="num-blue">{{ d.avg_duration?.toFixed(0) }}s</td>
              </tr>
              <tr v-if="!gaData.length"><td colspan="6" class="empty-cell">暂无数据，请先同步GA</td></tr>
            </tbody>
          </table>
        </div>

        <!-- Yandex Tab -->
        <div v-show="activeTab==='yandex'">
          <table class="table">
            <thead><tr><th>日期</th><th>点击</th><th>展示</th><th>收录数</th></tr></thead>
            <tbody>
              <tr v-for="d in yandexData" :key="d.date">
                <td style="font-weight:600">{{ d.date }}</td>
                <td class="num-blue">{{ d.clicks }}</td>
                <td class="num-green">{{ d.impressions }}</td>
                <td class="num-purple">{{ d.index_count }}</td>
              </tr>
              <tr v-if="!yandexData.length"><td colspan="4" class="empty-cell">暂无数据，请先同步Yandex</td></tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import apiClient from '../api'

const sites = ref([])
const selectedSiteId = ref(null)
const activeTab = ref('gsc')
const loading = ref(false)
const gscData = ref([])
const gaData = ref([])
const yandexData = ref([])
const syncing = ref({ gsc: false, ga: false, yandex: false })

const latestGSC = computed(() => gscData.value[0] || null)
const latestGA = computed(() => gaData.value[0] || null)

onMounted(async () => {
  const res = await apiClient.listSites().catch(() => ({ data: [] }))
  sites.value = res.data || []
  if (sites.value.length) {
    selectedSiteId.value = sites.value[0].id
    loadData()
  }
})

function onSiteChange() { loadData() }

async function loadData() {
  if (!selectedSiteId.value) return
  loading.value = true
  const id = selectedSiteId.value
  const [g, a, y] = await Promise.all([
    apiClient.getGSCData(id, 30).catch(() => ({ data: [] })),
    apiClient.getGAData(id).catch(() => ({ data: [] })),
    apiClient.getYandexData(id).catch(() => ({ data: [] })),
  ])
  gscData.value = g.data || []
  gaData.value = a.data || []
  yandexData.value = y.data || []
  loading.value = false
}

async function syncGSC() {
  syncing.value.gsc = true
  try { await apiClient.syncGSC(selectedSiteId.value); ElMessage.success('GSC同步成功'); loadData() } catch {}
  finally { syncing.value.gsc = false }
}
async function syncGA() {
  syncing.value.ga = true
  try { await apiClient.syncGA(selectedSiteId.value); ElMessage.success('GA同步成功'); loadData() } catch {}
  finally { syncing.value.ga = false }
}
async function syncYandex() {
  syncing.value.yandex = true
  try { await apiClient.syncYandex(selectedSiteId.value); ElMessage.success('Yandex同步成功'); loadData() } catch {}
  finally { syncing.value.yandex = false }
}
</script>

<style scoped>
.dm-page { display: flex; flex-direction: column; gap: 18px; }

.panel { background: var(--surface); border-radius: var(--r); box-shadow: var(--s1); border: 1.5px solid var(--border-light); padding: 18px 24px; }

.selector-bar { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; }
.selector-label { font-size: 13.5px; font-weight: 700; color: var(--ink-mid); white-space: nowrap; }
.sync-btns { display: flex; gap: 8px; margin-left: auto; }

.empty-hint { text-align: center; padding: 60px 24px; color: var(--ink-hint); }
.empty-icon { font-size: 48px; margin-bottom: 12px; }

.summary-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 14px; }
.summary-card { padding: 18px 20px; }
.sc-label { font-size: 11px; font-weight: 700; color: var(--ink-hint); text-transform: uppercase; letter-spacing: .07em; margin-bottom: 8px; }
.sc-value { font-size: 28px; font-weight: 800; font-family: 'JetBrains Mono', monospace; line-height: 1; margin-bottom: 6px; }
.sc-sub { font-size: 11.5px; color: var(--ink-soft); }

.tab-bar { display: flex; gap: 0; }
.tab-btn { padding: 10px 22px; font-size: 13px; font-weight: 700; border: 1.5px solid var(--border-light); border-bottom: none; background: var(--surface-2); color: var(--ink-soft); cursor: pointer; transition: all .15s; border-radius: var(--r-sm) var(--r-sm) 0 0; font-family: inherit; }
.tab-btn.active { background: var(--surface); color: var(--sun-deep); box-shadow: 0 -2px 8px rgba(150,60,0,.05); }
.tab-btn:hover:not(.active) { background: var(--sun-light); color: var(--sun); }

.tab-panel { border-top-left-radius: 0; border-top-right-radius: 0; padding: 20px 24px; }

.table { width: 100%; border-collapse: collapse; }
.table th { padding: 10px 14px; text-align: left; font-size: 11px; font-weight: 800; color: var(--ink-mid); text-transform: uppercase; letter-spacing: .08em; border-bottom: 2px solid var(--border); background: var(--surface-3); }
.table td { padding: 10px 14px; border-bottom: 1px solid var(--border-light); font-size: 13px; }
.table tr:hover td { background: var(--sun-light); }

.num-blue { color: var(--info); font-weight: 700; font-family: 'JetBrains Mono', monospace; font-size: 12px; }
.num-green { color: var(--ok); font-weight: 700; font-family: 'JetBrains Mono', monospace; font-size: 12px; }
.num-orange { color: var(--warn); font-weight: 700; font-family: 'JetBrains Mono', monospace; font-size: 12px; }
.num-purple { color: var(--purple); font-weight: 700; font-family: 'JetBrains Mono', monospace; font-size: 12px; }

.empty-cell { text-align: center; color: var(--ink-hint); padding: 30px !important; }

.btn { display: inline-flex; align-items: center; gap: 6px; padding: 9px 20px; border-radius: 9px; border: 1.5px solid var(--border); background: var(--surface); cursor: pointer; font-family: inherit; font-size: 13.5px; font-weight: 700; transition: all .18s; color: var(--ink-mid); }
.btn-primary { background: linear-gradient(135deg,var(--sun),var(--coral)); color: #fff; border-color: transparent; box-shadow: var(--s-amber); }
.btn-primary:hover { background: linear-gradient(135deg,var(--sun-deep),var(--sun)); }
.btn-primary:disabled { opacity: .6; cursor: not-allowed; }
.btn-sm { padding: 6px 14px; font-size: 12.5px; }
</style>
