<template>
  <div class="extlinks-page">

    <!-- 站点选择 -->
    <div class="panel selector-bar">
      <span class="selector-label">🔗 外链管理</span>
      <el-select v-model="siteId" placeholder="选择站点" style="width:260px" @change="onSiteChange">
        <el-option v-for="s in sites" :key="s.id" :label="s.name+' ('+s.domain+')'" :value="s.id" />
      </el-select>
    </div>

    <div v-if="!siteId" class="empty-hint panel">
      <div class="empty-icon">🔗</div>
      <div>请先选择站点</div>
    </div>

    <template v-else>
      <!-- 统计 -->
      <div class="stats-row">
        <div class="stat-card panel"><div class="sc-num blue">{{ stats.total_articles||0 }}</div><div class="sc-lbl">总文章</div></div>
        <div class="stat-card panel"><div class="sc-num orange">{{ stats.drafts||0 }}</div><div class="sc-lbl">草稿</div></div>
        <div class="stat-card panel"><div class="sc-num red">{{ stats.pending_review||0 }}</div><div class="sc-lbl">待审核</div></div>
        <div class="stat-card panel"><div class="sc-num green">{{ stats.published||0 }}</div><div class="sc-lbl">已发布</div></div>
        <div class="stat-card panel"><div class="sc-num purple">{{ stats.active_tasks||0 }}</div><div class="sc-lbl">运行中任务</div></div>
      </div>

      <!-- Tab 面板 -->
      <div class="panel tab-panel">
        <el-tabs v-model="activeTab">

          <!-- 关键词库 -->
          <el-tab-pane label="🔑 关键词库" name="libs">
            <div class="tab-bar">
              <el-button type="primary" @click="showLibForm=true">+ 新建关键词库</el-button>
            </div>
            <el-table :data="titleLibs" stripe>
              <el-table-column prop="name" label="库名" />
              <el-table-column prop="description" label="描述" show-overflow-tooltip />
              <el-table-column prop="title_count" label="关键词数" width="100" />
              <el-table-column label="操作" width="260">
                <template #default="{row}">
                  <el-button size="small" @click="openTitles(row)">查看关键词</el-button>
                  <el-button size="small" @click="openImport(row)">导入</el-button>
                  <el-button size="small" type="success" @click="doGSCImportDirect(row)" :loading="row._gscImporting">从GSC导入</el-button>
                  <el-button size="small" type="danger" text @click="deleteLib(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
            <el-dialog v-model="showLibForm" title="新建关键词库" width="420px">
              <el-form label-width="70px">
                <el-form-item label="库名"><el-input v-model="libForm.name" /></el-form-item>
                <el-form-item label="描述"><el-input v-model="libForm.description" /></el-form-item>
              </el-form>
              <template #footer><el-button @click="showLibForm=false">取消</el-button><el-button type="primary" @click="createLib">创建</el-button></template>
            </el-dialog>
            <!-- 查看关键词 -->
            <el-dialog v-model="showTitles" :title="'关键词库：'+(curLib?.name||'')" width="700px" top="5vh">
              <el-table :data="curTitles" stripe size="small" max-height="400">
                <el-table-column prop="title" label="关键词" /><el-table-column prop="keyword" label="附加词" width="140" />
                <el-table-column label="操作" width="70"><template #default="{row}"><el-button size="small" type="danger" text @click="delTitle(row)">删除</el-button></template></el-table-column>
              </el-table>
              <div style="margin-top:10px;text-align:center" v-if="titlesTotal>100"><el-pagination v-model:current-page="titlesPage" :page-size="100" :total="titlesTotal" layout="prev,pager,next,total" background @current-change="loadTitlesPage" /></div>
            </el-dialog>
            <!-- 导入关键词 -->
            <el-dialog v-model="showImport" title="导入关键词" width="520px">
              <p style="font-size:12px;color:#888;margin-bottom:8px">每行一个关键词，或「关键词|附加词」格式</p>
              <el-input v-model="importText" type="textarea" :rows="12" placeholder="外链建设&#10;SEO外链&#10;高质量外链|dofollow" />
              <template #footer><el-button @click="showImport=false">取消</el-button><el-button type="primary" @click="doImport" :loading="importing">导入</el-button></template>
            </el-dialog>
            <!-- 从GSC导入 -->
            <el-dialog v-model="showGSCImport" title="从GSC导入关键词" width="400px">
              <el-form label-width="100px">
                <el-form-item label="导入数量"><el-input-number v-model="gscImportLimit" :min="10" :max="500" /></el-form-item>
              </el-form>
              <template #footer><el-button @click="showGSCImport=false">取消</el-button><el-button type="primary" @click="doGSCImport" :loading="importing">从GSC导入</el-button></template>
            </el-dialog>
          </el-tab-pane>

          <!-- Prompt 模板 -->
          <el-tab-pane label="📝 Prompt 模板" name="prompts">
            <div class="tab-bar">
              <el-button type="primary" @click="editPrompt({})">+ 新建 Prompt</el-button>
            </div>
            <el-table :data="prompts" stripe>
              <el-table-column prop="name" label="名称" width="180" />
              <el-table-column prop="type" label="类型" width="100" />
              <el-table-column label="Prompt 预览" show-overflow-tooltip>
                <template #default="{row}"><span class="text-ellipsis">{{ row.content }}</span></template>
              </el-table-column>
              <el-table-column label="操作" width="140">
                <template #default="{row}">
                  <el-button size="small" @click="editPrompt(row)">编辑</el-button>
                  <el-button size="small" type="danger" text @click="delPrompt(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
            <el-dialog v-model="showPromptForm" :title="promptForm.id?'编辑 Prompt':'新建 Prompt'" width="700px" top="5vh">
              <el-form label-width="100px">
                <el-form-item label="名称"><el-input v-model="promptForm.name" /></el-form-item>
                <el-form-item label="类型"><el-select v-model="promptForm.type"><el-option value="content" label="内容生成" /><el-option value="keyword" label="关键词提取" /></el-select></el-form-item>
                <el-form-item label="Prompt">
                  <el-input v-model="promptForm.content" type="textarea" :rows="10" placeholder="支持变量：{{title}} {{keyword}} {{knowledge}}" />
                  <div style="font-size:11px;color:#888;margin-top:4px">可用变量：{{title}} {{keyword}} {{knowledge}}</div>
                </el-form-item>
                <el-form-item label="描述"><el-input v-model="promptForm.description" /></el-form-item>
              </el-form>
              <template #footer><el-button @click="showPromptForm=false">取消</el-button><el-button type="primary" @click="savePrompt">保存</el-button></template>
            </el-dialog>
          </el-tab-pane>

          <!-- 知识库 -->
          <el-tab-pane label="📚 知识库" name="knowledge">
            <div class="tab-bar">
              <el-button type="primary" @click="editKB({})">+ 新建知识库</el-button>
            </div>
            <el-table :data="knowledgeList" stripe>
              <el-table-column prop="name" label="名称" width="200" />
              <el-table-column label="内容预览" show-overflow-tooltip>
                <template #default="{row}"><span class="text-ellipsis">{{ row.content }}</span></template>
              </el-table-column>
              <el-table-column label="操作" width="140">
                <template #default="{row}">
                  <el-button size="small" @click="editKB(row)">编辑</el-button>
                  <el-button size="small" type="danger" text @click="delKB(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
            <el-dialog v-model="showKBForm" :title="kbForm.id?'编辑知识库':'新建知识库'" width="700px" top="5vh">
              <el-form label-width="70px">
                <el-form-item label="名称"><el-input v-model="kbForm.name" /></el-form-item>
                <el-form-item label="内容"><el-input v-model="kbForm.content" type="textarea" :rows="12" placeholder="填写外链相关背景知识、写作风格、品牌信息等" /></el-form-item>
              </el-form>
              <template #footer><el-button @click="showKBForm=false">取消</el-button><el-button type="primary" @click="saveKB">保存</el-button></template>
            </el-dialog>
          </el-tab-pane>

          <!-- 生成任务 -->
          <el-tab-pane label="⚙️ 生成任务" name="tasks">
            <div class="tab-bar">
              <el-button type="primary" @click="editTask({})">+ 新建任务</el-button>
            </div>
            <el-table :data="tasks" stripe>
              <el-table-column prop="name" label="任务名" />
              <el-table-column prop="title_lib_name" label="关键词库" width="130" />
              <el-table-column prop="prompt_name" label="Prompt" width="130" />
              <el-table-column label="状态" width="80">
                <template #default="{row}"><el-tag :type="row.status==='active'?'success':'info'" size="small">{{ row.status==='active'?'运行中':'已停止' }}</el-tag></template>
              </el-table-column>
              <el-table-column label="已生成" width="80"><template #default="{row}">{{ row.generated_count||0 }}</template></el-table-column>
              <el-table-column label="草稿数" width="80"><template #default="{row}">{{ row.draft_count||0 }}</template></el-table-column>
              <el-table-column label="操作" width="240">
                <template #default="{row}">
                  <el-button size="small" type="primary" @click="runTask(row)" :loading="row._running">立即生成</el-button>
                  <el-button size="small" v-if="row.status!=='active'" @click="startTask(row)">启动</el-button>
                  <el-button size="small" v-else @click="stopTask(row)">暂停</el-button>
                  <el-button size="small" @click="editTask(row)">编辑</el-button>
                  <el-button size="small" type="danger" text @click="delTask(row)">删除</el-button>
                </template>
              </el-table-column>
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
              <el-table-column prop="title" label="标题" show-overflow-tooltip />
              <el-table-column prop="task_name" label="任务" width="120" />
              <el-table-column prop="word_count" label="字数" width="70" />
              <el-table-column label="审核" width="80">
                <template #default="{row}"><el-tag size="small" :type="row.review_status==='approved'?'success':row.review_status==='rejected'?'danger':'warning'">{{ {approved:'通过',rejected:'拒绝',pending:'待审'}[row.review_status] }}</el-tag></template>
              </el-table-column>
              <el-table-column label="操作" width="250">
                <template #default="{row}">
                  <el-button size="small" @click="viewArt(row)">查看</el-button>
                  <el-button size="small" type="success" v-if="row.review_status==='pending'" @click="reviewArt(row,'approved')">通过</el-button>
                  <el-button size="small" type="danger" v-if="row.review_status==='pending'" @click="reviewArt(row,'rejected')">拒绝</el-button>
                  <el-button size="small" type="primary" v-if="row.review_status==='approved'&&row.status==='draft'" @click="publishArt(row)">发布</el-button>
                  <el-button size="small" type="danger" text @click="delArt(row)">删除</el-button>
                </template>
              </el-table-column>
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
      </div>
    </template>

    <!-- 公共外链库 -->
    <div class="panel" style="margin-top:20px">
      <div class="panel-header">
        <h2>📦 公共外链库 <span style="font-size:12px;font-weight:400;color:var(--ink-hint);margin-left:8px">所有用户共享</span></h2>
        <button class="btn btn-primary btn-sm" @click="showPoolImport=true">＋ 批量添加</button>
      </div>
      <el-table :data="pool" stripe max-height="400" size="small">
        <el-table-column type="index" label="#" width="50" />
        <el-table-column prop="name" label="网站名称" width="200" />
        <el-table-column prop="url" label="URL" show-overflow-tooltip />
        <el-table-column prop="created_at" label="添加时间" width="170" />
        <el-table-column label="操作" width="80">
          <template #default="{row}"><el-button size="small" type="danger" text @click="delPoolItem(row)">删除</el-button></template>
        </el-table-column>
      </el-table>
      <div v-if="!pool.length" style="text-align:center;padding:30px;color:var(--ink-hint);font-size:13px">暂无外链库数据，点击「批量添加」导入</div>
    </div>

    <!-- 批量导入弹窗 -->
    <el-dialog v-model="showPoolImport" title="批量添加外链到公共库" width="560px">
      <p style="font-size:12px;color:#888;margin-bottom:8px">每行一条，格式：<code>URL|网站名称</code> 或只填 URL（自动去重）</p>
      <el-input v-model="poolImportText" type="textarea" :rows="10" placeholder="https://example.com|示例站&#10;https://blog.xxx.com|XXX博客" />
      <template #footer>
        <el-button @click="showPoolImport=false">取消</el-button>
        <el-button type="primary" @click="doPoolImport" :loading="poolImporting">导入</el-button>
      </template>
    </el-dialog>

  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const activeTab = ref('libs')
const stats = ref({})
const sites = ref([])
const siteId = ref(null)

function onSiteChange() { loadAll() }
function loadAll() { if(!siteId.value)return; loadStats(); loadLibs(); loadPrompts(); loadKnowledge(); loadTasks(); loadArticles() }

// ── 关键词库 ──
const titleLibs = ref([])
const showLibForm = ref(false)
const libForm = ref({ name: '', description: '' })
const showTitles = ref(false)
const curLib = ref(null)
const curTitles = ref([])
const titlesPage = ref(1)
const titlesTotal = ref(0)
const showImport = ref(false)
const importText = ref('')
const importing = ref(false)
const showGSCImport = ref(false)
const gscImportLimit = ref(100)

async function loadLibs() { try { titleLibs.value = (await api.geoListTitleLibs({ site_id: siteId.value, content_type: 'extlink' })).data } catch {} }
async function createLib() { await api.geoCreateTitleLib({ name: libForm.value.name, description: libForm.value.description, site_id: siteId.value, content_type: 'extlink' }); showLibForm.value = false; libForm.value = { name: '', description: '' }; loadLibs(); ElMessage.success('创建成功') }
async function deleteLib(r) { await ElMessageBox.confirm('删除「' + r.name + '」？'); await api.geoDeleteTitleLib(r.id,'extlink'); loadLibs() }
async function openTitles(r) { curLib.value = r; titlesPage.value = 1; await loadTitlesPage(); showTitles.value = true }
async function loadTitlesPage() { const res = await api.geoListTitles(curLib.value.id, { page: titlesPage.value, per_page: 100 }); curTitles.value = res.data.items || []; titlesTotal.value = res.data.total || 0 }
async function delTitle(r) { await api.geoDeleteTitle(r.id); curTitles.value = curTitles.value.filter(t => t.id !== r.id); loadLibs() }
function openImport(r) { curLib.value = r; importText.value = ''; showImport.value = true }
async function doImport() { importing.value = true; try { const res = await api.geoImportTitles(curLib.value.id, { text: importText.value }); ElMessage.success('导入' + res.data.imported + '条'); showImport.value = false; loadLibs() } finally { importing.value = false } }
function openGSCImport(r) { curLib.value = r; gscImportLimit.value = 100; showGSCImport.value = true }
async function doGSCImport() { importing.value = true; try { const res = await api.geoImportFromGSC(curLib.value.id, { site_id: siteId.value, limit: gscImportLimit.value }); ElMessage.success('从GSC导入' + res.data.imported + '条'); showGSCImport.value = false; loadLibs() } catch (e) { ElMessage.error(e?.response?.data?.error || '导入失败') } finally { importing.value = false } }
async function doGSCImportDirect(r) { r._gscImporting = true; try { const res = await api.geoImportFromGSC(r.id, { site_id: siteId.value, limit: 0 }); ElMessage.success('从GSC导入 ' + res.data.imported + ' 条关键词（来源共' + res.data.source_total + '条，已过滤排名1-99）'); loadLibs() } catch (e) { ElMessage.error(e?.response?.data?.error || '导入失败，请先在数据中心同步GSC') } finally { r._gscImporting = false } }

// ── Prompt ──
const prompts = ref([])
const showPromptForm = ref(false)
const promptForm = ref({ id: 0, name: '', type: 'content', content: '', description: '' })

async function loadPrompts() { try { prompts.value = (await api.geoListPrompts({ content_type: 'extlink' })).data } catch {} }
function editPrompt(r) { promptForm.value = { id: r.id || 0, name: r.name || '', type: r.type || 'content', content: r.content || '', description: r.description || '' }; showPromptForm.value = true }
async function savePrompt() { const f = promptForm.value; if (!f.name || !f.content) { ElMessage.warning('名称和内容必填'); return } if (f.id) { await api.geoUpdatePrompt(f.id, f) } else { await api.geoCreatePrompt({ ...f, content_type: 'extlink' }) } showPromptForm.value = false; loadPrompts(); ElMessage.success('保存成功') }
async function delPrompt(r) { await ElMessageBox.confirm('删除？'); await api.geoDeletePrompt(r.id,'extlink'); loadPrompts() }

// ── 知识库 ──
const knowledgeList = ref([])
const showKBForm = ref(false)
const kbForm = ref({ id: 0, name: '', content: '' })

async function loadKnowledge() { try { knowledgeList.value = (await api.geoListKnowledge({ content_type: 'extlink' })).data } catch {} }
function editKB(r) { kbForm.value = { id: r.id || 0, name: r.name || '', content: r.content || '' }; showKBForm.value = true }
async function saveKB() { const f = kbForm.value; if (!f.name || !f.content) { ElMessage.warning('必填'); return } if (f.id) { await api.geoUpdateKnowledge(f.id, f) } else { await api.geoCreateKnowledge({ ...f, content_type: 'extlink' }) } showKBForm.value = false; loadKnowledge(); ElMessage.success('保存成功') }
async function delKB(r) { await ElMessageBox.confirm('删除？'); await api.geoDeleteKnowledge(r.id,'extlink'); loadKnowledge() }

// ── 生成任务 ──
const tasks = ref([])
const showTaskForm = ref(false)
const taskForm = ref({ id: 0, name: '', title_lib_id: null, prompt_id: null, knowledge_id: 0, draft_limit: 20, need_review: 1, is_loop: 0, auto_keywords: 1, auto_description: 1 })

async function loadTasks() { try { tasks.value = (await api.geoListTasks({ site_id: siteId.value, content_type: 'extlink' })).data } catch {} }
function editTask(r) { taskForm.value = { id: r.id || 0, name: r.name || '', title_lib_id: r.title_lib_id || null, prompt_id: r.prompt_id || null, knowledge_id: r.knowledge_id || 0, draft_limit: r.draft_limit || 20, need_review: r.need_review ?? 1, is_loop: r.is_loop || 0, auto_keywords: r.auto_keywords ?? 1, auto_description: r.auto_description ?? 1 }; showTaskForm.value = true }
async function saveTask() { const f = taskForm.value; if (!f.name || !f.title_lib_id || !f.prompt_id) { ElMessage.warning('任务名、关键词库、Prompt必选'); return } if (f.id) { await api.geoUpdateTask(f.id, f) } else { await api.geoCreateTask({ ...f, site_id: siteId.value, content_type: 'extlink' }) } showTaskForm.value = false; loadTasks(); ElMessage.success('保存成功') }
async function delTask(r) { await ElMessageBox.confirm('删除？'); await api.geoDeleteTask(r.id,'extlink'); loadTasks() }
async function startTask(r) { await api.geoStartTask(r.id); loadTasks() }
async function stopTask(r) { await api.geoStopTask(r.id); loadTasks() }
async function runTask(r) { r._running = true; try { const res = await api.geoRunTask(r.id); ElMessage.success('已生成: ' + res.data.title + ' (' + res.data.word_count + '字)'); loadTasks(); loadArticles(); loadStats() } catch (e) { ElMessage.error(e?.response?.data?.error || '生成失败') } finally { r._running = false } }

// ── 文章审核 ──
const articles = ref([])
const artPage = ref(1)
const artTotal = ref(0)
const artFilter = ref({ status: '', review_status: '' })
const showArticle = ref(false)
const curArticle = ref(null)

async function loadArticles() { try { const p = { page: artPage.value, per_page: 20, site_id: siteId.value, content_type: 'extlink' }; if (artFilter.value.status) p.status = artFilter.value.status; if (artFilter.value.review_status) p.review_status = artFilter.value.review_status; const res = await api.geoListArticles(p); articles.value = res.data.items; artTotal.value = res.data.total } catch {} }
async function viewArt(r) { try { curArticle.value = (await api.geoGetArticle(r.id)).data; showArticle.value = true } catch {} }
async function reviewArt(r, s) { await api.geoReviewArticle(r.id, { status: s }); ElMessage.success(s === 'approved' ? '已通过' : '已拒绝'); loadArticles(); loadStats() }
async function publishArt(r) { await api.geoPublishArticle(r.id); ElMessage.success('已发布'); loadArticles(); loadStats() }
async function delArt(r) { await ElMessageBox.confirm('删除？'); await api.geoDeleteArticle(r.id,'extlink'); loadArticles(); loadStats() }

async function loadStats() { try { stats.value = (await api.geoStats({ site_id: siteId.value, content_type: 'extlink' })).data } catch {} }

// ── 公共外链库 ──
const pool = ref([])
const showPoolImport = ref(false)
const poolImportText = ref('')
const poolImporting = ref(false)

async function loadPool() { try { pool.value = (await api.listExtlinkPool()).data } catch {} }
async function doPoolImport() {
  poolImporting.value = true
  try { const res = await api.importExtlinkPool({ text: poolImportText.value }); ElMessage.success('导入 ' + res.data.imported + ' 条'); showPoolImport.value = false; loadPool() }
  catch { ElMessage.error('导入失败') }
  finally { poolImporting.value = false }
}
async function delPoolItem(r) { await ElMessageBox.confirm('删除「' + (r.name || r.url) + '」？'); await api.deleteExtlinkPoolItem(r.id); loadPool() }

onMounted(async () => {
  const res = await api.listSites().catch(() => ({ data: [] }))
  sites.value = res.data || []
  if (sites.value.length) { siteId.value = sites.value[0].id; loadAll() }
  loadPool()
})
</script>

<style scoped>
.extlinks-page { display:flex; flex-direction:column; gap:16px; }
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:16px 22px; }
.selector-bar { display:flex; align-items:center; gap:12px; flex-wrap:wrap; }
.selector-label { font-size:13.5px; font-weight:700; color:var(--ink-mid); white-space:nowrap; }
.stats-row { display:flex; gap:14px; }
.stat-card { flex:1; text-align:center; padding:16px 10px; }
.sc-num { font-size:28px; font-weight:800; } .sc-lbl { font-size:12px; color:var(--ink-hint); margin-top:4px; }
.blue{color:#2563EB} .orange{color:#F59E0B} .red{color:#DC2626} .green{color:#16A34A} .purple{color:#7C3AED}
.tab-panel { padding:0; }
.tab-bar { margin-bottom:14px; display:flex; gap:10px; align-items:center; }
.text-ellipsis { display:inline-block; max-width:300px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:12px; color:#888; }
.panel-header { display:flex; align-items:center; justify-content:space-between; margin-bottom:14px; }
.panel-header h2 { font-size:15px; font-weight:700; }
.empty-hint { text-align:center; padding:50px; color:var(--ink-hint); }
.empty-icon { font-size:44px; margin-bottom:12px; }
.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 18px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-sm { padding:6px 14px; font-size:12.5px; }
:deep(.el-tabs__content) { padding: 16px 20px; }
</style>
