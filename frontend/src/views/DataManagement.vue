<template>
  <div class="data-mgmt">
    <!-- ─── Top toolbar: platform tabs + date range ─────────────────── -->
    <div class="panel toolbar">
      <div class="tb-row">
        <div class="tb-tabs">
          <button
            v-for="p in platforms"
            :key="p.code"
            :class="['pill', platform === p.code && 'active']"
            @click="switchPlatform(p.code)"
          >{{ p.label }}</button>
        </div>

        <div class="tb-date">
          <button
            v-for="preset in presets"
            :key="preset.key"
            :class="['pill pill-sm', activePreset === preset.key && 'active']"
            @click="applyPreset(preset.key)"
          >{{ preset.label }}</button>
          <el-date-picker
            v-model="dateRange"
            type="daterange"
            range-separator="~"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            format="YYYY-MM-DD"
            value-format="YYYY-MM-DD"
            size="default"
            :disabled-date="disabledDate"
            @change="onDateChange"
          />
          <button class="btn btn-refresh" @click="unifiedRefresh" :disabled="loading || syncing">
            {{ syncing ? '同步中...' : loading ? '加载中...' : '🔄 刷新同步' }}
          </button>
        </div>
      </div>
      <div class="tb-meta" v-if="dateRange[0] && dateRange[1]">
        <span class="meta-item">📅 {{ dateRange[0] }} → {{ dateRange[1] }}</span>
        <span class="meta-item" v-if="dayCount === 1">单日数据</span>
        <span class="meta-item" v-else>共 {{ dayCount }} 天</span>
        <span class="meta-item meta-warn" v-if="hasLagWarning">
          ⚠ 所选日期可能因平台数据延迟暂无数据（GSC/GA4 通常延迟 1-2 天）
        </span>
      </div>
    </div>

    <!-- ─── Overview state: card grid ───────────────────────────────── -->
    <div v-if="view === 'overview'" class="view-overview">
      <div class="section-title">
        {{ platformLabel }} · 总览
        <span class="site-count-badge" v-if="overview.cards.length">{{ overview.cards.length }} 个站点</span>
      </div>

      <div v-if="loading && !overview.cards.length" class="state-placeholder">加载中...</div>
      <div v-else-if="error" class="state-error">
        <div class="err-title">❌ {{ error }}</div>
        <div class="err-hint">如果还没授权该平台，请先到「系统管理 → 数据平台」完成授权</div>
      </div>
      <div v-else-if="!overview.cards.length" class="state-empty">
        暂无站点 — 请确认账号下已有站点 / Property，且完成验证
      </div>

      <div v-else class="card-grid">
        <div
          v-for="(c, idx) in overview.cards"
          :key="idx"
          :class="['site-card', !c.has_data && 'site-card-empty', c.error && 'site-card-err']"
          @click="openDetail(c)"
        >
          <button
            class="card-hide-btn"
            title="从数据总览隐藏该站点"
            @click.stop="hideCard(c)"
          >×</button>
          <div class="card-head">
            <div class="card-title">{{ c.display_name || c.site_url || c.host_id || c.property_id }}</div>
            <span v-if="c.verified === false" class="card-tag tag-warn">未验证</span>
            <span v-else-if="c.error" class="card-tag tag-err">错误</span>
            <span v-else-if="!c.has_data" class="card-tag tag-muted">无数据</span>
          </div>
          <div v-if="c.error" class="card-err-msg">{{ c.error }}</div>
          <template v-else>
            <div class="card-sub">{{ cardSubtitle(c) }}</div>
            <div class="card-kpis" v-if="platform !== 'ga4'">
              <div class="kpi">
                <div class="kpi-label">点击</div>
                <div class="kpi-value sv-blue">{{ fmt(c.clicks) }}</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">展示</div>
                <div class="kpi-value sv-green">{{ fmt(c.impressions || c.shows) }}</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">CTR</div>
                <div class="kpi-value sv-orange">{{ ((c.ctr || 0) * 100).toFixed(2) }}%</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">平均排名</div>
                <div class="kpi-value sv-purple">{{ (c.position || c.avg_position || 0).toFixed(1) }}</div>
              </div>
            </div>
            <div class="card-kpis" v-else>
              <div class="kpi">
                <div class="kpi-label">会话</div>
                <div class="kpi-value sv-blue">{{ fmt(c.sessions) }}</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">用户</div>
                <div class="kpi-value sv-green">{{ fmt(c.total_users) }}</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">新用户</div>
                <div class="kpi-value sv-orange">{{ fmt(c.new_users) }}</div>
              </div>
              <div class="kpi">
                <div class="kpi-label">浏览量</div>
                <div class="kpi-value sv-purple">{{ fmt(c.screen_page_views) }}</div>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>

    <!-- ─── Detail state ────────────────────────────────────────────── -->
    <div v-else-if="view === 'detail'" class="view-detail">
      <div class="detail-head">
        <button class="btn btn-back" @click="backToOverview">← 返回总览</button>
        <div class="detail-title">
          <span class="title-platform">{{ platformLabel }}</span>
          <span class="title-sep">·</span>
          <span class="title-site">{{ detail.siteName }}</span>
        </div>
        <button class="btn btn-sync" @click="unifiedRefresh" :disabled="loading || syncing">
          {{ syncing ? '同步中...' : loading ? '加载中...' : '🔄 刷新同步' }}
        </button>
      </div>

      <!-- Sub-tab bar -->
      <div class="panel sub-bar">
        <button
          v-for="t in currentSubTabs"
          :key="t.key"
          :class="['sub-pill', subTab === t.key && 'active']"
          @click="switchSubTab(t.key)"
        >
          <span class="sp-icon" v-if="t.icon">{{ t.icon }}</span>{{ t.label }}
        </button>
      </div>

      <div v-if="loading" class="state-placeholder">加载中...</div>
      <div v-else-if="error" class="state-error">
        <div class="err-title">❌ {{ error }}</div>
      </div>

      <template v-else>
        <!-- ═══════════════════════════════════════════════════════ -->
        <!--  SUB-TAB: 总览                                         -->
        <!-- ═══════════════════════════════════════════════════════ -->
        <template v-if="subTab === 'overview'">
          <div v-if="apiErrors" class="api-errors-banner">
            <div class="aeb-title">⚠️ 部分数据接口调用失败，已展示能拿到的数据：</div>
            <ul class="aeb-list">
              <li v-for="(msg, name) in apiErrors" :key="name">
                <span class="aeb-name">{{ name }}</span>：{{ msg }}
              </li>
            </ul>
          </div>

          <!-- Basic KPI strip -->
          <div class="kpi-strip">
            <div v-for="kpi in detailKPIs" :key="kpi.label" class="panel kpi-card">
              <div class="kpi-label">{{ kpi.label }}</div>
              <div :class="['kpi-value', 'kpi-big', kpi.class]">{{ kpi.value }}</div>
            </div>
          </div>

          <!-- Deep overview: rank buckets (GSC only) -->
          <div v-if="platform === 'gsc' && gscOverview" class="section">
            <div class="deep-kpi-row" v-if="gscOverview.total_queries || gscOverview.total_pages">
              <div class="deep-kpi-card"><div class="kpi-label">关键词数</div><div class="kpi-value kpi-big">{{ fmt(gscOverview.total_queries) }}</div></div>
              <div class="deep-kpi-card"><div class="kpi-label">页面数</div><div class="kpi-value kpi-big">{{ fmt(gscOverview.total_pages) }}</div></div>
            </div>
            <div v-if="!gscOverview.has_deep_data" class="hint-bar">
              ⚠ 当前显示的是基础同步数据。点击「同步深度数据」可获取完整关键词明细、排名分段、SERP 特征、收录状态等深度分析。
            </div>
            <!-- Rank bucket cards in overview -->
            <div v-if="latestBuckets.length" style="margin-top:14px">
              <div class="section-title">排名分布</div>
              <div class="bucket-grid">
                <div class="bucket-card" v-for="b in latestBuckets" :key="b.bucket" @click="subTab='buckets'; activeBucket=b.bucket">
                  <div class="bk-name">{{ b.bucket }}</div>
                  <div class="bk-big">{{ fmt(b.query_count) }} <small>词</small></div>
                  <div class="bk-row"><span>点击</span><b>{{ fmt(b.total_clicks) }}</b></div>
                  <div class="bk-row"><span>曝光</span><b>{{ fmt(b.total_impressions) }}</b></div>
                  <div class="bk-row"><span>CTR</span><b>{{ pct(b.avg_ctr) }}</b></div>
                </div>
              </div>
            </div>
          </div>

          <!-- Trend chart -->
          <div class="panel trend-panel" v-if="detail.trendRows.length >= 2">
            <div class="section-title">每日趋势</div>
            <div class="trend-chart-wrap">
              <svg :viewBox="`0 0 ${trendW} ${trendH}`" preserveAspectRatio="none" class="trend-svg">
                <defs>
                  <linearGradient id="trendGrad" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stop-color="var(--sun)" stop-opacity="0.4" />
                    <stop offset="100%" stop-color="var(--sun)" stop-opacity="0" />
                  </linearGradient>
                </defs>
                <polygon :points="trendAreaPoints" fill="url(#trendGrad)" />
                <polyline :points="trendLinePoints" fill="none" stroke="var(--sun)" stroke-width="2" vector-effect="non-scaling-stroke" />
              </svg>
            </div>
            <div class="trend-labels">
              <span>{{ detail.trendRows[0]?.date }}</span>
              <span>{{ detail.trendRows[detail.trendRows.length - 1]?.date }}</span>
            </div>
          </div>

          <!-- Top keywords + pages (unified: deep data preferred, basic as fallback) -->
          <div class="two-col">
            <div class="panel" v-if="mergedKeywords.length">
              <div class="panel-head">
                <div class="section-title" style="border:none;margin:0;padding:0">🔑 热门关键词 Top 20</div>
                <button class="btn btn-link" @click="switchSubTab('keywords')" v-if="platform === 'gsc'">查看全部 →</button>
              </div>
              <table class="table">
                <thead>
                  <tr><th>#</th><th>关键词</th><th>点击</th><th>展示</th><th>CTR</th><th>排名</th></tr>
                </thead>
                <tbody>
                  <tr v-for="(r, i) in mergedKeywords" :key="i">
                    <td class="rk">{{ i + 1 }}</td>
                    <td class="kw-cell" :title="r.query">
                      <a v-if="platform === 'gsc' && resolvedSiteId" class="link" @click="openKwDetail(r.query)">{{ r.query }}</a>
                      <span v-else>{{ r.query }}</span>
                    </td>
                    <td class="num sv-blue">{{ fmt(r.clicks) }}</td>
                    <td class="num sv-green">{{ fmt(r.impressions) }}</td>
                    <td class="num sv-orange">{{ ((r.ctr || 0) * 100).toFixed(2) }}%</td>
                    <td class="num sv-purple">{{ (r.position || 0).toFixed(1) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>

            <div class="panel" v-if="mergedPages.length">
              <div class="panel-head">
                <div class="section-title" style="border:none;margin:0;padding:0">📄 热门页面 Top 20</div>
                <button class="btn btn-link" @click="switchSubTab('pages')" v-if="platform === 'gsc'">查看全部 →</button>
              </div>
              <table class="table">
                <thead>
                  <tr><th>#</th><th>页面</th><th>点击</th><th>展示</th><th>CTR</th></tr>
                </thead>
                <tbody>
                  <tr v-for="(r, i) in mergedPages" :key="i">
                    <td class="rk">{{ i + 1 }}</td>
                    <td class="path-cell" :title="r.page || r.page_url">{{ cleanPath(r.page || r.page_url) }}</td>
                    <td class="num sv-blue">{{ fmt(r.clicks) }}</td>
                    <td class="num sv-green">{{ fmt(r.impressions) }}</td>
                    <td class="num sv-orange">{{ ((r.ctr || 0) * 100).toFixed(2) }}%</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Devices + Countries -->
          <div class="two-col">
            <div class="panel" v-if="detail.devices.length">
              <div class="section-title">设备分布</div>
              <div class="bar-list">
                <div v-for="d in detail.devices" :key="d.label" class="bar-row">
                  <div class="bar-head">
                    <span class="bar-name">{{ d.label }}</span>
                    <span class="bar-val">{{ fmt(d.clicks) }} · {{ d.pct.toFixed(1) }}%</span>
                  </div>
                  <div class="bar-track"><div class="bar-fill" :style="`width: ${d.pct}%`"></div></div>
                </div>
              </div>
            </div>

            <div class="panel" v-if="detail.countries.length">
              <div class="section-title">国家 / 地区 Top 10</div>
              <div class="bar-list">
                <div v-for="c in detail.countries" :key="c.label" class="bar-row">
                  <div class="bar-head">
                    <span class="bar-name">{{ (c.label || '').toUpperCase() }}</span>
                    <span class="bar-val">{{ fmt(c.clicks) }} · {{ c.pct.toFixed(1) }}%</span>
                  </div>
                  <div class="bar-track"><div class="bar-fill bar-green" :style="`width: ${c.pct}%`"></div></div>
                </div>
              </div>
            </div>
          </div>

          <div v-if="!detail.keywords.length && !detail.pages.length && !detail.trendRows.length && !gscOverview" class="state-empty">
            该日期范围内无数据
          </div>
        </template>

        <!-- ═══════════════════════════════════════════════════════ -->
        <!--  GSC SUB-TABS                                          -->
        <!-- ═══════════════════════════════════════════════════════ -->

        <!-- ── GSC 排名分布 ──────────────────────────────────────── -->
        <template v-if="subTab === 'buckets' && platform === 'gsc'">
          <div class="section" v-if="latestBuckets.length">
            <div class="bucket-grid">
              <div class="bucket-card" v-for="b in latestBuckets" :key="b.bucket"
                   :class="{selected: activeBucket === b.bucket}" @click="drillBucket(b.bucket)">
                <div class="bk-name">{{ b.bucket }}</div>
                <div class="bk-big">{{ fmt(b.query_count) }} <small>词</small></div>
                <div class="bk-row"><span>点击</span><b>{{ fmt(b.total_clicks) }}</b></div>
                <div class="bk-row"><span>曝光</span><b>{{ fmt(b.total_impressions) }}</b></div>
                <div class="bk-row"><span>CTR</span><b>{{ pct(b.avg_ctr) }}</b></div>
                <div class="bk-footer">点击查看该段关键词 →</div>
              </div>
            </div>
            <div class="panel" v-if="activeBucket && activeBucketData">
              <h3>排名段「{{ activeBucket }}」— TOP 关键词</h3>
              <el-table :data="activeBucketData.top_queries" stripe size="small">
                <el-table-column prop="query" label="关键词" min-width="250" show-overflow-tooltip>
                  <template #default="{row}">
                    <a class="link" @click="openKwDetail(row.query)">{{ row.query }}</a>
                  </template>
                </el-table-column>
                <el-table-column prop="clicks" label="点击" width="80" />
                <el-table-column prop="impressions" label="曝光" width="90" />
                <el-table-column label="排名" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
              </el-table>
              <h3 style="margin-top:20px">排名段「{{ activeBucket }}」— TOP 页面</h3>
              <el-table :data="activeBucketData.top_pages" stripe size="small">
                <el-table-column prop="url" label="页面" min-width="350" show-overflow-tooltip />
                <el-table-column prop="clicks" label="点击" width="80" />
                <el-table-column prop="impressions" label="曝光" width="90" />
              </el-table>
            </div>
          </div>
          <div v-else class="empty-state">暂无排名段数据。请先「同步深度数据」。</div>
        </template>

        <!-- ── GSC 关键词分析 ────────────────────────────────────── -->
        <template v-if="subTab === 'keywords' && platform === 'gsc'">
          <div class="panel">
            <div class="panel-head"><h3>🔍 关键词分析</h3>
              <el-button type="success" size="small" @click="exportCSV">📥 导出 CSV</el-button>
            </div>
            <div class="filter-row">
              <el-input v-model="kwSearch" placeholder="搜索关键词..." clearable style="width:200px" @keyup.enter="loadKeywords" />
              <el-select v-model="kwBucket" clearable placeholder="排名段" style="width:110px" @change="kwPage=1;loadKeywords()">
                <el-option value="" label="全部" /><el-option value="1-3" /><el-option value="4-10" />
                <el-option value="11-20" /><el-option value="21-50" /><el-option value="51-100" /><el-option value="100+" />
              </el-select>
              <el-select v-model="kwIntent" clearable placeholder="意图" style="width:110px" @change="kwPage=1;loadKeywords()">
                <el-option value="" label="全部" /><el-option value="informational" label="信息" /><el-option value="transactional" label="交易" />
                <el-option value="commercial" label="商业" /><el-option value="navigational" label="导航" />
              </el-select>
              <el-select v-model="kwBrand" clearable placeholder="品牌" style="width:110px" @change="kwPage=1;loadKeywords()">
                <el-option value="" label="全部" /><el-option value="brand" label="品牌词" /><el-option value="non_brand" label="非品牌" />
              </el-select>
              <el-checkbox v-model="kwQuestion" @change="kwPage=1;loadKeywords()">问题型</el-checkbox>
              <el-button type="primary" size="small" @click="kwPage=1;loadKeywords()">搜索</el-button>
            </div>
            <el-table :data="kwData.items" v-loading="kwLoading" stripe border size="small" style="width:100%">
              <el-table-column type="index" label="#" width="40" />
              <el-table-column prop="query" label="关键词" min-width="200" show-overflow-tooltip>
                <template #default="{row}"><a class="link" @click="openKwDetail(row.query)">{{ row.query }}</a></template>
              </el-table-column>
              <el-table-column prop="page_url" label="落地页" min-width="180" show-overflow-tooltip />
              <el-table-column prop="position" label="排名" width="70" sortable :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
              <el-table-column prop="clicks" label="点击" width="75" sortable />
              <el-table-column prop="impressions" label="曝光" width="80" sortable />
              <el-table-column label="CTR" width="75" :formatter="(_:any,__:any,___:any,r:any) => pct(r.ctr)" />
              <el-table-column prop="intent" label="意图" width="65">
                <template #default="{row}"><el-tag size="small" :type="intentType(row.intent)">{{ intentLabel(row.intent) }}</el-tag></template>
              </el-table-column>
              <el-table-column prop="brand" label="品牌" width="65">
                <template #default="{row}"><el-tag v-if="row.brand==='brand'" size="small" type="warning">品牌</el-tag></template>
              </el-table-column>
              <el-table-column label="" width="60" fixed="right">
                <template #default="{row}"><el-button link type="primary" size="small" @click="openKwDetail(row.query)">详情</el-button></template>
              </el-table-column>
            </el-table>
            <div class="pager" v-if="kwData.total>0">
              <span class="pager-info">共 {{ fmt(kwData.total) }} 条</span>
              <el-pagination layout="sizes, prev, pager, next, jumper" :total="kwData.total" v-model:current-page="kwPage" v-model:page-size="kwPageSize" :page-sizes="[25,50,100]" @size-change="loadKeywords" @current-change="loadKeywords" />
            </div>
          </div>
        </template>

        <!-- ── GSC 页面 ──────────────────────────────────────────── -->
        <template v-if="subTab === 'pages' && platform === 'gsc'">
          <div class="panel">
            <h3>📄 页面分析</h3>
            <div class="filter-row">
              <el-input v-model="pgSearch" placeholder="搜索 URL..." clearable style="width:300px" @keyup.enter="loadPages" />
              <el-button type="primary" size="small" @click="pgPage=1;loadPages()">搜索</el-button>
            </div>
            <el-table :data="pgData.items" v-loading="pgLoading" stripe border size="small">
              <el-table-column type="index" label="#" width="40" />
              <el-table-column prop="page_url" label="页面 URL" min-width="320" show-overflow-tooltip />
              <el-table-column prop="clicks" label="点击" width="80" sortable />
              <el-table-column prop="impressions" label="曝光" width="90" sortable />
              <el-table-column label="CTR" width="75" :formatter="(_:any,__:any,___:any,r:any) => pct(r.ctr)" />
              <el-table-column label="排名" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
              <el-table-column prop="query_count" label="词数" width="65" />
            </el-table>
            <div class="pager" v-if="pgData.total>0">
              <el-pagination layout="total, prev, pager, next" :total="pgData.total" v-model:current-page="pgPage" @current-change="loadPages" />
            </div>
          </div>
        </template>

        <!-- ── GSC SERP 特征 ─────────────────────────────────────── -->
        <template v-if="subTab === 'serp' && platform === 'gsc'">
          <div class="panel">
            <h3>🎯 SERP 特征分布</h3>
            <p class="hint" v-if="!appearanceData.length">暂无数据。同步深度数据后可看到 AI Overview / Featured Snippet / FAQ 等 40+ 种富结果的点击曝光分布。</p>
            <div class="serp-cards" v-if="appearanceData.length">
              <div class="serp-card" v-for="a in appearanceData" :key="a.appearance">
                <div class="sc-name">{{ serpLabel(a.appearance) }}</div>
                <div class="sc-stats">
                  <span><b>{{ fmt(a.impressions) }}</b> 曝光</span>
                  <span><b>{{ fmt(a.clicks) }}</b> 点击</span>
                  <span>CTR <b>{{ pct(a.ctr) }}</b></span>
                  <span>位置 <b>{{ a.position?.toFixed(1) }}</b></span>
                </div>
              </div>
            </div>
          </div>
        </template>

        <!-- ── GSC 收录状态 ──────────────────────────────────────── -->
        <template v-if="subTab === 'index' && platform === 'gsc'">
          <div class="panel">
            <div class="panel-head">
              <h3>📋 收录状态</h3>
              <el-button type="warning" size="small" :loading="inspecting" @click="runUrlInspection">🔍 检查 TOP 100 页面收录</el-button>
            </div>
            <p class="hint" v-if="!idxSummary.length">暂无数据。点击上方按钮对 TOP 100 页面逐个检查收录状态（耗时约 2 分钟）。</p>
            <div class="idx-summary" v-if="idxSummary.length">
              <div class="idx-card" v-for="s in idxSummary" :key="s.state"
                   :class="{selected: idxCoverage===s.state, ok: isOkState(s.state), warn: isWarnState(s.state), err: isErrState(s.state)}"
                   @click="idxCoverage=idxCoverage===s.state?'':s.state; loadUrlInspection()">
                <div class="ic-count">{{ s.count }}</div>
                <div class="ic-label">{{ s.state }}</div>
              </div>
            </div>
            <el-table v-if="idxData.items?.length" :data="idxData.items" stripe border size="small" style="margin-top:12px">
              <el-table-column prop="url" label="URL" min-width="300" show-overflow-tooltip />
              <el-table-column prop="coverage_state" label="覆盖状态" width="220" />
              <el-table-column prop="verdict" label="判决" width="70">
                <template #default="{row}"><el-tag :type="row.verdict==='PASS'?'success':row.verdict==='FAIL'?'danger':'warning'" size="small">{{ row.verdict }}</el-tag></template>
              </el-table-column>
              <el-table-column prop="page_fetch_state" label="抓取状态" width="140" />
              <el-table-column prop="last_crawl_time" label="上次抓取" width="120" :formatter="(_:any,__:any,___:any,r:any) => r.last_crawl_time?.slice(0,10)||'-'" />
              <el-table-column label="移动友好" width="70" align="center">
                <template #default="{row}"><span :style="{color:row.mobile_usable?'#67c23a':'#f56c6c'}">{{ row.mobile_usable?'✓':'✗' }}</span></template>
              </el-table-column>
            </el-table>
            <div class="pager" v-if="idxData.total>25">
              <el-pagination layout="total, prev, pager, next" :total="idxData.total" v-model:current-page="idxPage" @current-change="loadUrlInspection" />
            </div>
          </div>
        </template>

        <!-- ── GSC Sitemap ───────────────────────────────────────── -->
        <template v-if="subTab === 'sitemaps' && platform === 'gsc'">
          <div class="panel">
            <h3>🗺️ Sitemap 状态</h3>
            <p class="hint" v-if="!sitemapData.length">暂无数据。同步深度数据后可看到各 Sitemap 的提交/收录数量。</p>
            <el-table :data="sitemapData" v-if="sitemapData.length" stripe border size="small">
              <el-table-column prop="path" label="Sitemap URL" min-width="350" show-overflow-tooltip />
              <el-table-column prop="type" label="类型" width="90" />
              <el-table-column prop="errors" label="错误" width="65"><template #default="{row}"><span :style="{color:row.errors?'#f56c6c':'#67c23a',fontWeight:600}">{{ row.errors }}</span></template></el-table-column>
              <el-table-column prop="warnings" label="警告" width="65"><template #default="{row}"><span :style="{color:row.warnings?'#e6a23c':'#67c23a'}">{{ row.warnings }}</span></template></el-table-column>
              <el-table-column label="索引型" width="65" align="center"><template #default="{row}">{{ row.is_sitemaps_index?'✓':'' }}</template></el-table-column>
              <el-table-column prop="last_downloaded" label="最后下载" width="120" :formatter="(_:any,__:any,___:any,r:any) => r.last_downloaded?.slice(0,10)||'-'" />
            </el-table>
          </div>
        </template>

        <!-- ── GSC 机会 ──────────────────────────────────────────── -->
        <template v-if="subTab === 'opportunities' && platform === 'gsc'">
          <div class="panel">
            <h3>💡 关键词机会（按机会分排序）</h3>
            <p class="hint" v-if="!oppData.length">暂无机会数据。需要先同步深度数据且等待次日派生计算完成。</p>
            <el-table :data="oppData" v-if="oppData.length" stripe border size="small">
              <el-table-column type="index" label="#" width="40" />
              <el-table-column prop="query" label="关键词" min-width="200" show-overflow-tooltip>
                <template #default="{row}"><a class="link" @click="openKwDetail(row.query)">{{ row.query }}</a></template>
              </el-table-column>
              <el-table-column prop="page_url" label="落地页" min-width="180" show-overflow-tooltip />
              <el-table-column label="排名" width="65" :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
              <el-table-column prop="impressions" label="曝光" width="80" />
              <el-table-column label="实际CTR" width="80" :formatter="(_:any,__:any,___:any,r:any) => pct(r.actual_ctr)" />
              <el-table-column label="基准CTR" width="80" :formatter="(_:any,__:any,___:any,r:any) => pct(r.baseline_ctr)" />
              <el-table-column prop="opportunity_score" label="机会分" width="75" sortable>
                <template #default="{row}"><b style="color:#e6a23c">{{ row.opportunity_score }}</b></template>
              </el-table-column>
              <el-table-column prop="estimated_extra_clicks" label="预估月增点" width="90" />
              <el-table-column prop="type" label="类型" width="140" />
            </el-table>
          </div>
        </template>

        <!-- ── GSC 自蚕食 ────────────────────────────────────────── -->
        <template v-if="subTab === 'cannibal' && platform === 'gsc'">
          <div class="panel">
            <h3>⚠️ 自蚕食检测</h3>
            <p class="hint" v-if="!cannibalData.length">暂无自蚕食数据。需要先同步深度数据且等待次日派生计算完成。</p>
            <el-table :data="cannibalData" v-if="cannibalData.length" stripe border size="small">
              <el-table-column prop="query" label="关键词" min-width="200" show-overflow-tooltip>
                <template #default="{row}"><a class="link" @click="openKwDetail(row.query)">{{ row.query }}</a></template>
              </el-table-column>
              <el-table-column prop="page_count" label="争抢页面数" width="95">
                <template #default="{row}"><b style="color:#f56c6c">{{ row.page_count }}</b></template>
              </el-table-column>
              <el-table-column prop="severity" label="严重度" width="75">
                <template #default="{row}"><el-tag :type="row.severity==='high'?'danger':row.severity==='medium'?'warning':'info'" size="small">{{ row.severity }}</el-tag></template>
              </el-table-column>
              <el-table-column prop="total_impressions" label="总曝光" width="80" />
              <el-table-column prop="total_clicks" label="总点击" width="75" />
              <el-table-column label="争抢页面" min-width="350">
                <template #default="{row}">
                  <div v-for="(p,i) in row.competing_pages?.slice(0,3)" :key="i" class="cannibal-line">
                    <span class="cl-url">{{ p.url }}</span>
                    <span class="cl-stat">曝光{{ p.impressions }} · 位置{{ p.position?.toFixed(1) }}</span>
                  </div>
                  <div v-if="row.competing_pages?.length > 3" style="color:#999;font-size:11px">还有 {{ row.competing_pages.length - 3 }} 个页面...</div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </template>

        <!-- ── GSC 关键词变动 ────────────────────────────────────── -->
        <template v-if="subTab === 'diff' && platform === 'gsc'">
          <div class="panel">
            <h3>📊 关键词变动（周对比）</h3>
            <div class="filter-row">
              <el-radio-group v-model="diffType" @change="loadQueryDiff">
                <el-radio-button value="lost">🔻 丢失词</el-radio-button>
                <el-radio-button value="new">🔺 新词</el-radio-button>
              </el-radio-group>
            </div>
            <p class="hint" v-if="!diffData.length">暂无变动数据。需要先同步深度数据且等待次日派生计算完成。</p>
            <el-table :data="diffData" v-if="diffData.length" stripe border size="small">
              <el-table-column prop="query" label="关键词" min-width="280" show-overflow-tooltip />
              <el-table-column prop="prev_clicks" label="上期点击" width="90" />
              <el-table-column prop="curr_clicks" label="本期点击" width="90" />
              <el-table-column prop="click_delta" label="变化" width="80">
                <template #default="{row}"><span :style="{color:row.click_delta>0?'#67c23a':'#f56c6c',fontWeight:600}">{{ row.click_delta > 0 ? '+' : '' }}{{ row.click_delta }}</span></template>
              </el-table-column>
            </el-table>
          </div>
        </template>

        <!-- ═══════════════════════════════════════════════════════ -->
        <!--  GA4 SUB-TABS                                          -->
        <!-- ═══════════════════════════════════════════════════════ -->
        <template v-if="platform === 'ga4' && subTab !== 'overview'">
          <div class="panel">
            <div v-if="ga4Loading" class="loading-text">加载中...</div>
            <template v-if="subTab === 'landing'"><h3>入站页</h3>
              <el-table :data="ga4LandingPages" v-if="ga4LandingPages.length" stripe border size="small">
                <el-table-column type="index" width="40" /><el-table-column prop="landing_page" label="入站页" min-width="300" show-overflow-tooltip />
                <el-table-column prop="sessions" label="会话" width="75" /><el-table-column prop="users" label="用户" width="70" />
                <el-table-column label="参与率" width="70" :formatter="(_:any,__:any,___:any,r:any) => pct(r.engagement_rate)" />
                <el-table-column label="跳出率" width="70" :formatter="(_:any,__:any,___:any,r:any) => pct(r.bounce_rate)" />
                <el-table-column prop="conversions" label="转化" width="60" />
              </el-table></template>
            <template v-if="subTab === 'source'"><h3>流量来源</h3>
              <el-table :data="ga4Sources" v-if="ga4Sources.length" stripe border size="small">
                <el-table-column prop="source" label="来源" width="140" /><el-table-column prop="medium" label="媒介" width="110" /><el-table-column prop="channel_group" label="渠道" width="140" />
                <el-table-column prop="sessions" label="会话" width="75" /><el-table-column prop="users" label="用户" width="70" />
                <el-table-column label="参与率" width="70" :formatter="(_:any,__:any,___:any,r:any) => pct(r.engagement_rate)" /><el-table-column prop="conversions" label="转化" width="60" />
              </el-table></template>
            <template v-if="subTab === 'events'"><h3>事件</h3>
              <el-table :data="ga4Events" v-if="ga4Events.length" stripe border size="small">
                <el-table-column prop="event_name" label="事件" min-width="200" /><el-table-column prop="event_count" label="次数" width="80" />
                <el-table-column prop="users" label="用户" width="70" /><el-table-column prop="event_value" label="值" width="80" />
                <el-table-column label="转化" width="55"><template #default="{row}">{{ row.is_conversion?'✓':'' }}</template></el-table-column>
              </el-table></template>
            <template v-if="subTab === 'tech'"><h3>设备与技术</h3>
              <div class="filter-row"><el-radio-group v-model="techDim" @change="loadGA4Tab"><el-radio-button value="device">设备</el-radio-button><el-radio-button value="os">OS</el-radio-button><el-radio-button value="browser">浏览器</el-radio-button></el-radio-group></div>
              <el-table :data="ga4Tech" v-if="ga4Tech.length" stripe border size="small">
                <el-table-column prop="value" label="维度" min-width="180" /><el-table-column prop="sessions" label="会话" width="80" /><el-table-column prop="users" label="用户" width="70" />
                <el-table-column label="参与率" width="75" :formatter="(_:any,__:any,___:any,r:any) => pct(r.engagement_rate)" />
              </el-table></template>
            <template v-if="subTab === 'geo'"><h3>地理分布</h3>
              <el-table :data="ga4Geo" v-if="ga4Geo.length" stripe border size="small">
                <el-table-column prop="country" label="国家" width="110" /><el-table-column prop="region" label="地区" width="130" /><el-table-column prop="city" label="城市" width="130" />
                <el-table-column prop="sessions" label="会话" width="75" /><el-table-column prop="users" label="用户" width="70" />
              </el-table></template>
            <template v-if="subTab === 'hourly'"><h3>小时分布</h3>
              <el-table :data="ga4Hourly" v-if="ga4Hourly.length" stripe border size="small">
                <el-table-column label="小时" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.hour+':00'" /><el-table-column prop="sessions" label="会话" width="80" /><el-table-column prop="users" label="用户" width="70" /><el-table-column prop="screen_page_views" label="PV" width="70" />
              </el-table></template>
            <template v-if="subTab === 'dow'"><h3>星期分布</h3>
              <el-table :data="ga4DayOfWeek" v-if="ga4DayOfWeek.length" stripe border size="small">
                <el-table-column prop="name" label="星期" width="90" /><el-table-column prop="sessions" label="会话" width="80" /><el-table-column prop="users" label="用户" width="70" /><el-table-column prop="conversions" label="转化" width="65" />
              </el-table></template>
            <template v-if="subTab === 'scroll'"><h3>滚动深度</h3>
              <el-table :data="ga4Scroll" v-if="ga4Scroll.length" stripe border size="small">
                <el-table-column prop="page_path" label="页面" min-width="250" show-overflow-tooltip /><el-table-column label="滚动%" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.percent_scrolled+'%'" />
                <el-table-column prop="users" label="用户" width="70" /><el-table-column prop="event_count" label="次数" width="70" />
              </el-table></template>
            <template v-if="subTab === 'outbound'"><h3>外链点击</h3>
              <el-table :data="ga4Outbound" v-if="ga4Outbound.length" stripe border size="small">
                <el-table-column prop="link_domain" label="域名" width="180" /><el-table-column prop="link_url" label="URL" min-width="300" show-overflow-tooltip />
                <el-table-column prop="event_count" label="点击" width="70" /><el-table-column prop="users" label="用户" width="70" />
              </el-table></template>
            <template v-if="subTab === 'conversions'"><h3>转化明细</h3>
              <el-table :data="ga4Conversions" v-if="ga4Conversions.length" stripe border size="small">
                <el-table-column prop="event_name" label="事件" min-width="180" /><el-table-column prop="source" label="来源" width="120" /><el-table-column prop="medium" label="媒介" width="100" />
                <el-table-column prop="conversions" label="转化" width="70" /><el-table-column prop="total_revenue" label="收入" width="80" />
              </el-table></template>
          </div>
        </template>

        <!-- ═══════════════════════════════════════════════════════ -->
        <!--  Bing SUB-TABS                                         -->
        <!-- ═══════════════════════════════════════════════════════ -->
        <template v-if="platform === 'bing' && subTab !== 'overview'">
          <div class="panel">
            <div v-if="bingLoading" class="loading-text">加载中...</div>
            <template v-if="subTab === 'keywords'"><h3>Bing 关键词</h3>
              <el-input v-model="bingKwSearch" placeholder="搜索" clearable style="width:200px;margin-bottom:8px" @keyup.enter="loadBingTab" />
              <el-table :data="bingKeywords.items" v-if="bingKeywords.items?.length" stripe border size="small">
                <el-table-column type="index" width="40" /><el-table-column prop="query" label="关键词" min-width="240" show-overflow-tooltip />
                <el-table-column prop="clicks" label="点击" width="75" /><el-table-column prop="impressions" label="曝光" width="80" />
                <el-table-column label="CTR" width="75" :formatter="(_:any,__:any,___:any,r:any) => pct(r.ctr)" />
                <el-table-column label="位置" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.avg_impression_position?.toFixed(1)" />
              </el-table>
              <div class="pager" v-if="bingKeywords.total>25"><el-pagination layout="total, prev, pager, next" :total="bingKeywords.total" v-model:current-page="bingKwPage" @current-change="loadBingTab" /></div>
            </template>
            <template v-if="subTab === 'pages'"><h3>Bing 页面</h3>
              <el-table :data="bingPages.items" v-if="bingPages.items?.length" stripe border size="small">
                <el-table-column prop="page_url" label="页面" min-width="320" show-overflow-tooltip /><el-table-column prop="clicks" label="点击" width="75" /><el-table-column prop="impressions" label="曝光" width="80" />
                <el-table-column label="CTR" width="75" :formatter="(_:any,__:any,___:any,r:any) => pct(r.ctr)" /><el-table-column label="位置" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
              </el-table></template>
            <template v-if="subTab === 'backlinks'"><h3>Bing 反链</h3>
              <p class="hint" v-if="!bingBacklinks.length">暂无反链数据。</p>
              <el-table :data="bingBacklinks" v-if="bingBacklinks.length" stripe border size="small">
                <el-table-column prop="source_url" label="来源 URL" min-width="300" show-overflow-tooltip /><el-table-column prop="target_page" label="目标页" min-width="200" show-overflow-tooltip /><el-table-column prop="anchor_text" label="锚文本" width="140" />
              </el-table></template>
            <template v-if="subTab === 'bing_sitemaps'"><h3>Bing Sitemap</h3>
              <el-table :data="bingSitemapList" v-if="bingSitemapList.length" stripe border size="small">
                <el-table-column prop="sitemap_url" label="Sitemap" min-width="300" /><el-table-column prop="url_count" label="URL数" width="90" /><el-table-column prop="status" label="状态" width="90" />
              </el-table></template>
          </div>
        </template>

        <!-- ═══════════════════════════════════════════════════════ -->
        <!--  Yandex SUB-TABS                                       -->
        <!-- ═══════════════════════════════════════════════════════ -->
        <template v-if="platform === 'yandex' && subTab !== 'overview'">
          <div class="panel">
            <div v-if="yandexLoading" class="loading-text">加载中...</div>
            <template v-if="subTab === 'keywords'"><h3>Yandex 关键词</h3>
              <el-table :data="yandexKeywords.items" v-if="yandexKeywords.items?.length" stripe border size="small">
                <el-table-column type="index" width="40" /><el-table-column prop="query" label="关键词" min-width="240" show-overflow-tooltip />
                <el-table-column prop="clicks" label="点击" width="75" /><el-table-column prop="shows" label="曝光" width="80" />
                <el-table-column label="CTR" width="75" :formatter="(_:any,__:any,___:any,r:any) => pct(r.ctr)" />
                <el-table-column label="展示位" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.avg_show_position?.toFixed(1)" />
              </el-table></template>
            <template v-if="subTab === 'index_events'"><h3>索引事件</h3>
              <el-table :data="yandexIndexEvents" v-if="yandexIndexEvents.length" stripe border size="small">
                <el-table-column prop="date" label="日期" width="100" /><el-table-column prop="event_type" label="事件" min-width="220" /><el-table-column prop="count" label="数量" width="75" />
              </el-table></template>
            <template v-if="subTab === 'http_status'"><h3>HTTP 状态</h3>
              <el-table :data="yandexHttpStatus" v-if="yandexHttpStatus.length" stripe border size="small">
                <el-table-column prop="date" label="日期" width="100" /><el-table-column prop="status_code" label="状态码" min-width="200" /><el-table-column prop="count" label="数量" width="75" />
              </el-table></template>
            <template v-if="subTab === 'external_links'"><h3>外链</h3>
              <el-table :data="yandexExtLinks" v-if="yandexExtLinks.length" stripe border size="small">
                <el-table-column prop="source_url" label="来源" min-width="300" show-overflow-tooltip /><el-table-column prop="target_url" label="目标" min-width="200" show-overflow-tooltip />
              </el-table></template>
            <template v-if="subTab === 'indexed_urls'"><h3>索引 URL 样本</h3>
              <el-table :data="yandexIndexedUrls.items" v-if="yandexIndexedUrls.items?.length" stripe border size="small">
                <el-table-column prop="url" label="URL" min-width="400" show-overflow-tooltip /><el-table-column prop="status" label="状态" width="90" /><el-table-column prop="http_code" label="HTTP" width="65" />
              </el-table></template>
          </div>
        </template>
      </template>
    </div>

    <!-- ── 关键词详情弹窗 ──────────────────────────────────────── -->
    <el-dialog v-model="kwDetailVisible" :title="'🔑 关键词详情'" width="850px" destroy-on-close top="5vh">
      <div v-if="kwDetailData" class="kw-detail">
        <div class="kd-header">
          <div class="kd-query">{{ kwDetailData.query }}</div>
          <div class="kd-tags">
            <el-tag size="small" :type="intentType(kwDetailData.intent)">{{ intentLabel(kwDetailData.intent) }}</el-tag>
            <el-tag v-if="kwDetailData.brand==='brand'" size="small" type="warning">品牌词</el-tag>
            <el-tag v-if="kwDetailData.is_question" size="small" type="danger">问题型</el-tag>
            <el-tag v-if="kwDetailData.is_comparison" size="small">对比型</el-tag>
            <span class="kd-meta">{{ kwDetailData.word_count }} 词 · {{ kwDetailData.char_count }} 字</span>
          </div>
        </div>
        <h4>📈 90 天趋势</h4>
        <el-table :data="kwDetailData.trend" stripe size="small" max-height="260" border>
          <el-table-column prop="date" label="日期" width="95" /><el-table-column prop="clicks" label="点击" width="65" /><el-table-column prop="impressions" label="曝光" width="75" />
          <el-table-column label="CTR" width="70" :formatter="(_:any,__:any,___:any,r:any) => pct(r.ctr)" />
          <el-table-column label="排名" width="65" :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
        </el-table>
        <h4 style="margin-top:16px">📄 对应页面（TOP 20）</h4>
        <div v-if="kwDetailData.cannibalization" class="cannibal-warn">
          ⚠️ 自蚕食警告：该关键词有 <b>{{ kwDetailData.cannibal_page_count }}</b> 个页面在 TOP 20 争抢排名！
        </div>
        <el-table :data="kwDetailData.pages" stripe size="small" border>
          <el-table-column type="index" width="35" /><el-table-column prop="page_url" label="页面" min-width="300" show-overflow-tooltip />
          <el-table-column prop="clicks" label="点击" width="65" /><el-table-column prop="impressions" label="曝光" width="75" />
          <el-table-column label="排名" width="65" :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
        </el-table>
        <div v-if="kwDetailData.devices?.length" style="margin-top:16px">
          <h4>📱 设备分布</h4>
          <div class="mini-cards">
            <div class="mini-card" v-for="d in kwDetailData.devices" :key="d.device">
              <div class="mc-title">{{ d.device }}</div>
              <div class="mc-row"><span>点击</span><b>{{ d.clicks }}</b></div>
              <div class="mc-row"><span>曝光</span><b>{{ d.impressions }}</b></div>
              <div class="mc-row"><span>排名</span><b>{{ d.position?.toFixed(1) }}</b></div>
            </div>
          </div>
        </div>
        <div v-if="kwDetailData.countries?.length" style="margin-top:16px">
          <h4>🌍 国家分布（TOP 10）</h4>
          <el-table :data="kwDetailData.countries" stripe size="small" border>
            <el-table-column prop="country" label="国家" width="80" /><el-table-column prop="clicks" label="点击" width="65" />
            <el-table-column prop="impressions" label="曝光" width="75" />
            <el-table-column label="排名" width="65" :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
          </el-table>
        </div>
        <div v-if="kwDetailData.other_search_types?.length" style="margin-top:16px">
          <h4>🔎 其他搜索类型</h4>
          <div class="mini-cards">
            <div class="mini-card" v-for="s in kwDetailData.other_search_types" :key="s.search_type">
              <div class="mc-title">{{ s.search_type }}</div>
              <div class="mc-row"><span>点击</span><b>{{ s.clicks }}</b></div>
              <div class="mc-row"><span>曝光</span><b>{{ s.impressions }}</b></div>
            </div>
          </div>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

// ── Platforms ────────────────────────────────────────────────────────────
const platforms = [
  { code: 'gsc', label: 'GSC' },
  { code: 'ga4', label: 'GA4' },
  { code: 'bing', label: 'Bing' },
  { code: 'yandex', label: 'Yandex' },
]
const platform = ref<'gsc' | 'ga4' | 'bing' | 'yandex'>('gsc')
const platformLabel = computed(() => platforms.find(p => p.code === platform.value)?.label || '')

// ── Date range + presets ────────────────────────────────────────────────
function ymd(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
function daysAgo(n: number): string {
  const d = new Date()
  d.setDate(d.getDate() - n)
  return ymd(d)
}
const dateRange = ref<[string, string]>([daysAgo(29), daysAgo(2)])
const activePreset = ref<string>('28d')
const presets = [
  { key: 'recent1', label: '最近一天' },
  { key: '7d', label: '近 7 天' },
  { key: '28d', label: '近 28 天' },
  { key: '90d', label: '近 90 天' },
]
function applyPreset(key: string) {
  activePreset.value = key
  switch (key) {
    case 'recent1': dateRange.value = [daysAgo(2), daysAgo(2)]; break
    case '7d':      dateRange.value = [daysAgo(8), daysAgo(2)]; break
    case '28d':     dateRange.value = [daysAgo(29), daysAgo(2)]; break
    case '90d':     dateRange.value = [daysAgo(91), daysAgo(2)]; break
  }
  refreshCurrent()
}
function onDateChange() {
  activePreset.value = 'custom'
  refreshCurrent()
}
function disabledDate(date: Date): boolean {
  const earliest = new Date()
  earliest.setMonth(earliest.getMonth() - 16)
  return date > new Date() || date < earliest
}
const dayCount = computed(() => {
  if (!dateRange.value[0] || !dateRange.value[1]) return 0
  const s = new Date(dateRange.value[0]).getTime()
  const e = new Date(dateRange.value[1]).getTime()
  return Math.round((e - s) / 86400000) + 1
})
const hasLagWarning = computed(() => {
  if (!dateRange.value[1]) return false
  const end = new Date(dateRange.value[1]).getTime()
  const now = Date.now()
  return (now - end) / 86400000 < 2 && (platform.value === 'gsc' || platform.value === 'ga4')
})

// ── Deep data days param (derived from dateRange) ──────────────────────
const deepDays = computed(() => dayCount.value || 30)

// ── View state ──────────────────────────────────────────────────────────
const view = ref<'overview' | 'detail'>('overview')
const loading = ref(false)
const error = ref('')
const apiErrors = ref<Record<string, string> | null>(null)
const syncing = ref(false)

const overview = reactive({ cards: [] as any[] })
const detail = reactive({
  siteKey: '' as string,
  siteName: '' as string,
  kpis: {} as Record<string, number>,
  trendRows: [] as any[],
  keywords: [] as any[],
  pages: [] as any[],
  devices: [] as { label: string; clicks: number; pct: number }[],
  countries: [] as { label: string; clicks: number; pct: number }[],
})

// ── Sub-tabs ────────────────────────────────────────────────────────────
const subTab = ref('overview')

const gscSubTabs = [
  { key: 'overview', label: '总览', icon: '📊' },
  { key: 'buckets', label: '排名分布', icon: '📶' },
  { key: 'keywords', label: '关键词', icon: '🔑' },
  { key: 'pages', label: '页面', icon: '📄' },
  { key: 'serp', label: 'SERP特征', icon: '🎯' },
  { key: 'index', label: '收录状态', icon: '📋' },
  { key: 'sitemaps', label: 'Sitemap', icon: '🗺️' },
  { key: 'opportunities', label: '机会', icon: '💡' },
  { key: 'cannibal', label: '自蚕食', icon: '⚠️' },
  { key: 'diff', label: '变动', icon: '📊' },
]
const ga4SubTabs = [
  { key: 'overview', label: '总览' },
  { key: 'landing', label: '入站页' },
  { key: 'source', label: '来源' },
  { key: 'events', label: '事件' },
  { key: 'tech', label: '技术' },
  { key: 'geo', label: '地理' },
  { key: 'hourly', label: '小时' },
  { key: 'dow', label: '星期' },
  { key: 'scroll', label: '滚动' },
  { key: 'outbound', label: '外链点击' },
  { key: 'conversions', label: '转化' },
]
const bingSubTabs = [
  { key: 'overview', label: '总览' },
  { key: 'keywords', label: '关键词' },
  { key: 'pages', label: '页面' },
  { key: 'backlinks', label: '反链' },
  { key: 'bing_sitemaps', label: 'Sitemap' },
]
const yandexSubTabs = [
  { key: 'overview', label: '总览' },
  { key: 'keywords', label: '关键词' },
  { key: 'index_events', label: '索引事件' },
  { key: 'http_status', label: 'HTTP状态' },
  { key: 'external_links', label: '外链' },
  { key: 'indexed_urls', label: '索引URL' },
]

const currentSubTabs = computed(() => {
  switch (platform.value) {
    case 'gsc': return gscSubTabs
    case 'ga4': return ga4SubTabs
    case 'bing': return bingSubTabs
    case 'yandex': return yandexSubTabs
  }
  return []
})

function switchSubTab(key: string) {
  subTab.value = key
  loadSubTabData(key)
}

// ── Site resolution for deep data ───────────────────────────────────────
// The deep data API endpoints require a numeric site_id from the sites
// table. We resolve it by matching the detail's siteKey (domain/URL) to
// the sites list.
const allSites = ref<any[]>([])
// resolvedSiteId: resolved via backend API call, not client-side matching
const resolvedSiteId = ref<number | null>(null)

// Called when entering a detail view to resolve siteKey → site_id via backend
async function resolveSiteIdFromBackend() {
  if (!detail.siteKey) { resolvedSiteId.value = null; return }
  try {
    const r: any = await api.resolveSiteId(detail.siteKey)
    resolvedSiteId.value = r.data?.site_id || null
  } catch {
    // Fallback: try with siteName
    if (detail.siteName && detail.siteName !== detail.siteKey) {
      try {
        const r2: any = await api.resolveSiteId(detail.siteName)
        resolvedSiteId.value = r2.data?.site_id || null
      } catch { resolvedSiteId.value = null }
    } else {
      resolvedSiteId.value = null
    }
  }
}

// ── 5-minute in-memory cache ────────────────────────────────────────────
type CacheEntry = { at: number; data: any }
const cacheTTL = 5 * 60 * 1000
const cache = new Map<string, CacheEntry>()
function cacheKey(extra = '') {
  return `${platform.value}:${dateRange.value[0]}:${dateRange.value[1]}:${extra}`
}
function cacheGet(k: string): any | null {
  const hit = cache.get(k)
  if (!hit) return null
  if (Date.now() - hit.at > cacheTTL) { cache.delete(k); return null }
  return hit.data
}
function cacheSet(k: string, data: any) {
  cache.set(k, { at: Date.now(), data })
}

// ── Platform switch + main load ─────────────────────────────────────────
function switchPlatform(code: any) {
  if (platform.value === code) return
  platform.value = code
  view.value = 'overview'
  subTab.value = 'overview'
  error.value = ''
  loadOverview()
}

async function refreshCurrent(bypassCache = false) {
  if (bypassCache) {
    const prefix = `${platform.value}:${dateRange.value[0]}:${dateRange.value[1]}:`
    for (const k of Array.from(cache.keys())) {
      if (k.startsWith(prefix)) cache.delete(k)
    }
  }
  if (view.value === 'overview') await loadOverview()
  else {
    await loadDetail(detail.siteKey, detail.siteName)
    if (subTab.value !== 'overview') loadSubTabData(subTab.value)
  }
}

// Unified refresh: clears cache + refreshes basic data + triggers deep sync + reloads deep data
async function unifiedRefresh() {
  // Clear all cache
  cache.clear()
  if (view.value === 'overview') {
    await loadOverview()
    return
  }
  // In detail view: first resolve site_id, then refresh basic + deep sync
  await resolveSiteIdFromBackend()
  const sid = resolvedSiteId.value
  loading.value = true
  try {
    // 1. Reload basic platform data
    const baseParams = { start_date: dateRange.value[0], end_date: dateRange.value[1] }
    let res: any
    switch (platform.value) {
      case 'gsc':    res = await api.dashGSCSiteDetail({ ...baseParams, site_url: detail.siteKey }); break
      case 'ga4':    res = await api.dashGAPropertyDetail({ ...baseParams, property_id: detail.siteKey }); break
      case 'bing':   res = await api.dashBingSiteDetail({ ...baseParams, site_url: detail.siteKey }); break
      case 'yandex': res = await api.dashYandexHostDetail({ ...baseParams, host_id: detail.siteKey }); break
    }
    if (res?.data) digestDetail(res.data)
  } catch {} finally { loading.value = false }

  // 2. Trigger deep sync if site resolved
  if (sid) {
    syncing.value = true
    try {
      if (platform.value === 'gsc') await api.syncGSCDeep(sid)
      else if (platform.value === 'ga4') await api.syncGA4Deep(sid)
      else if (platform.value === 'bing') await api.syncBingDeep(sid)
      else if (platform.value === 'yandex') await api.syncYandexDeep(sid)
      ElMessage.success('数据刷新同步完成')
    } catch (e: any) {
      ElMessage.warning('基础数据已刷新，深度同步: ' + (e?.response?.data?.error || e?.message || '失败'))
    } finally { syncing.value = false }

    // 3. Reload deep data
    loadDeepOverviewData()
    if (subTab.value !== 'overview') loadSubTabData(subTab.value)
  }
}

// Merged data: prefer deep data (gscOverview.top_keywords) over basic (detail.keywords)
const mergedKeywords = computed(() => {
  if (gscOverview.value?.top_keywords?.length) {
    return gscOverview.value.top_keywords.map((k: any) => ({
      query: k.query, clicks: k.clicks, impressions: k.impressions, ctr: k.ctr, position: k.position,
    }))
  }
  return detail.keywords
})
const mergedPages = computed(() => {
  if (gscOverview.value?.top_pages?.length) {
    return gscOverview.value.top_pages.map((p: any) => ({
      page: p.page_url || p.page, page_url: p.page_url, clicks: p.clicks, impressions: p.impressions, ctr: p.ctr, position: p.position,
    }))
  }
  return detail.pages
})

function platformSiteKey(c: any): string {
  switch (platform.value) {
    case 'gsc':    return c.site_url || ''
    case 'ga4':    return c.property_id || ''
    case 'bing':   return c.site_url || ''
    case 'yandex': return c.host_id || ''
  }
  return ''
}

async function hideCard(c: any) {
  const key = platformSiteKey(c)
  if (!key) return
  const label = c.display_name || key
  try {
    await ElMessageBox.confirm(
      `确认将「${label}」从「${platformLabel.value}」总览中隐藏？隐藏后下次登录也不会再出现，除非在平台重新授权。`,
      '隐藏站点',
      { type: 'warning', confirmButtonText: '隐藏', cancelButtonText: '取消' }
    )
  } catch { return }
  try {
    await api.hidePlatformSite(platform.value, key)
    overview.cards = overview.cards.filter(x => platformSiteKey(x) !== key)
    cache.delete(cacheKey(''))
    ElMessage.success('已隐藏')
  } catch { /* interceptor */ }
}

async function loadOverview() {
  loading.value = true
  error.value = ''
  const key = cacheKey('')
  const cached = cacheGet(key)
  if (cached) {
    overview.cards = cached.cards || []
    loading.value = false
    return
  }
  const params = { start_date: dateRange.value[0], end_date: dateRange.value[1] }
  try {
    let res: any
    switch (platform.value) {
      case 'gsc':    res = await api.dashGSCOverview(params); break
      case 'ga4':    res = await api.dashGAOverview(params); break
      case 'bing':   res = await api.dashBingOverview(params); break
      case 'yandex': res = await api.dashYandexOverview(params); break
    }
    const data = res.data || { cards: [] }
    overview.cards = data.cards || []
    cacheSet(key, data)
  } catch (e: any) {
    error.value = e?.response?.data?.error || e?.message || '加载失败'
    overview.cards = []
  } finally {
    loading.value = false
  }
}

// ── Detail view ─────────────────────────────────────────────────────────
function cardSubtitle(c: any): string {
  switch (platform.value) {
    case 'gsc':    return c.is_domain ? 'Domain Property' : (c.site_url || '')
    case 'ga4':    return c.account_name ? `账户：${c.account_name}` : ''
    case 'bing':   return c.site_url || ''
    case 'yandex': return c.host_id || ''
  }
  return ''
}

function openDetail(c: any) {
  let siteKey = ''
  switch (platform.value) {
    case 'gsc':    siteKey = c.site_url; break
    case 'ga4':    siteKey = c.property_id; break
    case 'bing':   siteKey = c.site_url; break
    case 'yandex': siteKey = c.host_id; break
  }
  if (!siteKey) { ElMessage.warning('无法打开详情：站点标识缺失'); return }
  if (c.error) { ElMessage.warning(`该站点当前不可访问：${c.error}`); return }
  view.value = 'detail'
  subTab.value = 'overview'
  detail.siteName = c.display_name || siteKey
  loadDetail(siteKey, c.display_name || siteKey)
}

function backToOverview() {
  view.value = 'overview'
  subTab.value = 'overview'
  error.value = ''
  apiErrors.value = null
  // Reset deep data state
  gscOverview.value = null
  gscBuckets.value = []
  activeBucket.value = ''
}

async function loadDetail(siteKey: string, siteName: string) {
  loading.value = true
  error.value = ''
  apiErrors.value = null
  detail.siteKey = siteKey
  detail.siteName = siteName
  detail.kpis = {}
  detail.trendRows = []
  detail.keywords = []
  detail.pages = []
  detail.devices = []
  detail.countries = []

  const key = cacheKey(siteKey)
  const cached = cacheGet(key)
  if (cached) {
    digestDetail(cached)
    loading.value = false
    // Resolve site_id then load deep data
    resolveSiteIdFromBackend().then(() => loadDeepOverviewData())
    return
  }

  const baseParams = { start_date: dateRange.value[0], end_date: dateRange.value[1] }
  try {
    let res: any
    switch (platform.value) {
      case 'gsc':    res = await api.dashGSCSiteDetail({ ...baseParams, site_url: siteKey }); break
      case 'ga4':    res = await api.dashGAPropertyDetail({ ...baseParams, property_id: siteKey }); break
      case 'bing':   res = await api.dashBingSiteDetail({ ...baseParams, site_url: siteKey }); break
      case 'yandex': res = await api.dashYandexHostDetail({ ...baseParams, host_id: siteKey }); break
    }
    cacheSet(key, res.data)
    digestDetail(res.data)
  } catch (e: any) {
    error.value = e?.response?.data?.error || e?.message || '加载详情失败'
  } finally {
    loading.value = false
  }

  // Resolve site_id then load deep data
  await resolveSiteIdFromBackend()
  loadDeepOverviewData()
}

function digestDetail(d: any) {
  if (!d) return
  switch (platform.value) {
    case 'gsc':    return digestGSC(d)
    case 'ga4':    return digestGA(d)
    case 'bing':   return digestBing(d)
    case 'yandex': return digestYandex(d)
  }
}

function digestGSC(d: any) {
  const ov = (d.overview?.rows || [])[0] || {}
  detail.kpis = {
    clicks: ov.clicks || 0, impressions: ov.impressions || 0,
    ctr: ov.ctr || 0, position: ov.position || 0,
  }
  detail.trendRows = (d.trend?.rows || []).map((r: any) => ({
    date: r.keys?.[0] || '',
    clicks: r.clicks || 0, impressions: r.impressions || 0,
    ctr: r.ctr || 0,
  }))
  detail.keywords = (d.keywords?.rows || []).map((r: any) => ({
    query: r.keys?.[0] || '',
    clicks: r.clicks || 0, impressions: r.impressions || 0,
    ctr: r.ctr || 0, position: r.position || 0,
  }))
  detail.pages = (d.pages?.rows || []).map((r: any) => ({
    page: r.keys?.[0] || '',
    clicks: r.clicks || 0, impressions: r.impressions || 0,
    ctr: r.ctr || 0, position: r.position || 0,
  }))
  detail.devices = barize((d.devices?.rows || []).map((r: any) => ({
    label: deviceLabel(r.keys?.[0]), clicks: r.clicks || 0,
  })))
  detail.countries = barize((d.countries?.rows || []).slice(0, 10).map((r: any) => ({
    label: r.keys?.[0] || '', clicks: r.clicks || 0,
  })))
}

function digestGA(d: any) {
  const toNum = (v: any) => parseFloat(v?.value ?? v ?? '0') || 0
  const ovRow = (d.overview?.rows || [])[0]
  if (ovRow?.metricValues) {
    const m = ovRow.metricValues
    detail.kpis = {
      sessions: toNum(m[0]), users: toNum(m[1]), newUsers: toNum(m[2]),
      pageviews: toNum(m[3]), bounceRate: toNum(m[4]),
      engagementRate: toNum(m[6]),
    }
  }
  detail.trendRows = (d.trend?.rows || []).map((r: any) => {
    const raw = r.dimensionValues?.[0]?.value || ''
    const dt = raw.length === 8
      ? `${raw.slice(0, 4)}-${raw.slice(4, 6)}-${raw.slice(6, 8)}` : raw
    return {
      date: dt,
      clicks: toNum(r.metricValues?.[0]),
      impressions: toNum(r.metricValues?.[1]),
      ctr: 0,
    }
  })
  detail.keywords = []
  detail.pages = (d.pages?.rows || []).map((r: any) => ({
    page: r.dimensionValues?.[0]?.value || '',
    clicks: toNum(r.metricValues?.[0]),
    impressions: toNum(r.metricValues?.[1]),
    ctr: 0, position: 0,
  }))
  detail.devices = barize((d.devices?.rows || []).map((r: any) => ({
    label: deviceLabel(r.dimensionValues?.[0]?.value),
    clicks: toNum(r.metricValues?.[0]),
  })))
  detail.countries = barize((d.countries?.rows || []).slice(0, 10).map((r: any) => ({
    label: r.dimensionValues?.[0]?.value || '',
    clicks: toNum(r.metricValues?.[0]),
  })))
}

function digestBing(d: any) {
  apiErrors.value = d.api_errors && Object.keys(d.api_errors).length ? d.api_errors : null
  const posOf = (r: any) => Number(r?.AvgImpressionPosition || r?.AvgClickPosition || 0)
  const rows = (d.queries?.d || []).filter((r: any) => r && typeof r === 'object')
  let clicks = 0, impr = 0, wPos = 0
  rows.forEach((r: any) => {
    const c = Number(r.Clicks || 0)
    const i = Number(r.Impressions || 0)
    clicks += c; impr += i
    wPos += posOf(r) * i
  })
  detail.kpis = {
    clicks, impressions: impr,
    ctr: impr > 0 ? clicks / impr : 0,
    position: impr > 0 ? wPos / impr : 0,
  }
  detail.keywords = rows.slice(0, 20).map((r: any) => ({
    query: r.Query || '',
    clicks: Number(r.Clicks || 0), impressions: Number(r.Impressions || 0),
    ctr: r.Impressions ? Number(r.Clicks || 0) / Number(r.Impressions) : 0,
    position: posOf(r),
  }))
  const pages = (d.pages?.d || []).filter((r: any) => r && typeof r === 'object')
  detail.pages = pages.slice(0, 20).map((r: any) => ({
    page: r.Page || r.Url || '',
    clicks: Number(r.Clicks || 0), impressions: Number(r.Impressions || 0),
    ctr: r.Impressions ? Number(r.Clicks || 0) / Number(r.Impressions) : 0,
    position: posOf(r),
  }))
  detail.trendRows = []; detail.devices = []; detail.countries = []
}

function digestYandex(d: any) {
  const ind = d.history?.indicators || {}
  const clicks = sumArr(ind.TOTAL_CLICKS)
  const shows = sumArr(ind.TOTAL_SHOWS)
  detail.kpis = {
    clicks, impressions: shows,
    ctr: shows > 0 ? clicks / shows : 0, position: 0,
  }
  const clicksArr = ind.TOTAL_CLICKS || []
  const showsArr = ind.TOTAL_SHOWS || []
  detail.trendRows = clicksArr.map((p: any, i: number) => ({
    date: p.date || '',
    clicks: p.value || 0,
    impressions: (showsArr[i]?.value) || 0,
    ctr: 0,
  }))
  const popular = d.popular?.queries || d.popular?.popular_queries || []
  detail.keywords = popular.slice(0, 20).map((q: any) => {
    const inds = q.indicators || {}
    const kClicks = inds.TOTAL_CLICKS || 0
    const kShows = inds.TOTAL_SHOWS || 0
    return {
      query: q.query_text || q.query || '',
      clicks: kClicks, impressions: kShows,
      ctr: kShows ? kClicks / kShows : 0,
      position: inds.AVG_SHOW_POSITION || 0,
    }
  })
  detail.pages = []; detail.devices = []; detail.countries = []
}

// ── Helpers ─────────────────────────────────────────────────────────────
function sumArr(arr: any): number {
  if (!Array.isArray(arr)) return 0
  return arr.reduce((s, p) => s + (p.value || 0), 0)
}
function deviceLabel(raw: string): string {
  const lbl: Record<string, string> = {
    MOBILE: '手机', DESKTOP: '桌面', TABLET: '平板',
    mobile: '手机', desktop: '桌面', tablet: '平板',
  }
  return lbl[raw] || raw || '未知'
}
function barize(items: { label: string; clicks: number }[]) {
  const total = items.reduce((s, x) => s + x.clicks, 0) || 1
  return items.map(x => ({ ...x, pct: (x.clicks / total) * 100 }))
}
function cleanPath(u: string): string {
  if (!u) return ''
  return u.replace(/^https?:\/\/[^/]+/, '') || '/'
}
function fmt(n: number | undefined | null): string {
  if (n == null || isNaN(Number(n))) return '0'
  const v = Number(n)
  if (v >= 1e6) return (v / 1e6).toFixed(2) + 'M'
  if (v >= 1e3) return (v / 1e3).toFixed(1) + 'K'
  return Math.round(v).toLocaleString()
}
const pct = (v: number) => v != null ? (v * 100).toFixed(2) + '%' : '-'
const barWidth = (val: number, max: number) => max > 0 ? Math.max(2, (val / max) * 100) : 0

// ── Intent helpers ──
const intentLabel = (i: string) => ({ informational: '信息', transactional: '交易', commercial: '商业', navigational: '导航' }[i] || i || '-')
const intentType = (i: string): any => ({ informational: 'info', transactional: 'success', commercial: 'warning', navigational: '' }[i] || 'info')

// ── SERP label ──
const serpLabel = (s: string) => ({
  AI_OVERVIEW: '🤖 AI Overview', FEATURED_SNIPPET: '⭐ 精选摘要', TPF_FAQ: '❓ FAQ',
  REVIEW_SNIPPET: '⭐ 评分', VIDEO_RESULT: '🎬 视频', RICH_RESULT: '✨ 富结果',
  PRODUCT_SNIPPETS: '🛒 产品', RECIPE_RICH_SNIPPET: '🍳 食谱',
}[s] || s)

// ── Index state helpers ──
const isOkState = (s: string) => s?.includes('indexed') || s?.includes('Submitted and indexed')
const isWarnState = (s: string) => s?.includes('not indexed') || s?.includes('Discovered')
const isErrState = (s: string) => s?.includes('404') || s?.includes('noindex') || s?.includes('Soft 404') || s?.includes('robots')

// ── KPIs for detail strip ───────────────────────────────────────────────
const detailKPIs = computed(() => {
  const k = detail.kpis
  switch (platform.value) {
    case 'gsc':
    case 'bing':
    case 'yandex':
      return [
        { label: '点击', value: fmt(k.clicks), class: 'sv-blue' },
        { label: '展示', value: fmt(k.impressions), class: 'sv-green' },
        { label: 'CTR', value: ((k.ctr || 0) * 100).toFixed(2) + '%', class: 'sv-orange' },
        { label: '平均排名', value: (k.position || 0).toFixed(1), class: 'sv-purple' },
      ]
    case 'ga4':
      return [
        { label: '会话', value: fmt(k.sessions), class: 'sv-blue' },
        { label: '用户', value: fmt(k.users), class: 'sv-green' },
        { label: '浏览量', value: fmt(k.pageviews), class: 'sv-orange' },
        { label: '跳出率', value: ((k.bounceRate || 0) * 100).toFixed(1) + '%', class: 'sv-purple' },
      ]
  }
  return []
})

// ── Trend chart math ────────────────────────────────────────────────────
const trendW = 800
const trendH = 180
const trendLinePoints = computed(() => {
  const rows = detail.trendRows
  if (rows.length < 2) return ''
  const maxV = Math.max(...rows.map(r => r.clicks || 0), 1)
  return rows.map((r, i) => {
    const x = (i / (rows.length - 1)) * trendW
    const y = trendH - ((r.clicks || 0) / maxV) * (trendH - 10) - 5
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
})
const trendAreaPoints = computed(() => {
  const line = trendLinePoints.value
  if (!line) return ''
  return `0,${trendH} ${line} ${trendW},${trendH}`
})

// ═══════════════════════════════════════════════════════════════════════
//  DEEP DATA STATE & LOADERS
// ═══════════════════════════════════════════════════════════════════════

// ── GSC deep state ──
const gscOverview = ref<any>(null)
const gscBuckets = ref<any[]>([])
const latestBuckets = computed(() => {
  if (!gscBuckets.value.length) return []
  const d = gscBuckets.value[0]?.date
  return gscBuckets.value.filter((b: any) => b.date === d)
})
const activeBucket = ref('')
const activeBucketData = computed(() => {
  if (!activeBucket.value) return null
  return latestBuckets.value.find((b: any) => b.bucket === activeBucket.value)
})
const deepTrendMax = computed(() => Math.max(...(gscOverview.value?.trend?.map((d: any) => d.clicks) || [1])))

const kwData = ref<any>({ items: [], total: 0 }); const kwLoading = ref(false)
const kwSearch = ref(''); const kwBucket = ref(''); const kwIntent = ref(''); const kwBrand = ref(''); const kwQuestion = ref(false)
const kwPage = ref(1); const kwPageSize = ref(25)
const pgData = ref<any>({ items: [], total: 0 }); const pgLoading = ref(false); const pgSearch = ref(''); const pgPage = ref(1); const pgPageSize = ref(25)
const appearanceData = ref<any[]>([])
const idxData = ref<any>({ items: [], total: 0 }); const idxSummary = ref<any[]>([]); const idxCoverage = ref(''); const idxPage = ref(1); const inspecting = ref(false)
const sitemapData = ref<any[]>([])
const oppData = ref<any[]>([])
const cannibalData = ref<any[]>([])
const diffData = ref<any[]>([]); const diffType = ref('lost')
const kwDetailVisible = ref(false); const kwDetailData = ref<any>(null)

// ── GA4 deep state ──
const ga4Loading = ref(false)
const ga4Extended = ref<any[]>([]); const ga4LandingPages = ref<any[]>([]); const ga4Sources = ref<any[]>([]); const ga4Events = ref<any[]>([])
const ga4Tech = ref<any[]>([]); const ga4Geo = ref<any[]>([]); const ga4Hourly = ref<any[]>([])
const ga4DayOfWeek = ref<any[]>([]); const ga4Scroll = ref<any[]>([]); const ga4Outbound = ref<any[]>([]); const ga4Conversions = ref<any[]>([])
const techDim = ref('device')

// ── Bing deep state ──
const bingLoading = ref(false)
const bingKeywords = ref<any>({ items: [], total: 0 }); const bingPages = ref<any>({ items: [], total: 0 })
const bingBacklinks = ref<any[]>([]); const bingSitemapList = ref<any[]>([]); const bingKwSearch = ref(''); const bingKwPage = ref(1)

// ── Yandex deep state ──
const yandexLoading = ref(false)
const yandexKeywords = ref<any>({ items: [], total: 0 }); const yandexIndexEvents = ref<any[]>([]); const yandexHttpStatus = ref<any[]>([])
const yandexExtLinks = ref<any[]>([]); const yandexIndexedUrls = ref<any>({ items: [], total: 0 })

// ── Load deep overview data (called after site_id is resolved) ──
async function loadDeepOverviewData() {
  const sid = resolvedSiteId.value
  if (!sid) return

  if (platform.value === 'gsc') {
    Promise.all([
      api.getGSCOverviewDeep(sid, deepDays.value).then((r: any) => { gscOverview.value = r.data }).catch(() => {}),
      api.getGSCRankBuckets(sid, deepDays.value).then((r: any) => { gscBuckets.value = r.data || [] }).catch(() => {}),
    ])
  }
}

// ── Sub-tab data loader ──
function loadSubTabData(tab: string) {
  const sid = resolvedSiteId.value
  if (!sid && tab !== 'overview') return

  if (platform.value === 'gsc') {
    if (tab === 'keywords') loadKeywords()
    else if (tab === 'pages') loadPages()
    else if (tab === 'serp') loadAppearance()
    else if (tab === 'index') loadUrlInspection()
    else if (tab === 'sitemaps') loadSitemaps()
    else if (tab === 'opportunities') loadOpportunities()
    else if (tab === 'cannibal') loadCannibalization()
    else if (tab === 'diff') loadQueryDiff()
    else if (tab === 'buckets') { /* buckets already loaded */ }
  } else if (platform.value === 'ga4') {
    loadGA4Tab()
  } else if (platform.value === 'bing') {
    loadBingTab()
  } else if (platform.value === 'yandex') {
    loadYandexTab()
  }
}

// ── Sync deep data ──
async function syncDeep() {
  const sid = resolvedSiteId.value
  if (!sid) { ElMessage.warning('无法解析站点 ID，请确认站点已在站点管理中注册'); return }
  syncing.value = true
  try {
    if (platform.value === 'gsc') await api.syncGSCDeep(sid)
    else if (platform.value === 'ga4') await api.syncGA4Deep(sid)
    else if (platform.value === 'bing') await api.syncBingDeep(sid)
    else if (platform.value === 'yandex') await api.syncYandexDeep(sid)
    ElMessage.success('深度数据同步成功')
    loadDeepOverviewData()
    if (subTab.value !== 'overview') loadSubTabData(subTab.value)
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || e?.message || '同步失败')
  } finally {
    syncing.value = false
  }
}

function drillBucket(bucket: string) { activeBucket.value = activeBucket.value === bucket ? '' : bucket }

// ── GSC loaders ──
async function loadKeywords() {
  const sid = resolvedSiteId.value; if (!sid) return
  kwLoading.value = true
  try {
    const r: any = await api.getGSCKeywords(sid, { page: kwPage.value, page_size: kwPageSize.value, days: deepDays.value, search: kwSearch.value || undefined, bucket: kwBucket.value || undefined, intent: kwIntent.value || undefined, brand: kwBrand.value || undefined, is_question: kwQuestion.value ? 'true' : undefined })
    kwData.value = r.data || { items: [], total: 0 }
  } catch {} finally { kwLoading.value = false }
}
async function loadPages() {
  const sid = resolvedSiteId.value; if (!sid) return
  pgLoading.value = true
  try {
    const r: any = await api.getGSCDeepPages(sid, { page: pgPage.value, page_size: pgPageSize.value, days: deepDays.value, search: pgSearch.value || undefined })
    pgData.value = r.data || { items: [], total: 0 }
  } catch {} finally { pgLoading.value = false }
}
async function loadAppearance() {
  const sid = resolvedSiteId.value; if (!sid) return
  try { const r: any = await api.getGSCAppearance(sid, deepDays.value); appearanceData.value = r.data || [] } catch {}
}
async function loadUrlInspection() {
  const sid = resolvedSiteId.value; if (!sid) return
  try { const r: any = await api.getGSCUrlInspection(sid, { page: idxPage.value, coverage_state: idxCoverage.value || undefined }); idxData.value = r.data || { items: [], total: 0 }; idxSummary.value = r.data?.summary || [] } catch {}
}
async function runUrlInspection() {
  const sid = resolvedSiteId.value; if (!sid) return
  inspecting.value = true
  try { await api.syncGSCURLInspection(sid); ElMessage.success('URL 收录检查完成'); loadUrlInspection() } catch {} finally { inspecting.value = false }
}
async function loadSitemaps() {
  const sid = resolvedSiteId.value; if (!sid) return
  try { const r: any = await api.getGSCSitemaps(sid); sitemapData.value = r.data || [] } catch {}
}
async function loadOpportunities() {
  const sid = resolvedSiteId.value; if (!sid) return
  try { const r: any = await api.getGSCOpportunities(sid); oppData.value = r.data || [] } catch {}
}
async function loadCannibalization() {
  const sid = resolvedSiteId.value; if (!sid) return
  try { const r: any = await api.getGSCCannibalization(sid); cannibalData.value = r.data || [] } catch {}
}
async function loadQueryDiff() {
  const sid = resolvedSiteId.value; if (!sid) return
  try { const r: any = await api.getGSCQueryDiff(sid, diffType.value); diffData.value = r.data || [] } catch {}
}
async function openKwDetail(query: string) {
  const sid = resolvedSiteId.value; if (!sid) return
  try { const r: any = await api.getGSCKeywordDetail(sid, query); kwDetailData.value = r.data; kwDetailVisible.value = true } catch {}
}
async function exportCSV() {
  const sid = resolvedSiteId.value; if (!sid) return
  try { const r: any = await api.getGSCKeywordsExport(sid, deepDays.value); const blob = new Blob([r.data], { type: 'text/csv' }); const a = document.createElement('a'); a.href = URL.createObjectURL(blob); a.download = 'gsc_keywords.csv'; a.click() } catch { ElMessage.error('导出失败') }
}

// ── GA4 loaders ──
async function loadGA4Tab() {
  const sid = resolvedSiteId.value; if (!sid) return
  ga4Loading.value = true
  try {
    const d = deepDays.value
    if (subTab.value === 'overview') { /* overview uses basic data */ }
    else if (subTab.value === 'landing') { const r: any = await api.getGA4LandingPages(sid, { days: d }); ga4LandingPages.value = r.data?.items || r.data || [] }
    else if (subTab.value === 'source') { const r: any = await api.getGA4SourceMedium(sid, d); ga4Sources.value = r.data || [] }
    else if (subTab.value === 'events') { const r: any = await api.getGA4Events(sid, d); ga4Events.value = r.data || [] }
    else if (subTab.value === 'tech') { const r: any = await api.getGA4Tech(sid, techDim.value, d); ga4Tech.value = r.data || [] }
    else if (subTab.value === 'geo') { const r: any = await api.getGA4Geo(sid, d); ga4Geo.value = r.data || [] }
    else if (subTab.value === 'hourly') { const r: any = await api.getGA4Hourly(sid, d); ga4Hourly.value = r.data || [] }
    else if (subTab.value === 'dow') { const r: any = await api.getGA4DayOfWeek(sid); ga4DayOfWeek.value = r.data || [] }
    else if (subTab.value === 'scroll') { const r: any = await api.getGA4ScrollDepth(sid, d); ga4Scroll.value = r.data || [] }
    else if (subTab.value === 'outbound') { const r: any = await api.getGA4OutboundClicks(sid, d); ga4Outbound.value = r.data || [] }
    else if (subTab.value === 'conversions') { const r: any = await api.getGA4ConversionsDaily(sid, d); ga4Conversions.value = r.data || [] }
  } catch {} finally { ga4Loading.value = false }
}

// ── Bing loaders ──
async function loadBingTab() {
  const sid = resolvedSiteId.value; if (!sid) return
  bingLoading.value = true
  try {
    if (subTab.value === 'keywords') { const r: any = await api.getBingKeywords(sid, { page: bingKwPage.value, search: bingKwSearch.value || undefined }); bingKeywords.value = r.data || { items: [], total: 0 } }
    else if (subTab.value === 'pages') { const r: any = await api.getBingDeepPages(sid); bingPages.value = r.data || { items: [], total: 0 } }
    else if (subTab.value === 'backlinks') { const r: any = await api.getBingInboundLinks(sid); bingBacklinks.value = r.data || [] }
    else if (subTab.value === 'bing_sitemaps') { const r: any = await api.getBingSitemaps(sid); bingSitemapList.value = r.data || [] }
  } catch {} finally { bingLoading.value = false }
}

// ── Yandex loaders ──
async function loadYandexTab() {
  const sid = resolvedSiteId.value; if (!sid) return
  yandexLoading.value = true
  try {
    const d = deepDays.value
    if (subTab.value === 'keywords') { const r: any = await api.getYandexKeywords(sid); yandexKeywords.value = r.data || { items: [], total: 0 } }
    else if (subTab.value === 'index_events') { const r: any = await api.getYandexIndexEvents(sid, d); yandexIndexEvents.value = r.data || [] }
    else if (subTab.value === 'http_status') { const r: any = await api.getYandexHTTPStatus(sid, d); yandexHttpStatus.value = r.data || [] }
    else if (subTab.value === 'external_links') { const r: any = await api.getYandexExternalLinks(sid); yandexExtLinks.value = r.data || [] }
    else if (subTab.value === 'indexed_urls') { const r: any = await api.getYandexIndexedUrls(sid); yandexIndexedUrls.value = r.data || { items: [], total: 0 } }
  } catch {} finally { yandexLoading.value = false }
}

// ── Init ────────────────────────────────────────────────────────────────
onMounted(async () => {
  loadOverview()
  // Load sites for deep data resolution
  try {
    const r: any = await api.listSites()
    allSites.value = r.data || []
  } catch {}
})
</script>

<style scoped>
.data-mgmt { padding:22px 28px; min-height:calc(100vh - 60px); background:var(--bg); }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:16px 22px; }
.toolbar { margin-bottom:20px; }
.tb-row { display:flex; align-items:center; gap:20px; flex-wrap:wrap; }
.tb-tabs { display:flex; gap:8px; }
.tb-date { display:flex; align-items:center; gap:8px; flex-wrap:wrap; margin-left:auto; }
.tb-meta { display:flex; gap:16px; margin-top:12px; padding-top:10px; border-top:1px dashed var(--border-light); }
.meta-item { font-size:12px; color:var(--ink-hint); font-family:'JetBrains Mono', monospace; }
.meta-warn { color:#92400E; background:#FEF3C7; padding:2px 10px; border-radius:10px; font-family:inherit; }

.pill { padding:6px 16px; border-radius:20px; border:1.5px solid var(--border-light); background:var(--surface-2); font-size:13px; font-weight:700; color:var(--ink-soft); cursor:pointer; font-family:inherit; transition:all .15s; }
.pill.active { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.pill:hover:not(.active) { background:var(--sun-light); color:var(--sun-deep); border-color:var(--peach); }
.pill-sm { padding:5px 12px; font-size:12px; }

.btn { padding:7px 16px; border-radius:8px; border:1.5px solid var(--border-light); background:var(--surface); font-size:13px; font-weight:700; color:var(--ink-mid); cursor:pointer; font-family:inherit; transition:all .15s; }
.btn:hover:not(:disabled) { border-color:var(--sun); color:var(--sun-deep); }
.btn:disabled { opacity:.5; cursor:not-allowed; }
.btn-refresh { background:var(--sun-light); color:var(--sun-deep); border-color:var(--peach); }
.btn-back { background:var(--surface-2); margin-right:18px; }
.btn-sync { background:linear-gradient(135deg,var(--sun-light),#FFF7ED); color:var(--sun-deep); border-color:var(--peach); margin-left:auto; }
.btn-sync:hover:not(:disabled) { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; }
.btn-link { background:none; border:none; color:var(--sun-deep); font-weight:700; cursor:pointer; font-size:12px; padding:4px 8px; }
.btn-link:hover { text-decoration:underline; }

.section-title { font-size:14px; font-weight:800; color:var(--ink); margin:0 0 14px; display:flex; align-items:center; gap:10px; padding-bottom:6px; border-bottom:1.5px solid var(--border-light); }
.site-count-badge { font-size:11px; font-weight:700; padding:2px 10px; background:var(--sun-light); color:var(--sun-deep); border-radius:10px; }

.view-overview { padding-top:4px; }
.card-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(280px,1fr)); gap:14px; }
.site-card { position:relative; background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r); padding:16px 18px; cursor:pointer; transition:all .15s; box-shadow:var(--s1); }
.card-hide-btn { position:absolute; top:6px; right:8px; width:24px; height:24px; border:none; background:transparent; color:var(--ink-hint); font-size:18px; line-height:1; font-weight:700; border-radius:6px; cursor:pointer; opacity:0; transition:all .15s; z-index:2; }
.site-card:hover .card-hide-btn { opacity:.75; }
.card-hide-btn:hover { opacity:1 !important; background:#FEE2E2; color:#B91C1C; }
.site-card:hover { border-color:var(--sun); box-shadow:var(--s-amber); transform:translateY(-1px); }
.site-card-empty { opacity:.75; }
.site-card-empty:hover { opacity:.95; }
.site-card-err { border-color:#FECACA; background:#FEF2F2; }

.card-head { display:flex; justify-content:space-between; align-items:flex-start; gap:8px; margin-bottom:6px; }
.card-title { font-size:14px; font-weight:800; color:var(--ink); word-break:break-all; line-height:1.35; }
.card-tag { font-size:10.5px; font-weight:700; padding:2px 8px; border-radius:10px; white-space:nowrap; flex-shrink:0; }
.tag-warn { background:#FEF3C7; color:#92400E; }
.tag-err { background:#FEE2E2; color:#B91C1C; }
.tag-muted { background:var(--surface-3); color:var(--ink-hint); }
.card-sub { font-size:11.5px; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; margin-bottom:12px; word-break:break-all; min-height:14px; }
.card-err-msg { font-size:12px; color:#B91C1C; background:#FEF2F2; padding:8px 10px; border-radius:6px; margin-top:6px; }

.card-kpis { display:grid; grid-template-columns:repeat(2,1fr); gap:10px 18px; }
.kpi { }
.kpi-label { font-size:10.5px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.06em; margin-bottom:3px; }
.kpi-value { font-size:16px; font-weight:800; font-family:'JetBrains Mono',monospace; }
.sv-blue { color:#3B82F6; }
.sv-green { color:#10B981; }
.sv-orange { color:#F59E0B; }
.sv-purple { color:#8B5CF6; }

.view-detail { padding-top:4px; }
.detail-head { display:flex; align-items:center; margin-bottom:18px; flex-wrap:wrap; gap:12px; }
.detail-title { font-size:16px; font-weight:800; color:var(--ink); display:flex; align-items:center; gap:10px; }
.title-platform { background:var(--sun-light); color:var(--sun-deep); padding:3px 12px; border-radius:10px; font-size:13px; }
.title-sep { color:var(--ink-hint); }
.title-site { font-family:'JetBrains Mono',monospace; font-size:14px; word-break:break-all; }

/* Sub-tab bar */
.sub-bar { padding:10px 16px; display:flex; flex-wrap:wrap; gap:4px; margin-bottom:14px; }
.sub-pill { padding:6px 14px; border:1px solid var(--border-light); border-radius:14px; cursor:pointer; background:var(--surface-2); font-size:12px; font-weight:700; transition:all .15s; display:flex; align-items:center; gap:4px; color:var(--ink-soft); font-family:inherit; }
.sub-pill:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); }
.sub-pill.active { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.sp-icon { font-size:13px; }

.kpi-strip { display:grid; grid-template-columns:repeat(4,1fr); gap:14px; margin-bottom:18px; }
.kpi-card { text-align:left; padding:16px 20px; }
.kpi-big { font-size:24px; }

.trend-panel { margin-bottom:18px; }
.trend-chart-wrap { height:160px; background:var(--surface-2); border-radius:8px; padding:8px; margin-bottom:6px; }
.trend-svg { width:100%; height:100%; display:block; }
.trend-labels { display:flex; justify-content:space-between; font-size:11px; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; padding:0 8px; }

.two-col { display:grid; grid-template-columns:1fr 1fr; gap:14px; margin-bottom:18px; }
@media (max-width:1000px) { .two-col { grid-template-columns:1fr; } }

.table { width:100%; border-collapse:collapse; }
.table th { padding:9px 12px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.07em; border-bottom:2px solid var(--border); background:var(--surface-3); }
.table td { padding:9px 12px; border-bottom:1px solid var(--border-light); font-size:12.5px; }
.table tr:hover td { background:var(--sun-light); }
.rk { width:36px; color:var(--ink-hint); font-family:'JetBrains Mono',monospace; font-weight:700; }
.num { font-family:'JetBrains Mono',monospace; font-weight:700; text-align:right; white-space:nowrap; }
.kw-cell { max-width:280px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.path-cell { font-family:'JetBrains Mono',monospace; font-size:11.5px; color:var(--ink-soft); max-width:260px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }

.bar-list { display:flex; flex-direction:column; gap:10px; }
.bar-head { display:flex; justify-content:space-between; font-size:12px; margin-bottom:4px; }
.bar-name { font-weight:700; color:var(--ink); }
.bar-val { font-family:'JetBrains Mono',monospace; color:var(--ink-hint); }
.bar-track { height:8px; background:var(--surface-3); border-radius:4px; overflow:hidden; }
.bar-fill { height:100%; background:linear-gradient(90deg,var(--sun),var(--coral)); border-radius:4px; transition:width .3s; }
.bar-green { background:linear-gradient(90deg,#10B981,#059669); }

.state-placeholder,
.state-empty,
.state-error { padding:60px 20px; text-align:center; color:var(--ink-hint); font-size:14px; }
.state-error { color:#B91C1C; }

.api-errors-banner { background:#FFFBEB; border:1.5px solid #FCD34D; border-radius:var(--r-sm); padding:12px 16px; margin-bottom:14px; }
.aeb-title { font-size:13px; font-weight:800; color:#92400E; margin-bottom:6px; }
.aeb-list { margin:0; padding-left:18px; }
.aeb-list li { font-size:12.5px; color:#713F12; line-height:1.6; }
.aeb-name { font-family:'JetBrains Mono', monospace; background:#FEF3C7; padding:1px 6px; border-radius:4px; margin-right:4px; font-weight:700; }
.err-title { font-size:15px; font-weight:700; margin-bottom:6px; }
.err-hint { font-size:12px; color:var(--ink-hint); }

/* ── Deep data styles ──────────────────────────────────────────────── */
.section { }
.empty-state { text-align:center; padding:60px 20px; color:var(--ink-hint); font-size:15px; }
.hint { color:var(--ink-hint); font-size:13px; margin:0 0 12px; }
.hint-bar { background:#FFFBEB; border:1.5px solid #FCD34D; border-radius:8px; padding:10px 16px; font-size:13px; color:#92400E; margin-bottom:14px; }
.loading-text { text-align:center; padding:40px; color:var(--ink-hint); }

/* Deep KPI row */
.deep-kpi-row { display:grid; grid-template-columns:repeat(auto-fit,minmax(140px,1fr)); gap:12px; margin-bottom:16px; }
.deep-kpi-card { background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r); padding:16px 14px; text-align:center; box-shadow:var(--s1); }

/* Deep trend bars */
.mini-chart { max-height:300px; overflow-y:auto; }
.deep-bar-row { display:flex; align-items:center; gap:8px; padding:3px 0; }
.deep-bar-date { font-size:11px; color:var(--ink-hint); width:45px; text-align:right; flex-shrink:0; font-family:'JetBrains Mono',monospace; }
.deep-bar-num { font-size:11px; color:var(--ink-mid); width:40px; font-family:'JetBrains Mono',monospace; }

/* Rank buckets */
.bucket-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(150px,1fr)); gap:12px; margin-bottom:16px; }
.bucket-card { background:var(--surface); border:2px solid var(--border-light); border-radius:12px; padding:16px; text-align:center; cursor:pointer; transition:all .2s; box-shadow:var(--s1); }
.bucket-card:hover { border-color:var(--sun); transform:translateY(-2px); box-shadow:var(--s-amber); }
.bucket-card.selected { border-color:var(--sun); background:var(--sun-light); }
.bk-name { font-size:18px; font-weight:800; color:var(--sun-deep); margin-bottom:8px; }
.bk-big { font-size:22px; font-weight:700; color:var(--ink); margin-bottom:6px; }
.bk-big small { font-size:12px; color:var(--ink-hint); font-weight:400; }
.bk-row { display:flex; justify-content:space-between; font-size:12px; color:var(--ink-soft); padding:2px 0; }
.bk-row b { color:var(--ink); }
.bk-footer { font-size:11px; color:var(--sun-deep); margin-top:8px; }

/* Filter & pager */
.filter-row { display:flex; gap:8px; align-items:center; margin-bottom:12px; flex-wrap:wrap; }
.pager { margin-top:12px; display:flex; justify-content:flex-end; align-items:center; gap:12px; }
.pager-info { font-size:13px; color:var(--ink-hint); }

/* Panel head */
.panel-head { display:flex; justify-content:space-between; align-items:center; margin-bottom:12px; }
.panel-head h3 { margin:0; }
h3 { margin:0 0 12px; font-size:16px; font-weight:700; color:var(--ink); }
h4 { margin:0 0 8px; font-size:14px; color:var(--ink-soft); font-weight:700; }

/* Link */
.link { color:var(--sun-deep); cursor:pointer; text-decoration:none; font-weight:600; }
.link:hover { text-decoration:underline; }

/* SERP cards */
.serp-cards { display:grid; grid-template-columns:repeat(auto-fill,minmax(280px,1fr)); gap:10px; }
.serp-card { background:var(--surface-2); border:1.5px solid var(--border-light); border-radius:8px; padding:12px 16px; }
.sc-name { font-weight:700; margin-bottom:6px; color:var(--ink); }
.sc-stats { display:flex; gap:14px; font-size:12px; color:var(--ink-soft); }
.sc-stats b { color:var(--ink); }

/* Index summary */
.idx-summary { display:flex; gap:8px; flex-wrap:wrap; margin-bottom:12px; }
.idx-card { padding:10px 16px; border-radius:8px; border:2px solid var(--border-light); cursor:pointer; text-align:center; transition:all .15s; min-width:100px; background:var(--surface); }
.idx-card:hover { border-color:var(--sun); }
.idx-card.selected { border-color:var(--sun); background:var(--sun-light); }
.idx-card.ok { background:#F0FDF4; border-color:#BBF7D0; }
.idx-card.warn { background:#FFFBEB; border-color:#FDE68A; }
.idx-card.err { background:#FEF2F2; border-color:#FECACA; }
.ic-count { font-size:22px; font-weight:700; color:var(--ink); }
.ic-label { font-size:10px; color:var(--ink-hint); margin-top:2px; word-break:break-all; max-width:120px; }

/* Cannibalization */
.cannibal-line { font-size:12px; line-height:1.6; display:flex; gap:8px; }
.cl-url { color:var(--sun-deep); max-width:250px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.cl-stat { color:var(--ink-hint); white-space:nowrap; }
.cannibal-warn { background:#FEF2F2; border:1.5px solid #FECACA; border-radius:6px; padding:8px 14px; font-size:13px; color:#B91C1C; margin-bottom:8px; }

/* Keyword detail dialog */
.kw-detail {}
.kd-header { margin-bottom:16px; }
.kd-query { font-size:18px; font-weight:800; margin-bottom:8px; color:var(--ink); }
.kd-tags { display:flex; gap:6px; align-items:center; flex-wrap:wrap; }
.kd-meta { color:var(--ink-hint); font-size:12px; }
.mini-cards { display:flex; gap:10px; flex-wrap:wrap; }
.mini-card { background:var(--surface-2); border:1.5px solid var(--border-light); border-radius:8px; padding:10px 14px; min-width:120px; }
.mc-title { font-weight:700; font-size:13px; margin-bottom:6px; text-transform:capitalize; color:var(--ink); }
.mc-row { display:flex; justify-content:space-between; font-size:12px; color:var(--ink-soft); padding:1px 0; }
.mc-row b { color:var(--ink); }
</style>
