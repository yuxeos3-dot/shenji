<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push(`/sites/${$route.params.id}`)" class="back-btn">← 返回站点</a>
      <span class="back-sep">·</span>
      <span class="back-url">数据中心</span>
    </div>

    <div class="tab-bar">
      <button :class="['tab-btn', activeTab==='gsc'&&'active']" @click="activeTab='gsc'">📊 Google Search Console</button>
      <button :class="['tab-btn', activeTab==='ga'&&'active']" @click="activeTab='ga'">📈 Google Analytics</button>
      <button :class="['tab-btn', activeTab==='yandex'&&'active']" @click="activeTab='yandex'">🔍 Yandex Webmaster</button>
    </div>

    <div class="panel" style="border-top-left-radius:0;border-top-right-radius:0">
      <!-- GSC -->
      <div v-show="activeTab==='gsc'">
        <div class="panel-header">
          <h2>GSC 数据</h2>
          <button class="btn btn-primary btn-sm" @click="syncGSC" :disabled="syncing.gsc">{{ syncing.gsc ? '同步中...' : '↻ 同步GSC' }}</button>
        </div>
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
            <tr v-if="!gscData.length"><td colspan="5" class="empty-cell">暂无数据</td></tr>
          </tbody>
        </table>
      </div>

      <!-- GA -->
      <div v-show="activeTab==='ga'">
        <div class="panel-header">
          <h2>GA 数据</h2>
          <button class="btn btn-primary btn-sm" @click="syncGA" :disabled="syncing.ga">{{ syncing.ga ? '同步中...' : '↻ 同步GA' }}</button>
        </div>
        <table class="table">
          <thead><tr><th>日期</th><th>会话</th><th>用户</th><th>页面浏览</th><th>跳出率</th></tr></thead>
          <tbody>
            <tr v-for="d in gaData" :key="d.date">
              <td style="font-weight:600">{{ d.date }}</td>
              <td class="num-blue">{{ d.sessions }}</td>
              <td class="num-green">{{ d.users }}</td>
              <td class="num-orange">{{ d.pageviews }}</td>
              <td class="num-purple">{{ (d.bounce_rate*100).toFixed(1) }}%</td>
            </tr>
            <tr v-if="!gaData.length"><td colspan="5" class="empty-cell">暂无数据</td></tr>
          </tbody>
        </table>
      </div>

      <!-- Yandex -->
      <div v-show="activeTab==='yandex'">
        <div class="panel-header">
          <h2>Yandex 数据</h2>
          <button class="btn btn-primary btn-sm" @click="syncYandex" :disabled="syncing.yandex">{{ syncing.yandex ? '同步中...' : '↻ 同步Yandex' }}</button>
        </div>
        <table class="table">
          <thead><tr><th>日期</th><th>点击</th><th>展示</th><th>收录数</th></tr></thead>
          <tbody>
            <tr v-for="d in yandexData" :key="d.date">
              <td style="font-weight:600">{{ d.date }}</td>
              <td class="num-blue">{{ d.clicks }}</td>
              <td class="num-green">{{ d.impressions }}</td>
              <td class="num-purple">{{ d.index_count }}</td>
            </tr>
            <tr v-if="!yandexData.length"><td colspan="4" class="empty-cell">暂无数据</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import apiClient from '../api'

const route = useRoute()
const activeTab = ref('gsc')
const gscData = ref([])
const gaData = ref([])
const yandexData = ref([])
const syncing = ref({ gsc: false, ga: false, yandex: false })

async function loadData() {
  const id = route.params.id
  const [g, a, y] = await Promise.all([
    apiClient.getGSCData(id, 30).catch(() => ({ data: [] })),
    apiClient.getGAData(id).catch(() => ({ data: [] })),
    apiClient.getYandexData(id).catch(() => ({ data: [] })),
  ])
  gscData.value = g.data; gaData.value = a.data; yandexData.value = y.data
}

async function syncGSC() { syncing.value.gsc = true; try { await apiClient.syncGSC(route.params.id); ElMessage.success('GSC同步成功'); loadData() } catch {} finally { syncing.value.gsc = false } }
async function syncGA() { syncing.value.ga = true; try { await apiClient.syncGA(route.params.id); ElMessage.success('GA同步成功'); loadData() } catch {} finally { syncing.value.ga = false } }
async function syncYandex() { syncing.value.yandex = true; try { await apiClient.syncYandex(route.params.id); ElMessage.success('Yandex同步成功'); loadData() } catch {} finally { syncing.value.yandex = false } }

onMounted(loadData)
</script>

<style scoped>
.back-row { display:flex; align-items:center; gap:8px; margin-bottom:18px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); }
.back-sep { color:var(--ink-hint); }
.back-url { font-size:14px; font-weight:700; }

.tab-bar { display:flex; gap:0; }
.tab-btn { padding:10px 22px; font-size:13px; font-weight:700; border:1.5px solid var(--border-light); border-bottom:none; background:var(--surface-2); color:var(--ink-soft); cursor:pointer; transition:all .15s; border-radius:var(--r-sm) var(--r-sm) 0 0; font-family:inherit; }
.tab-btn.active { background:var(--surface); color:var(--sun-deep); box-shadow:0 -2px 8px rgba(150,60,0,.05); }
.tab-btn:hover:not(.active) { background:var(--sun-light); color:var(--sun); }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:16px; }
.panel-header h2 { font-size:15px; font-weight:700; }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep),var(--sun)); color:#fff; }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; }
.btn-sm { padding:6px 14px; font-size:12.5px; }

.table { width:100%; border-collapse:collapse; }
.table th { padding:10px 14px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.08em; border-bottom:2px solid var(--border); background:var(--surface-3); }
.table td { padding:10px 14px; border-bottom:1px solid var(--border-light); font-size:13px; }
.table tr:hover td { background:var(--sun-light); }

.num-blue { color:var(--info); font-weight:700; font-family:'JetBrains Mono',monospace; font-size:12px; }
.num-green { color:var(--ok); font-weight:700; font-family:'JetBrains Mono',monospace; font-size:12px; }
.num-orange { color:var(--warn); font-weight:700; font-family:'JetBrains Mono',monospace; font-size:12px; }
.num-purple { color:var(--purple); font-weight:700; font-family:'JetBrains Mono',monospace; font-size:12px; }

.empty-cell { text-align:center; color:var(--ink-hint); padding:30px !important; }
</style>
