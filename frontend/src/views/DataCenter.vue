<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push(`/sites/${$route.params.id}`)" class="back-btn">← 返回站点</a>
      <span class="back-sep">·</span>
      <span class="back-url">数据中心</span>
    </div>

    <div class="tab-bar">
      <button :class="['tab-btn', activeTab==='gsc'&&'active']" @click="activeTab='gsc'">📊 Search Console</button>
      <button :class="['tab-btn', activeTab==='ga'&&'active']" @click="activeTab='ga'">📈 Google Analytics</button>
      <button :class="['tab-btn', activeTab==='bing'&&'active']" @click="activeTab='bing'">🔷 Bing 站长</button>
      <button :class="['tab-btn', activeTab==='yandex'&&'active']" @click="activeTab='yandex'">🔍 Yandex</button>
    </div>

    <div class="main-panel">
      <!-- GSC -->
      <div v-show="activeTab==='gsc'">
        <div class="ph"><h2>Google Search Console</h2><button class="btn btn-primary btn-sm" @click="syncGSC" :disabled="syncing.gsc">{{ syncing.gsc?'同步中...':'↻ 同步GSC' }}</button></div>
        <template v-if="gscData.length">
          <div class="stat-row">
            <div class="sc"><div class="sc-l">总点击</div><div class="sc-v c">{{ fmt(gscData[0]?.clicks) }}</div></div>
            <div class="sc"><div class="sc-l">总展示</div><div class="sc-v g">{{ fmt(gscData[0]?.impressions) }}</div></div>
            <div class="sc"><div class="sc-l">平均CTR</div><div class="sc-v y">{{ ((gscData[0]?.ctr||0)*100).toFixed(2) }}%</div></div>
            <div class="sc"><div class="sc-l">平均排名</div><div class="sc-v p">{{ (gscData[0]?.position||0).toFixed(1) }}</div></div>
          </div>
          <div class="st">趋势数据</div>
          <table class="table"><thead><tr><th>日期</th><th>点击</th><th>展示</th><th>CTR</th><th>排名</th><th>热门关键词</th></tr></thead><tbody>
            <tr v-for="d in gscData" :key="d.date">
              <td class="dc">{{ d.date }}</td><td class="num c">{{ fmt(d.clicks) }}</td><td class="num g">{{ fmt(d.impressions) }}</td><td class="num y">{{ ((d.ctr||0)*100).toFixed(2) }}%</td><td class="num p">{{ (d.position||0).toFixed(1) }}</td><td class="kc">{{ topKW(d.keywords_json) }}</td>
            </tr>
          </tbody></table>
        </template>
        <div v-else class="eh">暂无GSC数据，请先点击「同步GSC」</div>
      </div>

      <!-- GA4 -->
      <div v-show="activeTab==='ga'">
        <div class="ph"><h2>Google Analytics 4</h2><button class="btn btn-primary btn-sm" @click="syncGA" :disabled="syncing.ga">{{ syncing.ga?'同步中...':'↻ 同步GA4' }}</button></div>
        <template v-if="gaData.length">
          <div class="stat-row">
            <div class="sc"><div class="sc-l">会话数</div><div class="sc-v c">{{ fmt(gaData[0]?.sessions) }}</div></div>
            <div class="sc"><div class="sc-l">用户数</div><div class="sc-v g">{{ fmt(gaData[0]?.users) }}</div></div>
            <div class="sc"><div class="sc-l">页面浏览</div><div class="sc-v y">{{ fmt(gaData[0]?.pageviews) }}</div></div>
            <div class="sc"><div class="sc-l">跳出率</div><div class="sc-v p">{{ ((gaData[0]?.bounce_rate||0)*100).toFixed(1) }}%</div></div>
            <div class="sc"><div class="sc-l">平均时长</div><div class="sc-v" style="color:var(--info)">{{ fmtDur(gaData[0]?.avg_duration) }}</div></div>
          </div>
          <div class="two-col">
            <div>
              <div class="st">流量渠道</div>
              <table class="table" v-if="gaChannels.length"><thead><tr><th>渠道</th><th>会话</th><th>用户</th><th>浏览</th></tr></thead><tbody>
                <tr v-for="ch in gaChannels" :key="ch.dim"><td><span class="cb">{{ ch.dim }}</span></td><td class="num c">{{ fmt(ch.sessions) }}</td><td class="num g">{{ fmt(ch.users) }}</td><td class="num y">{{ fmt(ch.pageviews) }}</td></tr>
              </tbody></table>
              <div v-else class="eh small">暂无渠道数据</div>
            </div>
            <div>
              <div class="st">热门页面 Top 20</div>
              <table class="table" v-if="gaPages.length"><thead><tr><th>页面路径</th><th>浏览</th><th>用户</th><th>跳出率</th></tr></thead><tbody>
                <tr v-for="pg in gaPages" :key="pg.path"><td class="pc" :title="pg.path">{{ pg.path }}</td><td class="num y">{{ fmt(pg.views) }}</td><td class="num g">{{ fmt(pg.users) }}</td><td class="num p">{{ (pg.bounceRate*100).toFixed(1) }}%</td></tr>
              </tbody></table>
              <div v-else class="eh small">暂无页面数据</div>
            </div>
          </div>
          <div class="st" style="margin-top:20px">同步历史</div>
          <table class="table"><thead><tr><th>日期</th><th>会话</th><th>用户</th><th>页面浏览</th><th>跳出率</th><th>平均时长</th></tr></thead><tbody>
            <tr v-for="d in gaData" :key="d.date"><td class="dc">{{ d.date }}</td><td class="num c">{{ fmt(d.sessions) }}</td><td class="num g">{{ fmt(d.users) }}</td><td class="num y">{{ fmt(d.pageviews) }}</td><td class="num p">{{ ((d.bounce_rate||0)*100).toFixed(1) }}%</td><td class="num" style="color:var(--info)">{{ fmtDur(d.avg_duration) }}</td></tr>
          </tbody></table>
        </template>
        <div v-else class="eh">暂无GA4数据。需在站点配置 GA4 Property ID 并完成 Google Analytics OAuth 授权</div>
      </div>

      <!-- Bing -->
      <div v-show="activeTab==='bing'">
        <div class="ph"><h2>Bing Webmaster Tools</h2><button class="btn btn-primary btn-sm" @click="syncBing" :disabled="syncing.bing">{{ syncing.bing?'同步中...':'↻ 同步Bing' }}</button></div>
        <template v-if="bingData.length">
          <div class="stat-row">
            <div class="sc"><div class="sc-l">点击数</div><div class="sc-v c">{{ fmt(bingData[0]?.clicks) }}</div></div>
            <div class="sc"><div class="sc-l">展示数</div><div class="sc-v g">{{ fmt(bingData[0]?.impressions) }}</div></div>
            <div class="sc"><div class="sc-l">CTR</div><div class="sc-v y">{{ ((bingData[0]?.ctr||0)*100).toFixed(2) }}%</div></div>
            <div class="sc"><div class="sc-l">平均排名</div><div class="sc-v p">{{ (bingData[0]?.avg_position||0).toFixed(1) }}</div></div>
            <div class="sc"><div class="sc-l">收录数</div><div class="sc-v" style="color:var(--info)">{{ fmt(bingData[0]?.index_count) }}</div></div>
          </div>
          <div class="two-col">
            <div>
              <div class="st">热门关键词</div>
              <table class="table" v-if="bingQueries.length"><thead><tr><th>关键词</th><th>点击</th><th>展示</th><th>CTR</th><th>排名</th></tr></thead><tbody>
                <tr v-for="q in bingQueries" :key="q.Query"><td class="kc">{{ q.Query }}</td><td class="num c">{{ fmt(q.Clicks) }}</td><td class="num g">{{ fmt(q.Impressions) }}</td><td class="num y">{{ ((q.CTR||0)*100).toFixed(2) }}%</td><td class="num p">{{ (q.AvgImpPos||0).toFixed(1) }}</td></tr>
              </tbody></table>
              <div v-else class="eh small">暂无数据</div>
            </div>
            <div>
              <div class="st">热门页面</div>
              <table class="table" v-if="bingPages.length"><thead><tr><th>页面</th><th>点击</th><th>展示</th><th>CTR</th></tr></thead><tbody>
                <tr v-for="p in bingPages" :key="p.Url"><td class="pc" :title="p.Url">{{ (p.Url||'').replace(/^https?:\/\/[^/]+/,'') || p.Url }}</td><td class="num c">{{ fmt(p.Clicks) }}</td><td class="num g">{{ fmt(p.Impressions) }}</td><td class="num y">{{ ((p.CTR||0)*100).toFixed(2) }}%</td></tr>
              </tbody></table>
              <div v-else class="eh small">暂无数据</div>
            </div>
          </div>
          <div class="st" style="margin-top:20px">同步历史</div>
          <table class="table"><thead><tr><th>日期</th><th>点击</th><th>展示</th><th>CTR</th><th>排名</th><th>收录数</th></tr></thead><tbody>
            <tr v-for="d in bingData" :key="d.date"><td class="dc">{{ d.date }}</td><td class="num c">{{ fmt(d.clicks) }}</td><td class="num g">{{ fmt(d.impressions) }}</td><td class="num y">{{ ((d.ctr||0)*100).toFixed(2) }}%</td><td class="num p">{{ (d.avg_position||0).toFixed(1) }}</td><td class="num" style="color:var(--info)">{{ fmt(d.index_count) }}</td></tr>
          </tbody></table>
        </template>
        <div v-else class="eh">暂无Bing数据。需要：<br>1. 站点详情填写「Bing Site URL」<br>2. 系统设置配置「Bing API Key」（<a href="https://www.bing.com/webmasters/home" target="_blank" style="color:var(--info);font-weight:600">Bing站长后台获取 →</a>）<br>3. 点击「同步Bing」</div>
      </div>

      <!-- Yandex -->
      <div v-show="activeTab==='yandex'">
        <div class="ph"><h2>Yandex Webmaster</h2><button class="btn btn-primary btn-sm" @click="syncYandex" :disabled="syncing.yandex">{{ syncing.yandex?'同步中...':'↻ 同步Yandex' }}</button></div>
        <template v-if="yandexData.length">
          <div class="stat-row">
            <div class="sc"><div class="sc-l">点击数</div><div class="sc-v c">{{ fmt(yandexData[0]?.clicks) }}</div></div>
            <div class="sc"><div class="sc-l">展示数</div><div class="sc-v g">{{ fmt(yandexData[0]?.impressions) }}</div></div>
            <div class="sc"><div class="sc-l">收录数</div><div class="sc-v p">{{ fmt(yandexData[0]?.index_count) }}</div></div>
          </div>
          <div class="two-col">
            <div>
              <div class="st">热门关键词</div>
              <table class="table" v-if="yandexKeywords.length"><thead><tr><th>关键词</th><th>点击</th><th>展示</th><th>排名</th></tr></thead><tbody>
                <tr v-for="k in yandexKeywords" :key="k.query_text"><td class="kc">{{ k.query_text }}</td><td class="num c">{{ fmt(k.indicators?.TOTAL_CLICKS||0) }}</td><td class="num g">{{ fmt(k.indicators?.TOTAL_SHOWS||0) }}</td><td class="num p">{{ (k.indicators?.AVG_SHOW_POSITION||0).toFixed(1) }}</td></tr>
              </tbody></table>
              <div v-else class="eh small">暂无关键词数据</div>
            </div>
            <div>
              <div class="st">同步历史</div>
              <table class="table"><thead><tr><th>日期</th><th>点击</th><th>展示</th><th>收录数</th></tr></thead><tbody>
                <tr v-for="d in yandexData" :key="d.date"><td class="dc">{{ d.date }}</td><td class="num c">{{ fmt(d.clicks) }}</td><td class="num g">{{ fmt(d.impressions) }}</td><td class="num p">{{ fmt(d.index_count) }}</td></tr>
              </tbody></table>
            </div>
          </div>
        </template>
        <div v-else class="eh">暂无Yandex数据，请配置 Yandex Host ID 并同步</div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import apiClient from '../api'

const route = useRoute()
const activeTab = ref('gsc')
const gscData = ref([])
const gaData = ref([])
const bingData = ref([])
const yandexData = ref([])
const syncing = ref({ gsc:false, ga:false, bing:false, yandex:false })

const fmt = (n) => { n=Math.round(n||0); if(n>=1000000)return(n/1000000).toFixed(1)+'M'; if(n>=1000)return(n/1000).toFixed(1)+'K'; return n.toLocaleString() }
const fmtDur = (s) => { s=Math.round(s||0); if(s<60)return s+'s'; return Math.floor(s/60)+'m'+(s%60)+'s' }

const topKW = (kwJson) => {
  try { const rows=(typeof kwJson==='string'?JSON.parse(kwJson):kwJson)?.rows||[]; return rows.slice(0,2).map(r=>r.keys?.[0]||'').filter(Boolean).join(', ')||'—' } catch { return '—' }
}
const gaChannels = computed(() => {
  try { const rows=(typeof gaData.value[0]?.channels_json==='string'?JSON.parse(gaData.value[0].channels_json):gaData.value[0]?.channels_json)?.rows||[]; return rows.map(r=>({dim:r.dimensionValues?.[0]?.value||'—',sessions:+r.metricValues?.[0]?.value||0,users:+r.metricValues?.[1]?.value||0,pageviews:+r.metricValues?.[2]?.value||0})).sort((a,b)=>b.sessions-a.sessions) } catch { return [] }
})
const gaPages = computed(() => {
  try { const rows=(typeof gaData.value[0]?.pages_json==='string'?JSON.parse(gaData.value[0].pages_json):gaData.value[0]?.pages_json)?.rows||[]; return rows.map(r=>({path:r.dimensionValues?.[0]?.value||'—',views:+r.metricValues?.[0]?.value||0,users:+r.metricValues?.[1]?.value||0,bounceRate:+r.metricValues?.[3]?.value||0})) } catch { return [] }
})
const bingQueries = computed(() => {
  try { const r=typeof bingData.value[0]?.queries_json==='string'?JSON.parse(bingData.value[0].queries_json):bingData.value[0]?.queries_json; return r?.d?.Results||[] } catch { return [] }
})
const bingPages = computed(() => {
  try { const r=typeof bingData.value[0]?.pages_json==='string'?JSON.parse(bingData.value[0].pages_json):bingData.value[0]?.pages_json; return r?.d?.Results||[] } catch { return [] }
})
const yandexKeywords = computed(() => {
  try { const r=typeof yandexData.value[0]?.keywords_json==='string'?JSON.parse(yandexData.value[0].keywords_json):yandexData.value[0]?.keywords_json; return Array.isArray(r)?r.slice(0,20):[] } catch { return [] }
})

async function loadData() {
  const id = route.params.id
  const [g,a,b,y] = await Promise.all([
    apiClient.getGSCData(id,30).catch(()=>({data:[]})),
    apiClient.getGAData(id).catch(()=>({data:[]})),
    apiClient.getBingData(id).catch(()=>({data:[]})),
    apiClient.getYandexData(id).catch(()=>({data:[]})),
  ])
  gscData.value=g.data||[]; gaData.value=a.data||[]; bingData.value=b.data||[]; yandexData.value=y.data||[]
}

async function syncGSC(){syncing.value.gsc=true;try{await apiClient.syncGSC(route.params.id);ElMessage.success('GSC同步成功');loadData()}catch(e){console.error(e);ElMessage.error('GSC同步失败')}finally{syncing.value.gsc=false}}
async function syncGA(){syncing.value.ga=true;try{await apiClient.syncGA(route.params.id);ElMessage.success('GA4同步成功');loadData()}catch(e){console.error(e);ElMessage.error('GA4同步失败')}finally{syncing.value.ga=false}}
async function syncBing(){syncing.value.bing=true;try{await apiClient.syncBing(route.params.id);ElMessage.success('Bing同步成功');loadData()}catch(e){console.error(e);ElMessage.error('Bing同步失败')}finally{syncing.value.bing=false}}
async function syncYandex(){syncing.value.yandex=true;try{await apiClient.syncYandex(route.params.id);ElMessage.success('Yandex同步成功');loadData()}catch(e){console.error(e);ElMessage.error('Yandex同步失败')}finally{syncing.value.yandex=false}}

onMounted(loadData)
watch(()=>route.params.id,()=>{if(route.params.id)loadData()})
</script>

<style scoped>
.back-row{display:flex;align-items:center;gap:8px;margin-bottom:18px}.back-btn{font-size:13px;font-weight:600;color:var(--sun)}.back-sep{color:var(--ink-hint)}.back-url{font-size:14px;font-weight:700}
.tab-bar{display:flex;gap:0}.tab-btn{padding:10px 22px;font-size:13px;font-weight:700;border:1.5px solid var(--border-light);border-bottom:none;background:var(--surface-2);color:var(--ink-soft);cursor:pointer;transition:all .15s;border-radius:var(--r-sm) var(--r-sm) 0 0;font-family:inherit}.tab-btn.active{background:var(--surface);color:var(--sun-deep)}.tab-btn:hover:not(.active){background:var(--sun-light);color:var(--sun)}
.main-panel{background:var(--surface);border-radius:var(--r);box-shadow:var(--s1);border:1.5px solid var(--border-light);border-top-left-radius:0;padding:22px 26px}
.ph{display:flex;align-items:center;justify-content:space-between;margin-bottom:18px}.ph h2{font-size:16px;font-weight:700;color:var(--ink)}
.stat-row{display:grid;grid-template-columns:repeat(5,1fr);gap:12px;margin-bottom:20px}
.sc{background:var(--surface-2);border:1.5px solid var(--border-light);border-radius:var(--r-sm);padding:14px 16px}
.sc-l{font-size:11px;font-weight:700;color:var(--ink-hint);text-transform:uppercase;letter-spacing:.06em;margin-bottom:6px}
.sc-v{font-size:22px;font-weight:800;font-family:'JetBrains Mono',monospace}
.st{font-size:13px;font-weight:700;color:var(--ink-mid);margin:16px 0 10px;padding-bottom:6px;border-bottom:1.5px solid var(--border-light)}
.two-col{display:grid;grid-template-columns:1fr 1fr;gap:20px}
.table{width:100%;border-collapse:collapse}.table th{padding:9px 12px;text-align:left;font-size:11px;font-weight:800;color:var(--ink-mid);text-transform:uppercase;letter-spacing:.07em;border-bottom:2px solid var(--border);background:var(--surface-3)}.table td{padding:9px 12px;border-bottom:1px solid var(--border-light);font-size:12.5px}.table tr:hover td{background:var(--sun-light)}
.dc{font-weight:600;color:var(--ink);font-size:12px;white-space:nowrap}
.num{font-family:'JetBrains Mono',monospace;font-size:12px;font-weight:700;text-align:right}
.c{color:var(--info)}.g{color:var(--ok)}.y{color:var(--warn)}.p{color:#7C3AED}
.kc{font-size:12px;color:var(--ink);max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.pc{font-size:11.5px;color:var(--ink-soft);max-width:220px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-family:'JetBrains Mono',monospace}
.cb{display:inline-block;padding:2px 8px;background:var(--sun-light);color:var(--sun-deep);border-radius:5px;font-size:11.5px;font-weight:600}
.eh{text-align:center;color:var(--ink-hint);padding:50px 20px;font-size:14px;line-height:2}.eh.small{padding:20px;font-size:13px}
.btn{display:inline-flex;align-items:center;gap:6px;padding:9px 20px;border-radius:9px;border:1.5px solid var(--border);background:var(--surface);cursor:pointer;font-family:inherit;font-size:13.5px;font-weight:700;transition:all .18s;color:var(--ink-mid)}.btn-primary{background:linear-gradient(135deg,var(--sun),var(--coral));color:#fff;border-color:transparent;box-shadow:var(--s-amber)}.btn-primary:disabled{opacity:.6;cursor:not-allowed}.btn-sm{padding:6px 14px;font-size:12.5px}
</style>
