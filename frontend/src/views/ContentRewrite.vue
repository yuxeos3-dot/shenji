<template>
  <div class="rewrite-page">
    <div class="panel selector-bar">
      <span class="selector-label">✍️ 选择站点</span>
      <el-select v-model="selectedSiteId" placeholder="请选择站点" style="width:260px" @change="onSiteChange">
        <el-option v-for="s in sites" :key="s.id" :label="s.name + ' (' + s.domain + ')'" :value="s.id" />
      </el-select>
    </div>

    <div v-if="!selectedSiteId" class="empty-hint panel">
      <div class="empty-icon">✍️</div>
      <div>请先选择一个站点</div>
    </div>

    <template v-else>
      <!-- 标签页 -->
      <el-tabs v-model="activeTab" class="rewrite-tabs">

        <!-- TDK 二创 -->
        <el-tab-pane label="📝 TDK 优化二创" name="tdk">
          <div class="tab-content">
            <div class="section-card">
              <div class="sc-title">原始内容输入</div>
              <div class="sc-desc">输入当前页面的 Title、Description，AI 将基于 SEO 最佳实践进行二次创作优化</div>
              <div class="form-grid">
                <div class="form-field">
                  <label>页面类型</label>
                  <el-select v-model="tdkForm.page_type" style="width:100%">
                    <el-option label="首页" value="首页" />
                    <el-option label="分类列表页" value="分类列表页" />
                    <el-option label="视频/文章详情页" value="视频详情页" />
                    <el-option label="搜索结果页" value="搜索结果页" />
                    <el-option label="标签页" value="标签页" />
                    <el-option label="关于我们" value="关于我们" />
                  </el-select>
                </div>
                <div class="form-field">
                  <label>目标关键词（用逗号分隔）</label>
                  <input class="field-input" v-model="tdkForm.keywords" placeholder="如：电影在线观看, 免费电影, 高清电影" />
                </div>
                <div class="form-field full-width">
                  <label>当前 Title <span class="char-count" :class="tdkForm.title.length>60?'over':''">{{ tdkForm.title.length }}/60</span></label>
                  <input class="field-input" v-model="tdkForm.title" placeholder="输入当前页面的 Title 标签内容" />
                </div>
                <div class="form-field full-width">
                  <label>当前 Description <span class="char-count" :class="tdkForm.desc.length>160?'over':''">{{ tdkForm.desc.length }}/160</span></label>
                  <textarea class="field-input field-textarea" v-model="tdkForm.desc" placeholder="输入当前页面的 Description 内容" rows="3"></textarea>
                </div>
                <div class="form-field full-width">
                  <label>补充说明（可选，如：站点定位、内容特色、地区等）</label>
                  <input class="field-input" v-model="tdkForm.extra" placeholder="如：主打日韩剧、面向东南亚用户等" />
                </div>
              </div>
              <div class="action-row">
                <button class="btn btn-primary" @click="rewriteTDK" :disabled="rewriting.tdk">
                  {{ rewriting.tdk ? '🤖 AI 生成中...' : '🤖 AI 生成优化版本' }}
                </button>
                <span class="action-hint">一次生成 3 个版本供选择</span>
              </div>
            </div>

            <!-- 生成结果 -->
            <div v-if="tdkResults.length" class="results-section">
              <div class="results-title">✨ AI 生成结果（点击复制）</div>
              <div class="result-cards">
                <div v-for="(r, i) in tdkResults" :key="i" class="result-card">
                  <div class="rc-version">版本 {{ i+1 }}</div>
                  <div class="rc-field">
                    <span class="rc-label">Title</span>
                    <div class="rc-value" @click="copyText(r.title)">{{ r.title }} <span class="copy-hint">点击复制</span></div>
                    <span class="rc-len" :class="r.title.length>60?'over':''">{{ r.title.length }} 字</span>
                  </div>
                  <div class="rc-field">
                    <span class="rc-label">Description</span>
                    <div class="rc-value" @click="copyText(r.desc)">{{ r.desc }} <span class="copy-hint">点击复制</span></div>
                    <span class="rc-len" :class="r.desc.length>160?'over':''">{{ r.desc.length }} 字</span>
                  </div>
                  <div class="rc-field" v-if="r.reason">
                    <span class="rc-label">优化说明</span>
                    <div class="rc-reason">{{ r.reason }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </el-tab-pane>

        <!-- 内容二创 -->
        <el-tab-pane label="🎬 视频/文章介绍二创" name="content">
          <div class="tab-content">
            <div class="section-card">
              <div class="sc-title">内容二次创作</div>
              <div class="sc-desc">将原始视频/文章介绍改写为 SEO 友好的独特内容，避免重复内容惩罚，提升相关度</div>
              <div class="form-grid">
                <div class="form-field">
                  <label>内容类型</label>
                  <el-select v-model="contentForm.type" style="width:100%">
                    <el-option label="电影介绍" value="电影介绍" />
                    <el-option label="电视剧介绍" value="电视剧介绍" />
                    <el-option label="综艺介绍" value="综艺介绍" />
                    <el-option label="动漫介绍" value="动漫介绍" />
                    <el-option label="新闻文章" value="新闻文章" />
                    <el-option label="其他内容" value="其他内容" />
                  </el-select>
                </div>
                <div class="form-field">
                  <label>改写风格</label>
                  <el-select v-model="contentForm.style" style="width:100%">
                    <el-option label="专业影评风格" value="专业影评" />
                    <el-option label="通俗易懂" value="通俗易懂" />
                    <el-option label="吸引眼球/标题党" value="吸引眼球" />
                    <el-option label="简洁精炼" value="简洁精炼" />
                    <el-option label="详细丰富" value="详细丰富" />
                  </el-select>
                </div>
                <div class="form-field">
                  <label>目标字数</label>
                  <el-select v-model="contentForm.length" style="width:100%">
                    <el-option label="100-150字（摘要）" value="100" />
                    <el-option label="200-300字（标准）" value="200" />
                    <el-option label="400-500字（详细）" value="400" />
                    <el-option label="600字以上（完整）" value="600" />
                  </el-select>
                </div>
                <div class="form-field">
                  <label>核心关键词</label>
                  <input class="field-input" v-model="contentForm.keywords" placeholder="如：高清, 在线观看, 免费" />
                </div>
                <div class="form-field full-width">
                  <label>原始内容</label>
                  <textarea class="field-input field-textarea" v-model="contentForm.original" placeholder="粘贴原始介绍/剧情内容..." rows="6"></textarea>
                </div>
              </div>
              <div class="action-row">
                <button class="btn btn-primary" @click="rewriteContent" :disabled="rewriting.content">
                  {{ rewriting.content ? '🤖 创作中...' : '🤖 AI 内容二创' }}
                </button>
                <span class="action-hint">生成 2 个改写版本</span>
              </div>
            </div>

            <div v-if="contentResults.length" class="results-section">
              <div class="results-title">✨ 二创结果（点击复制）</div>
              <div class="result-cards">
                <div v-for="(r, i) in contentResults" :key="i" class="result-card">
                  <div class="rc-version">版本 {{ i+1 }}（{{ r.style }}）</div>
                  <div class="rc-value content-value" @click="copyText(r.content)">{{ r.content }} <span class="copy-hint">点击复制</span></div>
                  <div class="rc-reason" v-if="r.note">💡 {{ r.note }}</div>
                </div>
              </div>
            </div>
          </div>
        </el-tab-pane>

        <!-- 历史记录 -->
        <el-tab-pane label="📚 创作历史" name="history">
          <div class="tab-content">
            <div class="panel" v-if="history.length">
              <table class="table">
                <thead><tr><th>类型</th><th>原始内容</th><th>生成版本</th><th>时间</th><th>操作</th></tr></thead>
                <tbody>
                  <tr v-for="h in history" :key="h.id">
                    <td><span class="type-badge">{{ h.type }}</span></td>
                    <td class="content-preview">{{ h.original?.substring(0, 60) }}...</td>
                    <td class="num-blue">{{ h.version_count }} 个版本</td>
                    <td class="time-cell">{{ h.created_at }}</td>
                    <td><button class="btn btn-xs" @click="loadHistory(h)">查看</button></td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-else class="empty-hint panel"><div class="empty-icon">📚</div><div>暂无创作历史</div></div>
          </div>
        </el-tab-pane>

      </el-tabs>
    </template>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import apiClient from '../api'

const sites = ref([])
const selectedSiteId = ref(null)
const activeTab = ref('tdk')
const rewriting = ref({ tdk: false, content: false })
const tdkResults = ref([])
const contentResults = ref([])
const history = ref([])

const tdkForm = ref({ page_type: '首页', keywords: '', title: '', desc: '', extra: '' })
const contentForm = ref({ type: '电影介绍', style: '专业影评', length: '200', keywords: '', original: '' })

function onSiteChange() { tdkResults.value = []; contentResults.value = [] }

async function rewriteTDK() {
  if (!tdkForm.value.title && !tdkForm.value.desc) { ElMessage.warning('请至少填写 Title 或 Description'); return }
  rewriting.value.tdk = true
  tdkResults.value = []
  try {
    const res = await apiClient.rewriteContent({
      site_id: selectedSiteId.value,
      mode: 'tdk',
      page_type: tdkForm.value.page_type,
      keywords: tdkForm.value.keywords,
      title: tdkForm.value.title,
      desc: tdkForm.value.desc,
      extra: tdkForm.value.extra,
    })
    tdkResults.value = res.data || []
    if (tdkResults.value.length) {
      ElMessage.success(`已生成 ${tdkResults.value.length} 个版本`)
      history.value.unshift({ id: Date.now(), type: 'tdk', original: tdkForm.value.title || tdkForm.value.desc, version_count: tdkResults.value.length, results: tdkResults.value, created_at: new Date().toLocaleString() })
    }
  } catch {}
  rewriting.value.tdk = false
}

async function rewriteContent() {
  if (!contentForm.value.original) { ElMessage.warning('请填写原始内容'); return }
  rewriting.value.content = true
  contentResults.value = []
  try {
    const res = await apiClient.rewriteContent({
      site_id: selectedSiteId.value,
      mode: 'content',
      type: contentForm.value.type,
      style: contentForm.value.style,
      length: contentForm.value.length,
      keywords: contentForm.value.keywords,
      original: contentForm.value.original,
    })
    contentResults.value = res.data || []
    if (contentResults.value.length) ElMessage.success(`已生成 ${contentResults.value.length} 个版本`)
  } catch {}
  rewriting.value.content = false
}

function copyText(text) {
  navigator.clipboard.writeText(text).then(() => ElMessage.success('已复制到剪贴板'))
}

function loadHistory(h) {
  // Restore result into the correct tab for review
  if (h.type === 'tdk') {
    activeTab.value = 'tdk'
    tdkResults.value = h.results || []
  } else {
    activeTab.value = 'content'
    contentResults.value = h.results || []
  }
  ElMessage.success('已加载历史记录')
}

onMounted(async () => {
  const res = await apiClient.listSites().catch(() => ({ data: [] }))
  sites.value = res.data || []
  if (sites.value.length) selectedSiteId.value = sites.value[0].id
})
</script>

<style scoped>
.rewrite-page { display:flex; flex-direction:column; gap:16px; }
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:16px 22px; }
.selector-bar { display:flex; align-items:center; gap:12px; flex-wrap:wrap; }
.selector-label { font-size:13.5px; font-weight:700; color:var(--ink-mid); white-space:nowrap; }

:deep(.rewrite-tabs .el-tabs__header) { margin-bottom:0; }
:deep(.rewrite-tabs .el-tabs__content) { padding-top:0; }

.tab-content { display:flex; flex-direction:column; gap:16px; padding-top:16px; }

.section-card { background:var(--surface); border:1.5px solid var(--border-light); border-radius:var(--r); padding:20px 24px; box-shadow:var(--s1); }
.sc-title { font-size:15px; font-weight:700; color:var(--ink); margin-bottom:4px; }
.sc-desc { font-size:13px; color:var(--ink-soft); margin-bottom:18px; }

.form-grid { display:grid; grid-template-columns:1fr 1fr; gap:14px; margin-bottom:16px; }
.form-field { display:flex; flex-direction:column; gap:6px; }
.form-field.full-width { grid-column:1/-1; }
.form-field label { font-size:12.5px; font-weight:600; color:var(--ink-mid); display:flex; justify-content:space-between; }
.char-count { font-weight:600; color:var(--ink-soft); }
.char-count.over { color:var(--err); }
.field-input { width:100%; padding:9px 12px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13.5px; font-family:inherit; background:var(--surface); color:var(--ink); outline:none; transition:border-color .18s; }
.field-input:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); }
.field-textarea { resize:vertical; min-height:80px; }

.action-row { display:flex; align-items:center; gap:12px; }
.action-hint { font-size:12px; color:var(--ink-hint); }

.results-section { display:flex; flex-direction:column; gap:12px; }
.results-title { font-size:14px; font-weight:700; color:var(--ink); }
.result-cards { display:flex; flex-direction:column; gap:12px; }
.result-card { background:linear-gradient(135deg,#FFFDF8,#FFF8EC); border:1.5px solid var(--peach); border-radius:var(--r-sm); padding:16px 18px; display:flex; flex-direction:column; gap:10px; }
.rc-version { font-size:12px; font-weight:800; color:var(--sun-deep); text-transform:uppercase; letter-spacing:.06em; }
.rc-field { display:flex; flex-direction:column; gap:4px; }
.rc-label { font-size:11px; font-weight:700; color:var(--ink-hint); text-transform:uppercase; letter-spacing:.06em; }
.rc-value { font-size:13.5px; color:var(--ink); line-height:1.6; cursor:pointer; padding:8px 12px; background:rgba(255,255,255,.7); border-radius:6px; border:1px dashed var(--border); position:relative; }
.rc-value:hover { background:#fff; border-color:var(--sun); }
.copy-hint { font-size:11px; color:var(--ink-hint); margin-left:8px; }
.rc-len { font-size:11.5px; color:var(--ink-soft); } .rc-len.over { color:var(--err); }
.rc-reason { font-size:12px; color:var(--ink-soft); background:var(--surface-3); padding:8px 12px; border-radius:6px; line-height:1.6; }
.content-value { font-size:13.5px; color:var(--ink); line-height:1.8; cursor:pointer; padding:12px; background:rgba(255,255,255,.7); border-radius:6px; border:1px dashed var(--border); white-space:pre-line; }
.content-value:hover { background:#fff; border-color:var(--sun); }

.type-badge { display:inline-block; padding:2px 8px; background:var(--sun-light); color:var(--sun-deep); border-radius:5px; font-size:11.5px; font-weight:700; }
.content-preview { font-size:12.5px; color:var(--ink-soft); max-width:300px; }
.time-cell { font-size:11.5px; color:var(--ink-hint); white-space:nowrap; }
.table { width:100%; border-collapse:collapse; }
.table th { padding:10px 14px; font-size:11px; font-weight:800; color:var(--ink-mid); text-transform:uppercase; letter-spacing:.08em; border-bottom:2px solid var(--border); background:var(--surface-3); text-align:left; }
.table td { padding:10px 14px; border-bottom:1px solid var(--border-light); }
.num-blue { color:var(--info); font-weight:700; }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 18px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; }
.btn-xs { padding:4px 12px; font-size:12px; border-radius:6px; }
.btn-xs:hover { background:var(--sun-light); color:var(--sun-deep); }

.empty-hint { text-align:center; padding:50px; color:var(--ink-hint); }
.empty-icon { font-size:44px; margin-bottom:12px; }
</style>
