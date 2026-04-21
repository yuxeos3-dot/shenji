<template>
  <div class="deep-data">
    <!-- Site selector -->
    <div class="panel toolbar">
      <div class="tb-row">
        <div class="tb-left">
          <label>站点：</label>
          <el-select v-model="siteId" filterable placeholder="选择站点" @change="onSiteChange" style="width: 280px;">
            <el-option v-for="s in sites" :key="s.id" :label="`${s.name} (${s.domain})`" :value="s.id" />
          </el-select>
        </div>
        <div class="tb-tabs">
          <button v-for="p in platforms" :key="p.code" :class="['pill', platform === p.code && 'active']" @click="platform = p.code">
            {{ p.label }}
          </button>
        </div>
        <div class="tb-right">
          <el-select v-model="days" style="width: 120px;" @change="refresh">
            <el-option :value="7" label="最近 7 天" />
            <el-option :value="30" label="最近 30 天" />
            <el-option :value="90" label="最近 90 天" />
          </el-select>
          <el-button type="primary" :loading="syncing" @click="syncDeep">同步深度数据</el-button>
        </div>
      </div>
    </div>

    <div v-if="!siteId" class="state-placeholder">请先选择站点</div>

    <!-- GSC Tab -->
    <template v-if="siteId && platform === 'gsc'">
      <div class="panel toolbar sub-tabs">
        <button v-for="t in gscTabs" :key="t.key" :class="['pill', gscTab === t.key && 'active']" @click="gscTab = t.key">{{ t.label }}</button>
      </div>

      <!-- GSC Overview -->
      <div v-if="gscTab === 'overview'" class="panel">
        <div class="metric-cards" v-if="gscOverview">
          <div class="metric-card"><div class="mc-label">总点击</div><div class="mc-value">{{ fmt(gscOverview.total_clicks) }}</div></div>
          <div class="metric-card"><div class="mc-label">总曝光</div><div class="mc-value">{{ fmt(gscOverview.total_impressions) }}</div></div>
          <div class="metric-card"><div class="mc-label">CTR</div><div class="mc-value">{{ (gscOverview.avg_ctr * 100).toFixed(2) }}%</div></div>
          <div class="metric-card"><div class="mc-label">平均位置</div><div class="mc-value">{{ gscOverview.avg_position?.toFixed(1) }}</div></div>
          <div class="metric-card"><div class="mc-label">关键词数</div><div class="mc-value">{{ fmt(gscOverview.total_queries) }}</div></div>
          <div class="metric-card"><div class="mc-label">页面数</div><div class="mc-value">{{ fmt(gscOverview.total_pages) }}</div></div>
        </div>
        <div v-else class="state-placeholder">暂无深度数据，请先点击"同步深度数据"</div>
      </div>

      <!-- GSC Rank Buckets -->
      <div v-if="gscTab === 'buckets'" class="panel">
        <h3>排名分布</h3>
        <div class="bucket-grid" v-if="gscBuckets.length">
          <div class="bucket-card" v-for="b in latestBuckets" :key="b.bucket">
            <div class="bc-name">{{ b.bucket }}</div>
            <div class="bc-queries">{{ b.query_count }} 词</div>
            <div class="bc-clicks">{{ fmt(b.total_clicks) }} 点击</div>
            <div class="bc-imp">{{ fmt(b.total_impressions) }} 曝光</div>
            <div class="bc-ctr">{{ (b.avg_ctr * 100).toFixed(1) }}%</div>
          </div>
        </div>
        <div v-else class="state-placeholder">暂无排名段数据</div>
      </div>

      <!-- GSC Keywords -->
      <div v-if="gscTab === 'keywords'" class="panel">
        <h3>关键词分析</h3>
        <div class="filter-row">
          <el-input v-model="kwSearch" placeholder="搜索关键词" clearable style="width:200px" @keyup.enter="loadKeywords" />
          <el-select v-model="kwBucket" clearable placeholder="排名段" style="width:120px" @change="loadKeywords">
            <el-option value="" label="全部" /><el-option value="1-3" label="1-3" /><el-option value="4-10" label="4-10" />
            <el-option value="11-20" label="11-20" /><el-option value="21-50" label="21-50" /><el-option value="51-100" label="51-100" />
            <el-option value="100+" label="100+" />
          </el-select>
          <el-select v-model="kwIntent" clearable placeholder="意图" style="width:130px" @change="loadKeywords">
            <el-option value="" label="全部" /><el-option value="informational" label="信息型" /><el-option value="transactional" label="交易型" />
            <el-option value="commercial" label="商业型" /><el-option value="navigational" label="导航型" />
          </el-select>
          <el-select v-model="kwBrand" clearable placeholder="品牌" style="width:120px" @change="loadKeywords">
            <el-option value="" label="全部" /><el-option value="brand" label="品牌词" /><el-option value="non_brand" label="非品牌词" />
          </el-select>
          <el-checkbox v-model="kwQuestion" label="仅问题型" @change="loadKeywords" />
          <el-button @click="loadKeywords">查询</el-button>
        </div>
        <el-table :data="kwData.items" v-loading="kwLoading" stripe border style="width:100%">
          <el-table-column prop="query" label="关键词" min-width="200" show-overflow-tooltip />
          <el-table-column prop="page_url" label="落地页" min-width="200" show-overflow-tooltip />
          <el-table-column prop="position" label="位置" width="80" sortable :formatter="(r:any) => r.position?.toFixed(1)" />
          <el-table-column prop="clicks" label="点击" width="80" sortable />
          <el-table-column prop="impressions" label="曝光" width="90" sortable />
          <el-table-column prop="ctr" label="CTR" width="80" :formatter="(r:any) => (r.ctr*100).toFixed(2)+'%'" />
          <el-table-column prop="intent" label="意图" width="80" />
          <el-table-column prop="brand" label="品牌" width="80" />
          <el-table-column prop="is_question" label="问题" width="60" :formatter="(r:any) => r.is_question ? 'Y' : ''" />
          <el-table-column label="操作" width="70" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" size="small" @click="openKeywordDetail(row.query)">详情</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div class="pager" v-if="kwData.total > 0">
          <el-pagination layout="total, sizes, prev, pager, next, jumper" :total="kwData.total"
            v-model:current-page="kwPage" v-model:page-size="kwPageSize"
            :page-sizes="[25,50,100,250]" @size-change="loadKeywords" @current-change="loadKeywords" />
          <el-button type="success" size="small" @click="exportCSV" style="margin-left:12px">导出 CSV</el-button>
        </div>
      </div>

      <!-- Keyword Detail Dialog -->
      <el-dialog v-model="kwDetailVisible" :title="'关键词详情: ' + kwDetailData?.query" width="800px" destroy-on-close>
        <div v-if="kwDetailData">
          <div class="kw-detail-meta">
            <el-tag>{{ kwDetailData.brand || 'unknown' }}</el-tag>
            <el-tag type="info">{{ kwDetailData.intent || 'unknown' }}</el-tag>
            <el-tag type="warning">{{ kwDetailData.length || '-' }}</el-tag>
            <el-tag v-if="kwDetailData.is_question" type="danger">问题型</el-tag>
            <el-tag v-if="kwDetailData.is_comparison" type="success">对比型</el-tag>
            <span class="meta-text">{{ kwDetailData.word_count }} 词 / {{ kwDetailData.char_count }} 字</span>
          </div>
          <h4>90 天趋势</h4>
          <el-table :data="kwDetailData.trend" stripe border max-height="300" size="small">
            <el-table-column prop="date" label="日期" width="100" />
            <el-table-column prop="clicks" label="点击" width="70" />
            <el-table-column prop="impressions" label="曝光" width="80" />
            <el-table-column prop="ctr" label="CTR" width="80" :formatter="(r:any) => (r.ctr*100).toFixed(2)+'%'" />
            <el-table-column prop="position" label="位置" width="70" :formatter="(r:any) => r.position?.toFixed(1)" />
          </el-table>
          <h4 style="margin-top:16px">对应页面 (TOP 20)</h4>
          <el-table :data="kwDetailData.pages" stripe border size="small">
            <el-table-column prop="page_url" label="页面" min-width="300" show-overflow-tooltip />
            <el-table-column prop="clicks" label="点击" width="70" />
            <el-table-column prop="impressions" label="曝光" width="80" />
            <el-table-column prop="position" label="位置" width="70" :formatter="(r:any) => r.position?.toFixed(1)" />
          </el-table>
        </div>
      </el-dialog>

      <!-- GSC Pages -->
      <div v-if="gscTab === 'pages'" class="panel">
        <h3>页面分析</h3>
        <div class="filter-row">
          <el-input v-model="pgSearch" placeholder="搜索URL" clearable style="width:300px" @keyup.enter="loadPages" />
          <el-button @click="loadPages">查询</el-button>
        </div>
        <el-table :data="pgData.items" v-loading="pgLoading" stripe border>
          <el-table-column prop="page_url" label="页面" min-width="300" show-overflow-tooltip />
          <el-table-column prop="clicks" label="点击" width="90" sortable />
          <el-table-column prop="impressions" label="曝光" width="100" sortable />
          <el-table-column prop="ctr" label="CTR" width="80" :formatter="(r:any) => (r.ctr*100).toFixed(2)+'%'" />
          <el-table-column prop="position" label="位置" width="80" :formatter="(r:any) => r.position?.toFixed(1)" />
          <el-table-column prop="query_count" label="词数" width="70" />
        </el-table>
        <div class="pager" v-if="pgData.total > 0">
          <el-pagination layout="total, prev, pager, next" :total="pgData.total"
            v-model:current-page="pgPage" v-model:page-size="pgPageSize" @current-change="loadPages" />
        </div>
      </div>

      <!-- GSC SERP Features -->
      <div v-if="gscTab === 'serp'" class="panel">
        <h3>SERP 特征分布</h3>
        <el-table :data="appearanceData" v-loading="appearanceLoading" stripe border>
          <el-table-column prop="appearance" label="SERP 特征" min-width="200" />
          <el-table-column prop="clicks" label="点击" width="90" />
          <el-table-column prop="impressions" label="曝光" width="100" />
          <el-table-column prop="ctr" label="CTR" width="80" :formatter="(r:any) => (r.ctr*100).toFixed(2)+'%'" />
          <el-table-column prop="position" label="位置" width="80" :formatter="(r:any) => r.position?.toFixed(1)" />
          <el-table-column prop="days_count" label="天数" width="70" />
        </el-table>
      </div>

      <!-- GSC URL Inspection -->
      <div v-if="gscTab === 'index'" class="panel">
        <h3>收录状态</h3>
        <div class="filter-row">
          <el-select v-model="idxCoverage" clearable placeholder="覆盖状态" style="width:250px" @change="loadUrlInspection">
            <el-option value="" label="全部" />
            <el-option v-for="s in idxSummary" :key="s.state" :value="s.state" :label="`${s.state} (${s.count})`" />
          </el-select>
          <el-button type="warning" :loading="inspecting" @click="runUrlInspection">检查 URL 收录</el-button>
        </div>
        <el-table :data="idxData.items" v-loading="idxLoading" stripe border>
          <el-table-column prop="url" label="URL" min-width="300" show-overflow-tooltip />
          <el-table-column prop="coverage_state" label="覆盖状态" width="200" />
          <el-table-column prop="verdict" label="判决" width="80" />
          <el-table-column prop="page_fetch_state" label="抓取状态" width="140" />
          <el-table-column prop="last_crawl_time" label="上次抓取" width="140" />
          <el-table-column prop="mobile_usable" label="移动友好" width="80" :formatter="(r:any) => r.mobile_usable ? 'Y' : 'N'" />
        </el-table>
        <div class="pager" v-if="idxData.total > 0">
          <el-pagination layout="total, prev, pager, next" :total="idxData.total"
            v-model:current-page="idxPage" @current-change="loadUrlInspection" />
        </div>
      </div>

      <!-- GSC Sitemaps -->
      <div v-if="gscTab === 'sitemaps'" class="panel">
        <h3>Sitemap 状态</h3>
        <el-table :data="sitemapData" v-loading="sitemapLoading" stripe border>
          <el-table-column prop="path" label="Sitemap" min-width="300" show-overflow-tooltip />
          <el-table-column prop="type" label="类型" width="100" />
          <el-table-column prop="errors" label="错误" width="70" />
          <el-table-column prop="warnings" label="警告" width="70" />
          <el-table-column prop="is_sitemaps_index" label="索引型" width="70" :formatter="(r:any) => r.is_sitemaps_index ? 'Y' : 'N'" />
          <el-table-column prop="last_submitted" label="最后提交" width="140" />
          <el-table-column prop="last_downloaded" label="最后下载" width="140" />
        </el-table>
      </div>

      <!-- GSC Opportunities -->
      <div v-if="gscTab === 'opportunities'" class="panel">
        <h3>关键词机会</h3>
        <el-table :data="oppData" v-loading="oppLoading" stripe border>
          <el-table-column prop="query" label="关键词" min-width="200" show-overflow-tooltip />
          <el-table-column prop="page_url" label="落地页" min-width="200" show-overflow-tooltip />
          <el-table-column prop="position" label="位置" width="70" :formatter="(r:any) => r.position?.toFixed(1)" />
          <el-table-column prop="impressions" label="曝光" width="80" />
          <el-table-column prop="actual_ctr" label="实际CTR" width="90" :formatter="(r:any) => (r.actual_ctr*100).toFixed(2)+'%'" />
          <el-table-column prop="baseline_ctr" label="基准CTR" width="90" :formatter="(r:any) => (r.baseline_ctr*100).toFixed(2)+'%'" />
          <el-table-column prop="opportunity_score" label="机会分" width="80" sortable />
          <el-table-column prop="estimated_extra_clicks" label="预估增点" width="90" />
          <el-table-column prop="type" label="类型" width="140" />
        </el-table>
      </div>

      <!-- GSC Cannibalization -->
      <div v-if="gscTab === 'cannibal'" class="panel">
        <h3>自蚕食检测</h3>
        <el-table :data="cannibalData" v-loading="cannibalLoading" stripe border>
          <el-table-column prop="query" label="关键词" min-width="200" show-overflow-tooltip />
          <el-table-column prop="page_count" label="争抢页面数" width="100" />
          <el-table-column prop="severity" label="严重度" width="80" />
          <el-table-column prop="total_impressions" label="总曝光" width="90" />
          <el-table-column prop="total_clicks" label="总点击" width="80" />
          <el-table-column label="争抢页面" min-width="300">
            <template #default="{ row }">
              <div v-for="(p, i) in row.competing_pages?.slice(0, 3)" :key="i" class="cannibal-page">
                {{ p.url }} (曝光{{ p.impressions }} 位置{{ p.position?.toFixed(1) }})
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- GSC Query Diff -->
      <div v-if="gscTab === 'diff'" class="panel">
        <h3>关键词变动</h3>
        <div class="filter-row">
          <el-radio-group v-model="diffType" @change="loadQueryDiff">
            <el-radio-button value="lost">丢失词</el-radio-button>
            <el-radio-button value="new">新词</el-radio-button>
          </el-radio-group>
        </div>
        <el-table :data="diffData" v-loading="diffLoading" stripe border>
          <el-table-column prop="query" label="关键词" min-width="250" show-overflow-tooltip />
          <el-table-column prop="prev_clicks" label="上期点击" width="90" />
          <el-table-column prop="curr_clicks" label="本期点击" width="90" />
          <el-table-column prop="click_delta" label="变化" width="80" />
        </el-table>
      </div>
    </template>

    <!-- GA4 Tab -->
    <template v-if="siteId && platform === 'ga4'">
      <div class="panel toolbar sub-tabs">
        <button v-for="t in ga4Tabs" :key="t.key" :class="['pill', ga4Tab === t.key && 'active']" @click="ga4Tab = t.key; loadGA4Tab()">{{ t.label }}</button>
      </div>
      <div class="panel">
        <div v-if="ga4Tab === 'overview'" >
          <h3>GA4 扩展数据</h3>
          <el-table :data="ga4Extended" v-loading="ga4Loading" stripe border max-height="500">
            <el-table-column prop="date" label="日期" width="110" />
            <el-table-column prop="total_users" label="总用户" width="90" />
            <el-table-column prop="active_users" label="活跃用户" width="90" />
            <el-table-column prop="sessions" label="会话" width="80" />
            <el-table-column prop="engaged_sessions" label="参与会话" width="90" />
            <el-table-column prop="engagement_rate" label="参与率" width="80" :formatter="(r:any) => (r.engagement_rate*100).toFixed(1)+'%'" />
            <el-table-column prop="bounce_rate" label="跳出率" width="80" :formatter="(r:any) => (r.bounce_rate*100).toFixed(1)+'%'" />
            <el-table-column prop="screen_page_views" label="PV" width="80" />
            <el-table-column prop="event_count" label="事件数" width="80" />
            <el-table-column prop="conversions" label="转化" width="70" />
          </el-table>
        </div>
        <div v-if="ga4Tab === 'landing'" >
          <h3>入站页</h3>
          <el-table :data="ga4LandingPages" v-loading="ga4Loading" stripe border>
            <el-table-column prop="landing_page" label="入站页" min-width="300" show-overflow-tooltip />
            <el-table-column prop="sessions" label="会话" width="80" />
            <el-table-column prop="users" label="用户" width="80" />
            <el-table-column prop="engagement_rate" label="参与率" width="80" :formatter="(r:any) => (r.engagement_rate*100).toFixed(1)+'%'" />
            <el-table-column prop="bounce_rate" label="跳出率" width="80" :formatter="(r:any) => (r.bounce_rate*100).toFixed(1)+'%'" />
            <el-table-column prop="conversions" label="转化" width="70" />
          </el-table>
        </div>
        <div v-if="ga4Tab === 'source'" >
          <h3>流量来源</h3>
          <el-table :data="ga4Sources" v-loading="ga4Loading" stripe border>
            <el-table-column prop="source" label="来源" width="150" />
            <el-table-column prop="medium" label="媒介" width="120" />
            <el-table-column prop="channel_group" label="渠道" width="150" />
            <el-table-column prop="sessions" label="会话" width="80" />
            <el-table-column prop="users" label="用户" width="80" />
            <el-table-column prop="engagement_rate" label="参与率" width="80" :formatter="(r:any) => (r.engagement_rate*100).toFixed(1)+'%'" />
            <el-table-column prop="conversions" label="转化" width="70" />
          </el-table>
        </div>
        <div v-if="ga4Tab === 'events'" >
          <h3>事件</h3>
          <el-table :data="ga4Events" v-loading="ga4Loading" stripe border>
            <el-table-column prop="event_name" label="事件名" min-width="200" />
            <el-table-column prop="event_count" label="次数" width="90" />
            <el-table-column prop="users" label="用户" width="80" />
            <el-table-column prop="event_value" label="事件值" width="90" />
            <el-table-column prop="is_conversion" label="转化" width="60" :formatter="(r:any) => r.is_conversion ? 'Y' : ''" />
          </el-table>
        </div>
        <div v-if="ga4Tab === 'tech'" >
          <h3>设备与技术</h3>
          <div class="filter-row">
            <el-radio-group v-model="techDim" @change="loadGA4Tab">
              <el-radio-button value="device">设备</el-radio-button>
              <el-radio-button value="os">操作系统</el-radio-button>
              <el-radio-button value="browser">浏览器</el-radio-button>
            </el-radio-group>
          </div>
          <el-table :data="ga4Tech" v-loading="ga4Loading" stripe border>
            <el-table-column prop="value" label="维度值" min-width="200" />
            <el-table-column prop="sessions" label="会话" width="90" />
            <el-table-column prop="users" label="用户" width="80" />
            <el-table-column prop="engagement_rate" label="参与率" width="90" :formatter="(r:any) => (r.engagement_rate*100).toFixed(1)+'%'" />
          </el-table>
        </div>
        <div v-if="ga4Tab === 'geo'" >
          <h3>地理分布</h3>
          <el-table :data="ga4Geo" v-loading="ga4Loading" stripe border>
            <el-table-column prop="country" label="国家" width="120" />
            <el-table-column prop="region" label="地区" width="150" />
            <el-table-column prop="city" label="城市" width="150" />
            <el-table-column prop="sessions" label="会话" width="80" />
            <el-table-column prop="users" label="用户" width="80" />
          </el-table>
        </div>
        <div v-if="ga4Tab === 'hourly'" >
          <h3>小时分布</h3>
          <el-table :data="ga4Hourly" v-loading="ga4Loading" stripe border>
            <el-table-column prop="hour" label="小时" width="70" :formatter="(r:any) => r.hour + ':00'" />
            <el-table-column prop="sessions" label="会话" width="90" />
            <el-table-column prop="users" label="用户" width="80" />
            <el-table-column prop="screen_page_views" label="PV" width="80" />
          </el-table>
        </div>
        <div v-if="ga4Tab === 'day_of_week'" >
          <h3>星期几分布</h3>
          <el-table :data="ga4DayOfWeek" v-loading="ga4Loading" stripe border>
            <el-table-column prop="name" label="星期" width="100" />
            <el-table-column prop="sessions" label="会话" width="90" />
            <el-table-column prop="users" label="用户" width="80" />
            <el-table-column prop="screen_page_views" label="PV" width="80" />
            <el-table-column prop="conversions" label="转化" width="70" />
          </el-table>
        </div>
        <div v-if="ga4Tab === 'scroll'" >
          <h3>滚动深度</h3>
          <el-table :data="ga4Scroll" v-loading="ga4Loading" stripe border>
            <el-table-column prop="page_path" label="页面" min-width="250" show-overflow-tooltip />
            <el-table-column prop="percent_scrolled" label="滚动%" width="80" :formatter="(r:any) => r.percent_scrolled + '%'" />
            <el-table-column prop="users" label="用户" width="80" />
            <el-table-column prop="event_count" label="次数" width="80" />
          </el-table>
        </div>
        <div v-if="ga4Tab === 'outbound'" >
          <h3>外链点击</h3>
          <el-table :data="ga4Outbound" v-loading="ga4Loading" stripe border>
            <el-table-column prop="link_domain" label="域名" width="200" />
            <el-table-column prop="link_url" label="URL" min-width="300" show-overflow-tooltip />
            <el-table-column prop="event_count" label="点击" width="80" />
            <el-table-column prop="users" label="用户" width="80" />
          </el-table>
        </div>
        <div v-if="ga4Tab === 'conversions'" >
          <h3>转化明细</h3>
          <el-table :data="ga4Conversions" v-loading="ga4Loading" stripe border>
            <el-table-column prop="event_name" label="事件" min-width="200" />
            <el-table-column prop="source" label="来源" width="120" />
            <el-table-column prop="medium" label="媒介" width="100" />
            <el-table-column prop="conversions" label="转化数" width="80" />
            <el-table-column prop="total_revenue" label="收入" width="90" />
          </el-table>
        </div>
      </div>
    </template>

    <!-- Bing Tab -->
    <template v-if="siteId && platform === 'bing'">
      <div class="panel toolbar sub-tabs">
        <button v-for="t in bingTabs" :key="t.key" :class="['pill', bingTab === t.key && 'active']" @click="bingTab = t.key; loadBingTab()">{{ t.label }}</button>
      </div>
      <div class="panel">
        <div v-if="bingTab === 'keywords'">
          <h3>Bing 关键词</h3>
          <el-input v-model="bingKwSearch" placeholder="搜索" clearable style="width:200px;margin-bottom:12px" @keyup.enter="loadBingTab" />
          <el-table :data="bingKeywords.items" v-loading="bingLoading" stripe border>
            <el-table-column prop="query" label="关键词" min-width="250" show-overflow-tooltip />
            <el-table-column prop="clicks" label="点击" width="80" />
            <el-table-column prop="impressions" label="曝光" width="90" />
            <el-table-column prop="ctr" label="CTR" width="80" :formatter="(r:any) => (r.ctr*100).toFixed(2)+'%'" />
            <el-table-column prop="avg_impression_position" label="位置" width="80" :formatter="(r:any) => r.avg_impression_position?.toFixed(1)" />
          </el-table>
          <div class="pager" v-if="bingKeywords.total > 0">
            <el-pagination layout="total, prev, pager, next" :total="bingKeywords.total"
              v-model:current-page="bingKwPage" @current-change="loadBingTab" />
          </div>
        </div>
        <div v-if="bingTab === 'pages'">
          <h3>Bing 页面</h3>
          <el-table :data="bingPages.items" v-loading="bingLoading" stripe border>
            <el-table-column prop="page_url" label="页面" min-width="300" show-overflow-tooltip />
            <el-table-column prop="clicks" label="点击" width="80" />
            <el-table-column prop="impressions" label="曝光" width="90" />
            <el-table-column prop="ctr" label="CTR" width="80" :formatter="(r:any) => (r.ctr*100).toFixed(2)+'%'" />
            <el-table-column prop="position" label="位置" width="80" :formatter="(r:any) => r.position?.toFixed(1)" />
          </el-table>
        </div>
        <div v-if="bingTab === 'backlinks'">
          <h3>Bing 反链</h3>
          <el-table :data="bingBacklinks" v-loading="bingLoading" stripe border>
            <el-table-column prop="source_url" label="来源URL" min-width="300" show-overflow-tooltip />
            <el-table-column prop="target_page" label="目标页" min-width="200" show-overflow-tooltip />
            <el-table-column prop="anchor_text" label="锚文本" width="150" />
          </el-table>
        </div>
        <div v-if="bingTab === 'sitemaps'">
          <h3>Bing Sitemap</h3>
          <el-table :data="bingSitemapList" v-loading="bingLoading" stripe border>
            <el-table-column prop="sitemap_url" label="Sitemap" min-width="300" />
            <el-table-column prop="url_count" label="URL数" width="100" />
            <el-table-column prop="status" label="状态" width="100" />
          </el-table>
        </div>
      </div>
    </template>

    <!-- Yandex Tab -->
    <template v-if="siteId && platform === 'yandex'">
      <div class="panel toolbar sub-tabs">
        <button v-for="t in yandexTabs" :key="t.key" :class="['pill', yandexTab === t.key && 'active']" @click="yandexTab = t.key; loadYandexTab()">{{ t.label }}</button>
      </div>
      <div class="panel">
        <div v-if="yandexTab === 'keywords'">
          <h3>Yandex 关键词</h3>
          <el-table :data="yandexKeywords.items" v-loading="yandexLoading" stripe border>
            <el-table-column prop="query" label="关键词" min-width="250" show-overflow-tooltip />
            <el-table-column prop="clicks" label="点击" width="80" />
            <el-table-column prop="shows" label="曝光" width="90" />
            <el-table-column prop="ctr" label="CTR" width="80" :formatter="(r:any) => (r.ctr*100).toFixed(2)+'%'" />
            <el-table-column prop="avg_show_position" label="展示位" width="80" :formatter="(r:any) => r.avg_show_position?.toFixed(1)" />
          </el-table>
          <div class="pager" v-if="yandexKeywords.total > 0">
            <el-pagination layout="total, prev, pager, next" :total="yandexKeywords.total"
              v-model:current-page="yandexKwPage" @current-change="loadYandexTab" />
          </div>
        </div>
        <div v-if="yandexTab === 'index_events'">
          <h3>索引事件</h3>
          <el-table :data="yandexIndexEvents" v-loading="yandexLoading" stripe border>
            <el-table-column prop="date" label="日期" width="110" />
            <el-table-column prop="event_type" label="事件类型" min-width="200" />
            <el-table-column prop="count" label="数量" width="80" />
          </el-table>
        </div>
        <div v-if="yandexTab === 'http_status'">
          <h3>HTTP 状态</h3>
          <el-table :data="yandexHttpStatus" v-loading="yandexLoading" stripe border>
            <el-table-column prop="date" label="日期" width="110" />
            <el-table-column prop="status_code" label="状态码" min-width="200" />
            <el-table-column prop="count" label="数量" width="80" />
          </el-table>
        </div>
        <div v-if="yandexTab === 'external_links'">
          <h3>外链</h3>
          <el-table :data="yandexExtLinks" v-loading="yandexLoading" stripe border>
            <el-table-column prop="source_url" label="来源URL" min-width="300" show-overflow-tooltip />
            <el-table-column prop="target_url" label="目标页" min-width="200" show-overflow-tooltip />
          </el-table>
        </div>
        <div v-if="yandexTab === 'indexed_urls'">
          <h3>索引 URL 样本</h3>
          <el-table :data="yandexIndexedUrls.items" v-loading="yandexLoading" stripe border>
            <el-table-column prop="url" label="URL" min-width="400" show-overflow-tooltip />
            <el-table-column prop="status" label="状态" width="100" />
            <el-table-column prop="http_code" label="HTTP" width="70" />
            <el-table-column prop="access_date" label="访问日期" width="110" />
          </el-table>
          <div class="pager" v-if="yandexIndexedUrls.total > 0">
            <el-pagination layout="total, prev, pager, next" :total="yandexIndexedUrls.total"
              v-model:current-page="yandexIdxPage" @current-change="loadYandexTab" />
          </div>
        </div>
      </div>
    </template>
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

const platforms = [
  { code: 'gsc', label: 'GSC' },
  { code: 'ga4', label: 'GA4' },
  { code: 'bing', label: 'Bing' },
  { code: 'yandex', label: 'Yandex' },
]

// GSC state
const gscTabs = [
  { key: 'overview', label: '总览' }, { key: 'buckets', label: '排名分布' },
  { key: 'keywords', label: '关键词' }, { key: 'pages', label: '页面' },
  { key: 'serp', label: 'SERP特征' }, { key: 'index', label: '收录状态' },
  { key: 'sitemaps', label: 'Sitemap' }, { key: 'opportunities', label: '机会' },
  { key: 'cannibal', label: '自蚕食' }, { key: 'diff', label: '关键词变动' },
]
const gscTab = ref('overview')
const gscOverview = ref<any>(null)
const gscBuckets = ref<any[]>([])
const latestBuckets = computed(() => {
  if (!gscBuckets.value.length) return []
  const latestDate = gscBuckets.value[0]?.date
  return gscBuckets.value.filter((b: any) => b.date === latestDate)
})

// Keywords
const kwData = ref<any>({ items: [], total: 0 })
const kwLoading = ref(false)
const kwSearch = ref('')
const kwBucket = ref('')
const kwIntent = ref('')
const kwBrand = ref('')
const kwQuestion = ref(false)
const kwPage = ref(1)
const kwPageSize = ref(25)

// Pages
const pgData = ref<any>({ items: [], total: 0 })
const pgLoading = ref(false)
const pgSearch = ref('')
const pgPage = ref(1)
const pgPageSize = ref(25)

// Appearance
const appearanceData = ref<any[]>([])
const appearanceLoading = ref(false)

// URL Inspection
const idxData = ref<any>({ items: [], total: 0 })
const idxSummary = ref<any[]>([])
const idxLoading = ref(false)
const idxCoverage = ref('')
const idxPage = ref(1)
const inspecting = ref(false)

// Sitemaps
const sitemapData = ref<any[]>([])
const sitemapLoading = ref(false)

// Opportunities
const oppData = ref<any[]>([])
const oppLoading = ref(false)

// Cannibalization
const cannibalData = ref<any[]>([])
const cannibalLoading = ref(false)

// Query Diff
const diffData = ref<any[]>([])
const diffLoading = ref(false)
const diffType = ref('lost')

// GA4 state
// Keyword detail dialog
const kwDetailVisible = ref(false)
const kwDetailData = ref<any>(null)

const ga4Tabs = [
  { key: 'overview', label: '总览' }, { key: 'landing', label: '入站页' },
  { key: 'source', label: '流量来源' }, { key: 'events', label: '事件' },
  { key: 'tech', label: '设备技术' }, { key: 'geo', label: '地理' },
  { key: 'hourly', label: '小时分布' }, { key: 'day_of_week', label: '星期分布' },
  { key: 'scroll', label: '滚动深度' }, { key: 'outbound', label: '外链点击' },
  { key: 'conversions', label: '转化明细' },
]
const ga4Tab = ref('overview')
const ga4Loading = ref(false)
const ga4Extended = ref<any[]>([])
const ga4LandingPages = ref<any[]>([])
const ga4Sources = ref<any[]>([])
const ga4Events = ref<any[]>([])
const ga4Tech = ref<any[]>([])
const ga4Geo = ref<any[]>([])
const ga4Hourly = ref<any[]>([])
const ga4DayOfWeek = ref<any[]>([])
const ga4Scroll = ref<any[]>([])
const ga4Outbound = ref<any[]>([])
const ga4Conversions = ref<any[]>([])
const techDim = ref('device')

// Bing state
const bingTabs = [
  { key: 'keywords', label: '关键词' }, { key: 'pages', label: '页面' },
  { key: 'backlinks', label: '反链' }, { key: 'sitemaps', label: 'Sitemap' },
]
const bingTab = ref('keywords')
const bingLoading = ref(false)
const bingKeywords = ref<any>({ items: [], total: 0 })
const bingPages = ref<any>({ items: [], total: 0 })
const bingBacklinks = ref<any[]>([])
const bingSitemapList = ref<any[]>([])
const bingKwSearch = ref('')
const bingKwPage = ref(1)

// Yandex state
const yandexTabs = [
  { key: 'keywords', label: '关键词' }, { key: 'index_events', label: '索引事件' },
  { key: 'http_status', label: 'HTTP状态' }, { key: 'external_links', label: '外链' },
  { key: 'indexed_urls', label: '索引URL' },
]
const yandexTab = ref('keywords')
const yandexLoading = ref(false)
const yandexKeywords = ref<any>({ items: [], total: 0 })
const yandexIndexEvents = ref<any[]>([])
const yandexHttpStatus = ref<any[]>([])
const yandexExtLinks = ref<any[]>([])
const yandexIndexedUrls = ref<any>({ items: [], total: 0 })
const yandexIdxPage = ref(1)
const yandexKwPage = ref(1)

const fmt = (n: number) => n?.toLocaleString() ?? '0'

onMounted(async () => {
  try {
    const r: any = await api.listSites()
    sites.value = r.data || []
    if (sites.value.length) siteId.value = sites.value[0].id
  } catch {}
})

watch(siteId, () => { if (siteId.value) refresh() })
watch(platform, () => { if (siteId.value) refresh() })

function onSiteChange() { refresh() }

async function refresh() {
  if (!siteId.value) return
  if (platform.value === 'gsc') {
    loadGSCOverview()
    loadGSCBuckets()
    if (gscTab.value === 'keywords') loadKeywords()
    if (gscTab.value === 'pages') loadPages()
    if (gscTab.value === 'serp') loadAppearance()
    if (gscTab.value === 'index') loadUrlInspection()
    if (gscTab.value === 'sitemaps') loadSitemaps()
    if (gscTab.value === 'opportunities') loadOpportunities()
    if (gscTab.value === 'cannibal') loadCannibalization()
    if (gscTab.value === 'diff') loadQueryDiff()
  } else if (platform.value === 'ga4') {
    loadGA4Tab()
  } else if (platform.value === 'bing') {
    loadBingTab()
  } else if (platform.value === 'yandex') {
    loadYandexTab()
  }
}

watch(gscTab, () => {
  if (!siteId.value) return
  if (gscTab.value === 'keywords') loadKeywords()
  else if (gscTab.value === 'pages') loadPages()
  else if (gscTab.value === 'serp') loadAppearance()
  else if (gscTab.value === 'index') loadUrlInspection()
  else if (gscTab.value === 'sitemaps') loadSitemaps()
  else if (gscTab.value === 'opportunities') loadOpportunities()
  else if (gscTab.value === 'cannibal') loadCannibalization()
  else if (gscTab.value === 'diff') loadQueryDiff()
})

async function syncDeep() {
  if (!siteId.value) return
  syncing.value = true
  try {
    if (platform.value === 'gsc') await api.syncGSCDeep(siteId.value)
    else if (platform.value === 'ga4') await api.syncGA4Deep(siteId.value)
    else if (platform.value === 'bing') await api.syncBingDeep(siteId.value)
    else if (platform.value === 'yandex') await api.syncYandexDeep(siteId.value)
    ElMessage.success('深度数据同步成功')
    refresh()
  } catch (e: any) {
    ElMessage.error(e?.message || '同步失败')
  } finally {
    syncing.value = false
  }
}

// Keyword detail
async function openKeywordDetail(query: string) {
  try {
    const r: any = await api.getGSCKeywordDetail(siteId.value!, query)
    kwDetailData.value = r.data
    kwDetailVisible.value = true
  } catch {}
}

// CSV export
async function exportCSV() {
  try {
    const response: any = await api.getGSCKeywordsExport(siteId.value!, days.value)
    const blob = new Blob([response.data], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'gsc_keywords.csv'
    a.click()
    URL.revokeObjectURL(url)
  } catch { ElMessage.error('导出失败') }
}

// GSC loaders
async function loadGSCOverview() {
  try { const r: any = await api.getGSCOverviewDeep(siteId.value!, days.value); gscOverview.value = r.data } catch {}
}
async function loadGSCBuckets() {
  try { const r: any = await api.getGSCRankBuckets(siteId.value!, days.value); gscBuckets.value = r.data || [] } catch {}
}
async function loadKeywords() {
  kwLoading.value = true
  try {
    const r: any = await api.getGSCKeywords(siteId.value!, {
      page: kwPage.value, page_size: kwPageSize.value, days: days.value,
      search: kwSearch.value || undefined, bucket: kwBucket.value || undefined,
      intent: kwIntent.value || undefined, brand: kwBrand.value || undefined,
      is_question: kwQuestion.value ? 'true' : undefined,
    })
    kwData.value = r.data || { items: [], total: 0 }
  } catch {} finally { kwLoading.value = false }
}
async function loadPages() {
  pgLoading.value = true
  try {
    const r: any = await api.getGSCDeepPages(siteId.value!, { page: pgPage.value, page_size: pgPageSize.value, days: days.value, search: pgSearch.value || undefined })
    pgData.value = r.data || { items: [], total: 0 }
  } catch {} finally { pgLoading.value = false }
}
async function loadAppearance() {
  appearanceLoading.value = true
  try { const r: any = await api.getGSCAppearance(siteId.value!, days.value); appearanceData.value = r.data || [] } catch {} finally { appearanceLoading.value = false }
}
async function loadUrlInspection() {
  idxLoading.value = true
  try {
    const r: any = await api.getGSCUrlInspection(siteId.value!, { page: idxPage.value, coverage_state: idxCoverage.value || undefined })
    idxData.value = r.data || { items: [], total: 0 }
    idxSummary.value = r.data?.summary || []
  } catch {} finally { idxLoading.value = false }
}
async function runUrlInspection() {
  inspecting.value = true
  try { await api.syncGSCURLInspection(siteId.value!); ElMessage.success('URL 检查完成'); loadUrlInspection() } catch {} finally { inspecting.value = false }
}
async function loadSitemaps() {
  sitemapLoading.value = true
  try { const r: any = await api.getGSCSitemaps(siteId.value!); sitemapData.value = r.data || [] } catch {} finally { sitemapLoading.value = false }
}
async function loadOpportunities() {
  oppLoading.value = true
  try { const r: any = await api.getGSCOpportunities(siteId.value!); oppData.value = r.data || [] } catch {} finally { oppLoading.value = false }
}
async function loadCannibalization() {
  cannibalLoading.value = true
  try { const r: any = await api.getGSCCannibalization(siteId.value!); cannibalData.value = r.data || [] } catch {} finally { cannibalLoading.value = false }
}
async function loadQueryDiff() {
  diffLoading.value = true
  try { const r: any = await api.getGSCQueryDiff(siteId.value!, diffType.value); diffData.value = r.data || [] } catch {} finally { diffLoading.value = false }
}

// GA4 loaders
async function loadGA4Tab() {
  ga4Loading.value = true
  try {
    if (ga4Tab.value === 'overview') { const r: any = await api.getGA4Extended(siteId.value!, days.value); ga4Extended.value = r.data || [] }
    else if (ga4Tab.value === 'landing') { const r: any = await api.getGA4LandingPages(siteId.value!, { days: days.value }); ga4LandingPages.value = r.data?.items || r.data || [] }
    else if (ga4Tab.value === 'source') { const r: any = await api.getGA4SourceMedium(siteId.value!, days.value); ga4Sources.value = r.data || [] }
    else if (ga4Tab.value === 'events') { const r: any = await api.getGA4Events(siteId.value!, days.value); ga4Events.value = r.data || [] }
    else if (ga4Tab.value === 'tech') { const r: any = await api.getGA4Tech(siteId.value!, techDim.value, days.value); ga4Tech.value = r.data || [] }
    else if (ga4Tab.value === 'geo') { const r: any = await api.getGA4Geo(siteId.value!, days.value); ga4Geo.value = r.data || [] }
    else if (ga4Tab.value === 'hourly') { const r: any = await api.getGA4Hourly(siteId.value!, days.value); ga4Hourly.value = r.data || [] }
    else if (ga4Tab.value === 'day_of_week') { const r: any = await api.getGA4DayOfWeek(siteId.value!); ga4DayOfWeek.value = r.data || [] }
    else if (ga4Tab.value === 'scroll') { const r: any = await api.getGA4ScrollDepth(siteId.value!, days.value); ga4Scroll.value = r.data || [] }
    else if (ga4Tab.value === 'outbound') { const r: any = await api.getGA4OutboundClicks(siteId.value!, days.value); ga4Outbound.value = r.data || [] }
    else if (ga4Tab.value === 'conversions') { const r: any = await api.getGA4ConversionsDaily(siteId.value!, days.value); ga4Conversions.value = r.data || [] }
  } catch {} finally { ga4Loading.value = false }
}

// Bing loaders
async function loadBingTab() {
  bingLoading.value = true
  try {
    if (bingTab.value === 'keywords') { const r: any = await api.getBingKeywords(siteId.value!, { page: bingKwPage.value, search: bingKwSearch.value || undefined }); bingKeywords.value = r.data || { items: [], total: 0 } }
    else if (bingTab.value === 'pages') { const r: any = await api.getBingDeepPages(siteId.value!); bingPages.value = r.data || { items: [], total: 0 } }
    else if (bingTab.value === 'backlinks') { const r: any = await api.getBingInboundLinks(siteId.value!); bingBacklinks.value = r.data || [] }
    else if (bingTab.value === 'sitemaps') { const r: any = await api.getBingSitemaps(siteId.value!); bingSitemapList.value = r.data || [] }
  } catch {} finally { bingLoading.value = false }
}

// Yandex loaders
async function loadYandexTab() {
  yandexLoading.value = true
  try {
    if (yandexTab.value === 'keywords') { const r: any = await api.getYandexKeywords(siteId.value!, { page: yandexKwPage.value }); yandexKeywords.value = r.data || { items: [], total: 0 } }
    else if (yandexTab.value === 'index_events') { const r: any = await api.getYandexIndexEvents(siteId.value!, days.value); yandexIndexEvents.value = r.data || [] }
    else if (yandexTab.value === 'http_status') { const r: any = await api.getYandexHTTPStatus(siteId.value!, days.value); yandexHttpStatus.value = r.data || [] }
    else if (yandexTab.value === 'external_links') { const r: any = await api.getYandexExternalLinks(siteId.value!); yandexExtLinks.value = r.data || [] }
    else if (yandexTab.value === 'indexed_urls') { const r: any = await api.getYandexIndexedUrls(siteId.value!, { page: yandexIdxPage.value }); yandexIndexedUrls.value = r.data || { items: [], total: 0 } }
  } catch {} finally { yandexLoading.value = false }
}
</script>

<style scoped>
.deep-data { padding: 0; }
.panel { background: #fff; border-radius: 8px; padding: 20px; margin-bottom: 16px; }
.toolbar { display: flex; flex-direction: column; gap: 8px; }
.tb-row { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.tb-left { display: flex; align-items: center; gap: 8px; }
.tb-right { display: flex; align-items: center; gap: 8px; margin-left: auto; }
.tb-tabs { display: flex; gap: 4px; }
.pill { padding: 6px 14px; border: 1px solid #e0e0e0; border-radius: 16px; cursor: pointer; background: #fff; font-size: 13px; transition: all .15s; }
.pill:hover { background: #f5f5f5; }
.pill.active { background: #409eff; color: #fff; border-color: #409eff; }
.sub-tabs { padding: 12px 20px; }
.metric-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 12px; }
.metric-card { background: #f9fafb; border-radius: 8px; padding: 16px; text-align: center; }
.mc-label { font-size: 12px; color: #888; margin-bottom: 4px; }
.mc-value { font-size: 22px; font-weight: 600; color: #333; }
.bucket-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; }
.bucket-card { background: #f0f9ff; border-radius: 8px; padding: 14px; text-align: center; border: 1px solid #e0efff; }
.bc-name { font-weight: 700; font-size: 16px; margin-bottom: 6px; color: #1a73e8; }
.bc-queries { font-size: 13px; color: #555; }
.bc-clicks { font-size: 13px; color: #333; font-weight: 500; }
.bc-imp { font-size: 12px; color: #999; }
.bc-ctr { font-size: 12px; color: #666; }
.filter-row { display: flex; gap: 8px; align-items: center; margin-bottom: 12px; flex-wrap: wrap; }
.pager { margin-top: 12px; display: flex; justify-content: flex-end; }
.state-placeholder { text-align: center; padding: 40px; color: #999; }
h3 { margin: 0 0 12px 0; font-size: 16px; }
.cannibal-page { font-size: 12px; color: #666; line-height: 1.6; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.kw-detail-meta { display: flex; gap: 8px; align-items: center; margin-bottom: 16px; flex-wrap: wrap; }
.kw-detail-meta .meta-text { color: #888; font-size: 13px; }
h4 { margin: 0 0 8px; font-size: 14px; color: #555; }
</style>
