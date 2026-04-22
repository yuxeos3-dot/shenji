<template>
  <div class="site-analysis">
    <!-- Site selector -->
    <div class="panel toolbar">
      <div class="tb-row">
        <label>选择站点：</label>
        <el-select v-model="siteId" filterable placeholder="选择站点" @change="loadAnalysis" style="width:300px">
          <el-option v-for="s in sites" :key="s.id" :label="`${s.name} (${s.domain})`" :value="s.id" />
        </el-select>
        <el-button type="primary" :loading="computing" @click="computeScore" style="margin-left:auto">
          {{ computing ? '计算中...' : '🔄 重新计算健康分' }}
        </el-button>
      </div>
    </div>

    <div v-if="!siteId" class="empty-state">📊 选择一个站点开始全站 SEO 分析</div>

    <template v-if="siteId && data">
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

      <!-- ─── Audit Info ────────────────────────────────────── -->
      <div class="panel info-row">
        <div class="info-item"><span class="info-label">最近审计</span><span class="info-val">{{ data.last_audit_date?.slice(0,10) || '从未' }}</span></div>
        <div class="info-item"><span class="info-label">审计状态</span><span class="info-val">{{ data.last_audit_status || '-' }}</span></div>
      </div>
    </template>

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
</style>
