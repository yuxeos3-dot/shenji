<template>
  <div class="analytics-page">
    <!-- Hero intro card -->
    <div class="hero-card">
      <div class="hero-head">
        <span class="hero-icon">🧠</span>
        <div>
          <div class="hero-title">全局数据智能分析 · Strategy Intelligence Engine</div>
          <div class="hero-sub">
            聚合 GSC / GA4 / Bing / Yandex 四大平台 + 全站审计 + 蜘蛛日志 + 内容产出 + 外链 + 协同任务的全量数据，
            通过 AI 大模型生成<strong>战略级运营建议</strong>——不是告诉你"H1 标签缺失"，而是告诉你<strong>"下个月该主攻哪些关键词、内容节奏怎么调、跟竞品的差异化切入点在哪"</strong>。
            每一条建议都附带具体数字和行动路径，让 SEO 从工具驱动升级为<strong>数据驱动的战略决策平台</strong>。
          </div>
        </div>
      </div>
      <div class="hero-toggle" @click="introExpanded=!introExpanded">
        <span>📋 分析引擎覆盖维度（{{ dataDimensions.length }} 大数据源）</span>
        <span class="hero-toggle-icon">{{ introExpanded ? '▲ 收起' : '▼ 展开查看' }}</span>
      </div>
      <div v-show="introExpanded" class="hero-text">
        <strong>已接入数据源</strong>：<br>
        ① GSC 搜索数据（点击 / 曝光 / CTR / 平均排位 / 关键词 / 页面）<br>
        ② GA4 用户数据（会话 / 用户 / PV / 渠道）<br>
        ③ Bing 索引数据 + Yandex 索引数据<br>
        ④ 蜘蛛爬取日志（Google / Bing / Baidu / Yandex 等主流蜘蛛 30 天活动）<br>
        ⑤ 站点审计报告（28 大类 200+ 检查点，技术 + SEO + 安全 + 性能）<br>
        ⑥ 内容生产流水线（GEO 文章 / 二创 / 关键词库 / 审核状态）<br>
        ⑦ 协同任务工作流（待分配 / 开发中 / 待验收 / 已完成）<br>
        ⑧ 外链管理（站点外链 + 公共外链池）<br>
        ⑨ 死链检测（影响站点数 / 总死链数）<br>
        ⑩ 推送日志（Sitemap / URL 提交成功率）<br><br>
        <strong>即将接入（数据对接中）</strong>：<br>
        🔜 同行竞品站点深度抓取 · 页面停留时长 · 视频完播率 · 关键词排名追踪 · 外链增速分析
      </div>
      <div class="hero-footer">
        <span class="hero-tag">📊 {{ dataDimensions.length }} 大数据源</span>
        <span class="hero-tag">🤖 AI 战略分析</span>
        <span class="hero-tag">🎯 关键词机会挖掘</span>
        <span class="hero-tag">📈 内容节奏指导</span>
        <span class="hero-tag">🏆 竞品差异化</span>
        <span class="hero-tag">🗓 3 月路线图</span>
      </div>
    </div>

    <!-- Aggregated metrics grid -->
    <div class="metrics-grid" v-loading="loading">
      <div class="metric-card" v-for="m in metrics" :key="m.label">
        <div class="mc-icon">{{ m.icon }}</div>
        <div class="mc-val" :class="m.cls">{{ m.value }}</div>
        <div class="mc-label">{{ m.label }}</div>
        <div class="mc-sub" v-if="m.sub">{{ m.sub }}</div>
      </div>
    </div>

    <!-- Detail sections -->
    <div class="section-grid" v-if="snap">
      <!-- GSC Top Keywords -->
      <div class="panel section-card" v-if="snap.gsc?.top_keywords?.length">
        <div class="sc-title">🔑 热门关键词 TOP {{ snap.gsc.top_keywords.length }}</div>
        <table class="mini-table">
          <thead><tr><th>关键词</th><th>点击</th><th>曝光</th></tr></thead>
          <tbody>
            <tr v-for="kw in snap.gsc.top_keywords" :key="kw.query">
              <td class="kw-text">{{ kw.query }}</td>
              <td class="num">{{ kw.clicks }}</td>
              <td class="num">{{ kw.impressions }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <!-- Spider Activity -->
      <div class="panel section-card" v-if="snap.spider?.top_spiders?.length">
        <div class="sc-title">🕷️ 蜘蛛活动 TOP 10（近 30 天）</div>
        <table class="mini-table">
          <thead><tr><th>蜘蛛</th><th>抓取次数</th></tr></thead>
          <tbody>
            <tr v-for="sp in snap.spider.top_spiders" :key="sp.spider">
              <td>{{ sp.spider }}</td>
              <td class="num">{{ sp.hits }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <!-- Coord Tasks -->
      <div class="panel section-card">
        <div class="sc-title">🤝 协同任务分布</div>
        <div class="coord-bars">
          <div v-for="(v, k) in snap.coord_tasks" :key="k" class="coord-item">
            <span class="ci-label">{{ coordLabel(k) }}</span>
            <span class="ci-val">{{ v }}</span>
          </div>
        </div>
      </div>
      <!-- Pending Data Sources -->
      <div class="panel section-card" v-if="snap.pending_sources?.length">
        <div class="sc-title">🔜 即将接入的数据维度</div>
        <div class="pending-list">
          <span v-for="s in snap.pending_sources" :key="s" class="pending-chip">{{ pendingLabel(s) }}</span>
        </div>
      </div>
    </div>

    <!-- AI Strategy button + result -->
    <div class="panel ai-panel">
      <div class="ai-header">
        <div>
          <div class="ai-title">🧠 AI 战略分析报告</div>
          <div class="ai-sub">基于上方全量数据，调用 AI 生成 6 大章节的战略指导（关键词机会 · 内容节奏 · 竞品切入 · 3 月路线图）</div>
        </div>
        <button class="btn btn-ai" @click="runAdvise" :disabled="advising">
          {{ advising ? '分析中，请稍候...' : '🚀 运行 AI 战略分析' }}
        </button>
      </div>
      <div v-if="adviseError" class="ai-error">❌ {{ adviseError }}</div>
      <div v-if="adviceHTML" class="ai-result" v-html="adviceHTML"></div>
      <div v-if="!adviceHTML && !advising && !adviseError" class="ai-placeholder">
        点击上方按钮运行分析。AI 将根据当前全量数据生成战略建议，通常耗时 30-60 秒。
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const loading = ref(false)
const snap = ref(null)
const advising = ref(false)
const adviceHTML = ref('')
const adviseError = ref('')
const introExpanded = ref(false)

const dataDimensions = [
  'GSC 搜索数据', 'GA4 用户数据', 'Bing 索引', 'Yandex 索引',
  '蜘蛛爬取日志', '站点审计报告', '内容生产', '协同任务',
  '外链管理', '死链检测', '推送日志', '用户活跃度',
]

const metrics = computed(() => {
  const s = snap.value
  if (!s) return []
  return [
    { icon: '🌐', label: '站点总数', value: s.sites?.total ?? 0, sub: `${s.sites?.active ?? 0} 启用 · ${s.sites?.audited ?? 0} 已审计`, cls: 'val-sun' },
    { icon: '🔍', label: 'GSC 总点击', value: (s.gsc?.total_clicks ?? 0).toLocaleString(), sub: `曝光 ${(s.gsc?.total_impressions ?? 0).toLocaleString()}`, cls: 'val-ok' },
    { icon: '👥', label: 'GA 总用户', value: (s.ga?.total_users ?? 0).toLocaleString(), sub: `会话 ${(s.ga?.total_sessions ?? 0).toLocaleString()}`, cls: 'val-ok' },
    { icon: '🕷️', label: '蜘蛛 30 天', value: (s.spider?.hits_30d ?? 0).toLocaleString(), sub: `7 天 ${(s.spider?.hits_7d ?? 0).toLocaleString()}`, cls: 'val-sun' },
    { icon: '📝', label: '内容产出', value: s.content?.total ?? 0, sub: `已发布 ${s.content?.published ?? 0} · 草稿 ${s.content?.draft ?? 0}`, cls: 'val-ok' },
    { icon: '🤝', label: '协同任务', value: Object.values(s.coord_tasks || {}).reduce((a, b) => a + b, 0), sub: `待分配 ${s.coord_tasks?.pending ?? 0} · 已完成 ${s.coord_tasks?.done ?? 0}`, cls: 'val-sun' },
    { icon: '🔗', label: '外链池', value: s.extlinks?.pool_size ?? 0, sub: `站点外链 ${s.extlinks?.site_extlinks ?? 0}`, cls: 'val-ok' },
    { icon: '💀', label: '死链', value: s.dead_links?.total ?? 0, sub: `影响 ${s.dead_links?.affected_sites ?? 0} 个站点`, cls: s.dead_links?.total > 0 ? 'val-warn' : 'val-ok' },
    { icon: '📤', label: '推送成功率', value: `${(s.push?.success_rate ?? 0).toFixed(1)}%`, sub: `30 天推送 ${s.push?.total_30d ?? 0} 次`, cls: (s.push?.success_rate ?? 100) < 80 ? 'val-warn' : 'val-ok' },
    { icon: '📊', label: 'Bing 索引', value: (s.index?.bing ?? 0).toLocaleString(), sub: `Yandex ${(s.index?.yandex ?? 0).toLocaleString()}`, cls: 'val-ok' },
    { icon: '🔐', label: '平台绑定', value: Object.values(s.platforms || {}).reduce((a, b) => a + b, 0) + ' 个', sub: `GSC ${s.platforms?.gsc ?? 0} GA ${s.platforms?.ga ?? 0} Bing ${s.platforms?.bing ?? 0}`, cls: 'val-sun' },
    { icon: '👤', label: '活跃用户', value: s.users?.active_7d ?? 0, sub: `总用户 ${s.users?.total ?? 0}`, cls: 'val-ok' },
  ]
})

function coordLabel(k) {
  return { pending: '📋 待分配', in_progress: '🛠 开发中', awaiting_answer: '❓ 待回复', under_review: '🔍 待验收', done: '✅ 已完成' }[k] || k
}
function pendingLabel(s) {
  return {
    competitor_crawl: '🏆 同行竞品抓取',
    dwell_time: '⏱ 页面停留时长',
    completion_rate: '📺 视频完播率',
    serp_rank_tracking: '📈 关键词排名追踪',
    backlink_velocity: '🔗 外链增速分析',
  }[s] || s
}

// Simple markdown to HTML — XSS-safe because we escape &, <, > first. — handles ## headers, **bold**, bullet lists,
// line breaks. Good enough for the AI's typical response format.
function md(text) {
  if (!text) return ''
  return text
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/^## (.+)$/gm, '<h2>$1</h2>')
    .replace(/^### (.+)$/gm, '<h3>$1</h3>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/^[-*] (.+)$/gm, '<li>$1</li>')
    .replace(/(<li>.*<\/li>\n?)+/g, (m) => '<ul>' + m + '</ul>')
    .replace(/\n{2,}/g, '<br><br>')
    .replace(/\n/g, '<br>')
}

async function loadSnapshot() {
  loading.value = true
  try {
    const res = await api.analyticsSnapshot()
    snap.value = res.data
  } catch (e) {
    console.error('加载快照失败', e)
  } finally {
    loading.value = false
  }
}

async function runAdvise() {
  advising.value = true
  adviseError.value = ''
  adviceHTML.value = ''
  try {
    const res = await api.analyticsAdvise()
    adviceHTML.value = md(res.data?.advice || '（AI 未返回内容）')
  } catch (e) {
    adviseError.value = e?.response?.data?.error || e?.message || '分析失败'
  } finally {
    advising.value = false
  }
}

onMounted(loadSnapshot)
</script>

<style scoped>
.analytics-page { display:flex; flex-direction:column; gap:20px; }

/* Hero intro card */
.hero-card { background:linear-gradient(135deg,#FFFDF8 0%,#FFF8EC 50%,#FFF2D8 100%); border:1.5px solid var(--peach); border-radius:var(--r); box-shadow:var(--s2); padding:22px 26px; }
.hero-head { display:flex; align-items:flex-start; gap:14px; margin-bottom:16px; padding-bottom:14px; border-bottom:1.5px solid var(--peach); }
.hero-icon { font-size:40px; flex-shrink:0; }
.hero-title { font-size:18px; font-weight:800; color:var(--ink); letter-spacing:-.3px; margin-bottom:6px; }
.hero-sub { font-size:13px; color:var(--ink-soft); line-height:1.8; max-width:900px; }
.hero-sub strong { color:var(--ink); }
.hero-toggle { display:flex; justify-content:space-between; align-items:center; cursor:pointer; padding:8px 12px; border-radius:var(--r-sm); background:rgba(255,255,255,.6); border:1px solid var(--peach); margin-bottom:10px; font-size:13px; font-weight:700; color:var(--sun-deep); user-select:none; }
.hero-toggle:hover { background:#fff; }
.hero-toggle-icon { font-size:12px; color:var(--ink-soft); }
.hero-text { font-size:13px; color:var(--ink-mid); line-height:1.85; margin-bottom:14px; }
.hero-text strong { color:var(--ink); font-weight:700; }
.hero-footer { display:flex; flex-wrap:wrap; gap:8px; padding-top:12px; border-top:1.5px solid var(--peach); }
.hero-tag { display:inline-flex; align-items:center; gap:5px; padding:5px 12px; background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-radius:20px; font-size:12px; font-weight:700; box-shadow:0 2px 8px rgba(249,115,22,.25); }

/* Metrics grid */
.metrics-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(180px,1fr)); gap:14px; }
.metric-card { background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r); padding:18px 16px; text-align:center; box-shadow:var(--s1); }
.mc-icon { font-size:28px; margin-bottom:6px; }
.mc-val { font-size:26px; font-weight:800; }
.mc-label { font-size:13px; font-weight:700; color:var(--ink-mid); margin-top:4px; }
.mc-sub { font-size:11px; color:var(--ink-soft); margin-top:4px; }
.val-sun { color:var(--sun-deep); }
.val-ok { color:#10B981; }
.val-warn { color:#EF4444; }

/* Section grid */
.section-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(340px,1fr)); gap:16px; }
.panel { background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r); box-shadow:var(--s1); }
.section-card { padding:18px 20px; }
.sc-title { font-size:15px; font-weight:800; color:var(--ink); margin-bottom:12px; }
.mini-table { width:100%; border-collapse:collapse; font-size:13px; }
.mini-table th { text-align:left; font-weight:700; color:var(--ink-mid); padding:6px 8px; border-bottom:1.5px solid var(--border-light); font-size:12px; }
.mini-table td { padding:5px 8px; border-bottom:1px solid var(--border-light); color:var(--ink-mid); }
.mini-table .num { text-align:right; font-family:'JetBrains Mono',monospace; font-weight:600; }
.kw-text { max-width:200px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-weight:600; color:var(--ink); }
.coord-bars { display:flex; flex-direction:column; gap:8px; }
.coord-item { display:flex; justify-content:space-between; align-items:center; padding:6px 10px; background:var(--surface-2); border-radius:var(--r-sm); }
.ci-label { font-size:13px; font-weight:600; }
.ci-val { font-size:16px; font-weight:800; color:var(--sun-deep); }
.pending-list { display:flex; flex-wrap:wrap; gap:8px; }
.pending-chip { padding:6px 14px; background:var(--surface-2); border:1px dashed var(--peach); border-radius:20px; font-size:12px; font-weight:600; color:var(--ink-mid); }

/* AI panel */
.ai-panel { padding:22px 26px; }
.ai-header { display:flex; justify-content:space-between; align-items:center; margin-bottom:16px; gap:20px; flex-wrap:wrap; }
.ai-title { font-size:18px; font-weight:800; color:var(--ink); }
.ai-sub { font-size:13px; color:var(--ink-soft); line-height:1.6; margin-top:4px; max-width:600px; }
.btn-ai { padding:14px 28px; background:linear-gradient(135deg,#8B5CF6,#7C3AED); color:#fff; font-weight:800; font-size:15px; border:none; border-radius:10px; cursor:pointer; transition:all .2s; box-shadow:0 4px 16px rgba(139,92,246,.3); white-space:nowrap; }
.btn-ai:hover:not(:disabled) { transform:translateY(-2px); box-shadow:0 8px 24px rgba(139,92,246,.45); }
.btn-ai:disabled { opacity:.6; cursor:not-allowed; }
.ai-error { background:#FEF2F2; border:1px solid #FECACA; padding:12px 16px; border-radius:var(--r-sm); color:#DC2626; font-size:13px; margin-bottom:16px; }
.ai-placeholder { text-align:center; color:var(--ink-hint); padding:40px 20px; font-size:14px; }
.ai-result { font-size:14px; line-height:1.85; color:var(--ink-mid); }
.ai-result :deep(h2) { font-size:17px; font-weight:800; color:var(--ink); margin:24px 0 12px; padding-bottom:8px; border-bottom:2px solid var(--peach); }
.ai-result :deep(h3) { font-size:15px; font-weight:700; color:var(--sun-deep); margin:16px 0 8px; }
.ai-result :deep(strong) { color:var(--ink); font-weight:700; }
.ai-result :deep(ul) { padding-left:20px; margin:8px 0; }
.ai-result :deep(li) { margin-bottom:4px; }
</style>
