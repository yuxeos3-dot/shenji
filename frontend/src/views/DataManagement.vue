<template>
  <div class="data-mgmt">
    <!-- ─── Top toolbar ─────────────────────────────────────────── -->
    <div class="panel toolbar">
      <div class="tb-row">
        <div class="tb-tabs">
          <button v-for="p in platforms" :key="p.code" :class="['pill', platform === p.code && 'active']" @click="switchPlatform(p.code)">{{ p.label }}</button>
        </div>
        <div class="tb-date" v-if="view === 'overview'">
          <button v-for="pr in presets" :key="pr.key" :class="['pill pill-sm', activePreset === pr.key && 'active']" @click="applyPreset(pr.key)">{{ pr.label }}</button>
        </div>
        <div class="tb-right" v-if="view === 'overview'">
          <button class="btn btn-refresh" @click="loadOverview()" :disabled="loading">{{ loading ? '加载中...' : '🔄 刷新' }}</button>
        </div>
      </div>
    </div>

    <!-- ─── Overview: site cards ────────────────────────────────── -->
    <div v-if="view === 'overview'" class="view-overview">
      <div class="section-title">{{ platformLabel }} · 总览
        <span class="badge" v-if="overview.cards.length">{{ overview.cards.length }} 个站点</span>
      </div>
      <div v-if="loading && !overview.cards.length" class="state-placeholder">加载中...</div>
      <div v-else-if="error" class="state-error">❌ {{ error }}</div>
      <div v-else-if="!overview.cards.length" class="state-empty">暂无站点</div>
      <div v-else class="card-grid">
        <div v-for="(c, i) in overview.cards" :key="i" :class="['site-card', c.error && 'card-err']" @click="openDetail(c)">
          <button class="card-hide" @click.stop="hideCard(c)" title="隐藏">×</button>
          <div class="card-name">{{ c.display_name || c.site_url || c.host_id || c.property_id }}</div>
          <div v-if="c.error" class="card-err-msg">{{ c.error }}</div>
          <div v-else class="card-kpis">
            <template v-if="platform !== 'ga4'">
              <div class="ck"><span class="ck-l">点击</span><span class="ck-v sv-blue">{{ fmt(c.clicks) }}</span></div>
              <div class="ck"><span class="ck-l">展示</span><span class="ck-v sv-green">{{ fmt(c.impressions || c.shows) }}</span></div>
              <div class="ck"><span class="ck-l">CTR</span><span class="ck-v sv-orange">{{ pct(c.ctr) }}</span></div>
              <div class="ck"><span class="ck-l">排名</span><span class="ck-v sv-purple">{{ (c.position||c.avg_position||0).toFixed(1) }}</span></div>
            </template>
            <template v-else>
              <div class="ck"><span class="ck-l">会话</span><span class="ck-v sv-blue">{{ fmt(c.sessions) }}</span></div>
              <div class="ck"><span class="ck-l">用户</span><span class="ck-v sv-green">{{ fmt(c.total_users) }}</span></div>
              <div class="ck"><span class="ck-l">PV</span><span class="ck-v sv-orange">{{ fmt(c.screen_page_views) }}</span></div>
              <div class="ck"><span class="ck-l">新用户</span><span class="ck-v sv-purple">{{ fmt(c.new_users) }}</span></div>
            </template>
          </div>
        </div>
      </div>
    </div>

    <!-- ─── Detail view ─────────────────────────────────────────── -->
    <div v-if="view === 'detail'" class="view-detail">
      <div class="detail-head">
        <button class="btn" @click="backToOverview">← 返回总览</button>
        <div class="detail-title"><span class="tp">{{ platformLabel }}</span> · {{ detail.siteName }}</div>
        <div class="detail-days">
          <el-select v-model="days" size="small" style="width:110px" @change="reloadCurrentView">
            <el-option :value="7" label="近 7 天" /><el-option :value="30" label="近 30 天" /><el-option :value="90" label="近 90 天" />
          </el-select>
        </div>
        <button class="btn btn-sync" @click="syncAndReload" :disabled="syncing">
          {{ syncing ? '同步中...' : '🔄 刷新同步' }}
        </button>
      </div>

      <!-- Sub-tabs -->
      <div class="panel sub-bar">
        <button v-for="t in currentSubTabs" :key="t.key" :class="['sub-pill', subTab === t.key && 'active']" @click="subTab = t.key; loadSubTab()">
          <span v-if="t.icon" class="sp-i">{{ t.icon }}</span>{{ t.label }}
        </button>
      </div>

      <div v-if="loading" class="state-placeholder">加载中...</div>
      <div v-else-if="error" class="state-error">❌ {{ error }}</div>

      <!-- ═══════════ 总览 sub-tab ═══════════ -->
      <template v-else-if="subTab === 'overview'">
        <div v-if="!d.total_clicks && !d.sessions && !d.trend?.length" class="state-empty">
          暂无数据。点击上方「🔄 刷新同步」拉取平台数据。
        </div>
        <template v-else>
          <!-- KPI strip -->
          <div class="kpi-strip">
            <div v-for="k in kpis" :key="k.label" class="panel kpi-card">
              <div class="kpi-label">{{ k.label }}</div><div :class="['kpi-val', k.cls]">{{ k.value }}</div>
            </div>
          </div>
          <div v-if="d.has_deep_data === false" class="hint-bar">⚠ 当前为基础数据。点击「刷新同步」获取完整关键词明细、排名分段、SERP 特征等深度分析。</div>

          <!-- Rank buckets (GSC) -->
          <div v-if="d.buckets?.length" class="section">
            <div class="section-title">📶 排名分布</div>
            <div class="bucket-grid">
              <div v-for="b in d.buckets" :key="b.bucket" :class="['bucket-card', activeBucket===b.bucket && 'selected']" @click="drillBucket(b.bucket)">
                <div class="bk-name">{{ b.bucket }}</div>
                <div class="bk-big">{{ fmt(b.query_count) }} <small>词</small></div>
                <div class="bk-row"><span>点击</span><b>{{ fmt(b.total_clicks) }}</b></div>
                <div class="bk-row"><span>曝光</span><b>{{ fmt(b.total_impressions) }}</b></div>
                <div class="bk-row"><span>CTR</span><b>{{ pct(b.avg_ctr) }}</b></div>
                <div class="bk-footer">点击查看 →</div>
              </div>
            </div>
            <!-- Drill-down -->
            <div class="panel" v-if="activeBucket && drillData" style="margin-top:12px">
              <h3>排名段「{{ activeBucket }}」</h3>
              <el-table :data="drillData.tq" stripe size="small" v-if="drillData.tq?.length" style="margin-bottom:12px">
                <el-table-column prop="query" label="关键词" min-width="250" show-overflow-tooltip>
                  <template #default="{row}"><a class="link" @click="openKwDetail(row.query)">{{ row.query }}</a></template>
                </el-table-column>
                <el-table-column prop="clicks" label="点击" width="80" /><el-table-column prop="impressions" label="曝光" width="90" />
              </el-table>
              <el-table :data="drillData.tp" stripe size="small" v-if="drillData.tp?.length">
                <el-table-column prop="url" label="页面" min-width="350" show-overflow-tooltip /><el-table-column prop="clicks" label="点击" width="80" />
              </el-table>
            </div>
          </div>

          <!-- Trend -->
          <div class="panel" v-if="d.trend?.length >= 2">
            <div class="section-title">📈 每日趋势</div>
            <div class="trend-wrap">
              <svg :viewBox="`0 0 ${tW} ${tH}`" preserveAspectRatio="none" class="trend-svg">
                <defs><linearGradient id="tg" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="var(--sun)" stop-opacity=".4"/><stop offset="100%" stop-color="var(--sun)" stop-opacity="0"/></linearGradient></defs>
                <polygon :points="tArea" fill="url(#tg)"/><polyline :points="tLine" fill="none" stroke="var(--sun)" stroke-width="2" vector-effect="non-scaling-stroke"/>
              </svg>
            </div>
            <div class="trend-labels"><span>{{ d.trend[0]?.date }}</span><span>{{ d.trend[d.trend.length-1]?.date }}</span></div>
          </div>

          <!-- SERP appearances (GSC) -->
          <div class="panel" v-if="d.appearances?.length">
            <div class="section-title">🎯 SERP 特征</div>
            <div class="serp-cards">
              <div class="serp-card" v-for="a in d.appearances" :key="a.appearance">
                <div class="sc-name">{{ serpLabel(a.appearance) }}</div>
                <div class="sc-stats"><span><b>{{ fmt(a.impressions) }}</b> 曝光</span><span><b>{{ fmt(a.clicks) }}</b> 点击</span><span>CTR <b>{{ pct(a.ctr) }}</b></span></div>
              </div>
            </div>
          </div>

          <!-- Keywords + Pages -->
          <div class="two-col">
            <div class="panel" v-if="d.top_keywords?.length">
              <div class="panel-head"><div class="section-title" style="border:none;margin:0;padding:0">🔑 热门关键词</div>
                <button class="btn btn-link" @click="subTab='keywords';loadSubTab()">查看全部 →</button>
              </div>
              <table class="tbl"><thead><tr><th>#</th><th>关键词</th><th>点击</th><th>展示</th><th>CTR</th><th>排名</th></tr></thead>
                <tbody><tr v-for="(r,i) in d.top_keywords" :key="i">
                  <td class="rk">{{ i+1 }}</td>
                  <td class="kw"><a class="link" @click="openKwDetail(r.query)">{{ r.query }}</a></td>
                  <td class="num sv-blue">{{ fmt(r.clicks) }}</td><td class="num sv-green">{{ fmt(r.impressions) }}</td>
                  <td class="num sv-orange">{{ pct(r.ctr) }}</td><td class="num sv-purple">{{ (r.position||0).toFixed?.(1)||'-' }}</td>
                </tr></tbody>
              </table>
            </div>
            <div class="panel" v-if="d.top_pages?.length">
              <div class="panel-head"><div class="section-title" style="border:none;margin:0;padding:0">📄 热门页面</div>
                <button class="btn btn-link" @click="subTab='pages';loadSubTab()">查看全部 →</button>
              </div>
              <table class="tbl"><thead><tr><th>#</th><th>页面</th><th>点击</th><th>展示</th></tr></thead>
                <tbody><tr v-for="(r,i) in d.top_pages" :key="i">
                  <td class="rk">{{ i+1 }}</td><td class="pg">{{ cleanPath(r.page_url||r.page) }}</td>
                  <td class="num sv-blue">{{ fmt(r.clicks) }}</td><td class="num sv-green">{{ fmt(r.impressions) }}</td>
                </tr></tbody>
              </table>
            </div>
          </div>

          <!-- Devices + Countries -->
          <div class="two-col">
            <div class="panel" v-if="d.devices?.length">
              <div class="section-title">📱 设备分布</div>
              <div class="bar-list"><div v-for="x in withPct(d.devices)" :key="x.label" class="bar-item">
                <div class="bar-head"><span class="bar-name">{{ devLabel(x.label) }}</span><span class="bar-val">{{ fmt(x.clicks) }} · {{ x.pct.toFixed(1) }}%</span></div>
                <div class="bar-track"><div class="bar-fill" :style="{width:x.pct+'%'}"></div></div>
              </div></div>
            </div>
            <div class="panel" v-if="d.countries?.length">
              <div class="section-title">🌍 国家/地区</div>
              <div class="bar-list"><div v-for="x in withPct(d.countries)" :key="x.label" class="bar-item">
                <div class="bar-head"><span class="bar-name">{{ (x.label||'').toUpperCase() }}</span><span class="bar-val">{{ fmt(x.clicks) }} · {{ x.pct.toFixed(1) }}%</span></div>
                <div class="bar-track"><div class="bar-fill bar-green" :style="{width:x.pct+'%'}"></div></div>
              </div></div>
            </div>
          </div>
        </template>
      </template>

      <!-- ═══════════ GSC deep sub-tabs ═══════════ -->
      <template v-else-if="platform === 'gsc' && subTab !== 'overview'">
        <!-- Keywords -->
        <div v-if="subTab==='keywords'" class="panel">
          <div class="panel-head"><h3>🔍 关键词分析</h3><el-button type="success" size="small" @click="exportCSV">📥 导出 CSV</el-button></div>
          <div class="filter-row">
            <el-input v-model="kwSearch" placeholder="搜索..." clearable style="width:200px" @keyup.enter="loadSubTab" />
            <el-select v-model="kwBucket" clearable placeholder="排名段" style="width:110px" @change="kwPage=1;loadSubTab()">
              <el-option value="" label="全部"/><el-option value="1-3"/><el-option value="4-10"/><el-option value="11-20"/><el-option value="21-50"/><el-option value="51-100"/><el-option value="100+"/>
            </el-select>
            <el-select v-model="kwIntent" clearable placeholder="意图" style="width:100px" @change="kwPage=1;loadSubTab()">
              <el-option value="" label="全部"/><el-option value="informational" label="信息"/><el-option value="transactional" label="交易"/><el-option value="commercial" label="商业"/>
            </el-select>
            <el-button size="small" @click="kwPage=1;loadSubTab()">查询</el-button>
          </div>
          <el-table :data="kwData.items" v-loading="subLoading" stripe border size="small">
            <el-table-column type="index" width="40"/><el-table-column prop="query" label="关键词" min-width="200" show-overflow-tooltip>
              <template #default="{row}"><a class="link" @click="openKwDetail(row.query)">{{ row.query }}</a></template>
            </el-table-column>
            <el-table-column prop="page_url" label="落地页" min-width="180" show-overflow-tooltip/>
            <el-table-column prop="position" label="排名" width="65" :formatter="(_:any,__:any,___:any,r:any)=>r.position?.toFixed(1)"/>
            <el-table-column prop="clicks" label="点击" width="70" sortable/><el-table-column prop="impressions" label="曝光" width="75" sortable/>
            <el-table-column label="CTR" width="70" :formatter="(_:any,__:any,___:any,r:any)=>pct(r.ctr)"/>
            <el-table-column label="" width="55"><template #default="{row}"><el-button link size="small" @click="openKwDetail(row.query)">详情</el-button></template></el-table-column>
          </el-table>
          <div class="pager" v-if="kwData.total>0"><el-pagination layout="total,sizes,prev,pager,next" :total="kwData.total" v-model:current-page="kwPage" v-model:page-size="kwPageSize" :page-sizes="[25,50,100]" @size-change="loadSubTab" @current-change="loadSubTab"/></div>
        </div>
        <!-- Pages -->
        <div v-if="subTab==='pages'" class="panel">
          <h3>📄 页面分析</h3>
          <el-input v-model="pgSearch" placeholder="搜索URL..." clearable style="width:300px;margin-bottom:8px" @keyup.enter="loadSubTab"/>
          <el-table :data="pgData.items" v-loading="subLoading" stripe border size="small">
            <el-table-column type="index" width="40"/><el-table-column prop="page_url" label="URL" min-width="320" show-overflow-tooltip/>
            <el-table-column prop="clicks" label="点击" width="80" sortable/><el-table-column prop="impressions" label="曝光" width="90" sortable/>
            <el-table-column label="CTR" width="70" :formatter="(_:any,__:any,___:any,r:any)=>pct(r.ctr)"/><el-table-column prop="query_count" label="词数" width="60"/>
          </el-table>
          <div class="pager" v-if="pgData.total>0"><el-pagination layout="total,prev,pager,next" :total="pgData.total" v-model:current-page="pgPage" @current-change="loadSubTab"/></div>
        </div>
        <!-- SERP Features -->
        <div v-if="subTab==='serp'" class="panel"><h3>🎯 SERP 特征</h3>
          <p class="hint" v-if="!appearanceList.length">暂无。同步后可看到 AI Overview / Featured Snippet 等分布。</p>
          <div class="serp-cards" v-if="appearanceList.length"><div class="serp-card" v-for="a in appearanceList" :key="a.appearance">
            <div class="sc-name">{{ serpLabel(a.appearance) }}</div>
            <div class="sc-stats"><span><b>{{ fmt(a.impressions) }}</b> 曝光</span><span><b>{{ fmt(a.clicks) }}</b> 点击</span><span>CTR <b>{{ pct(a.ctr) }}</b></span></div>
          </div></div>
        </div>
        <!-- Index Status -->
        <div v-if="subTab==='index'" class="panel">
          <div class="panel-head"><h3>📋 收录状态</h3><el-button type="warning" size="small" :loading="inspecting" @click="runInspection">🔍 检查 TOP 100</el-button></div>
          <p class="hint" v-if="!idxSummary.length">暂无。点击按钮检查 TOP 100 页面收录（约 2 分钟）。</p>
          <div class="idx-summary" v-if="idxSummary.length"><div v-for="s in idxSummary" :key="s.state" :class="['idx-card', idxFilter===s.state&&'selected', isOk(s.state)&&'ok', isWarn(s.state)&&'warn', isErr(s.state)&&'err']" @click="idxFilter=idxFilter===s.state?'':s.state;loadSubTab()">
            <div class="ic-count">{{ s.count }}</div><div class="ic-label">{{ s.state }}</div>
          </div></div>
          <el-table :data="idxItems" v-if="idxItems.length" stripe border size="small" style="margin-top:12px">
            <el-table-column prop="url" label="URL" min-width="300" show-overflow-tooltip/>
            <el-table-column prop="coverage_state" label="状态" width="200"/>
            <el-table-column prop="verdict" label="判决" width="65"><template #default="{row}"><el-tag :type="row.verdict==='PASS'?'success':'danger'" size="small">{{ row.verdict }}</el-tag></template></el-table-column>
            <el-table-column prop="last_crawl_time" label="上次抓取" width="100" :formatter="(_:any,__:any,___:any,r:any)=>r.last_crawl_time?.slice(0,10)||'-'"/>
          </el-table>
        </div>
        <!-- Sitemap -->
        <div v-if="subTab==='sitemaps'" class="panel"><h3>🗺️ Sitemap</h3>
          <p class="hint" v-if="!sitemapList.length">暂无。</p>
          <el-table :data="sitemapList" v-if="sitemapList.length" stripe border size="small">
            <el-table-column prop="path" label="URL" min-width="350" show-overflow-tooltip/><el-table-column prop="errors" label="错误" width="60"/><el-table-column prop="warnings" label="警告" width="60"/>
          </el-table>
        </div>
        <!-- Opportunities -->
        <div v-if="subTab==='opportunities'" class="panel"><h3>💡 机会</h3>
          <p class="hint" v-if="!oppList.length">暂无。同步后等待计算。</p>
          <el-table :data="oppList" v-if="oppList.length" stripe border size="small">
            <el-table-column prop="query" label="关键词" min-width="200" show-overflow-tooltip><template #default="{row}"><a class="link" @click="openKwDetail(row.query)">{{ row.query }}</a></template></el-table-column>
            <el-table-column label="排名" width="60" :formatter="(_:any,__:any,___:any,r:any)=>r.position?.toFixed(1)"/>
            <el-table-column prop="opportunity_score" label="机会分" width="70" sortable><template #default="{row}"><b style="color:#e6a23c">{{ row.opportunity_score }}</b></template></el-table-column>
            <el-table-column prop="estimated_extra_clicks" label="预估月增" width="80"/>
          </el-table>
        </div>
        <!-- Cannibalization -->
        <div v-if="subTab==='cannibal'" class="panel"><h3>⚠️ 自蚕食</h3>
          <p class="hint" v-if="!cannibalList.length">暂无。</p>
          <el-table :data="cannibalList" v-if="cannibalList.length" stripe border size="small">
            <el-table-column prop="query" label="关键词" min-width="200" show-overflow-tooltip/>
            <el-table-column prop="page_count" label="争抢页面" width="80"><template #default="{row}"><b style="color:#f56c6c">{{ row.page_count }}</b></template></el-table-column>
            <el-table-column prop="severity" label="严重度" width="70"/><el-table-column prop="total_impressions" label="总曝光" width="80"/>
          </el-table>
        </div>
        <!-- Diff -->
        <div v-if="subTab==='diff'" class="panel"><h3>📊 关键词变动</h3>
          <div class="filter-row"><el-radio-group v-model="diffType" @change="loadSubTab"><el-radio-button value="lost">丢失词</el-radio-button><el-radio-button value="new">新词</el-radio-button></el-radio-group></div>
          <p class="hint" v-if="!diffList.length">暂无。</p>
          <el-table :data="diffList" v-if="diffList.length" stripe border size="small">
            <el-table-column prop="query" label="关键词" min-width="250"/><el-table-column prop="prev_clicks" label="上期" width="80"/><el-table-column prop="curr_clicks" label="本期" width="80"/>
            <el-table-column prop="click_delta" label="变化" width="70"><template #default="{row}"><span :style="{color:row.click_delta>0?'#67c23a':'#f56c6c',fontWeight:600}">{{ row.click_delta>0?'+':'' }}{{ row.click_delta }}</span></template></el-table-column>
          </el-table>
        </div>
      </template>

      <!-- ═══════════ GA4/Bing/Yandex deep sub-tabs ═══════════ -->
      <template v-else-if="subTab !== 'overview'">
        <div class="panel">
          <div v-if="subLoading" class="state-placeholder">加载中...</div>
          <!-- GA4 -->
          <template v-if="platform==='ga4' && subTab==='landing'"><h3>入站页</h3><el-table :data="gaLanding" v-if="gaLanding.length" stripe border size="small"><el-table-column prop="landing_page" label="入站页" min-width="300" show-overflow-tooltip/><el-table-column prop="sessions" label="会话" width="75"/><el-table-column prop="users" label="用户" width="70"/><el-table-column label="参与率" width="70" :formatter="(_:any,__:any,___:any,r:any)=>pct(r.engagement_rate)"/></el-table></template>
          <template v-if="platform==='ga4' && subTab==='source'"><h3>流量来源</h3><el-table :data="gaSources" v-if="gaSources.length" stripe border size="small"><el-table-column prop="source" label="来源" width="140"/><el-table-column prop="medium" label="媒介" width="110"/><el-table-column prop="sessions" label="会话" width="75"/><el-table-column prop="users" label="用户" width="70"/></el-table></template>
          <template v-if="platform==='ga4' && subTab==='events'"><h3>事件</h3><el-table :data="gaEvents" v-if="gaEvents.length" stripe border size="small"><el-table-column prop="event_name" label="事件" min-width="200"/><el-table-column prop="event_count" label="次数" width="80"/><el-table-column prop="users" label="用户" width="70"/></el-table></template>
          <template v-if="platform==='ga4' && subTab==='tech'"><h3>技术</h3><div class="filter-row"><el-radio-group v-model="techDim" @change="loadSubTab"><el-radio-button value="device">设备</el-radio-button><el-radio-button value="os">OS</el-radio-button><el-radio-button value="browser">浏览器</el-radio-button></el-radio-group></div><el-table :data="gaTech" v-if="gaTech.length" stripe border size="small"><el-table-column prop="value" label="维度" min-width="180"/><el-table-column prop="sessions" label="会话" width="80"/><el-table-column prop="users" label="用户" width="70"/></el-table></template>
          <template v-if="platform==='ga4' && subTab==='geo'"><h3>地理</h3><el-table :data="gaGeo" v-if="gaGeo.length" stripe border size="small"><el-table-column prop="country" label="国家" width="110"/><el-table-column prop="region" label="地区" width="130"/><el-table-column prop="sessions" label="会话" width="75"/></el-table></template>
          <template v-if="platform==='ga4' && subTab==='hourly'"><h3>小时</h3><el-table :data="gaHourly" v-if="gaHourly.length" stripe border size="small"><el-table-column label="时间" width="70" :formatter="(_:any,__:any,___:any,r:any)=>r.hour+':00'"/><el-table-column prop="sessions" label="会话" width="80"/><el-table-column prop="screen_page_views" label="PV" width="70"/></el-table></template>
          <!-- Bing -->
          <template v-if="platform==='bing' && subTab==='keywords'"><h3>Bing 关键词</h3><el-table :data="bingKw.items" v-if="bingKw.items?.length" stripe border size="small"><el-table-column prop="query" label="关键词" min-width="240"/><el-table-column prop="clicks" label="点击" width="75"/><el-table-column prop="impressions" label="曝光" width="80"/></el-table></template>
          <template v-if="platform==='bing' && subTab==='pages'"><h3>Bing 页面</h3><el-table :data="bingPg.items" v-if="bingPg.items?.length" stripe border size="small"><el-table-column prop="page_url" label="页面" min-width="320"/><el-table-column prop="clicks" label="点击" width="75"/></el-table></template>
          <template v-if="platform==='bing' && subTab==='backlinks'"><h3>反链</h3><el-table :data="bingBL" v-if="bingBL.length" stripe border size="small"><el-table-column prop="source_url" label="来源" min-width="300"/><el-table-column prop="anchor_text" label="锚文本" width="140"/></el-table></template>
          <!-- Yandex -->
          <template v-if="platform==='yandex' && subTab==='keywords'"><h3>Yandex 关键词</h3><el-table :data="yandexKw.items" v-if="yandexKw.items?.length" stripe border size="small"><el-table-column prop="query" label="关键词" min-width="240"/><el-table-column prop="clicks" label="点击" width="75"/><el-table-column prop="shows" label="曝光" width="80"/></el-table></template>
          <template v-if="platform==='yandex' && subTab==='index_events'"><h3>索引事件</h3><el-table :data="yandexIE" v-if="yandexIE.length" stripe border size="small"><el-table-column prop="date" label="日期" width="100"/><el-table-column prop="event_type" label="事件" min-width="220"/><el-table-column prop="count" label="数量" width="75"/></el-table></template>
          <template v-if="platform==='yandex' && subTab==='http_status'"><h3>HTTP 状态</h3><el-table :data="yandexHS" v-if="yandexHS.length" stripe border size="small"><el-table-column prop="date" label="日期" width="100"/><el-table-column prop="status_code" label="状态码" min-width="200"/><el-table-column prop="count" label="数量" width="75"/></el-table></template>
          <template v-if="platform==='yandex' && subTab==='external_links'"><h3>外链</h3><el-table :data="yandexEL" v-if="yandexEL.length" stripe border size="small"><el-table-column prop="source_url" label="来源" min-width="300"/><el-table-column prop="target_url" label="目标" min-width="200"/></el-table></template>
        </div>
      </template>
    </div>

    <!-- ─── Keyword detail dialog ───────────────────────────────── -->
    <el-dialog v-model="kwDlgShow" title="🔑 关键词详情" width="850px" destroy-on-close top="5vh">
      <div v-if="kwDlg">
        <div style="font-size:18px;font-weight:700;margin-bottom:8px">{{ kwDlg.query }}</div>
        <div style="display:flex;gap:6px;margin-bottom:16px;flex-wrap:wrap">
          <el-tag size="small">{{ kwDlg.intent||'unknown' }}</el-tag>
          <el-tag v-if="kwDlg.brand==='brand'" size="small" type="warning">品牌词</el-tag>
          <el-tag v-if="kwDlg.is_question" size="small" type="danger">问题型</el-tag>
          <el-tag v-if="kwDlg.cannibalization" size="small" type="danger">自蚕食 ({{ kwDlg.cannibal_page_count }} 页)</el-tag>
        </div>
        <h4>📈 趋势</h4>
        <el-table :data="kwDlg.trend" stripe size="small" max-height="250" border>
          <el-table-column prop="date" label="日期" width="95"/><el-table-column prop="clicks" label="点击" width="65"/><el-table-column prop="impressions" label="曝光" width="75"/>
          <el-table-column label="排名" width="65" :formatter="(_:any,__:any,___:any,r:any)=>r.position?.toFixed(1)"/>
        </el-table>
        <h4 style="margin-top:16px">📄 对应页面</h4>
        <el-table :data="kwDlg.pages" stripe size="small" border>
          <el-table-column prop="page_url" label="页面" min-width="300" show-overflow-tooltip/><el-table-column prop="clicks" label="点击" width="65"/><el-table-column prop="impressions" label="曝光" width="75"/>
        </el-table>
        <div v-if="kwDlg.devices?.length" style="margin-top:16px"><h4>📱 设备</h4>
          <div style="display:flex;gap:8px;flex-wrap:wrap"><div v-for="x in kwDlg.devices" :key="x.device" style="background:#f9fafb;border:1px solid #eee;border-radius:8px;padding:8px 14px;min-width:100px">
            <div style="font-weight:600;font-size:13px;text-transform:capitalize">{{ x.device }}</div>
            <div style="font-size:12px;color:#666">点击 <b>{{ x.clicks }}</b> · 排名 <b>{{ x.position?.toFixed(1) }}</b></div>
          </div></div>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

// ── Config ──
const platforms = [{ code: 'gsc', label: 'GSC' }, { code: 'ga4', label: 'GA4' }, { code: 'bing', label: 'Bing' }, { code: 'yandex', label: 'Yandex' }]
const platform = ref<string>('gsc')
const platformLabel = computed(() => platforms.find(p => p.code === platform.value)?.label || '')
const days = ref(30)
const view = ref<'overview'|'detail'>('overview')
const loading = ref(false)
const syncing = ref(false)
const error = ref('')
const subTab = ref('overview')
const subLoading = ref(false)

const presets = [{ key: '7d', label: '近 7 天' }, { key: '28d', label: '近 28 天' }, { key: '90d', label: '近 90 天' }]
const activePreset = ref('28d')
function applyPreset(k: string) { activePreset.value = k; days.value = k === '7d' ? 7 : k === '28d' ? 28 : 90; loadOverview() }

// ── Sub-tabs config ──
const gscSubs = [
  { key: 'overview', label: '总览', icon: '📊' }, { key: 'buckets', label: '排名分布', icon: '📶' },
  { key: 'keywords', label: '关键词', icon: '🔑' }, { key: 'pages', label: '页面', icon: '📄' },
  { key: 'serp', label: 'SERP特征', icon: '🎯' }, { key: 'index', label: '收录状态', icon: '📋' },
  { key: 'sitemaps', label: 'Sitemap', icon: '🗺️' }, { key: 'opportunities', label: '机会', icon: '💡' },
  { key: 'cannibal', label: '自蚕食', icon: '⚠️' }, { key: 'diff', label: '变动', icon: '📊' },
]
const ga4Subs = [{ key: 'overview', label: '总览' }, { key: 'landing', label: '入站页' }, { key: 'source', label: '来源' }, { key: 'events', label: '事件' }, { key: 'tech', label: '技术' }, { key: 'geo', label: '地理' }, { key: 'hourly', label: '小时' }]
const bingSubs = [{ key: 'overview', label: '总览' }, { key: 'keywords', label: '关键词' }, { key: 'pages', label: '页面' }, { key: 'backlinks', label: '反链' }]
const yandexSubs = [{ key: 'overview', label: '总览' }, { key: 'keywords', label: '关键词' }, { key: 'index_events', label: '索引事件' }, { key: 'http_status', label: 'HTTP状态' }, { key: 'external_links', label: '外链' }]
const currentSubTabs = computed(() => ({ gsc: gscSubs, ga4: ga4Subs, bing: bingSubs, yandex: yandexSubs }[platform.value] || []))

// ── Helpers ──
const fmt = (n: any) => { const v = Number(n||0); return v >= 1e6 ? (v/1e6).toFixed(1)+'M' : v >= 1e3 ? (v/1e3).toFixed(1)+'K' : Math.round(v).toLocaleString() }
const pct = (v: any) => v != null ? (Number(v)*100).toFixed(2)+'%' : '-'
const cleanPath = (u: string) => (u||'').replace(/^https?:\/\/[^/]+/, '') || '/'
const devLabel = (s: string) => ({ MOBILE: '手机', DESKTOP: '桌面', TABLET: '平板', mobile: '手机', desktop: '桌面', tablet: '平板' }[s] || s)
const serpLabel = (s: string) => ({ AI_OVERVIEW: '🤖 AI Overview', FEATURED_SNIPPET: '⭐ 精选摘要', TPF_FAQ: '❓ FAQ', REVIEW_SNIPPET: '⭐ 评分', VIDEO_RESULT: '🎬 视频', RICH_RESULT: '✨ 富结果' }[s] || s)
const isOk = (s: string) => s?.includes('indexed'); const isWarn = (s: string) => s?.includes('not indexed'); const isErr = (s: string) => s?.includes('404') || s?.includes('noindex')
function withPct(items: any[]) { const total = items.reduce((s: number, x: any) => s + Number(x.clicks||0), 0) || 1; return items.map((x: any) => ({ ...x, pct: (Number(x.clicks||0) / total) * 100 })) }

// ── Overview ──
const overview = ref<{ cards: any[] }>({ cards: [] })
async function loadOverview() {
  loading.value = true; error.value = ''
  const params = { start_date: '', end_date: '' } // dashboard endpoints handle their own date logic
  try {
    let res: any
    switch (platform.value) {
      case 'gsc': res = await api.dashGSCOverview(params); break
      case 'ga4': res = await api.dashGAOverview(params); break
      case 'bing': res = await api.dashBingOverview(params); break
      case 'yandex': res = await api.dashYandexOverview(params); break
    }
    overview.value.cards = res.data?.cards || []
  } catch (e: any) { error.value = e?.message || '加载失败'; overview.value.cards = [] }
  finally { loading.value = false }
}
function switchPlatform(code: string) { platform.value = code; view.value = 'overview'; subTab.value = 'overview'; loadOverview() }
function platformSiteKey(c: any) { return ({ gsc: c.site_url, ga4: c.property_id, bing: c.site_url, yandex: c.host_id }[platform.value]) || '' }
async function hideCard(c: any) {
  const key = platformSiteKey(c); if (!key) return
  try { await ElMessageBox.confirm(`隐藏「${c.display_name||key}」？`, '隐藏', { type: 'warning' }) } catch { return }
  try { await api.hidePlatformSite(platform.value, key); overview.value.cards = overview.value.cards.filter(x => platformSiteKey(x) !== key); ElMessage.success('已隐藏') } catch {}
}

// ── Detail ──
const detail = ref({ siteId: 0, siteName: '', siteKey: '' })
const d = ref<any>({}) // unified data from backend
const activeBucket = ref('')
const drillData = computed(() => {
  if (!activeBucket.value || !d.value.buckets) return null
  const b = d.value.buckets.find((x: any) => x.bucket === activeBucket.value)
  if (!b) return null
  let tq: any[] = []; let tp: any[] = []
  try { tq = typeof b.top_queries_json === 'string' ? JSON.parse(b.top_queries_json) : b.top_queries_json || [] } catch {}
  try { tp = typeof b.top_pages_json === 'string' ? JSON.parse(b.top_pages_json) : b.top_pages_json || [] } catch {}
  return { tq, tp }
})
function drillBucket(b: string) { activeBucket.value = activeBucket.value === b ? '' : b }

// KPIs computed from unified data
const kpis = computed(() => {
  const v = d.value
  if (platform.value === 'ga4') return [
    { label: '会话', value: fmt(v.sessions), cls: 'sv-blue' }, { label: '用户', value: fmt(v.users), cls: 'sv-green' },
    { label: 'PV', value: fmt(v.pageviews), cls: 'sv-orange' },
    { label: '跳出率', value: pct(v.bounce_rate), cls: 'sv-purple' },
    { label: '参与率', value: pct(v.engagement_rate), cls: 'sv-blue' },
    { label: '新用户', value: fmt(v.new_users), cls: 'sv-green' },
  ]
  return [
    { label: '点击', value: fmt(v.total_clicks), cls: 'sv-blue' }, { label: '展示', value: fmt(v.total_impressions), cls: 'sv-green' },
    { label: 'CTR', value: pct(v.avg_ctr), cls: 'sv-orange' }, { label: '排名', value: (v.avg_position||0).toFixed?.(1)||'-', cls: 'sv-purple' },
  ]
})

// Trend SVG
const tW = 800; const tH = 180
const tLine = computed(() => { const rows = d.value.trend || []; if (rows.length < 2) return ''; const max = Math.max(...rows.map((r:any) => r.clicks||0), 1); return rows.map((r:any, i:number) => { const x = (i/(rows.length-1))*tW; const y = tH - ((r.clicks||0)/max)*(tH-10) - 5; return `${x.toFixed(1)},${y.toFixed(1)}` }).join(' ') })
const tArea = computed(() => { const l = tLine.value; return l ? `0,${tH} ${l} ${tW},${tH}` : '' })

async function openDetail(c: any) {
  const siteKey = platformSiteKey(c)
  if (!siteKey || c.error) return
  detail.value.siteKey = siteKey
  detail.value.siteName = c.display_name || siteKey
  detail.value.siteId = 0
  view.value = 'detail'; subTab.value = 'overview'; activeBucket.value = ''
  d.value = {}

  // Step 1: Load data from platform API immediately (always works, no site_id needed)
  await loadFromPlatformAPI()

  // Step 2: Try to resolve site_id in background for deep data (non-blocking, fire-and-forget)
  resolveAndLoadDeep().catch(() => {})
}

// Load data directly from platform dashboard API (GSC/GA4/Bing/Yandex)
// This is the primary data source — always works regardless of site_id
function daysAgo(n: number) { const d = new Date(); d.setDate(d.getDate() - n); return d.toISOString().slice(0, 10) }

async function loadFromPlatformAPI() {
  loading.value = true; error.value = ''
  const sk = detail.value.siteKey
  const baseParams = { start_date: daysAgo(days.value + 1), end_date: daysAgo(2) }
  try {
    let res: any
    switch (platform.value) {
      case 'gsc':    res = await api.dashGSCSiteDetail({ ...baseParams, site_url: sk }); break
      case 'ga4':    res = await api.dashGAPropertyDetail({ ...baseParams, property_id: sk }); break
      case 'bing':   res = await api.dashBingSiteDetail({ ...baseParams, site_url: sk }); break
      case 'yandex': res = await api.dashYandexHostDetail({ ...baseParams, host_id: sk }); break
    }
    digestPlatformData(res.data)
  } catch (e: any) { error.value = e?.response?.data?.error || e?.message || '加载失败' }
  finally { loading.value = false }
}

// Parse platform API response into unified d.value
function digestPlatformData(raw: any) {
  if (!raw) return
  const p = platform.value
  if (p === 'gsc') {
    const ov = (raw.overview?.rows || [])[0] || {}
    const kws = (raw.keywords?.rows || []).map((r:any) => ({ query: r.keys?.[0]||'', clicks: r.clicks||0, impressions: r.impressions||0, ctr: r.ctr||0, position: r.position||0 }))
    const pgs = (raw.pages?.rows || []).map((r:any) => ({ page_url: r.keys?.[0]||'', clicks: r.clicks||0, impressions: r.impressions||0, ctr: r.ctr||0 }))
    const trend = (raw.trend?.rows || []).map((r:any) => ({ date: r.keys?.[0]||'', clicks: r.clicks||0, impressions: r.impressions||0, ctr: r.ctr||0 }))
    const devs = (raw.devices?.rows || []).map((r:any) => ({ label: r.keys?.[0]||'', clicks: r.clicks||0 }))
    const ctrs = (raw.countries?.rows || []).slice(0,10).map((r:any) => ({ label: r.keys?.[0]||'', clicks: r.clicks||0 }))
    d.value = { platform: 'gsc', total_clicks: ov.clicks||0, total_impressions: ov.impressions||0, avg_ctr: ov.ctr||0, avg_position: ov.position||0,
      trend, top_keywords: kws, top_pages: pgs, devices: devs, countries: ctrs, has_deep_data: false }
  } else if (p === 'ga4') {
    const toNum = (v:any) => parseFloat(v?.value ?? v ?? '0') || 0
    const ovRow = (raw.overview?.rows || [])[0]
    const mv = ovRow?.metricValues || []
    // GA4 overview metrics order: sessions(0), totalUsers(1), newUsers(2), screenPageViews(3), bounceRate(4), averageSessionDuration(5), engagementRate(6)
    const sessions = toNum(mv[0]), gaUsers = toNum(mv[1]), newUsers = toNum(mv[2]), pvs = toNum(mv[3])
    const br = mv.length > 4 ? toNum(mv[4]) : 0
    const er = mv.length > 6 ? toNum(mv[6]) : 0
    const trend = (raw.trend?.rows || []).map((r:any) => { const dt = r.dimensionValues?.[0]?.value||''; return { date: dt.length===8 ? `${dt.slice(0,4)}-${dt.slice(4,6)}-${dt.slice(6,8)}` : dt, clicks: toNum(r.metricValues?.[0]), impressions: toNum(r.metricValues?.[1]) } })
    const pgs = (raw.pages?.rows || []).map((r:any) => ({ page_url: r.dimensionValues?.[0]?.value||'', clicks: toNum(r.metricValues?.[0]), impressions: toNum(r.metricValues?.[1]) }))
    const devs = (raw.devices?.rows || []).map((r:any) => ({ label: r.dimensionValues?.[0]?.value||'', clicks: toNum(r.metricValues?.[0]) }))
    const ctrs = (raw.countries?.rows || []).slice(0,10).map((r:any) => ({ label: r.dimensionValues?.[0]?.value||'', clicks: toNum(r.metricValues?.[0]) }))
    d.value = { platform: 'ga4', sessions, users: gaUsers, new_users: newUsers, pageviews: pvs,
      bounce_rate: br, engagement_rate: er, trend, top_pages: pgs, devices: devs, countries: ctrs, has_deep_data: false }
  } else if (p === 'bing') {
    const rows = (raw.queries?.d || []).filter((r:any) => r && typeof r === 'object')
    let clicks=0, impr=0
    rows.forEach((r:any) => { clicks += Number(r.Clicks||0); impr += Number(r.Impressions||0) })
    const kws = rows.slice(0,20).map((r:any) => ({ query: r.Query||'', clicks: Number(r.Clicks||0), impressions: Number(r.Impressions||0), position: Number(r.AvgImpPos||0) }))
    const pgs = (raw.pages?.d || []).slice(0,20).map((r:any) => ({ page_url: r.Page||r.Url||'', clicks: Number(r.Clicks||0), impressions: Number(r.Impressions||0) }))
    d.value = { platform: 'bing', total_clicks: clicks, total_impressions: impr, avg_ctr: impr>0?clicks/impr:0, avg_position: 0,
      top_keywords: kws, top_pages: pgs, has_deep_data: false }
  } else if (p === 'yandex') {
    const ind = raw.history?.indicators || {}
    const sumA = (a:any[]) => (a||[]).reduce((s:number, x:any) => s + (x.value||0), 0)
    const clicks = sumA(ind.TOTAL_CLICKS); const shows = sumA(ind.TOTAL_SHOWS)
    const trend = (ind.TOTAL_CLICKS||[]).map((x:any, i:number) => ({ date: x.date||'', clicks: x.value||0, impressions: (ind.TOTAL_SHOWS||[])[i]?.value||0 }))
    const pop = raw.popular?.queries || raw.popular?.popular_queries || []
    const kws = pop.slice(0,20).map((q:any) => { const i = q.indicators||{}; return { query: q.query_text||q.query||'', clicks: i.TOTAL_CLICKS||0, impressions: i.TOTAL_SHOWS||0, position: i.AVG_SHOW_POSITION||0 } })
    d.value = { platform: 'yandex', total_clicks: clicks, total_impressions: shows, avg_ctr: shows>0?clicks/shows:0,
      trend, top_keywords: kws, has_deep_data: false }
  }
}

// Try to resolve site_id and load deep data in background (optional, enhances with buckets/appearance/etc)
async function resolveAndLoadDeep() {
  try {
    const r: any = await api.resolveSiteId(detail.value.siteKey)
    detail.value.siteId = r.data?.site_id || 0
  } catch {
    try { const r2: any = await api.resolveSiteId(detail.value.siteName); detail.value.siteId = r2.data?.site_id || 0 } catch { detail.value.siteId = 0 }
  }
  const sid = detail.value.siteId
  if (!sid) return // No site_id — platform API data is sufficient

  // Load deep data to enhance the view (buckets, appearances, etc.)
  try {
    const res: any = await api.getUnifiedData(sid, platform.value, days.value)
    const deep = res.data || {}
    // Merge deep data on top of platform data (deep data has more detail)
    if (deep.has_deep_data) {
      d.value = { ...d.value, ...deep }
    } else {
      // Only add supplementary fields from unified (buckets/appearances) without overwriting
      if (deep.buckets?.length) d.value.buckets = deep.buckets
      if (deep.appearances?.length) d.value.appearances = deep.appearances
    }
  } catch {} // Silently fail — platform API data is already showing
}

// Refresh: reload platform data + trigger deep sync if site_id available
async function syncAndReload() {
  syncing.value = true
  // Always reload platform API data first
  await loadFromPlatformAPI()

  // If we have site_id, also trigger deep sync
  const sid = detail.value.siteId
  if (sid) {
    try {
      if (platform.value === 'gsc') await api.syncGSCDeep(sid)
      else if (platform.value === 'ga4') await api.syncGA4Deep(sid)
      else if (platform.value === 'bing') await api.syncBingDeep(sid)
      else if (platform.value === 'yandex') await api.syncYandexDeep(sid)
    } catch {} // Deep sync failure is OK — platform data is already loaded
    // Reload deep data
    resolveAndLoadDeep()
  }
  syncing.value = false
  ElMessage.success('数据已刷新')
}

function backToOverview() { view.value = 'overview'; subTab.value = 'overview'; error.value = ''; d.value = {} }

// ── Sub-tab loaders (deep data from existing endpoints) ──
const kwData = ref<any>({ items: [], total: 0 }); const kwSearch = ref(''); const kwBucket = ref(''); const kwIntent = ref(''); const kwPage = ref(1); const kwPageSize = ref(25)
const pgData = ref<any>({ items: [], total: 0 }); const pgSearch = ref(''); const pgPage = ref(1)
const appearanceList = ref<any[]>([]); const sitemapList = ref<any[]>([]); const oppList = ref<any[]>([]); const cannibalList = ref<any[]>([])
const diffList = ref<any[]>([]); const diffType = ref('lost')
const idxSummary = ref<any[]>([]); const idxItems = ref<any[]>([]); const idxFilter = ref(''); const inspecting = ref(false)
const gaLanding = ref<any[]>([]); const gaSources = ref<any[]>([]); const gaEvents = ref<any[]>([]); const gaTech = ref<any[]>([]); const gaGeo = ref<any[]>([]); const gaHourly = ref<any[]>([]); const techDim = ref('device')
const bingKw = ref<any>({ items: [] }); const bingPg = ref<any>({ items: [] }); const bingBL = ref<any[]>([])
const yandexKw = ref<any>({ items: [] }); const yandexIE = ref<any[]>([]); const yandexHS = ref<any[]>([]); const yandexEL = ref<any[]>([])
const kwDlgShow = ref(false); const kwDlg = ref<any>(null)

// FIX #1: reload when days changes
async function reloadCurrentView() {
  if (view.value === 'overview') { loadOverview(); return }
  await loadFromPlatformAPI()
  resolveAndLoadDeep()
  if (subTab.value !== 'overview') loadSubTab()
}

async function loadSubTab() {
  const sid = detail.value.siteId
  if (!sid) { subLoading.value = false; return } // FIX #12: don't show infinite spinner
  subLoading.value = true
  try {
    const t = subTab.value; const dd = days.value
    if (platform.value === 'gsc') {
      if (t === 'buckets' || t === 'overview') { subLoading.value = false; return } // buckets loaded from unified data
      if (t === 'keywords') { const r: any = await api.getGSCKeywords(sid, { page: kwPage.value, page_size: kwPageSize.value, days: dd, search: kwSearch.value||undefined, bucket: kwBucket.value||undefined, intent: kwIntent.value||undefined }); kwData.value = r.data || { items: [], total: 0 } }
      else if (t === 'pages') { const r: any = await api.getGSCDeepPages(sid, { page: pgPage.value, days: dd, search: pgSearch.value||undefined }); pgData.value = r.data || { items: [], total: 0 } }
      else if (t === 'serp') { const r: any = await api.getGSCAppearance(sid, dd); appearanceList.value = r.data || [] }
      else if (t === 'index') { const r: any = await api.getGSCUrlInspection(sid, { coverage_state: idxFilter.value||undefined }); idxItems.value = r.data?.items || []; idxSummary.value = r.data?.summary || [] }
      else if (t === 'sitemaps') { const r: any = await api.getGSCSitemaps(sid); sitemapList.value = r.data || [] }
      else if (t === 'opportunities') { const r: any = await api.getGSCOpportunities(sid); oppList.value = r.data || [] }
      else if (t === 'cannibal') { const r: any = await api.getGSCCannibalization(sid); cannibalList.value = r.data || [] }
      else if (t === 'diff') { const r: any = await api.getGSCQueryDiff(sid, diffType.value); diffList.value = r.data || [] }
    } else if (platform.value === 'ga4') {
      if (t === 'landing') { const r: any = await api.getGA4LandingPages(sid, { days: dd }); gaLanding.value = r.data?.items || r.data || [] }
      else if (t === 'source') { const r: any = await api.getGA4SourceMedium(sid, dd); gaSources.value = r.data || [] }
      else if (t === 'events') { const r: any = await api.getGA4Events(sid, dd); gaEvents.value = r.data || [] }
      else if (t === 'tech') { const r: any = await api.getGA4Tech(sid, techDim.value, dd); gaTech.value = r.data || [] }
      else if (t === 'geo') { const r: any = await api.getGA4Geo(sid, dd); gaGeo.value = r.data || [] }
      else if (t === 'hourly') { const r: any = await api.getGA4Hourly(sid, dd); gaHourly.value = r.data || [] }
    } else if (platform.value === 'bing') {
      if (t === 'keywords') { const r: any = await api.getBingKeywords(sid); bingKw.value = r.data || { items: [] } }
      else if (t === 'pages') { const r: any = await api.getBingDeepPages(sid); bingPg.value = r.data || { items: [] } }
      else if (t === 'backlinks') { const r: any = await api.getBingInboundLinks(sid); bingBL.value = r.data || [] }
    } else if (platform.value === 'yandex') {
      if (t === 'keywords') { const r: any = await api.getYandexKeywords(sid); yandexKw.value = r.data || { items: [] } }
      else if (t === 'index_events') { const r: any = await api.getYandexIndexEvents(sid, dd); yandexIE.value = r.data || [] }
      else if (t === 'http_status') { const r: any = await api.getYandexHTTPStatus(sid, dd); yandexHS.value = r.data || [] }
      else if (t === 'external_links') { const r: any = await api.getYandexExternalLinks(sid); yandexEL.value = r.data || [] }
    }
  } catch {} finally { subLoading.value = false }
}
async function openKwDetail(q: string) { const sid = detail.value.siteId; if (!sid) return; try { const r: any = await api.getGSCKeywordDetail(sid, q); kwDlg.value = r.data; kwDlgShow.value = true } catch {} }
async function exportCSV() { const sid = detail.value.siteId; if (!sid) return; try { const r: any = await api.getGSCKeywordsExport(sid, days.value); const a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([r.data])); a.download = 'keywords.csv'; a.click() } catch { ElMessage.error('导出失败') } }
async function runInspection() { const sid = detail.value.siteId; if (!sid) return; inspecting.value = true; try { await api.syncGSCURLInspection(sid); ElMessage.success('检查完成'); loadSubTab() } catch {} finally { inspecting.value = false } }

onMounted(() => loadOverview())
</script>

<style scoped>
.data-mgmt { padding:20px 24px; min-height:calc(100vh - 60px); background:var(--bg); }
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:16px 20px; margin-bottom:14px; }
.toolbar { margin-bottom:16px; }
.tb-row { display:flex; align-items:center; gap:16px; flex-wrap:wrap; }
.tb-tabs { display:flex; gap:6px; }
.tb-date { display:flex; gap:6px; margin-left:auto; }
.tb-right { }
.pill { padding:6px 16px; border-radius:20px; border:1.5px solid var(--border-light); background:var(--surface-2); font-size:13px; font-weight:700; color:var(--ink-soft); cursor:pointer; transition:all .15s; }
.pill.active { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; }
.pill:hover:not(.active) { background:var(--sun-light); color:var(--sun-deep); }
.pill-sm { padding:5px 12px; font-size:12px; }
.btn { padding:7px 14px; border-radius:8px; border:1.5px solid var(--border-light); background:var(--surface); font-size:13px; font-weight:700; color:var(--ink-mid); cursor:pointer; transition:all .15s; }
.btn:hover:not(:disabled) { border-color:var(--sun); color:var(--sun-deep); }
.btn:disabled { opacity:.5; cursor:not-allowed; }
.btn-refresh,.btn-sync { background:var(--sun-light); color:var(--sun-deep); border-color:var(--peach); }
.btn-link { background:none; border:none; color:var(--sun-deep); font-size:12px; padding:4px 8px; }
.section-title { font-size:14px; font-weight:800; color:var(--ink); margin:0 0 12px; padding-bottom:6px; border-bottom:1.5px solid var(--border-light); display:flex; align-items:center; gap:8px; }
.badge { font-size:11px; font-weight:700; padding:2px 10px; background:var(--sun-light); color:var(--sun-deep); border-radius:10px; }
.card-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(260px,1fr)); gap:12px; }
.site-card { background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r); padding:14px 16px; cursor:pointer; transition:all .15s; position:relative; }
.site-card:hover { border-color:var(--sun); transform:translateY(-1px); box-shadow:var(--s-amber); }
.card-hide { position:absolute; top:6px; right:8px; width:22px; height:22px; border:none; background:transparent; color:var(--ink-hint); font-size:16px; cursor:pointer; opacity:0; border-radius:6px; }
.site-card:hover .card-hide { opacity:.7; }
.card-hide:hover { background:#FEE2E2; color:#B91C1C; opacity:1!important; }
.card-name { font-size:14px; font-weight:800; color:var(--ink); margin-bottom:10px; word-break:break-all; }
.card-err { border-color:#FECACA; background:#FEF2F2; }
.card-err-msg { font-size:12px; color:#B91C1C; }
.card-kpis { display:grid; grid-template-columns:repeat(2,1fr); gap:8px; }
.ck {}
.ck-l { font-size:10.5px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; }
.ck-v { font-size:15px; font-weight:800; display:block; }
.sv-blue { color:#3B82F6; } .sv-green { color:#10B981; } .sv-orange { color:#F59E0B; } .sv-purple { color:#8B5CF6; }
.detail-head { display:flex; align-items:center; gap:14px; margin-bottom:14px; flex-wrap:wrap; }
.detail-title { font-size:16px; font-weight:800; flex:1; }
.tp { background:var(--sun-light); color:var(--sun-deep); padding:2px 10px; border-radius:8px; font-size:13px; }
.detail-days { }
.sub-bar { padding:10px 16px; display:flex; flex-wrap:wrap; gap:4px; }
.sub-pill { padding:6px 14px; border:1px solid #eee; border-radius:14px; cursor:pointer; background:#fafafa; font-size:12px; transition:all .15s; display:flex; align-items:center; gap:3px; }
.sub-pill:hover { background:#fff3e0; border-color:#ffcc80; }
.sub-pill.active { background:var(--sun); color:#fff; border-color:var(--sun); }
.sp-i { font-size:13px; }
.kpi-strip { display:grid; grid-template-columns:repeat(4,1fr); gap:12px; margin-bottom:14px; }
.kpi-card { text-align:left; padding:14px 18px; }
.kpi-label { font-size:10.5px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; margin-bottom:4px; }
.kpi-val { font-size:22px; font-weight:800; }
.hint-bar { background:#FFFBE6; border:1px solid #FFE58F; border-radius:8px; padding:10px 16px; font-size:13px; color:#AD8B00; margin-bottom:14px; }
.hint { color:#aaa; font-size:13px; margin:0 0 12px; }
.section { margin-bottom:14px; }
.bucket-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(140px,1fr)); gap:10px; }
.bucket-card { background:#f8fbff; border:2px solid #e8f0fe; border-radius:10px; padding:14px; text-align:center; cursor:pointer; transition:all .2s; }
.bucket-card:hover { border-color:#409eff; transform:translateY(-2px); }
.bucket-card.selected { border-color:var(--sun); background:#fff8f0; }
.bk-name { font-size:17px; font-weight:800; color:#1a73e8; margin-bottom:6px; }
.bk-big { font-size:20px; font-weight:700; margin-bottom:4px; }
.bk-big small { font-size:12px; color:#999; font-weight:400; }
.bk-row { display:flex; justify-content:space-between; font-size:12px; color:#666; padding:1px 0; }
.bk-row b { color:#333; }
.bk-footer { font-size:11px; color:#409eff; margin-top:6px; }
.two-col { display:grid; grid-template-columns:1fr 1fr; gap:14px; margin-bottom:14px; }
@media (max-width:1000px) { .two-col { grid-template-columns:1fr; } }
.tbl { width:100%; border-collapse:collapse; }
.tbl th { padding:8px 10px; text-align:left; font-size:11px; font-weight:800; color:var(--ink-mid); border-bottom:2px solid var(--border); background:var(--surface-3); }
.tbl td { padding:8px 10px; border-bottom:1px solid var(--border-light); font-size:12.5px; }
.tbl tr:hover td { background:var(--sun-light); }
.rk { width:32px; color:var(--ink-hint); font-weight:700; }
.num { font-weight:700; text-align:right; white-space:nowrap; }
.kw { max-width:250px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.pg { font-size:11.5px; color:var(--ink-soft); max-width:250px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.link { color:#409eff; cursor:pointer; } .link:hover { text-decoration:underline; }
.panel-head { display:flex; justify-content:space-between; align-items:center; margin-bottom:10px; }
h3 { margin:0 0 10px; font-size:15px; font-weight:600; }
h4 { margin:0 0 8px; font-size:14px; color:#555; }
.trend-wrap { height:160px; background:var(--surface-2); border-radius:8px; padding:8px; margin-bottom:4px; }
.trend-svg { width:100%; height:100%; display:block; }
.trend-labels { display:flex; justify-content:space-between; font-size:11px; color:var(--ink-hint); padding:0 8px; }
.serp-cards { display:grid; grid-template-columns:repeat(auto-fill,minmax(260px,1fr)); gap:8px; }
.serp-card { background:#f9fafb; border:1px solid #eee; border-radius:8px; padding:10px 14px; }
.sc-name { font-weight:600; margin-bottom:4px; }
.sc-stats { display:flex; gap:12px; font-size:12px; color:#666; } .sc-stats b { color:#333; }
.bar-list { display:flex; flex-direction:column; gap:8px; }
.bar-head { display:flex; justify-content:space-between; font-size:12px; margin-bottom:3px; }
.bar-name { font-weight:700; } .bar-val { color:var(--ink-hint); }
.bar-track { height:7px; background:var(--surface-3); border-radius:4px; overflow:hidden; }
.bar-fill { height:100%; background:linear-gradient(90deg,var(--sun),var(--coral)); border-radius:4px; }
.bar-green { background:linear-gradient(90deg,#10B981,#059669); }
.filter-row { display:flex; gap:6px; align-items:center; margin-bottom:10px; flex-wrap:wrap; }
.pager { margin-top:10px; display:flex; justify-content:flex-end; }
.idx-summary { display:flex; gap:6px; flex-wrap:wrap; margin-bottom:10px; }
.idx-card { padding:8px 14px; border-radius:8px; border:2px solid #eee; cursor:pointer; text-align:center; min-width:80px; transition:all .15s; }
.idx-card:hover { border-color:#409eff; } .idx-card.selected { border-color:var(--sun); background:#fff8f0; }
.idx-card.ok { background:#f0f9eb; border-color:#c2e7b0; } .idx-card.warn { background:#fdf6ec; border-color:#f5dab1; } .idx-card.err { background:#fef0f0; border-color:#fbc4c4; }
.ic-count { font-size:20px; font-weight:700; } .ic-label { font-size:9px; color:#888; max-width:100px; word-break:break-all; }
.state-placeholder,.state-empty,.state-error { padding:50px 20px; text-align:center; color:var(--ink-hint); font-size:14px; }
.state-error { color:#B91C1C; }
.loading-text { text-align:center; padding:30px; color:#aaa; }
</style>
