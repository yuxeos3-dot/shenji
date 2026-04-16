<template>
  <div>
    <div class="back-row">
      <a href="#" @click.prevent="$router.push('/reports')" class="back-btn">← 返回汇报列表</a>
      <span class="back-sep">·</span>
      <span class="back-url">写汇报 · {{ today }}</span>
    </div>

    <div class="panel">
      <div class="tip-banner">
        💡 填写流水账即可，点「✨ AI 润色生成日报」自动整理成规范格式。<strong>「Agent 自动化应用」为必填项</strong>，且只接受实际跑通的业务场景。
      </div>

      <!-- 1. 今日做的事 -->
      <div class="form-group">
        <label class="section-label">
          <span class="section-num">1</span>
          📝 今日做的事（流水账即可，AI 会整理）
        </label>
        <textarea v-model="raw.done" rows="5"
          placeholder="随便写今天做的事情，比如：
- 给 hl365.com 跑了一次巡检，发现 42 个问题
- 和开发对齐了 Schema 改造排期
- 改了外链管理页的筛选条件"></textarea>
      </div>

      <!-- 2. 核心数据 -->
      <div class="form-group">
        <label class="section-label">
          <span class="section-num">2</span>
          📊 核心数据
        </label>
        <textarea v-model="raw.coreData" rows="3"
          placeholder="核心数据变化（如 GSC 点击、GA 会话、排名变化等）
- hl365.com 昨日点击 1240，环比 +8.5%
- Top 10 关键词排名平均前进 2.3 位"></textarea>
      </div>

      <!-- 3. 明日规划 -->
      <div class="form-group">
        <label class="section-label">
          <span class="section-num">3</span>
          📌 明日工作规划
        </label>
        <textarea v-model="raw.tomorrow" rows="4"
          placeholder="明天打算做什么
- 完成剩余 3 个站点的 Schema 部署
- 整理本周外链增长数据"></textarea>
      </div>

      <!-- 4. 问题 -->
      <div class="form-group">
        <label class="section-label">
          <span class="section-num">4</span>
          ⚠️ 遇到问题
        </label>
        <textarea v-model="raw.issues" rows="2"
          placeholder="可选，没问题写「无」即可"></textarea>
      </div>

      <!-- 5. Agent 自动化应用（必填） -->
      <div class="form-group">
        <label class="section-label required">
          <span class="section-num">5</span>
          🤖 Agent 自动化应用（必须有，至少 1 条）
        </label>
        <div class="rules-box">
          <div class="rules-head">📋 填写规则</div>
          <div class="rules-ok">✅ 只填写「今天已经实际跑通并产生结果」的内容 · 实际业务场景 · 自动化产出 · 人效提升</div>
          <div class="rules-no">❌ 禁止：学习 AI / 调研工具 / 研究自动化 / 部署环境（任一出现日报直接退回）</div>
        </div>

        <div v-for="(e, idx) in agentEntries" :key="idx" class="agent-card">
          <div class="agent-head">
            <span class="agent-idx">自动化应用 #{{ idx + 1 }}</span>
            <button v-if="agentEntries.length > 1" class="btn-icon" @click="agentEntries.splice(idx, 1)">✕ 删除</button>
          </div>

          <div class="agent-field">
            <label>跑通场景</label>
            <input v-model="e.scenario" placeholder="例：xx 站点自动巡检 + AI 审计报告生成" />
          </div>
          <div class="agent-field">
            <label>自动化流程</label>
            <textarea v-model="e.flow" rows="2" placeholder="例：每日凌晨 3 点 cron 触发抓取 → AI 调用审计 → 生成 Word 报告 → Telegram 推送"></textarea>
          </div>
          <div class="agent-field">
            <label>自动化产出</label>
            <textarea v-model="e.output" rows="2" placeholder="例：8 份站点审计报告 + 324 个具体优化建议，自动入仓"></textarea>
          </div>
          <div class="agent-row-2">
            <div class="agent-field">
              <label>节省工时</label>
              <input v-model="e.hours_saved" placeholder="例：4 小时/天" />
            </div>
            <div class="agent-field">
              <label>是否入库（集团工作流库）</label>
              <select v-model="e.in_library">
                <option value="">请选择</option>
                <option value="是">是</option>
                <option value="否">否</option>
              </select>
            </div>
          </div>
          <div class="agent-field">
            <label>封装 Skill / 工作流名称</label>
            <input v-model="e.skill_name" placeholder="例：seo-audit-daily" />
          </div>
        </div>

        <button class="btn btn-xs" @click="addAgent" style="margin-top:8px">+ 增加一条自动化应用</button>
      </div>

      <!-- AI 润色 + 预览 -->
      <div class="form-actions">
        <button class="btn btn-polish" @click="polish" :disabled="polishing">
          {{ polishing ? '润色中...' : '✨ AI 润色生成日报' }}
        </button>
      </div>

      <!-- 润色结果预览 + 提交 -->
      <div v-if="polishedContent" class="polished-panel">
        <div class="polished-head">
          <span>✅ 已生成日报（可在下方直接修改）</span>
          <button class="btn btn-xs" @click="copyPolished">📋 复制</button>
        </div>
        <textarea v-model="polishedContent" rows="20" class="polished-text"></textarea>
        <div style="margin-top:16px; display:flex; gap:10px;">
          <button class="btn btn-primary" @click="submit" :disabled="submitting">
            {{ submitting ? '提交中...' : '☀ 提交日报' }}
          </button>
          <button class="btn" @click="polishedContent = ''">清空重来</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'

const router = useRouter()
const today = new Date().toISOString().slice(0, 10)
const submitting = ref(false)
const polishing = ref(false)

const raw = ref({ done: '', coreData: '', tomorrow: '', issues: '' })
const agentEntries = ref([ newAgent() ])
const polishedContent = ref('')

function newAgent() {
  return { scenario: '', flow: '', output: '', hours_saved: '', skill_name: '', in_library: '' }
}
function addAgent() { agentEntries.value.push(newAgent()) }

onMounted(async () => {
  try {
    const res = await api.getReport(today)
    if (res.data?.content) {
      polishedContent.value = res.data.content
    }
  } catch {}
})

async function polish() {
  // 前端预校验
  if (!raw.value.done.trim()) {
    ElMessage.warning('请先填写「今日做的事」'); return
  }
  if (!agentEntries.value.length) {
    ElMessage.warning('Agent 自动化应用为必填项'); return
  }
  for (let i = 0; i < agentEntries.value.length; i++) {
    const e = agentEntries.value[i]
    const miss = ['scenario','flow','output','hours_saved','skill_name','in_library']
      .filter(k => !String(e[k] || '').trim())
    if (miss.length) {
      ElMessage.error(`Agent 自动化应用 #${i+1} 有未填字段：${miss.join('、')}`); return
    }
  }

  polishing.value = true
  try {
    const res = await api.polishReport({
      done_raw: raw.value.done,
      core_data: raw.value.coreData,
      tomorrow_raw: raw.value.tomorrow,
      issues: raw.value.issues,
      agent_entries: agentEntries.value,
    })
    polishedContent.value = res.data.content
    ElMessage.success('日报已生成，请检查后提交')
  } catch (e) {
    const msg = e?.response?.data?.error || 'AI 润色失败'
    ElMessage.error(msg)
  } finally {
    polishing.value = false
  }
}

async function submit() {
  if (!polishedContent.value.trim()) { ElMessage.warning('日报内容为空'); return }
  submitting.value = true
  try {
    await api.submitReport({
      content: polishedContent.value,
      tasks_done: [],
      tasks_todo: [],
      issues: '',
    })
    ElMessage.success('日报提交成功')
    router.push('/reports')
  } catch {
    ElMessage.error('提交失败')
  } finally { submitting.value = false }
}

async function copyPolished() {
  try {
    await navigator.clipboard.writeText(polishedContent.value)
    ElMessage.success('已复制')
  } catch {}
}
</script>

<style scoped>
.back-row { display:flex; align-items:center; gap:8px; margin-bottom:18px; }
.back-btn { font-size:13px; font-weight:600; color:var(--sun); }
.back-sep { color:var(--ink-hint); }
.back-url { font-size:14px; font-weight:700; }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }

.tip-banner { background:#FFF9EB; border:1.5px solid #F59E0B; border-radius:10px; padding:10px 14px; font-size:13px; color:#78350F; margin-bottom:20px; line-height:1.6; }

.form-group { margin-bottom:22px; }
.section-label { display:flex; align-items:center; gap:8px; margin-bottom:9px; font-weight:700; color:var(--ink-mid); font-size:14px; }
.section-label.required::after { content:'必填'; background:#FEE2E2; color:#B91C1C; font-size:11px; padding:2px 7px; border-radius:4px; margin-left:6px; font-weight:600; }
.section-num { display:inline-flex; align-items:center; justify-content:center; width:22px; height:22px; background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-radius:50%; font-size:12px; font-weight:700; }
.form-group textarea, .form-group input, .form-group select {
  width:100%; padding:10px 13px; border:1.5px solid var(--border); border-radius:9px;
  font-size:14px; font-family:inherit; background:var(--surface); color:var(--ink);
  transition:border-color .18s, box-shadow .18s; outline:none; line-height:1.65;
  box-sizing:border-box;
}
.form-group textarea:focus, .form-group input:focus, .form-group select:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.14); background:#FFFDF9; }
.form-group textarea { resize:vertical; min-height:80px; }

.rules-box { background:#F1F5F9; border-left:4px solid #3B82F6; padding:10px 14px; border-radius:6px; margin-bottom:12px; font-size:12.5px; line-height:1.75; }
.rules-head { font-weight:700; color:#1E40AF; margin-bottom:4px; }
.rules-ok { color:#065F46; }
.rules-no { color:#B91C1C; }

.agent-card { border:1.5px solid #E2E8F0; border-radius:10px; padding:14px 16px; margin-bottom:12px; background:#FAFBFC; }
.agent-head { display:flex; justify-content:space-between; align-items:center; margin-bottom:10px; }
.agent-idx { font-weight:700; font-size:13px; color:#1E40AF; background:#DBEAFE; padding:3px 10px; border-radius:4px; }
.agent-field { margin-bottom:10px; }
.agent-field label { display:block; font-size:12px; font-weight:600; color:var(--ink-mid); margin-bottom:5px; }
.agent-row-2 { display:grid; grid-template-columns:1fr 1fr; gap:12px; }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep),var(--sun)); color:#fff; box-shadow:0 8px 24px rgba(249,115,22,.48); }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; }
.btn-polish { background:linear-gradient(135deg,#6366F1,#8B5CF6); color:#fff; border-color:transparent; padding:11px 28px; font-size:14px; box-shadow:0 4px 12px rgba(99,102,241,.35); }
.btn-polish:hover { background:linear-gradient(135deg,#4F46E5,#7C3AED); box-shadow:0 8px 20px rgba(99,102,241,.5); }
.btn-polish:disabled { opacity:.6; cursor:not-allowed; }
.btn-xs { padding:4px 12px; font-size:12px; border-radius:6px; }
.btn-xs:hover { background:var(--sun-glow); border-color:var(--sun); color:var(--sun-deep); }
.btn-icon { background:none; border:none; cursor:pointer; font-size:12px; color:var(--err); padding:2px 6px; transition:opacity .15s; }
.btn-icon:hover { opacity:.7; }
.form-actions { margin-top:22px; text-align:center; }

.polished-panel { margin-top:24px; padding:18px 20px; background:#F0FDF4; border:1.5px solid #10B981; border-radius:12px; }
.polished-head { display:flex; justify-content:space-between; align-items:center; margin-bottom:10px; font-weight:700; color:#065F46; font-size:13.5px; }
.polished-text { width:100%; padding:14px; border:1.5px solid #86EFAC; border-radius:9px; font-family:'JetBrains Mono','Consolas',monospace; font-size:13px; line-height:1.75; background:#fff; box-sizing:border-box; resize:vertical; }
</style>
