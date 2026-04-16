<template>
<div>
  <div class="page-top"><h1>🤖 GEO 管理</h1></div>
  <div class="stats-row">
    <div class="stat-card panel"><div class="sc-num blue">{{ stats.total_articles||0 }}</div><div class="sc-lbl">总文章</div></div>
    <div class="stat-card panel"><div class="sc-num orange">{{ stats.drafts||0 }}</div><div class="sc-lbl">草稿</div></div>
    <div class="stat-card panel"><div class="sc-num red">{{ stats.pending_review||0 }}</div><div class="sc-lbl">待审核</div></div>
    <div class="stat-card panel"><div class="sc-num green">{{ stats.published||0 }}</div><div class="sc-lbl">已发布</div></div>
  </div>
  <el-tabs v-model="activeTab" type="border-card">
    <!-- GSC 关键词 -->
    <el-tab-pane label="🌍 GSC 关键词" name="gsc">
      <div class="tab-bar">
        <el-select v-model="gscSiteId" placeholder="选择站点" style="width:240px" @change="loadGSC">
          <el-option v-for="s in sites" :key="s.id" :label="s.name+' ('+s.domain+')'" :value="s.id" />
        </el-select>
        <el-button @click="syncGSC" :loading="syncing" v-if="gscSiteId">↻ 同步GSC</el-button>
        <el-input v-model="gscSearch" placeholder="搜索关键词" style="width:180px" clearable />
      </div>
      <el-table :data="gscFiltered.slice((gscPage-1)*50,gscPage*50)" stripe max-height="500" v-loading="gscLoading">
        <el-table-column type="index" label="#" width="50" />
        <el-table-column prop="query" label="关键词" />
        <el-table-column prop="clicks" label="点击" width="80" sortable />
        <el-table-column prop="impressions" label="展示" width="80" sortable />
        <el-table-column label="CTR" width="80"><template #default="{row}">{{ row.ctr?(row.ctr*100).toFixed(2)+'%':'-' }}</template></el-table-column>
        <el-table-column label="排名" width="80"><template #default="{row}">{{ row.position?.toFixed(1) }}</template></el-table-column>
      </el-table>
      <div style="margin-top:10px;text-align:center" v-if="gscFiltered.length>50">
        <el-pagination v-model:current-page="gscPage" :page-size="50" :total="gscFiltered.length" layout="prev,pager,next" background />
      </div>
    </el-tab-pane>
    <!-- 关键词库 -->
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
            <el-button size="small" type="success" @click="openGSCImport(row)">从GSC导入</el-button>
            <el-button size="small" type="danger" @click="deleteLib(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-dialog v-model="showLibForm" title="新建关键词库" width="460px">
        <el-form label-width="80px">
          <el-form-item label="名称"><el-input v-model="libForm.name" /></el-form-item>
          <el-form-item label="描述"><el-input v-model="libForm.description" /></el-form-item>
        </el-form>
        <template #footer><el-button @click="showLibForm=false">取消</el-button><el-button type="primary" @click="createLib">创建</el-button></template>
      </el-dialog>
      <el-dialog v-model="showTitles" :title="'关键词列表 — '+curLib?.name" width="700px">
        <el-table :data="curTitles" max-height="400" stripe size="small">
          <el-table-column prop="title" label="关键词" /><el-table-column prop="keyword" label="附加词" width="140" />
          <el-table-column label="已用" width="60"><template #default="{row}">{{ row.is_used?'✅':'—' }}</template></el-table-column>
          <el-table-column label="" width="60"><template #default="{row}"><el-button size="small" type="danger" text @click="delTitle(row)">删</el-button></template></el-table-column>
        </el-table>
      </el-dialog>
      <el-dialog v-model="showImport" title="手动导入关键词" width="560px">
        <p style="font-size:12px;color:#888;margin-bottom:8px">每行一条，格式：<code>关键词|附加词</code> 或只填关键词</p>
        <el-input v-model="importText" type="textarea" :rows="10" placeholder="SEO优化|搜索引擎&#10;外链建设&#10;关键词排名|排名" />
        <template #footer><el-button @click="showImport=false">取消</el-button><el-button type="primary" @click="doImport" :loading="importing">导入</el-button></template>
      </el-dialog>
      <el-dialog v-model="showGSCImport" title="从 GSC 导入关键词" width="500px">
        <el-form label-width="100px">
          <el-form-item label="选择站点">
            <el-select v-model="gscImportSiteId" style="width:100%">
              <el-option v-for="s in sites" :key="s.id" :label="s.name+' ('+s.domain+')'" :value="s.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="导入数量"><el-input-number v-model="gscImportLimit" :min="10" :max="500" /></el-form-item>
        </el-form>
        <p style="font-size:12px;color:#888">将从该站点最新 GSC 数据中按点击量排序导入 Top N 关键词（自动去重）</p>
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
        <el-table-column prop="name" label="名称" /><el-table-column prop="char_count" label="字数" width="100" /><el-table-column prop="created_at" label="创建时间" width="180" />
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
        <el-table-column label="状态" width="80"><template #default="{row}"><el-tag size="small" :type="row.status==='published'?'success':'info'">{{ row.status==='published'?'已发布':'草稿' }}</el-tag></template></el-table-column>
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
          <div style="margin-bottom:10px"><el-tag size="small">{{ curArticle.ai_model }}</el-tag><el-tag size="small" type="info" style="margin-left:6px">{{ curArticle.word_count }}字</el-tag><el-tag v-if="curArticle.keywords" size="small" type="warning" style="margin-left:6px">{{ curArticle.keywords }}</el-tag></div>
          <div v-if="curArticle.meta_description" style="background:#F0F9FF;border-left:3px solid #3B82F6;padding:8px 12px;margin-bottom:12px;font-size:13px;color:#1E40AF;border-radius:4px">{{ curArticle.meta_description }}</div>
          <div style="font-size:14px;line-height:1.85;white-space:pre-wrap;max-height:60vh;overflow-y:auto;padding:12px;background:#FAFAFA;border-radius:8px;border:1px solid #eee" v-html="(curArticle.content||'').replace(/\n/g,'<br>')"></div>
        </div>
      </el-dialog>
    </el-tab-pane>
  </el-tabs>
</div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const activeTab = ref('gsc')
const stats = ref({})
const sites = ref([])

// GSC
const gscSiteId = ref(null); const gscKeywords = ref([]); const gscSearch = ref(''); const gscPage = ref(1); const gscLoading = ref(false); const syncing = ref(false)
const gscFiltered = computed(() => { let kws = [...gscKeywords.value]; if (gscSearch.value) { const q = gscSearch.value.toLowerCase(); kws = kws.filter(k => k.query?.toLowerCase().includes(q)) } return kws.sort((a,b) => (b.clicks||0)-(a.clicks||0)) })
async function loadGSC() { if (!gscSiteId.value) return; gscLoading.value = true; try { const res = await api.getGSCData(gscSiteId.value, 30); const d = res.data||[]; if (d.length) { const kj = d[0].keywords_json; gscKeywords.value = typeof kj==='string'?JSON.parse(kj):kj||[] } else { gscKeywords.value = [] } } catch {} finally { gscLoading.value = false } }
async function syncGSC() { syncing.value = true; try { await api.syncGSCData(gscSiteId.value); ElMessage.success('同步成功'); loadGSC() } catch(e) { ElMessage.error(e?.response?.data?.error||'同步失败') } finally { syncing.value = false } }

// Keyword Libraries
const titleLibs = ref([]); const showLibForm = ref(false); const libForm = ref({name:'',description:''}); const showTitles = ref(false); const curLib = ref(null); const curTitles = ref([]); const showImport = ref(false); const importText = ref(''); const importing = ref(false)
const showGSCImport = ref(false); const gscImportSiteId = ref(null); const gscImportLimit = ref(100)
async function loadLibs() { try { titleLibs.value = (await api.geoListTitleLibs()).data } catch {} }
async function createLib() { await api.geoCreateTitleLib(libForm.value); showLibForm.value=false; libForm.value={name:'',description:''}; loadLibs(); ElMessage.success('创建成功') }
async function deleteLib(r) { await ElMessageBox.confirm('删除「'+r.name+'」及所有关键词？'); await api.geoDeleteTitleLib(r.id); loadLibs() }
async function openTitles(r) { curLib.value=r; curTitles.value=(await api.geoListTitles(r.id)).data; showTitles.value=true }
async function delTitle(r) { await api.geoDeleteTitle(r.id); curTitles.value=curTitles.value.filter(t=>t.id!==r.id); loadLibs() }
function openImport(r) { curLib.value=r; importText.value=''; showImport.value=true }
async function doImport() { importing.value=true; try { const res=await api.geoImportTitles(curLib.value.id,{text:importText.value}); ElMessage.success('导入'+res.data.imported+'条'); showImport.value=false; loadLibs() } finally { importing.value=false } }
function openGSCImport(r) { curLib.value=r; gscImportSiteId.value=sites.value[0]?.id||null; gscImportLimit.value=100; showGSCImport.value=true }
async function doGSCImport() { if(!gscImportSiteId.value){ElMessage.warning('请选择站点');return} importing.value=true; try { const res=await api.geoImportFromGSC(curLib.value.id,{site_id:gscImportSiteId.value,limit:gscImportLimit.value}); ElMessage.success('从GSC导入'+res.data.imported+'条（共'+res.data.source_total+'条可用）'); showGSCImport.value=false; loadLibs() } catch(e){ElMessage.error(e?.response?.data?.error||'导入失败')} finally{importing.value=false} }

// Prompts
const prompts = ref([]); const showPromptForm = ref(false); const promptForm = ref({id:0,name:'',type:'content',content:'',description:''})
async function loadPrompts() { try { prompts.value=(await api.geoListPrompts()).data } catch {} }
function editPrompt(r) { promptForm.value={id:r.id||0,name:r.name||'',type:r.type||'content',content:r.content||'',description:r.description||''}; showPromptForm.value=true }
async function savePrompt() { const f=promptForm.value; if(!f.name||!f.content){ElMessage.warning('名称和内容必填');return} if(f.id){await api.geoUpdatePrompt(f.id,f)}else{await api.geoCreatePrompt(f)} showPromptForm.value=false; loadPrompts(); ElMessage.success('保存成功') }
async function delPrompt(r) { await ElMessageBox.confirm('删除此Prompt？'); await api.geoDeletePrompt(r.id); loadPrompts() }

// Knowledge
const knowledgeList = ref([]); const showKBForm = ref(false); const kbForm = ref({id:0,name:'',content:''})
async function loadKnowledge() { try { knowledgeList.value=(await api.geoListKnowledge()).data } catch {} }
function editKB(r) { kbForm.value={id:r.id||0,name:r.name||'',content:r.content||''}; showKBForm.value=true }
async function saveKB() { const f=kbForm.value; if(!f.name||!f.content){ElMessage.warning('必填');return} if(f.id){await api.geoUpdateKnowledge(f.id,f)}else{await api.geoCreateKnowledge(f)} showKBForm.value=false; loadKnowledge(); ElMessage.success('保存成功') }
async function delKB(r) { await ElMessageBox.confirm('删除？'); await api.geoDeleteKnowledge(r.id); loadKnowledge() }

// Tasks
const tasks = ref([]); const showTaskForm = ref(false); const taskForm = ref({id:0,name:'',title_lib_id:null,prompt_id:null,knowledge_id:0,draft_limit:20,need_review:1,is_loop:0,auto_keywords:1,auto_description:1})
async function loadTasks() { try { tasks.value=(await api.geoListTasks()).data } catch {} }
function editTask(r) { taskForm.value={id:r.id||0,name:r.name||'',title_lib_id:r.title_lib_id||null,prompt_id:r.prompt_id||null,knowledge_id:r.knowledge_id||0,draft_limit:r.draft_limit||20,need_review:r.need_review??1,is_loop:r.is_loop||0,auto_keywords:r.auto_keywords??1,auto_description:r.auto_description??1}; showTaskForm.value=true }
async function saveTask() { const f=taskForm.value; if(!f.name||!f.title_lib_id||!f.prompt_id){ElMessage.warning('任务名、关键词库、Prompt必选');return} if(f.id){await api.geoUpdateTask(f.id,f)}else{await api.geoCreateTask(f)} showTaskForm.value=false; loadTasks(); ElMessage.success('保存成功') }
async function delTask(r) { await ElMessageBox.confirm('删除任务及文章？'); await api.geoDeleteTask(r.id); loadTasks() }
async function startTask(r) { await api.geoStartTask(r.id); loadTasks() }
async function stopTask(r) { await api.geoStopTask(r.id); loadTasks() }
async function runTask(r) { r._running=true; try { const res=await api.geoRunTask(r.id); ElMessage.success('已生成: '+res.data.title+' ('+res.data.word_count+'字)'); loadTasks(); loadArticles(); loadStats() } catch(e){ElMessage.error(e?.response?.data?.error||'生成失败')} finally{r._running=false} }

// Articles
const articles = ref([]); const artPage = ref(1); const artTotal = ref(0); const artFilter = ref({status:'',review_status:''}); const showArticle = ref(false); const curArticle = ref(null)
async function loadArticles() { try { const p={page:artPage.value,per_page:20}; if(artFilter.value.status)p.status=artFilter.value.status; if(artFilter.value.review_status)p.review_status=artFilter.value.review_status; const res=await api.geoListArticles(p); articles.value=res.data.items; artTotal.value=res.data.total } catch {} }
async function viewArt(r) { try { curArticle.value=(await api.geoGetArticle(r.id)).data; showArticle.value=true } catch {} }
async function reviewArt(r,s) { await api.geoReviewArticle(r.id,{status:s}); ElMessage.success(s==='approved'?'已通过':'已拒绝'); loadArticles(); loadStats() }
async function publishArt(r) { await api.geoPublishArticle(r.id); ElMessage.success('已发布'); loadArticles(); loadStats() }
async function delArt(r) { await ElMessageBox.confirm('删除？'); await api.geoDeleteArticle(r.id); loadArticles(); loadStats() }

async function loadStats() { try { stats.value=(await api.geoStats()).data } catch {} }
onMounted(async () => { loadStats(); const res=await api.listSites().catch(()=>({data:[]})); sites.value=res.data||[]; if(sites.value.length){gscSiteId.value=sites.value[0].id;loadGSC()} loadLibs(); loadPrompts(); loadKnowledge(); loadTasks(); loadArticles() })
</script>

<style scoped>
.page-top{margin-bottom:18px} .page-top h1{font-size:20px;font-weight:800}
.stats-row{display:flex;gap:14px;margin-bottom:18px} .stat-card{flex:1;text-align:center;padding:16px 10px}
.sc-num{font-size:28px;font-weight:800} .sc-lbl{font-size:12px;color:var(--ink-hint);margin-top:4px}
.blue{color:#2563EB} .orange{color:#F59E0B} .red{color:#DC2626} .green{color:#16A34A} .purple{color:#7C3AED}
.tab-bar{margin-bottom:14px;display:flex;gap:10px;align-items:center}
.panel{background:var(--surface);border-radius:var(--r);box-shadow:var(--s1);border:1.5px solid var(--border-light);padding:16px 20px}
.text-ellipsis{display:inline-block;max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px;color:#888}
</style>
