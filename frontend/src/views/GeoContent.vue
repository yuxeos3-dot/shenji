<template>
<div>
  <div class="page-top"><h1>🤖 GEO 管理</h1></div>

  <!-- 站点选择 -->
  <div class="panel selector-bar">
    <span class="selector-label">🌐 选择站点</span>
    <el-select v-model="siteId" placeholder="请选择站点" style="width:280px" @change="onSiteChange">
      <el-option v-for="s in sites" :key="s.id" :label="s.name+' ('+s.domain+')'" :value="s.id" />
    </el-select>
  </div>

  <div v-if="!siteId" class="empty-hint panel"><div>请先选择站点</div></div>

  <template v-else>
    <div class="stats-row">
      <div class="stat-card panel"><div class="sc-num blue">{{ stats.total_articles||0 }}</div><div class="sc-lbl">总文章</div></div>
      <div class="stat-card panel"><div class="sc-num orange">{{ stats.drafts||0 }}</div><div class="sc-lbl">草稿</div></div>
      <div class="stat-card panel"><div class="sc-num red">{{ stats.pending_review||0 }}</div><div class="sc-lbl">待审核</div></div>
      <div class="stat-card panel"><div class="sc-num green">{{ stats.published||0 }}</div><div class="sc-lbl">已发布</div></div>
    </div>

    <el-tabs v-model="activeTab" type="border-card">
      <!-- 关键词库（含GSC导入） -->
      <el-tab-pane label="🔑 关键词库" name="libs">
        <div class="tab-bar">
          <el-button type="primary" @click="showLibForm=true">+ 新建关键词库</el-button>
        </div>
        <el-table :data="titleLibs" stripe>
          <el-table-column prop="name" label="名称" />
          <el-table-column prop="description" label="描述" />
          <el-table-column prop="title_count" label="关键词数" width="100" />
          <el-table-column label="操作" width="320">
            <template #default="{row}">
              <el-button size="small" @click="openTitles(row)">查看</el-button>
              <el-button size="small" @click="openImport(row)">手动导入</el-button>
              <el-button size="small" type="success" @click="doGSCImportDirect(row)" :loading="gscLoadingIds.has(row.id)">从GSC导入</el-button>
              <el-button size="small" type="danger" @click="deleteLib(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
        <!-- 新建 -->
        <el-dialog v-model="showLibForm" title="新建关键词库" width="460px">
          <el-form label-width="80px">
            <el-form-item label="名称"><el-input v-model="libForm.name" /></el-form-item>
            <el-form-item label="描述"><el-input v-model="libForm.description" /></el-form-item>
          </el-form>
          <template #footer><el-button @click="showLibForm=false">取消</el-button><el-button type="primary" @click="createLib">创建</el-button></template>
        </el-dialog>
        <!-- 查看 -->
        <el-dialog v-model="showTitles" :title="'关键词列表 — '+(curLib?.name||'')+ ' (共'+titlesTotal+'条)'" width="860px">
          <el-table :data="curTitles" max-height="400" stripe size="small">
            <el-table-column prop="title" label="关键词" />
            <el-table-column prop="keyword" label="附加词" width="140" />
            <el-table-column label="已用" width="60"><template #default="{row}">{{ row.is_used?'✅':'—' }}</template></el-table-column>
            <el-table-column label="操作" width="220">
              <template #default="{row}">
                <el-button size="small" type="primary" @click="openAIGen(row)">🤖 AI 生成</el-button>
                <el-button size="small" type="danger" text @click="delTitle(row)">删</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div style="margin-top:10px;text-align:center" v-if="titlesTotal>100">
            <el-pagination v-model:current-page="titlesPage" :page-size="100" :total="titlesTotal" layout="prev,pager,next,total" background @current-change="loadTitlesPage" />
          </div>
        </el-dialog>
        <!-- 手动导入 -->
        <el-dialog v-model="showImport" title="手动导入关键词" width="560px">
          <p style="font-size:12px;color:#888;margin-bottom:8px">每行一条：<code>关键词|附加词</code> 或只填关键词</p>
          <el-input v-model="importText" type="textarea" :rows="10" placeholder="SEO优化|搜索引擎&#10;外链建设" />
          <template #footer><el-button @click="showImport=false">取消</el-button><el-button type="primary" @click="doImport" :loading="importing">导入</el-button></template>
        </el-dialog>
        <!-- GSC导入 -->
        <el-dialog v-model="showGSCImport" title="从 GSC 导入关键词" width="500px">
          <el-form label-width="100px">
            <el-form-item label="导入数量"><el-input-number v-model="gscImportLimit" :min="10" :max="500" /></el-form-item>
          </el-form>
          <p style="font-size:12px;color:#888">从当前站点最新 GSC 数据按点击量排序导入 Top N 关键词（自动去重）</p>
          <template #footer><el-button @click="showGSCImport=false">取消</el-button><el-button type="primary" @click="doGSCImport" :loading="importing">导入</el-button></template>
        </el-dialog>
      </el-tab-pane>

      <!-- Prompt -->
      <el-tab-pane label="📝 Prompt 模板" name="prompts">
        <div class="tab-bar"><el-button type="primary" @click="editPrompt({})">+ 新建</el-button></div>
        <el-table :data="prompts" stripe>
          <el-table-column prop="name" label="名称" /><el-table-column prop="type" label="类型" width="100" />
          <el-table-column label="内容预览"><template #default="{row}"><span class="text-ellipsis">{{ row.content?.slice(0,80) }}</span></template></el-table-column>
          <el-table-column label="操作" width="140"><template #default="{row}"><el-button size="small" @click="editPrompt(row)">编辑</el-button><el-button size="small" type="danger" @click="delPrompt(row)">删除</el-button></template></el-table-column>
        </el-table>
        <el-dialog v-model="showPromptForm" :title="promptForm.id?'编辑':'新建'" width="700px">
          <el-form label-width="80px">
            <el-form-item label="名称"><el-input v-model="promptForm.name" /></el-form-item>
            <el-form-item label="类型"><el-select v-model="promptForm.type"><el-option value="content" label="内容生成" /><el-option value="keyword" label="关键词提取" /></el-select></el-form-item>
            <el-form-item label="Prompt"><el-input v-model="promptForm.content" type="textarea" :rows="10" placeholder="支持变量：{{title}} {{keyword}} {{knowledge}}" />
              <div style="font-size:11px;color:#888;margin-top:4px">可用变量：{{title}} {{keyword}} {{knowledge}}</div>
            </el-form-item>
          </el-form>
          <template #footer><el-button @click="showPromptForm=false">取消</el-button><el-button type="primary" @click="savePrompt">保存</el-button></template>
        </el-dialog>
      </el-tab-pane>

      <!-- 知识库 -->
      <el-tab-pane label="📖 知识库" name="knowledge">
        <div class="tab-bar"><el-button type="primary" @click="editKB({})">+ 新建</el-button></div>
        <el-table :data="knowledgeList" stripe>
          <el-table-column prop="name" label="名称" /><el-table-column prop="char_count" label="字数" width="100" />
          <el-table-column label="操作" width="140"><template #default="{row}"><el-button size="small" @click="editKB(row)">编辑</el-button><el-button size="small" type="danger" @click="delKB(row)">删除</el-button></template></el-table-column>
        </el-table>
        <el-dialog v-model="showKBForm" :title="kbForm.id?'编辑':'新建'" width="700px">
          <el-form label-width="80px">
            <el-form-item label="名称"><el-input v-model="kbForm.name" /></el-form-item>
            <el-form-item label="内容"><el-input v-model="kbForm.content" type="textarea" :rows="12" /></el-form-item>
          </el-form>
          <template #footer><el-button @click="showKBForm=false">取消</el-button><el-button type="primary" @click="saveKB">保存</el-button></template>
        </el-dialog>
      </el-tab-pane>

      <!-- 生成任务 -->
      <el-tab-pane label="⚙️ 生成任务" name="tasks">
        <div class="tab-bar"><el-button type="primary" @click="editTask({})">+ 新建任务</el-button></div>
        <el-table :data="tasks" stripe>
          <el-table-column prop="name" label="任务名" /><el-table-column prop="title_lib_name" label="关键词库" width="130" /><el-table-column prop="prompt_name" label="Prompt" width="130" />
          <el-table-column label="状态" width="90"><template #default="{row}"><el-tag :type="row.status==='active'?'success':'info'" size="small">{{ row.status==='active'?'运行中':'暂停' }}</el-tag></template></el-table-column>
          <el-table-column prop="created_count" label="已生成" width="70" /><el-table-column prop="draft_count" label="草稿" width="60" />
          <el-table-column label="操作" width="300"><template #default="{row}">
            <el-button size="small" type="success" @click="runTask(row)" :loading="row._running">生成一篇</el-button>
            <el-button size="small" v-if="row.status!=='active'" @click="startTask(row)">启动</el-button>
            <el-button size="small" v-else @click="stopTask(row)">暂停</el-button>
            <el-button size="small" @click="editTask(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="delTask(row)">删除</el-button>
          </template></el-table-column>
        </el-table>
        <el-dialog v-model="showTaskForm" :title="taskForm.id?'编辑任务':'新建任务'" width="560px">
          <el-form label-width="110px">
            <el-form-item label="任务名"><el-input v-model="taskForm.name" /></el-form-item>
            <el-form-item label="关键词库"><el-select v-model="taskForm.title_lib_id" style="width:100%"><el-option v-for="l in titleLibs" :key="l.id" :label="l.name+' ('+l.title_count+'条)'" :value="l.id" /></el-select></el-form-item>
            <el-form-item label="Prompt"><el-select v-model="taskForm.prompt_id" style="width:100%"><el-option v-for="p in prompts" :key="p.id" :label="p.name" :value="p.id" /></el-select></el-form-item>
            <el-form-item label="知识库"><el-select v-model="taskForm.knowledge_id" style="width:100%" clearable><el-option :value="0" label="不使用" /><el-option v-for="k in knowledgeList" :key="k.id" :label="k.name" :value="k.id" /></el-select></el-form-item>
            <el-form-item label="草稿上限"><el-input-number v-model="taskForm.draft_limit" :min="1" :max="500" /></el-form-item>
            <el-form-item label="需人工审核"><el-switch v-model="taskForm.need_review" :active-value="1" :inactive-value="0" /></el-form-item>
            <el-form-item label="循环模式"><el-switch v-model="taskForm.is_loop" :active-value="1" :inactive-value="0" /></el-form-item>
            <el-form-item label="自动提关键词"><el-switch v-model="taskForm.auto_keywords" :active-value="1" :inactive-value="0" /></el-form-item>
            <el-form-item label="自动生成描述"><el-switch v-model="taskForm.auto_description" :active-value="1" :inactive-value="0" /></el-form-item>
          </el-form>
          <template #footer><el-button @click="showTaskForm=false">取消</el-button><el-button type="primary" @click="saveTask">保存</el-button></template>
        </el-dialog>
      </el-tab-pane>

      <!-- 文章审核 -->
      <el-tab-pane label="📄 文章审核" name="articles">
        <div class="tab-bar">
          <el-select v-model="artFilter.review_status" placeholder="审核状态" clearable style="width:130px" @change="loadArticles"><el-option value="pending" label="待审核" /><el-option value="approved" label="已通过" /><el-option value="rejected" label="已拒绝" /></el-select>
          <el-select v-model="artFilter.status" placeholder="发布状态" clearable style="width:130px" @change="loadArticles"><el-option value="draft" label="草稿" /><el-option value="published" label="已发布" /></el-select>
        </div>
        <el-table :data="articles" stripe>
          <el-table-column prop="title" label="标题" show-overflow-tooltip /><el-table-column prop="task_name" label="任务" width="120" /><el-table-column prop="word_count" label="字数" width="70" />
          <el-table-column label="审核" width="80"><template #default="{row}"><el-tag size="small" :type="row.review_status==='approved'?'success':row.review_status==='rejected'?'danger':'warning'">{{ {approved:'通过',rejected:'拒绝',pending:'待审'}[row.review_status] }}</el-tag></template></el-table-column>
          <el-table-column label="操作" width="250"><template #default="{row}">
            <el-button size="small" @click="viewArt(row)">查看</el-button>
            <el-button size="small" type="success" v-if="row.review_status==='pending'" @click="reviewArt(row,'approved')">通过</el-button>
            <el-button size="small" type="danger" v-if="row.review_status==='pending'" @click="reviewArt(row,'rejected')">拒绝</el-button>
            <el-button size="small" type="primary" v-if="row.review_status==='approved'&&row.status==='draft'" @click="publishArt(row)">发布</el-button>
            <el-button size="small" type="danger" text @click="delArt(row)">删除</el-button>
          </template></el-table-column>
        </el-table>
        <div style="margin-top:10px;text-align:center" v-if="artTotal>20"><el-pagination v-model:current-page="artPage" :page-size="20" :total="artTotal" layout="prev,pager,next,total" background @current-change="loadArticles" /></div>
        <el-dialog v-model="showArticle" :title="curArticle?.title" width="800px" top="5vh">
          <div v-if="curArticle">
            <div style="margin-bottom:10px"><el-tag size="small">{{ curArticle.ai_model }}</el-tag><el-tag size="small" type="info" style="margin-left:6px">{{ curArticle.word_count }}字</el-tag></div>
            <div v-if="curArticle.meta_description" style="background:#F0F9FF;border-left:3px solid #3B82F6;padding:8px 12px;margin-bottom:12px;font-size:13px;color:#1E40AF;border-radius:4px">{{ curArticle.meta_description }}</div>
            <pre style="font-size:14px;line-height:1.85;white-space:pre-wrap;max-height:60vh;overflow-y:auto;padding:12px;background:#FAFAFA;border-radius:8px;border:1px solid #eee;font-family:inherit;margin:0">{{ curArticle.content }}</pre>
          </div>
        </el-dialog>
      </el-tab-pane>
    </el-tabs>
  </template>

  <!-- Shared workflow dialog (AI per-keyword generate) -->
  <KeywordAIWorkflow
    v-model="showAIGen"
    :site-id="siteId"
    content-type="geo"
    :title-id="aiGenRow?.id || 0"
    :keyword="aiGenRow?.keyword || aiGenRow?.title || ''"
    :title-text="aiGenRow?.title || ''"
    :knowledge-options="knowledgeList"
    :site-page-count="sitePageCount"
    @applied="onAIGenApplied"
  />
</div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import KeywordAIWorkflow from '../components/KeywordAIWorkflow.vue'

const activeTab = ref('libs')
const stats = ref({})
const sites = ref([])
const siteId = ref(null)
const gscLoadingIds = ref(new Set())

// Per-keyword AI generate workflow state
const showAIGen = ref(false)
const aiGenRow = ref(null)
const sitePageCount = ref(0) // for hint in the workflow dialog

function onSiteChange() { loadAll() }
function loadAll() { if(!siteId.value)return; loadStats(); loadLibs(); loadPrompts(); loadKnowledge(); loadTasks(); loadArticles(); loadSitePageCount() }

// Count site_pages so we can warn users if they haven't added any.
// The workflow backend uses these as the internal-link source.
async function loadSitePageCount() {
  try {
    const res = await api.listSitePages(siteId.value)
    sitePageCount.value = (res.data || []).length
  } catch { sitePageCount.value = 0 }
}

function openAIGen(row) {
  aiGenRow.value = row
  showAIGen.value = true
}
function onAIGenApplied({ article_id }) {
  ElMessage.success('✅ 已保存为文章 #' + article_id)
  loadArticles()
  loadStats()
  // Reload titles to reflect the new is_used=1 badge
  if (curLib.value) loadTitlesPage()
}

// Libs
const titleLibs = ref([]); const showLibForm = ref(false); const libForm = ref({name:'',description:''}); const showTitles = ref(false); const curLib = ref(null); const curTitles = ref([]); const titlesPage = ref(1); const titlesTotal = ref(0); const showImport = ref(false); const importText = ref(''); const importing = ref(false)
const showGSCImport = ref(false); const gscImportLimit = ref(100)
async function loadLibs() { try { titleLibs.value = (await api.geoListTitleLibs({site_id:siteId.value,content_type:'geo'})).data } catch (e) { console.error('加载关键词库失败', e) } }
async function createLib() { await api.geoCreateTitleLib({name:libForm.value.name,description:libForm.value.description,site_id:siteId.value,content_type:'geo'}); showLibForm.value=false; libForm.value={name:'',description:''}; loadLibs(); ElMessage.success('创建成功') }
async function deleteLib(r) { await ElMessageBox.confirm('删除「'+r.name+'」？'); await api.geoDeleteTitleLib(r.id,'geo'); loadLibs() }
async function openTitles(r) { curLib.value=r; titlesPage.value=1; await loadTitlesPage(); showTitles.value=true }
async function loadTitlesPage() { const res=await api.geoListTitles(curLib.value.id,{page:titlesPage.value,per_page:100}); curTitles.value=res.data.items||[]; titlesTotal.value=res.data.total||0 }
async function delTitle(r) { await api.geoDeleteTitle(r.id); curTitles.value=curTitles.value.filter(t=>t.id!==r.id); loadLibs() }
function openImport(r) { curLib.value=r; importText.value=''; showImport.value=true }
async function doImport() { importing.value=true; try { const res=await api.geoImportTitles(curLib.value.id,{text:importText.value}); ElMessage.success('导入'+res.data.imported+'条'); showImport.value=false; loadLibs() } finally{importing.value=false} }
function openGSCImport(r) { curLib.value=r; gscImportLimit.value=100; showGSCImport.value=true }
async function doGSCImport() { importing.value=true; try { const res=await api.geoImportFromGSC(curLib.value.id,{site_id:siteId.value,limit:gscImportLimit.value}); ElMessage.success('从GSC导入'+res.data.imported+'条'); showGSCImport.value=false; loadLibs() } catch(e){ElMessage.error(e?.response?.data?.error||'导入失败')} finally{importing.value=false} }
async function doGSCImportDirect(r) { if(gscLoadingIds.value.has(r.id))return; gscLoadingIds.value.add(r.id); try { const res=await api.geoImportFromGSC(r.id,{site_id:siteId.value,limit:0}); ElMessage.success('从GSC导入 '+res.data.imported+' 条关键词（来源共'+res.data.source_total+'条，已过滤排名1-99）'); loadLibs() } catch(e){ElMessage.error(e?.response?.data?.error||'导入失败，请先在数据中心同步GSC')} finally{gscLoadingIds.value.delete(r.id)} }

// Prompts
const prompts = ref([]); const showPromptForm = ref(false); const promptForm = ref({id:0,name:'',type:'content',content:'',description:''})
async function loadPrompts() { try { prompts.value=(await api.geoListPrompts({content_type:'geo'})).data } catch (e) { console.error('加载Prompt失败', e) } }
function editPrompt(r) { promptForm.value={id:r.id||0,name:r.name||'',type:r.type||'content',content:r.content||'',description:r.description||''}; showPromptForm.value=true }
async function savePrompt() { const f=promptForm.value; if(!f.name||!f.content){ElMessage.warning('名称和内容必填');return} if(f.id){await api.geoUpdatePrompt(f.id,f)}else{await api.geoCreatePrompt({...f,content_type:'geo'})} showPromptForm.value=false; loadPrompts(); ElMessage.success('保存成功') }
async function delPrompt(r) { await ElMessageBox.confirm('删除？'); await api.geoDeletePrompt(r.id,'geo'); loadPrompts() }

// Knowledge
const knowledgeList = ref([]); const showKBForm = ref(false); const kbForm = ref({id:0,name:'',content:''})
async function loadKnowledge() { try { knowledgeList.value=(await api.geoListKnowledge({content_type:'geo'})).data } catch (e) { console.error('加载知识库失败', e) } }
function editKB(r) { kbForm.value={id:r.id||0,name:r.name||'',content:r.content||''}; showKBForm.value=true }
async function saveKB() { const f=kbForm.value; if(!f.name||!f.content){ElMessage.warning('必填');return} if(f.id){await api.geoUpdateKnowledge(f.id,f)}else{await api.geoCreateKnowledge({...f,content_type:'geo'})} showKBForm.value=false; loadKnowledge(); ElMessage.success('保存成功') }
async function delKB(r) { await ElMessageBox.confirm('删除？'); await api.geoDeleteKnowledge(r.id,'geo'); loadKnowledge() }

// Tasks
const tasks = ref([]); const showTaskForm = ref(false); const taskForm = ref({id:0,name:'',title_lib_id:null,prompt_id:null,knowledge_id:0,draft_limit:20,need_review:1,is_loop:0,auto_keywords:1,auto_description:1})
async function loadTasks() { try { tasks.value=(await api.geoListTasks({site_id:siteId.value,content_type:'geo'})).data } catch (e) { console.error('加载任务失败', e) } }
function editTask(r) { taskForm.value={id:r.id||0,name:r.name||'',title_lib_id:r.title_lib_id||null,prompt_id:r.prompt_id||null,knowledge_id:r.knowledge_id||0,draft_limit:r.draft_limit||20,need_review:r.need_review??1,is_loop:r.is_loop||0,auto_keywords:r.auto_keywords??1,auto_description:r.auto_description??1}; showTaskForm.value=true }
async function saveTask() { const f=taskForm.value; if(!f.name||!f.title_lib_id||!f.prompt_id){ElMessage.warning('任务名、关键词库、Prompt必选');return} if(f.id){await api.geoUpdateTask(f.id,f)}else{await api.geoCreateTask({...f,site_id:siteId.value,content_type:'geo'})} showTaskForm.value=false; loadTasks(); ElMessage.success('保存成功') }
async function delTask(r) { await ElMessageBox.confirm('删除？'); await api.geoDeleteTask(r.id,'geo'); loadTasks() }
async function startTask(r) { await api.geoStartTask(r.id); loadTasks() }
async function stopTask(r) { await api.geoStopTask(r.id); loadTasks() }
async function runTask(r) { r._running=true; try { const res=await api.geoRunTask(r.id); ElMessage.success('已生成: '+res.data.title+' ('+res.data.word_count+'字)'); loadTasks(); loadArticles(); loadStats() } catch(e){ElMessage.error(e?.response?.data?.error||'生成失败')} finally{r._running=false} }

// Articles
const articles = ref([]); const artPage = ref(1); const artTotal = ref(0); const artFilter = ref({status:'',review_status:''}); const showArticle = ref(false); const curArticle = ref(null)
async function loadArticles() { try { const p={page:artPage.value,per_page:20,site_id:siteId.value,content_type:'geo'}; if(artFilter.value.status)p.status=artFilter.value.status; if(artFilter.value.review_status)p.review_status=artFilter.value.review_status; const res=await api.geoListArticles(p); articles.value=res.data.items; artTotal.value=res.data.total } catch (e) { console.error('加载文章失败', e) } }
async function viewArt(r) { try { curArticle.value=(await api.geoGetArticle(r.id)).data; showArticle.value=true } catch (e) { console.error('查看文章失败', e) } }
async function reviewArt(r,s) { await api.geoReviewArticle(r.id,{status:s}); ElMessage.success(s==='approved'?'已通过':'已拒绝'); loadArticles(); loadStats() }
async function publishArt(r) { await api.geoPublishArticle(r.id); ElMessage.success('已发布'); loadArticles(); loadStats() }
async function delArt(r) { await ElMessageBox.confirm('删除？'); await api.geoDeleteArticle(r.id,'geo'); loadArticles(); loadStats() }

async function loadStats() { try { stats.value=(await api.geoStats({site_id:siteId.value,content_type:'geo'})).data } catch (e) { console.error('加载统计失败', e) } }
onMounted(async () => { const res=await api.listSites().catch(()=>({data:[]})); sites.value=res.data||[]; if(sites.value.length){siteId.value=sites.value[0].id;loadAll()} })
</script>

<style scoped>
.page-top{margin-bottom:18px} .page-top h1{font-size:20px;font-weight:800}
.selector-bar{display:flex;align-items:center;gap:12px;margin-bottom:18px}
.selector-label{font-weight:700;color:var(--ink-mid);font-size:13px}
.empty-hint{text-align:center;padding:40px;color:var(--ink-hint)}
.stats-row{display:flex;gap:14px;margin-bottom:18px} .stat-card{flex:1;text-align:center;padding:16px 10px}
.sc-num{font-size:28px;font-weight:800} .sc-lbl{font-size:12px;color:var(--ink-hint);margin-top:4px}
.blue{color:#2563EB} .orange{color:#F59E0B} .red{color:#DC2626} .green{color:#16A34A}
.tab-bar{margin-bottom:14px;display:flex;gap:10px;align-items:center}
.panel{background:var(--surface);border-radius:var(--r);box-shadow:var(--s1);border:1.5px solid var(--border-light);padding:16px 20px}
.text-ellipsis{display:inline-block;max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px;color:#888}
</style>
