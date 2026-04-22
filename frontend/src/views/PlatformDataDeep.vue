<template>
  <div class="deep-data">
    <!-- ─── Top bar ──────────────────────────────────────────── -->
    <div class="panel toolbar">
      <div class="tb-row">
        <div class="tb-left">
          <label>站点：</label>
          <el-select v-model="siteId" filterable placeholder="选择站点" @change="onSiteChange" style="width:280px">
            <el-option v-for="s in sites" :key="s.id" :label="`${s.name} (${s.domain})`" :value="s.id" />
          </el-select>
        </div>
        <div class="tb-tabs">
          <button v-for="p in platforms" :key="p.code" :class="['pill', platform === p.code && 'active']" @click="platform = p.code">{{ p.label }}</button>
        </div>
        <div class="tb-right">
          <el-select v-model="days" style="width:120px" @change="refresh">
            <el-option :value="7" label="近 7 天" /><el-option :value="30" label="近 30 天" /><el-option :value="90" label="近 90 天" />
          </el-select>
          <el-button type="primary" :loading="syncing" @click="syncDeep">
            {{ syncing ? '同步中...' : '🔄 同步深度数据' }}
          </el-button>
        </div>
      </div>
    </div>
    <div v-if="!siteId" class="empty-state">📊 请先选择一个站点开始分析</div>

    <!-- ═══════════════════════════════════════════════════════════ -->
    <!--  GSC Platform                                              -->
    <!-- ═══════════════════════════════════════════════════════════ -->
    <template v-if="siteId && platform === 'gsc'">
      <!-- Sub-tab bar -->
      <div class="panel sub-bar">
        <button v-for="t in gscTabs" :key="t.key" :class="['sub-pill', gscTab === t.key && 'active']" @click="gscTab = t.key">
          <span class="sp-icon">{{ t.icon }}</span>{{ t.label }}
        </button>
      </div>

      <!-- ── GSC 总览 ─────────────────────────────────────────── -->
      <template v-if="gscTab === 'overview'">
        <div v-if="!gscOverview && !loading" class="empty-state">
          <p>暂无数据。点击上方<b>「同步深度数据」</b>拉取 GSC 全量关键词 / 页面 / SERP 特征数据。</p>
          <p style="color:#aaa;font-size:12px">同步一次约需 1~3 分钟，取决于站点规模。</p>
        </div>
        <div v-if="gscOverview" class="section">
          <!-- KPI cards -->
          <div class="kpi-row">
            <div class="kpi-card"><div class="kpi-label">总点击</div><div class="kpi-val blue">{{ fmt(gscOverview.total_clicks) }}</div></div>
            <div class="kpi-card"><div class="kpi-label">总曝光</div><div class="kpi-val green">{{ fmt(gscOverview.total_impressions) }}</div></div>
            <div class="kpi-card"><div class="kpi-label">平均 CTR</div><div class="kpi-val orange">{{ pct(gscOverview.avg_ctr) }}</div></div>
            <div class="kpi-card"><div class="kpi-label">平均排名</div><div class="kpi-val purple">{{ gscOverview.avg_position?.toFixed(1) }}</div></div>
            <div class="kpi-card"><div class="kpi-label">关键词数</div><div class="kpi-val">{{ fmt(gscOverview.total_queries) }}</div></div>
            <div class="kpi-card"><div class="kpi-label">页面数</div><div class="kpi-val">{{ fmt(gscOverview.total_pages) }}</div></div>
          </div>
          <div v-if="!gscOverview.has_deep_data" class="hint-bar">
            ⚠ 当前显示的是基础同步数据。点击「同步深度数据」可获取完整关键词明细、排名分段、SERP 特征、收录状态等深度分析。
          </div>
          <!-- Trend chart (simple ASCII-style bar) -->
          <div class="panel" v-if="gscOverview.trend?.length">
            <h3>📈 每日趋势（点击）</h3>
            <div class="mini-chart">
              <div class="bar-row" v-for="d in gscOverview.trend" :key="d.date">
                <span class="bar-date">{{ d.date?.slice(5) }}</span>
                <div class="bar-track"><div class="bar-fill" :style="{width: barWidth(d.clicks, trendMax)+'%'}"></div></div>
                <span class="bar-num">{{ d.clicks }}</span>
              </div>
            </div>
          </div>
          <!-- Top keywords -->
          <div class="panel" v-if="gscOverview.top_keywords?.length">
            <div class="panel-head">
              <h3>🔑 TOP 关键词</h3>
              <el-button link type="primary" @click="gscTab='keywords'">查看全部 →</el-button>
            </div>
            <el-table :data="gscOverview.top_keywords" stripe size="small" max-height="400">
              <el-table-column type="index" label="#" width="40" />
              <el-table-column prop="query" label="关键词" min-width="220" show-overflow-tooltip>
                <template #default="{row}">
                  <a class="link" @click="openKwDetail(row.query)">{{ row.query }}</a>
                </template>
              </el-table-column>
              <el-table-column prop="clicks" label="点击" width="80" sortable />
              <el-table-column prop="impressions" label="曝光" width="90" sortable />
              <el-table-column label="CTR" width="80" :formatter="(_:any,__:any,___:any,r:any) => pct(r.ctr)" />
              <el-table-column label="排名" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
            </el-table>
          </div>
          <!-- Top pages -->
          <div class="panel" v-if="gscOverview.top_pages?.length">
            <div class="panel-head">
              <h3>📄 TOP 页面</h3>
              <el-button link type="primary" @click="gscTab='pages'">查看全部 →</el-button>
            </div>
            <el-table :data="gscOverview.top_pages" stripe size="small" max-height="400">
              <el-table-column type="index" label="#" width="40" />
              <el-table-column prop="page_url" label="页面 URL" min-width="320" show-overflow-tooltip />
              <el-table-column prop="clicks" label="点击" width="80" sortable />
              <el-table-column prop="impressions" label="曝光" width="90" sortable />
              <el-table-column label="CTR" width="80" :formatter="(_:any,__:any,___:any,r:any) => pct(r.ctr)" />
              <el-table-column label="排名" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
            </el-table>
          </div>
        </div>
      </template>

      <!-- ── GSC 排名分布 ─────────────────────────────────────── -->
      <template v-if="gscTab === 'buckets'">
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
          <!-- Drill-down: show top queries/pages of selected bucket -->
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

      <!-- ── GSC 关键词分析 ───────────────────────────────────── -->
      <template v-if="gscTab === 'keywords'">
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

      <!-- ── GSC 页面 ─────────────────────────────────────────── -->
      <template v-if="gscTab === 'pages'">
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

      <!-- ── GSC SERP 特征 ────────────────────────────────────── -->
      <template v-if="gscTab === 'serp'">
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

      <!-- ── GSC 收录状态 ─────────────────────────────────────── -->
      <template v-if="gscTab === 'index'">
        <div class="panel">
          <div class="panel-head">
            <h3>📋 收录状态</h3>
            <el-button type="warning" size="small" :loading="inspecting" @click="runUrlInspection">🔍 检查 TOP 100 页面收录</el-button>
          </div>
          <p class="hint" v-if="!idxSummary.length">暂无数据。点击上方按钮对 TOP 100 页面逐个检查收录状态（耗时约 2 分钟）。</p>
          <!-- Coverage summary cards -->
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

      <!-- ── GSC Sitemap ──────────────────────────────────────── -->
      <template v-if="gscTab === 'sitemaps'">
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

      <!-- ── GSC 机会 ─────────────────────────────────────────── -->
      <template v-if="gscTab === 'opportunities'">
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

      <!-- ── GSC 自蚕食 ───────────────────────────────────────── -->
      <template v-if="gscTab === 'cannibal'">
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

      <!-- ── GSC 关键词变动 ───────────────────────────────────── -->
      <template v-if="gscTab === 'diff'">
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
    </template>

    <!-- ═══════════════════════════════════════════════════════════ -->
    <!--  GA4 / Bing / Yandex (simplified but functional)          -->
    <!-- ═══════════════════════════════════════════════════════════ -->
    <template v-if="siteId && platform === 'ga4'">
      <div class="panel sub-bar">
        <button v-for="t in ga4Tabs" :key="t.key" :class="['sub-pill', ga4Tab === t.key && 'active']" @click="ga4Tab = t.key; loadGA4Tab()">{{ t.label }}</button>
      </div>
      <div class="panel">
        <div v-if="ga4Loading" class="loading-text">加载中...</div>
        <template v-if="ga4Tab === 'overview'">
          <h3>GA4 扩展指标</h3>
          <p class="hint" v-if="!ga4Extended.length">暂无数据。请先同步深度数据。</p>
          <el-table :data="ga4Extended" v-if="ga4Extended.length" stripe border size="small" max-height="500">
            <el-table-column prop="date" label="日期" width="100" />
            <el-table-column prop="total_users" label="总用户" width="80" /><el-table-column prop="active_users" label="活跃" width="65" />
            <el-table-column prop="sessions" label="会话" width="70" /><el-table-column prop="engaged_sessions" label="参与" width="65" />
            <el-table-column label="参与率" width="70" :formatter="(_:any,__:any,___:any,r:any) => pct(r.engagement_rate)" />
            <el-table-column label="跳出率" width="70" :formatter="(_:any,__:any,___:any,r:any) => pct(r.bounce_rate)" />
            <el-table-column prop="screen_page_views" label="PV" width="65" />
            <el-table-column prop="event_count" label="事件" width="65" /><el-table-column prop="conversions" label="转化" width="60" />
          </el-table>
        </template>
        <template v-if="ga4Tab === 'landing'"><h3>入站页</h3>
          <el-table :data="ga4LandingPages" v-if="ga4LandingPages.length" stripe border size="small">
            <el-table-column type="index" width="40" /><el-table-column prop="landing_page" label="入站页" min-width="300" show-overflow-tooltip />
            <el-table-column prop="sessions" label="会话" width="75" /><el-table-column prop="users" label="用户" width="70" />
            <el-table-column label="参与率" width="70" :formatter="(_:any,__:any,___:any,r:any) => pct(r.engagement_rate)" />
            <el-table-column label="跳出率" width="70" :formatter="(_:any,__:any,___:any,r:any) => pct(r.bounce_rate)" />
            <el-table-column prop="conversions" label="转化" width="60" />
          </el-table></template>
        <template v-if="ga4Tab === 'source'"><h3>流量来源</h3>
          <el-table :data="ga4Sources" v-if="ga4Sources.length" stripe border size="small">
            <el-table-column prop="source" label="来源" width="140" /><el-table-column prop="medium" label="媒介" width="110" /><el-table-column prop="channel_group" label="渠道" width="140" />
            <el-table-column prop="sessions" label="会话" width="75" /><el-table-column prop="users" label="用户" width="70" />
            <el-table-column label="参与率" width="70" :formatter="(_:any,__:any,___:any,r:any) => pct(r.engagement_rate)" /><el-table-column prop="conversions" label="转化" width="60" />
          </el-table></template>
        <template v-if="ga4Tab === 'events'"><h3>事件</h3>
          <el-table :data="ga4Events" v-if="ga4Events.length" stripe border size="small">
            <el-table-column prop="event_name" label="事件" min-width="200" /><el-table-column prop="event_count" label="次数" width="80" />
            <el-table-column prop="users" label="用户" width="70" /><el-table-column prop="event_value" label="值" width="80" />
            <el-table-column label="转化" width="55"><template #default="{row}">{{ row.is_conversion?'✓':'' }}</template></el-table-column>
          </el-table></template>
        <template v-if="ga4Tab === 'tech'"><h3>设备与技术</h3>
          <div class="filter-row"><el-radio-group v-model="techDim" @change="loadGA4Tab"><el-radio-button value="device">设备</el-radio-button><el-radio-button value="os">OS</el-radio-button><el-radio-button value="browser">浏览器</el-radio-button></el-radio-group></div>
          <el-table :data="ga4Tech" v-if="ga4Tech.length" stripe border size="small">
            <el-table-column prop="value" label="维度" min-width="180" /><el-table-column prop="sessions" label="会话" width="80" /><el-table-column prop="users" label="用户" width="70" />
            <el-table-column label="参与率" width="75" :formatter="(_:any,__:any,___:any,r:any) => pct(r.engagement_rate)" />
          </el-table></template>
        <template v-if="ga4Tab === 'geo'"><h3>地理分布</h3>
          <el-table :data="ga4Geo" v-if="ga4Geo.length" stripe border size="small">
            <el-table-column prop="country" label="国家" width="110" /><el-table-column prop="region" label="地区" width="130" /><el-table-column prop="city" label="城市" width="130" />
            <el-table-column prop="sessions" label="会话" width="75" /><el-table-column prop="users" label="用户" width="70" />
          </el-table></template>
        <template v-if="ga4Tab === 'hourly'"><h3>小时分布</h3>
          <el-table :data="ga4Hourly" v-if="ga4Hourly.length" stripe border size="small">
            <el-table-column label="小时" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.hour+':00'" /><el-table-column prop="sessions" label="会话" width="80" /><el-table-column prop="users" label="用户" width="70" /><el-table-column prop="screen_page_views" label="PV" width="70" />
          </el-table></template>
        <template v-if="ga4Tab === 'dow'"><h3>星期分布</h3>
          <el-table :data="ga4DayOfWeek" v-if="ga4DayOfWeek.length" stripe border size="small">
            <el-table-column prop="name" label="星期" width="90" /><el-table-column prop="sessions" label="会话" width="80" /><el-table-column prop="users" label="用户" width="70" /><el-table-column prop="conversions" label="转化" width="65" />
          </el-table></template>
        <template v-if="ga4Tab === 'scroll'"><h3>滚动深度</h3>
          <el-table :data="ga4Scroll" v-if="ga4Scroll.length" stripe border size="small">
            <el-table-column prop="page_path" label="页面" min-width="250" show-overflow-tooltip /><el-table-column label="滚动%" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.percent_scrolled+'%'" />
            <el-table-column prop="users" label="用户" width="70" /><el-table-column prop="event_count" label="次数" width="70" />
          </el-table></template>
        <template v-if="ga4Tab === 'outbound'"><h3>外链点击</h3>
          <el-table :data="ga4Outbound" v-if="ga4Outbound.length" stripe border size="small">
            <el-table-column prop="link_domain" label="域名" width="180" /><el-table-column prop="link_url" label="URL" min-width="300" show-overflow-tooltip />
            <el-table-column prop="event_count" label="点击" width="70" /><el-table-column prop="users" label="用户" width="70" />
          </el-table></template>
        <template v-if="ga4Tab === 'conversions'"><h3>转化明细</h3>
          <el-table :data="ga4Conversions" v-if="ga4Conversions.length" stripe border size="small">
            <el-table-column prop="event_name" label="事件" min-width="180" /><el-table-column prop="source" label="来源" width="120" /><el-table-column prop="medium" label="媒介" width="100" />
            <el-table-column prop="conversions" label="转化" width="70" /><el-table-column prop="total_revenue" label="收入" width="80" />
          </el-table></template>
      </div>
    </template>

    <template v-if="siteId && platform === 'bing'">
      <div class="panel sub-bar">
        <button v-for="t in bingTabs" :key="t.key" :class="['sub-pill', bingTab === t.key && 'active']" @click="bingTab = t.key; loadBingTab()">{{ t.label }}</button>
      </div>
      <div class="panel">
        <div v-if="bingLoading" class="loading-text">加载中...</div>
        <template v-if="bingTab === 'keywords'"><h3>Bing 关键词</h3>
          <el-input v-model="bingKwSearch" placeholder="搜索" clearable style="width:200px;margin-bottom:8px" @keyup.enter="loadBingTab" />
          <el-table :data="bingKeywords.items" v-if="bingKeywords.items?.length" stripe border size="small">
            <el-table-column type="index" width="40" /><el-table-column prop="query" label="关键词" min-width="240" show-overflow-tooltip />
            <el-table-column prop="clicks" label="点击" width="75" /><el-table-column prop="impressions" label="曝光" width="80" />
            <el-table-column label="CTR" width="75" :formatter="(_:any,__:any,___:any,r:any) => pct(r.ctr)" />
            <el-table-column label="位置" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.avg_impression_position?.toFixed(1)" />
          </el-table>
          <div class="pager" v-if="bingKeywords.total>25"><el-pagination layout="total, prev, pager, next" :total="bingKeywords.total" v-model:current-page="bingKwPage" @current-change="loadBingTab" /></div>
        </template>
        <template v-if="bingTab === 'pages'"><h3>Bing 页面</h3>
          <el-table :data="bingPages.items" v-if="bingPages.items?.length" stripe border size="small">
            <el-table-column prop="page_url" label="页面" min-width="320" show-overflow-tooltip /><el-table-column prop="clicks" label="点击" width="75" /><el-table-column prop="impressions" label="曝光" width="80" />
            <el-table-column label="CTR" width="75" :formatter="(_:any,__:any,___:any,r:any) => pct(r.ctr)" /><el-table-column label="位置" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
          </el-table></template>
        <template v-if="bingTab === 'backlinks'"><h3>Bing 反链</h3>
          <p class="hint" v-if="!bingBacklinks.length">暂无反链数据。</p>
          <el-table :data="bingBacklinks" v-if="bingBacklinks.length" stripe border size="small">
            <el-table-column prop="source_url" label="来源 URL" min-width="300" show-overflow-tooltip /><el-table-column prop="target_page" label="目标页" min-width="200" show-overflow-tooltip /><el-table-column prop="anchor_text" label="锚文本" width="140" />
          </el-table></template>
        <template v-if="bingTab === 'sitemaps'"><h3>Bing Sitemap</h3>
          <el-table :data="bingSitemapList" v-if="bingSitemapList.length" stripe border size="small">
            <el-table-column prop="sitemap_url" label="Sitemap" min-width="300" /><el-table-column prop="url_count" label="URL数" width="90" /><el-table-column prop="status" label="状态" width="90" />
          </el-table></template>
      </div>
    </template>

    <template v-if="siteId && platform === 'yandex'">
      <div class="panel sub-bar">
        <button v-for="t in yandexTabs" :key="t.key" :class="['sub-pill', yandexTab === t.key && 'active']" @click="yandexTab = t.key; loadYandexTab()">{{ t.label }}</button>
      </div>
      <div class="panel">
        <div v-if="yandexLoading" class="loading-text">加载中...</div>
        <template v-if="yandexTab === 'keywords'"><h3>Yandex 关键词</h3>
          <el-table :data="yandexKeywords.items" v-if="yandexKeywords.items?.length" stripe border size="small">
            <el-table-column type="index" width="40" /><el-table-column prop="query" label="关键词" min-width="240" show-overflow-tooltip />
            <el-table-column prop="clicks" label="点击" width="75" /><el-table-column prop="shows" label="曝光" width="80" />
            <el-table-column label="CTR" width="75" :formatter="(_:any,__:any,___:any,r:any) => pct(r.ctr)" />
            <el-table-column label="展示位" width="70" :formatter="(_:any,__:any,___:any,r:any) => r.avg_show_position?.toFixed(1)" />
          </el-table></template>
        <template v-if="yandexTab === 'index_events'"><h3>索引事件</h3>
          <el-table :data="yandexIndexEvents" v-if="yandexIndexEvents.length" stripe border size="small">
            <el-table-column prop="date" label="日期" width="100" /><el-table-column prop="event_type" label="事件" min-width="220" /><el-table-column prop="count" label="数量" width="75" />
          </el-table></template>
        <template v-if="yandexTab === 'http_status'"><h3>HTTP 状态</h3>
          <el-table :data="yandexHttpStatus" v-if="yandexHttpStatus.length" stripe border size="small">
            <el-table-column prop="date" label="日期" width="100" /><el-table-column prop="status_code" label="状态码" min-width="200" /><el-table-column prop="count" label="数量" width="75" />
          </el-table></template>
        <template v-if="yandexTab === 'external_links'"><h3>外链</h3>
          <el-table :data="yandexExtLinks" v-if="yandexExtLinks.length" stripe border size="small">
            <el-table-column prop="source_url" label="来源" min-width="300" show-overflow-tooltip /><el-table-column prop="target_url" label="目标" min-width="200" show-overflow-tooltip />
          </el-table></template>
        <template v-if="yandexTab === 'indexed_urls'"><h3>索引 URL 样本</h3>
          <el-table :data="yandexIndexedUrls.items" v-if="yandexIndexedUrls.items?.length" stripe border size="small">
            <el-table-column prop="url" label="URL" min-width="400" show-overflow-tooltip /><el-table-column prop="status" label="状态" width="90" /><el-table-column prop="http_code" label="HTTP" width="65" />
          </el-table></template>
      </div>
    </template>

    <!-- ── 关键词详情弹窗 ─────────────────────────────────────── -->
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
        <el-table :data="kwDetailData.pages" stripe size="small" border>
          <el-table-column type="index" width="35" /><el-table-column prop="page_url" label="页面" min-width="300" show-overflow-tooltip />
          <el-table-column prop="clicks" label="点击" width="65" /><el-table-column prop="impressions" label="曝光" width="75" />
          <el-table-column label="排名" width="65" :formatter="(_:any,__:any,___:any,r:any) => r.position?.toFixed(1)" />
        </el-table>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const sites = ref<any[]>([])
const siteId = ref<number | null>(null)
const platform = ref('gsc')
const days = ref(30)
const syncing = ref(false)
const loading = ref(false)
const platforms = [
  { code: 'gsc', label: 'GSC' }, { code: 'ga4', label: 'GA4' },
  { code: 'bing', label: 'Bing' }, { code: 'yandex', label: 'Yandex' },
]
const fmt = (n: number) => n?.toLocaleString?.() ?? '0'
const pct = (v: number) => v != null ? (v * 100).toFixed(2) + '%' : '-'
const barWidth = (val: number, max: number) => max > 0 ? Math.max(2, (val / max) * 100) : 0
const trendMax = computed(() => Math.max(...(gscOverview.value?.trend?.map((d: any) => d.clicks) || [1])))

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

// ── GSC state ──
const gscTabs = [
  { key: 'overview', label: '总览', icon: '📊' }, { key: 'buckets', label: '排名分布', icon: '📶' },
  { key: 'keywords', label: '关键词', icon: '🔑' }, { key: 'pages', label: '页面', icon: '📄' },
  { key: 'serp', label: 'SERP特征', icon: '🎯' }, { key: 'index', label: '收录状态', icon: '📋' },
  { key: 'sitemaps', label: 'Sitemap', icon: '🗺️' }, { key: 'opportunities', label: '机会', icon: '💡' },
  { key: 'cannibal', label: '自蚕食', icon: '⚠️' }, { key: 'diff', label: '变动', icon: '📊' },
]
const gscTab = ref('overview')
const gscOverview = ref<any>(null)
const gscBuckets = ref<any[]>([])
const latestBuckets = computed(() => { if (!gscBuckets.value.length) return []; const d = gscBuckets.value[0]?.date; return gscBuckets.value.filter((b: any) => b.date === d) })
const activeBucket = ref('')
const activeBucketData = computed(() => { if (!activeBucket.value) return null; return latestBuckets.value.find((b: any) => b.bucket === activeBucket.value) })
const kwData = ref<any>({ items: [], total: 0 }); const kwLoading = ref(false); const kwSearch = ref(''); const kwBucket = ref(''); const kwIntent = ref(''); const kwBrand = ref(''); const kwQuestion = ref(false); const kwPage = ref(1); const kwPageSize = ref(25)
const pgData = ref<any>({ items: [], total: 0 }); const pgLoading = ref(false); const pgSearch = ref(''); const pgPage = ref(1); const pgPageSize = ref(25)
const appearanceData = ref<any[]>([]); const appearanceLoading = ref(false)
const idxData = ref<any>({ items: [], total: 0 }); const idxSummary = ref<any[]>([]); const idxLoading = ref(false); const idxCoverage = ref(''); const idxPage = ref(1); const inspecting = ref(false)
const sitemapData = ref<any[]>([]); const sitemapLoading = ref(false)
const oppData = ref<any[]>([]); const oppLoading = ref(false)
const cannibalData = ref<any[]>([]); const cannibalLoading = ref(false)
const diffData = ref<any[]>([]); const diffLoading = ref(false); const diffType = ref('lost')
const kwDetailVisible = ref(false); const kwDetailData = ref<any>(null)

// ── GA4 state ──
const ga4Tabs = [
  { key: 'overview', label: '总览' }, { key: 'landing', label: '入站页' }, { key: 'source', label: '来源' },
  { key: 'events', label: '事件' }, { key: 'tech', label: '技术' }, { key: 'geo', label: '地理' },
  { key: 'hourly', label: '小时' }, { key: 'dow', label: '星期' }, { key: 'scroll', label: '滚动' },
  { key: 'outbound', label: '外链点击' }, { key: 'conversions', label: '转化' },
]
const ga4Tab = ref('overview'); const ga4Loading = ref(false)
const ga4Extended = ref<any[]>([]); const ga4LandingPages = ref<any[]>([]); const ga4Sources = ref<any[]>([]); const ga4Events = ref<any[]>([])
const ga4Tech = ref<any[]>([]); const ga4Geo = ref<any[]>([]); const ga4Hourly = ref<any[]>([])
const ga4DayOfWeek = ref<any[]>([]); const ga4Scroll = ref<any[]>([]); const ga4Outbound = ref<any[]>([]); const ga4Conversions = ref<any[]>([])
const techDim = ref('device')

// ── Bing state ──
const bingTabs = [{ key: 'keywords', label: '关键词' }, { key: 'pages', label: '页面' }, { key: 'backlinks', label: '反链' }, { key: 'sitemaps', label: 'Sitemap' }]
const bingTab = ref('keywords'); const bingLoading = ref(false)
const bingKeywords = ref<any>({ items: [], total: 0 }); const bingPages = ref<any>({ items: [], total: 0 })
const bingBacklinks = ref<any[]>([]); const bingSitemapList = ref<any[]>([]); const bingKwSearch = ref(''); const bingKwPage = ref(1)

// ── Yandex state ──
const yandexTabs = [{ key: 'keywords', label: '关键词' }, { key: 'index_events', label: '索引事件' }, { key: 'http_status', label: 'HTTP状态' }, { key: 'external_links', label: '外链' }, { key: 'indexed_urls', label: '索引URL' }]
const yandexTab = ref('keywords'); const yandexLoading = ref(false)
const yandexKeywords = ref<any>({ items: [], total: 0 }); const yandexIndexEvents = ref<any[]>([]); const yandexHttpStatus = ref<any[]>([])
const yandexExtLinks = ref<any[]>([]); const yandexIndexedUrls = ref<any>({ items: [], total: 0 })

onMounted(async () => { try { const r: any = await api.listSites(); sites.value = r.data || []; if (sites.value.length) { siteId.value = sites.value[0].id } } catch {} })
watch(siteId, () => { if (siteId.value) refresh() })
watch(platform, () => { if (siteId.value) refresh() })
function onSiteChange() { refresh() }

async function refresh() {
  if (!siteId.value) return
  if (platform.value === 'gsc') { loadGSCOverview(); loadGSCBuckets(); refreshGSCSubTab() }
  else if (platform.value === 'ga4') { loadGA4Tab() }
  else if (platform.value === 'bing') { loadBingTab() }
  else if (platform.value === 'yandex') { loadYandexTab() }
}
function refreshGSCSubTab() {
  const t = gscTab.value
  if (t === 'keywords') loadKeywords(); else if (t === 'pages') loadPages()
  else if (t === 'serp') loadAppearance(); else if (t === 'index') loadUrlInspection()
  else if (t === 'sitemaps') loadSitemaps(); else if (t === 'opportunities') loadOpportunities()
  else if (t === 'cannibal') loadCannibalization(); else if (t === 'diff') loadQueryDiff()
}
watch(gscTab, refreshGSCSubTab)

async function syncDeep() {
  if (!siteId.value) return; syncing.value = true
  try {
    if (platform.value === 'gsc') await api.syncGSCDeep(siteId.value)
    else if (platform.value === 'ga4') await api.syncGA4Deep(siteId.value)
    else if (platform.value === 'bing') await api.syncBingDeep(siteId.value)
    else if (platform.value === 'yandex') await api.syncYandexDeep(siteId.value)
    ElMessage.success('深度数据同步成功'); refresh()
  } catch (e: any) { ElMessage.error(e?.message || '同步失败') } finally { syncing.value = false }
}
function drillBucket(bucket: string) { activeBucket.value = activeBucket.value === bucket ? '' : bucket }

// ── GSC loaders ──
async function loadGSCOverview() { loading.value = true; try { const r: any = await api.getGSCOverviewDeep(siteId.value!, days.value); gscOverview.value = r.data } catch {} finally { loading.value = false } }
async function loadGSCBuckets() { try { const r: any = await api.getGSCRankBuckets(siteId.value!, days.value); gscBuckets.value = r.data || [] } catch {} }
async function loadKeywords() { kwLoading.value = true; try { const r: any = await api.getGSCKeywords(siteId.value!, { page: kwPage.value, page_size: kwPageSize.value, days: days.value, search: kwSearch.value || undefined, bucket: kwBucket.value || undefined, intent: kwIntent.value || undefined, brand: kwBrand.value || undefined, is_question: kwQuestion.value ? 'true' : undefined }); kwData.value = r.data || { items: [], total: 0 } } catch {} finally { kwLoading.value = false } }
async function loadPages() { pgLoading.value = true; try { const r: any = await api.getGSCDeepPages(siteId.value!, { page: pgPage.value, page_size: pgPageSize.value, days: days.value, search: pgSearch.value || undefined }); pgData.value = r.data || { items: [], total: 0 } } catch {} finally { pgLoading.value = false } }
async function loadAppearance() { appearanceLoading.value = true; try { const r: any = await api.getGSCAppearance(siteId.value!, days.value); appearanceData.value = r.data || [] } catch {} finally { appearanceLoading.value = false } }
async function loadUrlInspection() { idxLoading.value = true; try { const r: any = await api.getGSCUrlInspection(siteId.value!, { page: idxPage.value, coverage_state: idxCoverage.value || undefined }); idxData.value = r.data || { items: [], total: 0 }; idxSummary.value = r.data?.summary || [] } catch {} finally { idxLoading.value = false } }
async function runUrlInspection() { inspecting.value = true; try { await api.syncGSCURLInspection(siteId.value!); ElMessage.success('URL 收录检查完成'); loadUrlInspection() } catch {} finally { inspecting.value = false } }
async function loadSitemaps() { sitemapLoading.value = true; try { const r: any = await api.getGSCSitemaps(siteId.value!); sitemapData.value = r.data || [] } catch {} finally { sitemapLoading.value = false } }
async function loadOpportunities() { oppLoading.value = true; try { const r: any = await api.getGSCOpportunities(siteId.value!); oppData.value = r.data || [] } catch {} finally { oppLoading.value = false } }
async function loadCannibalization() { cannibalLoading.value = true; try { const r: any = await api.getGSCCannibalization(siteId.value!); cannibalData.value = r.data || [] } catch {} finally { cannibalLoading.value = false } }
async function loadQueryDiff() { diffLoading.value = true; try { const r: any = await api.getGSCQueryDiff(siteId.value!, diffType.value); diffData.value = r.data || [] } catch {} finally { diffLoading.value = false } }
async function openKwDetail(query: string) { try { const r: any = await api.getGSCKeywordDetail(siteId.value!, query); kwDetailData.value = r.data; kwDetailVisible.value = true } catch {} }
async function exportCSV() { try { const r: any = await api.getGSCKeywordsExport(siteId.value!, days.value); const blob = new Blob([r.data], { type: 'text/csv' }); const a = document.createElement('a'); a.href = URL.createObjectURL(blob); a.download = 'gsc_keywords.csv'; a.click() } catch { ElMessage.error('导出失败') } }

// ── GA4 loaders ──
async function loadGA4Tab() { ga4Loading.value = true; try { const s = siteId.value!, d = days.value
  if (ga4Tab.value === 'overview') { const r: any = await api.getGA4Extended(s, d); ga4Extended.value = r.data || [] }
  else if (ga4Tab.value === 'landing') { const r: any = await api.getGA4LandingPages(s, { days: d }); ga4LandingPages.value = r.data?.items || r.data || [] }
  else if (ga4Tab.value === 'source') { const r: any = await api.getGA4SourceMedium(s, d); ga4Sources.value = r.data || [] }
  else if (ga4Tab.value === 'events') { const r: any = await api.getGA4Events(s, d); ga4Events.value = r.data || [] }
  else if (ga4Tab.value === 'tech') { const r: any = await api.getGA4Tech(s, techDim.value, d); ga4Tech.value = r.data || [] }
  else if (ga4Tab.value === 'geo') { const r: any = await api.getGA4Geo(s, d); ga4Geo.value = r.data || [] }
  else if (ga4Tab.value === 'hourly') { const r: any = await api.getGA4Hourly(s, d); ga4Hourly.value = r.data || [] }
  else if (ga4Tab.value === 'dow') { const r: any = await api.getGA4DayOfWeek(s); ga4DayOfWeek.value = r.data || [] }
  else if (ga4Tab.value === 'scroll') { const r: any = await api.getGA4ScrollDepth(s, d); ga4Scroll.value = r.data || [] }
  else if (ga4Tab.value === 'outbound') { const r: any = await api.getGA4OutboundClicks(s, d); ga4Outbound.value = r.data || [] }
  else if (ga4Tab.value === 'conversions') { const r: any = await api.getGA4ConversionsDaily(s, d); ga4Conversions.value = r.data || [] }
} catch {} finally { ga4Loading.value = false } }

// ── Bing loaders ──
async function loadBingTab() { bingLoading.value = true; try { const s = siteId.value!
  if (bingTab.value === 'keywords') { const r: any = await api.getBingKeywords(s, { page: bingKwPage.value, search: bingKwSearch.value || undefined }); bingKeywords.value = r.data || { items: [], total: 0 } }
  else if (bingTab.value === 'pages') { const r: any = await api.getBingDeepPages(s); bingPages.value = r.data || { items: [], total: 0 } }
  else if (bingTab.value === 'backlinks') { const r: any = await api.getBingInboundLinks(s); bingBacklinks.value = r.data || [] }
  else if (bingTab.value === 'sitemaps') { const r: any = await api.getBingSitemaps(s); bingSitemapList.value = r.data || [] }
} catch {} finally { bingLoading.value = false } }

// ── Yandex loaders ──
async function loadYandexTab() { yandexLoading.value = true; try { const s = siteId.value!, d = days.value
  if (yandexTab.value === 'keywords') { const r: any = await api.getYandexKeywords(s); yandexKeywords.value = r.data || { items: [], total: 0 } }
  else if (yandexTab.value === 'index_events') { const r: any = await api.getYandexIndexEvents(s, d); yandexIndexEvents.value = r.data || [] }
  else if (yandexTab.value === 'http_status') { const r: any = await api.getYandexHTTPStatus(s, d); yandexHttpStatus.value = r.data || [] }
  else if (yandexTab.value === 'external_links') { const r: any = await api.getYandexExternalLinks(s); yandexExtLinks.value = r.data || [] }
  else if (yandexTab.value === 'indexed_urls') { const r: any = await api.getYandexIndexedUrls(s); yandexIndexedUrls.value = r.data || { items: [], total: 0 } }
} catch {} finally { yandexLoading.value = false } }
</script>

<style scoped>
.deep-data { padding: 0; }
.panel { background: #fff; border-radius: 10px; padding: 20px; margin-bottom: 14px; box-shadow: 0 1px 4px rgba(0,0,0,.04); }
.toolbar { padding: 14px 20px; }
.tb-row { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.tb-left { display: flex; align-items: center; gap: 8px; }
.tb-right { display: flex; align-items: center; gap: 8px; margin-left: auto; }
.tb-tabs { display: flex; gap: 4px; }
.pill { padding: 7px 16px; border: 1px solid #e0e0e0; border-radius: 18px; cursor: pointer; background: #fff; font-size: 13px; transition: all .15s; }
.pill:hover { background: #f5f5f5; }
.pill.active { background: #ff8c00; color: #fff; border-color: #ff8c00; }
.sub-bar { padding: 10px 16px; display: flex; flex-wrap: wrap; gap: 4px; }
.sub-pill { padding: 6px 14px; border: 1px solid #eee; border-radius: 14px; cursor: pointer; background: #fafafa; font-size: 12px; transition: all .15s; display: flex; align-items: center; gap: 4px; }
.sub-pill:hover { background: #fff3e0; border-color: #ffcc80; }
.sub-pill.active { background: #ff8c00; color: #fff; border-color: #ff8c00; }
.sp-icon { font-size: 13px; }
.section { }
.empty-state { text-align: center; padding: 60px 20px; color: #999; font-size: 15px; }
.hint { color: #aaa; font-size: 13px; margin: 0 0 12px; }
.hint-bar { background: #fffbe6; border: 1px solid #ffe58f; border-radius: 8px; padding: 10px 16px; font-size: 13px; color: #ad8b00; margin-bottom: 14px; }
.loading-text { text-align: center; padding: 40px; color: #aaa; }

/* KPI */
.kpi-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; margin-bottom: 16px; }
.kpi-card { background: linear-gradient(135deg, #fafbfc, #fff); border: 1px solid #f0f0f0; border-radius: 10px; padding: 16px 14px; text-align: center; }
.kpi-label { font-size: 12px; color: #999; margin-bottom: 6px; }
.kpi-val { font-size: 24px; font-weight: 700; color: #333; }
.kpi-val.blue { color: #409eff; } .kpi-val.green { color: #67c23a; } .kpi-val.orange { color: #e6a23c; } .kpi-val.purple { color: #9b59b6; }

/* Trend bars */
.mini-chart { max-height: 300px; overflow-y: auto; }
.bar-row { display: flex; align-items: center; gap: 8px; padding: 3px 0; }
.bar-date { font-size: 11px; color: #999; width: 45px; text-align: right; flex-shrink: 0; }
.bar-track { flex: 1; height: 18px; background: #f5f5f5; border-radius: 4px; overflow: hidden; }
.bar-fill { height: 100%; background: linear-gradient(90deg, #ffcc80, #ff8c00); border-radius: 4px; transition: width .3s; }
.bar-num { font-size: 11px; color: #666; width: 40px; }

/* Rank buckets */
.bucket-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 12px; margin-bottom: 16px; }
.bucket-card { background: #f8fbff; border: 2px solid #e8f0fe; border-radius: 12px; padding: 16px; text-align: center; cursor: pointer; transition: all .2s; }
.bucket-card:hover { border-color: #409eff; transform: translateY(-2px); box-shadow: 0 4px 12px rgba(64,158,255,.15); }
.bucket-card.selected { border-color: #ff8c00; background: #fff8f0; }
.bk-name { font-size: 18px; font-weight: 800; color: #1a73e8; margin-bottom: 8px; }
.bk-big { font-size: 22px; font-weight: 700; color: #333; margin-bottom: 6px; }
.bk-big small { font-size: 12px; color: #999; font-weight: 400; }
.bk-row { display: flex; justify-content: space-between; font-size: 12px; color: #666; padding: 2px 0; }
.bk-row b { color: #333; }
.bk-footer { font-size: 11px; color: #409eff; margin-top: 8px; }

/* Filter & pager */
.filter-row { display: flex; gap: 8px; align-items: center; margin-bottom: 12px; flex-wrap: wrap; }
.pager { margin-top: 12px; display: flex; justify-content: flex-end; align-items: center; gap: 12px; }
.pager-info { font-size: 13px; color: #999; }

/* Panel head */
.panel-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
.panel-head h3 { margin: 0; }
h3 { margin: 0 0 12px; font-size: 16px; font-weight: 600; }

/* Link */
.link { color: #409eff; cursor: pointer; text-decoration: none; }
.link:hover { text-decoration: underline; }

/* SERP cards */
.serp-cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 10px; }
.serp-card { background: #f9fafb; border: 1px solid #eee; border-radius: 8px; padding: 12px 16px; }
.sc-name { font-weight: 600; margin-bottom: 6px; }
.sc-stats { display: flex; gap: 14px; font-size: 12px; color: #666; }
.sc-stats b { color: #333; }

/* Index summary */
.idx-summary { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 12px; }
.idx-card { padding: 10px 16px; border-radius: 8px; border: 2px solid #eee; cursor: pointer; text-align: center; transition: all .15s; min-width: 100px; }
.idx-card:hover { border-color: #409eff; }
.idx-card.selected { border-color: #ff8c00; background: #fff8f0; }
.idx-card.ok { background: #f0f9eb; border-color: #c2e7b0; }
.idx-card.warn { background: #fdf6ec; border-color: #f5dab1; }
.idx-card.err { background: #fef0f0; border-color: #fbc4c4; }
.ic-count { font-size: 22px; font-weight: 700; }
.ic-label { font-size: 10px; color: #888; margin-top: 2px; word-break: break-all; max-width: 120px; }

/* Cannibalization */
.cannibal-line { font-size: 12px; line-height: 1.6; display: flex; gap: 8px; }
.cl-url { color: #409eff; max-width: 250px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cl-stat { color: #999; white-space: nowrap; }

/* Keyword detail */
.kw-detail {}
.kd-header { margin-bottom: 16px; }
.kd-query { font-size: 18px; font-weight: 700; margin-bottom: 8px; }
.kd-tags { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; }
.kd-meta { color: #999; font-size: 12px; }
h4 { margin: 0 0 8px; font-size: 14px; color: #555; }
</style>
