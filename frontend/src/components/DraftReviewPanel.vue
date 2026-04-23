<template>
  <!-- Shared review panel for GEO + 外链. Shows geo_articles rows
       filtered by content_type. Status filter lets admins see pending
       (待审核) / approved (已通过) / published (已发布) / rejected. -->
  <div>
    <div class="filter-row">
      <el-radio-group v-model="filter.review_status" size="small" @change="loadPage">
        <el-radio-button value="pending">待审核</el-radio-button>
        <el-radio-button value="approved">已通过</el-radio-button>
        <el-radio-button value="rejected">已驳回</el-radio-button>
        <el-radio-button value="">全部</el-radio-button>
      </el-radio-group>
      <el-radio-group v-model="filter.status" size="small" style="margin-left:10px" @change="loadPage">
        <el-radio-button value="">不限</el-radio-button>
        <el-radio-button value="draft">未发布</el-radio-button>
        <el-radio-button value="published">已发布</el-radio-button>
      </el-radio-group>
      <span style="flex:1"></span>
      <button class="btn btn-xs" @click="loadPage">🔄 刷新</button>
    </div>

    <table class="table">
      <thead>
        <tr>
          <th style="width:38%">标题</th>
          <th style="width:200px">关键词</th>
          <th style="width:70px">字数</th>
          <th style="width:100px">审核</th>
          <th style="width:90px">状态</th>
          <th style="width:140px">生成时间</th>
          <th style="width:260px">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="a in articles" :key="a.id">
          <td style="font-weight:600">{{ a.title }}</td>
          <td style="color:var(--ink-soft);font-size:12px;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" :title="a.keywords">{{ a.keywords || '—' }}</td>
          <td>{{ a.word_count }}</td>
          <td>
            <span class="badge" :class="reviewClass(a.review_status)">
              {{ reviewLabel(a.review_status) }}
            </span>
          </td>
          <td>
            <span class="badge" :class="a.status === 'published' ? 'badge-ok' : 'badge-muted'">
              {{ a.status === 'published' ? '已发布' : '未发布' }}
            </span>
          </td>
          <td style="font-size:11px;color:var(--ink-soft)">{{ a.created_at?.slice(0, 16) }}</td>
          <td>
            <div class="action-btns">
              <button class="btn btn-xs" @click="openView(a)">查看</button>
              <button
                v-if="a.review_status === 'pending'"
                class="btn btn-xs btn-ok-xs"
                @click="approve(a)"
              >通过</button>
              <button
                v-if="a.review_status === 'pending'"
                class="btn btn-xs btn-danger-xs"
                @click="reject(a)"
              >驳回</button>
              <button
                v-if="a.review_status === 'approved' && a.status !== 'published'"
                class="btn btn-xs btn-primary-xs"
                @click="publish(a)"
              >发布</button>
              <button class="btn btn-xs btn-danger-xs" @click="del(a)">删除</button>
            </div>
          </td>
        </tr>
        <tr v-if="!articles.length">
          <td colspan="7" class="empty-cell">
            {{ loading ? '加载中...' : '当前过滤条件下没有任何文章' }}
          </td>
        </tr>
      </tbody>
    </table>

    <div v-if="total > perPage" class="pag-row">
      <el-pagination
        v-model:current-page="page"
        :page-size="perPage"
        :total="total"
        layout="prev, pager, next, total"
        @current-change="loadPage"
      />
    </div>

    <!-- ── Article viewer / editor ── -->
    <el-dialog
      v-model="viewVisible"
      title="📄 文章详情"
      width="840px"
      top="3vh"
      :close-on-click-modal="false"
    >
      <div v-if="viewing" class="view-body">
        <div class="view-row">
          <label>标题</label>
          <el-input v-model="viewing.title" />
        </div>
        <div class="view-grid">
          <div class="view-row">
            <label>Meta Title</label>
            <el-input v-model="viewing.meta_title" />
          </div>
          <div class="view-row">
            <label>字数</label>
            <el-input :model-value="String(viewing.word_count || 0)" readonly />
          </div>
        </div>
        <div class="view-row">
          <label>Meta Description</label>
          <el-input v-model="viewing.meta_description" type="textarea" :rows="2" />
        </div>
        <div class="view-row">
          <label>正文</label>
          <el-input v-model="viewing.content" type="textarea" :rows="18" />
        </div>
        <div v-if="viewing.reviewer_note" class="reviewer-note">
          <b>审核意见：</b>{{ viewing.reviewer_note }}
        </div>
      </div>
      <template #footer>
        <el-button @click="viewVisible = false">关闭</el-button>
        <el-button type="primary" @click="saveEdit">💾 保存修改</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const props = defineProps({
  contentType: { type: String, default: 'geo' },
  siteId: { type: [Number, String], default: 0 },
})

const articles = ref([])
const total = ref(0)
const page = ref(1)
const perPage = 20
const loading = ref(false)
const filter = ref({ review_status: 'pending', status: '' })

watch(() => props.siteId, () => loadPage())

onMounted(loadPage)

async function loadPage() {
  loading.value = true
  try {
    const params = {
      page: page.value,
      per_page: perPage,
      content_type: props.contentType,
    }
    if (props.siteId) params.site_id = props.siteId
    if (filter.value.review_status) params.review_status = filter.value.review_status
    if (filter.value.status) params.status = filter.value.status
    const res = await api.geoListArticles(params)
    articles.value = res.data.items || []
    total.value = res.data.total || 0
  } catch (e) {
    console.error(e)
    ElMessage.error('加载文章列表失败')
  } finally {
    loading.value = false
  }
}

function reviewLabel(s) {
  return { pending: '待审核', approved: '已通过', rejected: '已驳回' }[s] || s
}
function reviewClass(s) {
  return { pending: 'badge-warn', approved: 'badge-ok', rejected: 'badge-err' }[s] || 'badge-muted'
}

const viewVisible = ref(false)
const viewing = ref(null)

async function openView(a) {
  try {
    const res = await api.geoGetArticle(a.id)
    viewing.value = res.data
    viewVisible.value = true
  } catch (e) {
    ElMessage.error('加载文章失败')
  }
}

async function saveEdit() {
  if (!viewing.value) return
  try {
    await api.geoUpdateArticle(viewing.value.id, {
      title: viewing.value.title,
      content: viewing.value.content,
      keywords: viewing.value.keywords,
      meta_title: viewing.value.meta_title || '',
      meta_description: viewing.value.meta_description,
      category: viewing.value.category || '',
    })
    ElMessage.success('已保存')
    viewVisible.value = false
    loadPage()
  } catch (e) {
    ElMessage.error('保存失败')
  }
}

async function approve(a) {
  try {
    await api.geoReviewArticle(a.id, { status: 'approved', note: '' })
    ElMessage.success('✅ 已通过')
    loadPage()
  } catch (e) {
    ElMessage.error('审核失败')
  }
}

async function reject(a) {
  try {
    const { value } = await ElMessageBox.prompt('请填写驳回理由', '驳回', {
      confirmButtonText: '驳回',
      cancelButtonText: '取消',
      inputPlaceholder: '可写：需要调整结构 / 事实核对 / 用词再克制一些 等',
      inputValidator: v => (v && v.trim().length >= 2) || '请至少写几个字',
    })
    await api.geoReviewArticle(a.id, { status: 'rejected', note: value })
    ElMessage.success('已驳回')
    loadPage()
  } catch (e) {
    if (e !== 'cancel') console.error(e)
  }
}

async function publish(a) {
  try {
    await ElMessageBox.confirm('确认发布？发布后会被标记为已发布状态。', '发布确认', {
      type: 'warning',
    })
    await api.geoPublishArticle(a.id)
    ElMessage.success('🎉 已发布')
    loadPage()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('发布失败')
  }
}

async function del(a) {
  try {
    await ElMessageBox.confirm(`删除文章「${a.title}」？`, '删除确认', { type: 'warning' })
    await api.geoDeleteArticle(a.id, props.contentType)
    ElMessage.success('已删除')
    loadPage()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败')
  }
}

defineExpose({ reload: loadPage })
</script>

<style scoped>
.filter-row { display: flex; align-items: center; gap: 8px; margin-bottom: 14px; }

.table { width: 100%; border-collapse: collapse; background: var(--surface); border-radius: 10px; overflow: hidden; }
.table th {
  padding: 10px 12px; text-align: left; font-size: 11px; font-weight: 800;
  color: var(--ink-mid); text-transform: uppercase; letter-spacing: .08em;
  border-bottom: 2px solid var(--border); background: var(--surface-3);
}
.table td { padding: 10px 12px; border-bottom: 1px solid var(--border-light); font-size: 13px; vertical-align: middle; }
.table tr:hover td { background: var(--sun-light); }

.badge { display: inline-block; font-size: 11px; font-weight: 800; padding: 2px 10px; border-radius: 20px; }
.badge-ok { background: var(--ok-bg); color: var(--ok); border: 1px solid var(--ok-border); }
.badge-warn { background: var(--warn-bg); color: var(--warn); border: 1px solid var(--warn-border); }
.badge-err { background: var(--err-bg); color: var(--err); border: 1px solid var(--err-border); }
.badge-muted { background: var(--surface-3); color: var(--ink-hint); border: 1px solid var(--border); }

.action-btns { display: flex; gap: 4px; flex-wrap: wrap; }
.btn {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 4px 10px; border-radius: 6px; border: 1.5px solid var(--border);
  background: var(--surface); cursor: pointer; font-family: inherit;
  font-size: 12px; font-weight: 700; color: var(--ink-mid); transition: all .18s;
}
.btn:hover { background: var(--sun-light); border-color: var(--peach); color: var(--sun-deep); }
.btn-xs { padding: 3px 10px; font-size: 11px; }
.btn-ok-xs { color: var(--ok); border-color: var(--ok-border); background: var(--ok-bg); }
.btn-ok-xs:hover { background: var(--ok); color: #fff; border-color: var(--ok); }
.btn-primary-xs { color: #fff; background: linear-gradient(135deg, var(--sun), var(--coral)); border-color: transparent; }
.btn-primary-xs:hover { background: linear-gradient(135deg, var(--sun-deep), var(--sun)); color: #fff; }
.btn-danger-xs { color: var(--err); border-color: var(--err-border); background: var(--err-bg); }
.btn-danger-xs:hover { background: var(--err); color: #fff; border-color: var(--err); }

.empty-cell { text-align: center; color: var(--ink-hint); padding: 40px !important; font-size: 13px; }
.pag-row { display: flex; justify-content: center; margin-top: 16px; }

.view-body { display: flex; flex-direction: column; gap: 12px; }
.view-row { display: flex; flex-direction: column; gap: 4px; }
.view-row label { font-size: 11px; font-weight: 800; color: var(--ink-mid); text-transform: uppercase; letter-spacing: .06em; }
.view-grid { display: grid; grid-template-columns: 2fr 1fr; gap: 12px; }
.reviewer-note {
  padding: 10px 14px; background: var(--warn-bg); border: 1px solid var(--warn-border);
  border-radius: 8px; font-size: 13px; color: var(--warn);
}
</style>
