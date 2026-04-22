<template>
  <div class="site-analysis">
    <!-- Site selector -->
    <div class="panel toolbar">
      <div class="tb-row">
        <label>选择站点：</label>
        <el-select v-model="siteId" filterable placeholder="选择站点" @change="loadAnalysis" style="width:300px">
          <el-option v-for="s in sites" :key="s.id" :label="`${s.name} (${s.domain})`" :value="s.id" />
        </el-select>
        <el-button :loading="analyzing" @click="runPipeline" style="margin-left:auto">
          {{ analyzing ? '分析中...' : '🔬 运行完整分析' }}
        </el-button>
        <el-button type="primary" :loading="computing" @click="computeScore">
          {{ computing ? '计算中...' : '🔄 刷新健康分' }}
        </el-button>
      </div>
    </div>

    <div v-if="!siteId" class="empty-state">📊 选择一个站点开始全站 SEO 分析</div>

    <!-- Tab bar -->
    <div v-if="siteId" class="panel sub-bar">
      <button v-for="t in tabs" :key="t.key" :class="['sub-pill', tab === t.key && 'active']" @click="tab = t.key; loadTabData()">
        <span class="sp-i">{{ t.icon }}</span>{{ t.label }}
      </button>
    </div>

    <template v-if="siteId && tab === 'overview' && data">
      <!-- ─── Health Score Hero ─────────────────────────────── -->
      <div class="hero-row">
        <div class="panel hero-score">
          <div class="score-ring" :class="'grade-' + (data.grade||'F').toLowerCase()">
            <div class="score-num">{{ data.health_score || 0 }}</div>
            <div class="score-grade">{{ data.grade || '-' }}</div>
          </div>
          <div class="score-meta">
            <div class="score-label">站点健康评分</div>
            <div class="score-delta" :class="data.delta > 0 ? 'up' : data.delta < 0 ? 'down' : ''">
              {{ data.delta > 0 ? '+' : '' }}{{ data.delta || 0 }} 分
            </div>
          </div>
        </div>
        <div class="panel hero-stats">
          <div class="stat-row" v-for="(v, k) in subScores" :key="k">
            <span class="stat-label">{{ dimensionLabel(k) }}</span>
            <div class="stat-bar"><div class="stat-fill" :style="{width: v+'%'}" :class="barColor(v)"></div></div>
            <span class="stat-val">{{ v }}</span>
          </div>
        </div>
        <div class="panel hero-summary">
          <div class="summary-item"><span class="si-icon">🔴</span><span class="si-num">{{ data.findings?.length || 0 }}</span><span class="si-label">待处理问题</span></div>
          <div class="summary-item"><span class="si-icon">⚡</span><span class="si-num">{{ data.anomalies?.length || 0 }}</span><span class="si-label">异常事件</span></div>
          <div class="summary-item"><span class="si-icon">💡</span><span class="si-num">{{ data.opportunities || 0 }}</span><span class="si-label">优化机会</span></div>
          <div class="summary-item"><span class="si-icon">⚠️</span><span class="si-num">{{ data.cannibalization || 0 }}</span><span class="si-label">自蚕食</span></div>
          <div class="summary-item"><span class="si-icon">💀</span><span class="si-num">{{ data.dead_links || 0 }}</span><span class="si-label">死链</span></div>
        </div>
      </div>

      <!-- ─── Findings (问题列表) ───────────────────────────── -->
      <div class="panel" v-if="data.findings?.length">
        <h3>🔍 待处理问题（按严重度排序）</h3>
        <el-table :data="data.findings" stripe border size="small">
          <el-table-column prop="severity" label="严重度" width="70">
            <template #default="{row}">
              <el-tag :type="sevType(row.severity)" size="small">{{ row.severity }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="module" label="模块" width="100" />
          <el-table-column prop="title" label="问题" min-width="300" show-overflow-tooltip />
          <el-table-column prop="page_url" label="页面" min-width="200" show-overflow-tooltip />
          <el-table-column prop="detected_at" label="发现时间" width="100" :formatter="(_:any,__:any,___:any,r:any) => r.detected_at?.slice(0,10)||'-'" />
        </el-table>
      </div>

      <!-- ─── Anomalies (异常事件) ──────────────────────────── -->
      <div class="panel" v-if="data.anomalies?.length">
        <h3>⚡ 近期异常事件</h3>
        <el-table :data="data.anomalies" stripe border size="small">
          <el-table-column prop="severity" label="严重度" width="70">
            <template #default="{row}"><el-tag :type="sevType(row.severity)" size="small">{{ row.severity }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="metric_name" label="指标" width="100" />
          <el-table-column prop="platform" label="平台" width="65" />
          <el-table-column prop="direction" label="方向" width="55" />
          <el-table-column prop="event_date" label="日期" width="100" />
          <el-table-column label="观测值" width="80" :formatter="(_:any,__:any,___:any,r:any) => r.observed_value?.toFixed?.(1)||'-'" />
          <el-table-column label="期望值" width="80" :formatter="(_:any,__:any,___:any,r:any) => r.expected_value?.toFixed?.(1)||'-'" />
          <el-table-column label="偏差σ" width="65" :formatter="(_:any,__:any,___:any,r:any) => r.deviation_sigma?.toFixed?.(1)||'-'" />
          <el-table-column prop="status" label="状态" width="80" />
        </el-table>
      </div>

      <!-- ─── Timeline (时间线) ─────────────────────────────── -->
      <div class="panel" v-if="data.timeline?.length">
        <h3>📅 事件时间线</h3>
        <div class="timeline">
          <div class="tl-item" v-for="t in data.timeline" :key="t.occurred_at">
            <div class="tl-dot" :class="'tl-' + (t.severity||'info')"></div>
            <div class="tl-content">
              <div class="tl-head">
                <span class="tl-category">{{ t.event_category }}</span>
                <span class="tl-date">{{ t.occurred_at?.slice(0,10) }}</span>
              </div>
              <div class="tl-title">{{ t.title }}</div>
            </div>
          </div>
        </div>
      </div>

      <!-- ─── Reports ───────────────────────────────────────── -->
      <div class="panel">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px">
          <h3>📝 分析报告</h3>
          <el-button type="success" size="small" :loading="generating" @click="generateReport">
            {{ generating ? '生成中...' : '📝 生成月度报告' }}
          </el-button>
        </div>
        <el-table :data="reports" stripe border size="small" v-if="reports.length">
          <el-table-column prop="report_type" label="类型" width="70" />
          <el-table-column prop="period_start" label="开始" width="100" />
          <el-table-column prop="period_end" label="结束" width="100" />
          <el-table-column prop="total_score" label="评分" width="60" />
          <el-table-column prop="status" label="状态" width="80">
            <template #default="{row}"><el-tag :type="row.status==='ready'?'success':row.status==='failed'?'danger':'warning'" size="small">{{ row.status }}</el-tag></template>
          </el-table-column>
          <el-table-column prop="generated_at" label="生成时间" width="140" :formatter="(_:any,__:any,___:any,r:any) => r.generated_at?.slice(0,16)||'-'" />
          <el-table-column label="" width="70">
            <template #default="{row}"><el-button v-if="row.status==='ready'" link size="small" @click="viewReport(row.id)">查看</el-button></template>
          </el-table-column>
        </el-table>
        <p v-else style="color:#999;font-size:13px">暂无报告。点击上方按钮生成第一份分析报告。</p>
      </div>

      <!-- ─── Report viewer dialog ──────────────────────────── -->
      <el-dialog v-model="reportDlg" title="📝 站点分析报告" width="900px" top="3vh">
        <div v-if="reportContent" v-html="renderMD(reportContent.markdown_text)" class="report-md"></div>
      </el-dialog>

      <!-- ─── Audit Info ────────────────────────────────────── -->
      <div class="panel info-row">
        <div class="info-item"><span class="info-label">最近审计</span><span class="info-val">{{ data.last_audit_date?.slice(0,10) || '从未' }}</span></div>
        <div class="info-item"><span class="info-label">审计状态</span><span class="info-val">{{ data.last_audit_status || '-' }}</span></div>
      </div>
    </template>

    <!-- ═══════ 竞品 Tab ═══════ -->
    <div v-if="siteId && tab === 'competitors'" class="panel">
      <div style="display:flex;justify-content:space-between;margin-bottom:12px">
        <h3>🏆 竞品管理</h3>
        <el-button size="small" @click="showAddCompetitor = true">+ 添加竞品</el-button>
      </div>
      <el-table :data="competitorsList" stripe border size="small">
        <el-table-column prop="domain" label="域名" min-width="200" /><el-table-column prop="name" label="名称" width="120" />
        <el-table-column prop="tier" label="层级" width="80" /><el-table-column prop="notes" label="备注" min-width="150" />
        <el-table-column label="" width="60"><template #default="{row}"><el-button link type="danger" size="small" @click="delCompetitor(row.id)">删除</el-button></template></el-table-column>
      </el-table>
      <el-dialog v-model="showAddCompetitor" title="添加竞品" width="500px">
        <el-form label-width="80px">
          <el-form-item label="域名"><el-input v-model="compForm.domain" placeholder="competitor.com" /></el-form-item>
          <el-form-item label="名称"><el-input v-model="compForm.name" /></el-form-item>
          <el-form-item label="层级"><el-select v-model="compForm.tier"><el-option value="direct" label="直接竞争" /><el-option value="indirect" label="间接" /><el-option value="aspirational" label="对标" /></el-select></el-form-item>
          <el-form-item label="备注"><el-input v-model="compForm.notes" type="textarea" /></el-form-item>
        </el-form>
        <template #footer><el-button @click="showAddCompetitor=false">取消</el-button><el-button type="primary" @click="addCompetitor">确定</el-button></template>
      </el-dialog>
    </div>

    <!-- ═══════ SERP观察 Tab ═══════ -->
    <div v-if="siteId && tab === 'serp'" class="panel">
      <div style="display:flex;justify-content:space-between;margin-bottom:12px">
        <h3>🔍 SERP 真实位置观察</h3>
        <el-button size="small" @click="showAddSerp = true">+ 录入观察</el-button>
      </div>
      <el-table :data="serpList" stripe border size="small">
        <el-table-column prop="query" label="关键词" min-width="200" /><el-table-column prop="engine" label="引擎" width="70" />
        <el-table-column prop="our_position" label="我们位置" width="80" /><el-table-column prop="our_url" label="落地页" min-width="200" show-overflow-tooltip />
        <el-table-column prop="observed_at" label="观察时间" width="100" :formatter="(_:any,__:any,___:any,r:any)=>r.observed_at?.slice(0,10)" />
      </el-table>
      <el-dialog v-model="showAddSerp" title="录入 SERP 观察" width="500px">
        <el-form label-width="80px">
          <el-form-item label="关键词"><el-input v-model="serpForm.query" /></el-form-item>
          <el-form-item label="引擎"><el-select v-model="serpForm.engine"><el-option value="google" /><el-option value="bing" /><el-option value="yandex" /></el-select></el-form-item>
          <el-form-item label="我们位置"><el-input-number v-model="serpForm.our_position" :min="0" :max="200" /></el-form-item>
          <el-form-item label="落地页"><el-input v-model="serpForm.our_url" /></el-form-item>
          <el-form-item label="备注"><el-input v-model="serpForm.notes" /></el-form-item>
        </el-form>
        <template #footer><el-button @click="showAddSerp=false">取消</el-button><el-button type="primary" @click="addSerp">确定</el-button></template>
      </el-dialog>
    </div>

    <!-- ═══════ AIO观察 Tab ═══════ -->
    <div v-if="siteId && tab === 'aio'" class="panel">
      <div style="display:flex;justify-content:space-between;margin-bottom:12px">
        <h3>🤖 AI Overview 观察</h3>
        <el-button size="small" @click="showAddAio = true">+ 录入观察</el-button>
      </div>
      <el-table :data="aioList" stripe border size="small">
        <el-table-column prop="query" label="关键词" min-width="200" />
        <el-table-column prop="has_aio" label="有AIO" width="65"><template #default="{row}"><span :style="{color:row.has_aio?'#67c23a':'#999'}">{{ row.has_aio ? '✓' : '✗' }}</span></template></el-table-column>
        <el-table-column prop="our_domain_cited" label="引用我们" width="75"><template #default="{row}"><span :style="{color:row.our_domain_cited?'#67c23a':'#f56c6c'}">{{ row.our_domain_cited ? '✓' : '✗' }}</span></template></el-table-column>
        <el-table-column prop="aio_text" label="AIO内容" min-width="200" show-overflow-tooltip />
        <el-table-column prop="observed_at" label="时间" width="100" :formatter="(_:any,__:any,___:any,r:any)=>r.observed_at?.slice(0,10)" />
      </el-table>
      <el-dialog v-model="showAddAio" title="录入 AIO 观察" width="500px">
        <el-form label-width="90px">
          <el-form-item label="关键词"><el-input v-model="aioForm.query" /></el-form-item>
          <el-form-item label="有AIO"><el-switch v-model="aioForm.has_aio" /></el-form-item>
          <el-form-item label="AIO内容"><el-input v-model="aioForm.aio_text" type="textarea" :rows="3" /></el-form-item>
          <el-form-item label="引用我们"><el-switch v-model="aioForm.our_domain_cited" /></el-form-item>
          <el-form-item label="备注"><el-input v-model="aioForm.notes" /></el-form-item>
        </el-form>
        <template #footer><el-button @click="showAddAio=false">取消</el-button><el-button type="primary" @click="addAio">确定</el-button></template>
      </el-dialog>
    </div>

    <!-- ═══════ 算法更新 Tab ═══════ -->
    <div v-if="tab === 'algorithm'" class="panel">
      <div style="display:flex;justify-content:space-between;margin-bottom:12px">
        <h3>📡 搜索算法更新追踪</h3>
        <el-button size="small" @click="showAddAlgo = true">+ 登记更新</el-button>
      </div>
      <el-table :data="algoList" stripe border size="small">
        <el-table-column prop="engine" label="引擎" width="70" /><el-table-column prop="update_name" label="更新名称" min-width="200" />
        <el-table-column prop="update_type" label="类型" width="100" />
        <el-table-column prop="announced_at" label="公布时间" width="110" :formatter="(_:any,__:any,___:any,r:any)=>r.announced_at?.slice(0,10)||'-'" />
        <el-table-column prop="official" label="官方" width="55"><template #default="{row}">{{ row.official ? '✓' : '' }}</template></el-table-column>
        <el-table-column prop="summary" label="摘要" min-width="200" show-overflow-tooltip />
      </el-table>
      <el-dialog v-model="showAddAlgo" title="登记算法更新" width="500px">
        <el-form label-width="80px">
          <el-form-item label="引擎"><el-select v-model="algoForm.engine"><el-option value="google" /><el-option value="bing" /><el-option value="yandex" /></el-select></el-form-item>
          <el-form-item label="名称"><el-input v-model="algoForm.update_name" placeholder="March 2026 Core Update" /></el-form-item>
          <el-form-item label="类型"><el-select v-model="algoForm.update_type"><el-option value="core" label="核心" /><el-option value="spam" label="垃圾" /><el-option value="helpful_content" label="有用内容" /><el-option value="product_review" label="产品评测" /><el-option value="other" label="其他" /></el-select></el-form-item>
          <el-form-item label="公布日期"><el-date-picker v-model="algoForm.announced_at" type="date" value-format="YYYY-MM-DD" /></el-form-item>
          <el-form-item label="官方"><el-switch v-model="algoForm.official" /></el-form-item>
          <el-form-item label="摘要"><el-input v-model="algoForm.summary" type="textarea" /></el-form-item>
        </el-form>
        <template #footer><el-button @click="showAddAlgo=false">取消</el-button><el-button type="primary" @click="addAlgo">确定</el-button></template>
      </el-dialog>
    </div>

    <!-- ═══════ 时间线 Tab (full) ═══════ -->
    <div v-if="siteId && tab === 'timeline'" class="panel">
      <div style="display:flex;justify-content:space-between;margin-bottom:12px">
        <h3>📅 事件时间线（完整）</h3>
        <el-button size="small" @click="showAddEvent = true">+ 添加事件</el-button>
      </div>
      <el-table :data="timelineList" stripe border size="small">
        <el-table-column prop="event_category" label="类别" width="80" /><el-table-column prop="event_type" label="类型" width="120" />
        <el-table-column prop="title" label="标题" min-width="250" /><el-table-column prop="severity" label="严重度" width="70" />
        <el-table-column prop="occurred_at" label="时间" width="110" :formatter="(_:any,__:any,___:any,r:any)=>r.occurred_at?.slice(0,10)" />
        <el-table-column prop="source_type" label="来源" width="60" />
      </el-table>
      <el-dialog v-model="showAddEvent" title="添加时间线事件" width="500px">
        <el-form label-width="80px">
          <el-form-item label="类别"><el-select v-model="eventForm.event_category"><el-option value="deploy" label="部署" /><el-option value="infra" label="基础设施" /><el-option value="content" label="内容" /><el-option value="external" label="外部" /><el-option value="manual" label="手动" /></el-select></el-form-item>
          <el-form-item label="类型"><el-input v-model="eventForm.event_type" placeholder="如: backend_deploy" /></el-form-item>
          <el-form-item label="标题"><el-input v-model="eventForm.title" /></el-form-item>
          <el-form-item label="严重度"><el-select v-model="eventForm.severity"><el-option value="info" /><el-option value="medium" /><el-option value="high" /><el-option value="critical" /></el-select></el-form-item>
          <el-form-item label="日期"><el-date-picker v-model="eventForm.occurred_at" type="date" value-format="YYYY-MM-DD" /></el-form-item>
        </el-form>
        <template #footer><el-button @click="showAddEvent=false">取消</el-button><el-button type="primary" @click="addEvent">确定</el-button></template>
      </el-dialog>
    </div>

    <div v-if="loading" class="empty-state">加载中...</div>
    <div v-if="error" class="empty-state" style="color:#f56c6c">{{ error }}</div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const sites = ref<any[]>([])
const siteId = ref<number | null>(null)
const data = ref<any>(null)
const loading = ref(false)
const computing = ref(false)
const analyzing = ref(false)
const error = ref('')

const subScores = computed(() => {
  if (!data.value?.sub_scores) return {}
  const s = typeof data.value.sub_scores === 'string' ? JSON.parse(data.value.sub_scores) : data.value.sub_scores
  return s || {}
})

const dimensionLabel = (k: string) => ({
  traffic: '流量', ranking: '排名', ctr: 'CTR', technical: '技术',
  indexation: '收录', content: '内容', backlink: '外链', security: '安全',
  ux: '用户体验', stability: '稳定性',
}[k] || k)

const barColor = (v: number) => v >= 80 ? 'bar-green' : v >= 60 ? 'bar-yellow' : v >= 40 ? 'bar-orange' : 'bar-red'

const sevType = (s: string): any => {
  if (s === '致命' || s === 'critical') return 'danger'
  if (s === '严重' || s === 'high') return 'warning'
  return 'info'
}

const tab = ref('overview')
const tabs = [
  { key: 'overview', label: '总览', icon: '📊' },
  { key: 'competitors', label: '竞品', icon: '🏆' },
  { key: 'serp', label: 'SERP观察', icon: '🔍' },
  { key: 'aio', label: 'AIO观察', icon: '🤖' },
  { key: 'algorithm', label: '算法更新', icon: '📡' },
  { key: 'timeline', label: '时间线', icon: '📅' },
  { key: 'reports', label: '报告', icon: '📝' },
]

// Competitors
const competitorsList = ref<any[]>([])
const showAddCompetitor = ref(false)
const compForm = ref({ domain: '', name: '', tier: 'direct', notes: '' })

// SERP
const serpList = ref<any[]>([])
const showAddSerp = ref(false)
const serpForm = ref({ query: '', engine: 'google', our_position: 0, our_url: '', notes: '' })

// AIO
const aioList = ref<any[]>([])
const showAddAio = ref(false)
const aioForm = ref({ query: '', has_aio: false, aio_text: '', our_domain_cited: false, notes: '' })

// Algorithm updates
const algoList = ref<any[]>([])
const showAddAlgo = ref(false)
const algoForm = ref({ engine: 'google', update_name: '', update_type: 'core', announced_at: '', official: false, summary: '' })

// Timeline
const timelineList = ref<any[]>([])
const showAddEvent = ref(false)
const eventForm = ref({ event_category: 'manual', event_type: '', title: '', severity: 'info', occurred_at: '' })

const reports = ref<any[]>([])
const generating = ref(false)
const reportDlg = ref(false)
const reportContent = ref<any>(null)

// Simple markdown to HTML (basic)
function renderMD(md: string): string {
  if (!md) return ''
  return md
    .replace(/^### (.*$)/gm, '<h3>$1</h3>')
    .replace(/^## (.*$)/gm, '<h2>$1</h2>')
    .replace(/^# (.*$)/gm, '<h1>$1</h1>')
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.*?)\*/g, '<em>$1</em>')
    .replace(/^- (.*$)/gm, '<li>$1</li>')
    .replace(/(<li>.*<\/li>)/gs, '<ul>$1</ul>')
    .replace(/\n\n/g, '<br><br>')
    .replace(/\n/g, '<br>')
}

async function generateReport() {
  if (!siteId.value) return
  generating.value = true
  try {
    await api.generateReport(siteId.value, 'monthly')
    ElMessage.success('报告生成中，请稍后刷新查看')
    setTimeout(loadReports, 5000)
  } catch (e: any) { ElMessage.error(e?.message || '生成失败') }
  finally { generating.value = false }
}

async function loadReports() {
  if (!siteId.value) return
  try { const r: any = await api.listSiteReports(siteId.value); reports.value = r.data || [] } catch {}
}

async function viewReport(id: number) {
  try { const r: any = await api.getSiteReport(id); reportContent.value = r.data; reportDlg.value = true } catch {}
}

async function loadTabData() {
  if (!siteId.value) return
  const s = siteId.value
  try {
    if (tab.value === 'competitors') { const r: any = await api.listCompetitors(s); competitorsList.value = r.data || [] }
    else if (tab.value === 'serp') { const r: any = await api.listSERPObservations(s); serpList.value = r.data || [] }
    else if (tab.value === 'aio') { const r: any = await api.listAIOObservations(s); aioList.value = r.data || [] }
    else if (tab.value === 'algorithm') { const r: any = await api.listAlgorithmUpdates(); algoList.value = r.data || [] }
    else if (tab.value === 'timeline') { const r: any = await api.listTimelineEvents(s); timelineList.value = r.data || [] }
    else if (tab.value === 'reports') { loadReports() }
  } catch {}
}

async function addCompetitor() {
  if (!siteId.value || !compForm.value.domain) return
  await api.createCompetitor(siteId.value, compForm.value)
  showAddCompetitor.value = false; compForm.value = { domain: '', name: '', tier: 'direct', notes: '' }
  ElMessage.success('已添加'); loadTabData()
}
async function delCompetitor(id: number) { await api.deleteCompetitor(id); loadTabData() }

async function addSerp() {
  if (!siteId.value || !serpForm.value.query) return
  await api.createSERPObservation(siteId.value, serpForm.value)
  showAddSerp.value = false; serpForm.value = { query: '', engine: 'google', our_position: 0, our_url: '', notes: '' }
  ElMessage.success('已录入'); loadTabData()
}

async function addAio() {
  if (!siteId.value || !aioForm.value.query) return
  await api.createAIOObservation(siteId.value, aioForm.value)
  showAddAio.value = false; aioForm.value = { query: '', has_aio: false, aio_text: '', our_domain_cited: false, notes: '' }
  ElMessage.success('已录入'); loadTabData()
}

async function addAlgo() {
  if (!algoForm.value.update_name) return
  await api.createAlgorithmUpdate(algoForm.value)
  showAddAlgo.value = false; algoForm.value = { engine: 'google', update_name: '', update_type: 'core', announced_at: '', official: false, summary: '' }
  ElMessage.success('已登记'); loadTabData()
}

async function addEvent() {
  if (!siteId.value || !eventForm.value.title) return
  await api.createTimelineEvent(siteId.value, eventForm.value)
  showAddEvent.value = false; eventForm.value = { event_category: 'manual', event_type: '', title: '', severity: 'info', occurred_at: '' }
  ElMessage.success('已添加'); loadTabData()
}

onMounted(async () => {
  try { const r: any = await api.listSites(); sites.value = r.data || [] } catch {}
})

async function loadAnalysis() {
  if (!siteId.value) return
  loading.value = true; error.value = ''; data.value = null
  try {
    const r: any = await api.getSiteAnalysisSummary(siteId.value)
    data.value = r.data || {}
    // If no health score, compute one
    if (!data.value.health_score) {
      await api.computeHealthScore(siteId.value)
      const r2: any = await api.getSiteAnalysisSummary(siteId.value)
      data.value = r2.data || {}
    }
  } catch (e: any) { error.value = e?.message || '加载失败' }
  finally { loading.value = false }
  loadReports()
}

async function runPipeline() {
  if (!siteId.value) return
  analyzing.value = true
  try {
    await api.runAnalysisPipeline(siteId.value)
    ElMessage.success('分析管道已启动，3秒后刷新...')
    setTimeout(() => loadAnalysis(), 3000)
  } catch (e: any) { ElMessage.error(e?.message || '启动失败') }
  finally { analyzing.value = false }
}

async function computeScore() {
  if (!siteId.value) return
  computing.value = true
  try {
    await api.computeHealthScore(siteId.value)
    ElMessage.success('健康分已重新计算')
    loadAnalysis()
  } catch (e: any) { ElMessage.error(e?.message || '计算失败') }
  finally { computing.value = false }
}
</script>

<style scoped>
.site-analysis { padding: 20px 24px; min-height: calc(100vh - 60px); }
.panel { background: var(--surface); border-radius: var(--r); box-shadow: var(--s1); border: 1.5px solid var(--border-light); padding: 16px 20px; margin-bottom: 14px; }
.toolbar { margin-bottom: 16px; }
.tb-row { display: flex; align-items: center; gap: 12px; }
.empty-state { text-align: center; padding: 60px 20px; color: #999; font-size: 15px; }
h3 { margin: 0 0 12px; font-size: 15px; font-weight: 700; }

/* Hero */
.hero-row { display: grid; grid-template-columns: 240px 1fr 200px; gap: 14px; margin-bottom: 14px; }
@media (max-width: 1100px) { .hero-row { grid-template-columns: 1fr; } }
.hero-score { display: flex; align-items: center; gap: 16px; padding: 24px; }
.score-ring { width: 100px; height: 100px; border-radius: 50%; display: flex; flex-direction: column; align-items: center; justify-content: center; border: 4px solid #e0e0e0; }
.grade-s { border-color: #10B981; background: #ECFDF5; } .grade-a { border-color: #3B82F6; background: #EFF6FF; }
.grade-b { border-color: #F59E0B; background: #FFFBEB; } .grade-c { border-color: #F97316; background: #FFF7ED; }
.grade-d { border-color: #EF4444; background: #FEF2F2; } .grade-f { border-color: #991B1B; background: #FEE2E2; }
.score-num { font-size: 28px; font-weight: 800; line-height: 1; }
.score-grade { font-size: 14px; font-weight: 700; color: #666; }
.score-meta { }
.score-label { font-size: 13px; color: #999; margin-bottom: 4px; }
.score-delta { font-size: 16px; font-weight: 700; }
.score-delta.up { color: #10B981; } .score-delta.down { color: #EF4444; }

/* Sub-scores */
.hero-stats { padding: 12px 16px; }
.stat-row { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.stat-label { font-size: 11px; width: 50px; color: #888; text-align: right; }
.stat-bar { flex: 1; height: 8px; background: #f0f0f0; border-radius: 4px; overflow: hidden; }
.stat-fill { height: 100%; border-radius: 4px; transition: width .3s; }
.bar-green { background: #10B981; } .bar-yellow { background: #F59E0B; }
.bar-orange { background: #F97316; } .bar-red { background: #EF4444; }
.stat-val { font-size: 11px; font-weight: 700; width: 24px; text-align: right; }

/* Summary */
.hero-summary { display: flex; flex-direction: column; gap: 8px; padding: 12px 16px; }
.summary-item { display: flex; align-items: center; gap: 8px; }
.si-icon { font-size: 16px; } .si-num { font-size: 18px; font-weight: 800; width: 30px; }
.si-label { font-size: 12px; color: #888; }

/* Timeline */
.timeline { max-height: 400px; overflow-y: auto; }
.tl-item { display: flex; gap: 12px; padding: 8px 0; border-bottom: 1px solid #f5f5f5; }
.tl-dot { width: 10px; height: 10px; border-radius: 50%; margin-top: 4px; flex-shrink: 0; background: #ccc; }
.tl-info { background: #93C5FD; } .tl-medium { background: #FBBF24; }
.tl-high { background: #F97316; } .tl-critical { background: #EF4444; }
.tl-head { display: flex; gap: 8px; font-size: 11px; color: #999; margin-bottom: 2px; }
.tl-category { background: #f0f0f0; padding: 1px 6px; border-radius: 4px; }
.tl-title { font-size: 13px; }

/* Info */
.info-row { display: flex; gap: 30px; }
.info-item { }
.info-label { font-size: 11px; color: #999; margin-right: 6px; }
.info-val { font-size: 13px; font-weight: 600; }
.sub-bar { padding: 10px 16px; display: flex; flex-wrap: wrap; gap: 4px; }
.sub-pill { padding: 6px 14px; border: 1px solid #eee; border-radius: 14px; cursor: pointer; background: #fafafa; font-size: 12px; transition: all .15s; display: flex; align-items: center; gap: 3px; }
.sub-pill:hover { background: #fff3e0; border-color: #ffcc80; }
.sub-pill.active { background: var(--sun, #ff8c00); color: #fff; border-color: var(--sun, #ff8c00); }
.sp-i { font-size: 13px; }
.report-md { font-size: 14px; line-height: 1.8; }
.report-md h1 { font-size: 20px; margin: 16px 0 8px; border-bottom: 2px solid #eee; padding-bottom: 6px; }
.report-md h2 { font-size: 17px; margin: 14px 0 6px; color: #333; }
.report-md h3 { font-size: 15px; margin: 10px 0 4px; }
.report-md ul { padding-left: 20px; margin: 6px 0; }
.report-md li { margin: 3px 0; }
.report-md strong { color: #e6a23c; }
</style>
