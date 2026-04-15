<template>
  <div class="settings-page">
    <el-tabs v-model="activeTab" class="settings-tabs">

      <!-- ── 基础配置 ── -->
      <el-tab-pane label="🏠 基础配置" name="basic">
        <div class="tab-content">
          <div class="section-card">
            <div class="section-title">系统基本设置</div>
            <div class="field-row">
              <label class="field-label">系统标题</label>
              <input class="field-input" v-model="settings.site_title" placeholder="SEO巡站管理系统" />
              <div class="field-hint">显示在浏览器标签和页面顶部</div>
            </div>
          </div>
          <div class="save-bar">
            <button class="btn btn-primary" @click="save" :disabled="saving">{{ saving ? '保存中...' : '💾 保存配置' }}</button>
          </div>
        </div>
      </el-tab-pane>

      <!-- ── AI 配置 ── -->
      <el-tab-pane label="🤖 人工智能" name="ai">
        <div class="tab-content">
          <div class="section-card">
            <div class="section-title">AI 接口配置</div>
            <div class="section-desc">用于对爬取页面进行 SEO 分析的 AI 大语言模型接口</div>
            <div class="field-row">
              <label class="field-label">接口地址</label>
              <input class="field-input" v-model="settings.ai_api_url" placeholder="https://api.openai.com/v1" />
            </div>
            <div class="field-row">
              <label class="field-label">接口密钥（API Key）</label>
              <input class="field-input" v-model="settings.ai_api_key" type="password" placeholder="••••••" autocomplete="new-password" />
              <div class="field-hint">密钥已加密保存，填写新值可覆盖原密钥</div>
            </div>
            <div class="field-row">
              <label class="field-label">模型名称</label>
              <input class="field-input" v-model="settings.ai_model" placeholder="claude-sonnet-4-6 / gpt-4o" />
              <div class="field-hint">填写模型的完整名称，例如 claude-sonnet-4-6、gpt-4o、gemini-2.0-flash 等</div>
            </div>
            <div class="action-row">
              <button class="btn btn-test" @click="testAI" :disabled="testing.ai">{{ testing.ai ? '检测中...' : '🔌 测试接口连通性' }}</button>
            </div>
          </div>
          <div class="save-bar">
            <button class="btn btn-primary" @click="save" :disabled="saving">{{ saving ? '保存中...' : '💾 保存配置' }}</button>
          </div>
        </div>
      </el-tab-pane>

      <!-- ── 数据平台 ── -->
      <el-tab-pane label="🔗 数据平台" name="platforms">
        <div class="tab-content">

          <!-- Google -->
          <div class="section-card">
            <div class="section-title">
              <span class="platform-icon">G</span> Google 平台（GSC + GA）
            </div>
            <div class="section-desc">Google Search Console 与 Google Analytics 共用一套 OAuth 授权</div>
            <div class="field-row">
              <label class="field-label">客户端 ID（Client ID）</label>
              <input class="field-input" v-model="settings.gsc_client_id" placeholder="xxxx.apps.googleusercontent.com" />
            </div>
            <div class="field-row">
              <label class="field-label">客户端密钥（Client Secret）</label>
              <input class="field-input" v-model="settings.gsc_client_secret" type="password" placeholder="••••••" autocomplete="new-password" />
            </div>
            <div class="field-row">
              <label class="field-label">回调地址（Redirect URI）</label>
              <input class="field-input" v-model="settings.gsc_redirect_uri" placeholder="https://yourdomain.com/api/v1/oauth/gsc/callback" />
              <div class="field-hint">需要在 Google Cloud Console 的已授权重定向 URI 中填写此地址</div>
            </div>
            <div class="action-row">
              <button class="btn btn-google" @click="authGSC">📊 授权 Search Console</button>
              <button class="btn btn-google" @click="authGA">📈 授权 Analytics</button>
            </div>
          </div>

          <!-- Yandex -->
          <div class="section-card">
            <div class="section-title">
              <span class="platform-icon" style="background:#FF0000">Я</span> Yandex 平台
            </div>
            <div class="section-desc">Yandex Webmaster 数据接入</div>
            <div class="field-row">
              <label class="field-label">应用 ID（Client ID）</label>
              <input class="field-input" v-model="settings.yandex_client_id" placeholder="Yandex 应用 ID" />
            </div>
            <div class="field-row">
              <label class="field-label">应用密钥（Client Secret）</label>
              <input class="field-input" v-model="settings.yandex_client_secret" type="password" placeholder="••••••" autocomplete="new-password" />
            </div>
            <div class="action-row">
              <button class="btn btn-yandex" @click="authYandex">🔗 授权 Yandex Webmaster</button>
            </div>
          </div>

          <div class="save-bar">
            <button class="btn btn-primary" @click="save" :disabled="saving">{{ saving ? '保存中...' : '💾 保存配置' }}</button>
          </div>
        </div>
      </el-tab-pane>

      <!-- ── 消息通知 ── -->
      <el-tab-pane label="📱 消息通知" name="notify">
        <div class="tab-content">
          <div class="section-card">
            <div class="section-title">Telegram 机器人推送</div>
            <div class="section-desc">审计完成、发现问题时自动向 Telegram 发送消息</div>
            <div class="field-row">
              <label class="field-label">机器人令牌（Bot Token）</label>
              <input class="field-input" v-model="settings.telegram_bot_token" type="password" placeholder="••••••" autocomplete="new-password" />
              <div class="field-hint">在 Telegram 中找 @BotFather 创建机器人后获取</div>
            </div>
            <div class="field-row">
              <label class="field-label">聊天 ID（Chat ID）</label>
              <input class="field-input" v-model="settings.telegram_chat_id" placeholder="-100xxxxxxxxxx 或个人 ID" />
              <div class="field-hint">可通过 @userinfobot 或 @getidsbot 获取</div>
            </div>
            <div class="action-row">
              <button class="btn btn-test" @click="testTG" :disabled="testing.tg">{{ testing.tg ? '发送中...' : '📨 发送测试消息' }}</button>
            </div>
          </div>
          <div class="save-bar">
            <button class="btn btn-primary" @click="save" :disabled="saving">{{ saving ? '保存中...' : '💾 保存配置' }}</button>
          </div>
        </div>
      </el-tab-pane>

      <!-- ── 巡检调度 ── -->
      <el-tab-pane label="⏰ 全局调度" name="schedule">
        <div class="tab-content">
          <div class="section-card">
            <div class="section-title">全局自动巡检设置</div>
            <div class="section-desc">
              全局调度对所有未设置独立计划的站点生效。如需对某个站点单独设置巡检频率，请进入该站点的详情页面设置。
            </div>

            <div class="field-row">
              <label class="field-label">启用全局自动巡检</label>
              <div class="toggle-wrap">
                <label class="toggle-label" :class="autoAudit?'toggle-on':'toggle-off'" @click="autoAudit=!autoAudit">
                  <span class="toggle-switch"></span>
                  <span class="toggle-text">{{ autoAudit ? '已开启' : '已关闭' }}</span>
                </label>
              </div>
            </div>

            <template v-if="autoAudit">
              <div class="schedule-mode-row">
                <label class="field-label">调度模式</label>
                <div class="mode-cards">
                  <div :class="['mode-card', scheduleMode==='interval'&&'mode-active']" @click="scheduleMode='interval'">
                    <div class="mode-icon">⏱</div>
                    <div class="mode-name">按间隔</div>
                    <div class="mode-desc">每隔 N 小时执行一次</div>
                  </div>
                  <div :class="['mode-card', scheduleMode==='daily'&&'mode-active']" @click="scheduleMode='daily'">
                    <div class="mode-icon">🕘</div>
                    <div class="mode-name">按时间</div>
                    <div class="mode-desc">每天固定时间点执行</div>
                  </div>
                  <div :class="['mode-card', scheduleMode==='cron'&&'mode-active']" @click="scheduleMode='cron'">
                    <div class="mode-icon">⚙️</div>
                    <div class="mode-name">自定义</div>
                    <div class="mode-desc">使用 Cron 表达式</div>
                  </div>
                </div>
              </div>

              <div class="field-row" v-if="scheduleMode==='interval'">
                <label class="field-label">巡检间隔（小时）</label>
                <div class="number-input-row">
                  <button class="num-btn" @click="intervalHours=Math.max(1,intervalHours-1)">－</button>
                  <input class="field-input num-input" type="number" v-model.number="intervalHours" min="1" max="168" />
                  <button class="num-btn" @click="intervalHours=Math.min(168,intervalHours+1)">＋</button>
                  <span class="num-unit">小时巡查一次</span>
                </div>
                <div class="field-hint">取值范围 1 ~ 168 小时（1 周）</div>
              </div>

              <div class="field-row" v-if="scheduleMode==='daily'">
                <label class="field-label">每天巡检时间</label>
                <div class="hour-selector">
                  <span
                    v-for="h in 24" :key="h-1"
                    :class="['hour-chip', dailyHour===(h-1)&&'hour-active']"
                    @click="dailyHour=h-1"
                  >{{ String(h-1).padStart(2,'0') }}时</span>
                </div>
                <div class="field-hint">当前选择：每天 {{ String(dailyHour).padStart(2,'0') }}:00 自动巡检</div>
              </div>

              <div class="field-row" v-if="scheduleMode==='cron'">
                <label class="field-label">Cron 表达式</label>
                <input class="field-input" v-model="settings.auto_audit_cron" placeholder="0 9 * * *" style="font-family:'JetBrains Mono',monospace" />
                <div class="field-hint">格式：分 时 日 月 周。例：<code>0 9 * * *</code> 表示每天早上9点</div>
              </div>
            </template>

            <div class="field-row">
              <label class="field-label">最大并发巡检数</label>
              <div class="number-input-row">
                <button class="num-btn" @click="settings.max_concurrent_audit=Math.max(1,Number(settings.max_concurrent_audit)-1)">－</button>
                <input class="field-input num-input" type="number" v-model.number="settings.max_concurrent_audit" min="1" max="20" />
                <button class="num-btn" @click="settings.max_concurrent_audit=Math.min(20,Number(settings.max_concurrent_audit)+1)">＋</button>
                <span class="num-unit">个站点同时巡检</span>
              </div>
              <div class="field-hint">建议不超过服务器 CPU 核心数，避免资源耗尽</div>
            </div>
          </div>

          <div class="save-bar">
            <button class="btn btn-primary" @click="saveSchedule" :disabled="saving">{{ saving ? '保存中...' : '💾 保存调度配置' }}</button>
          </div>
        </div>
      </el-tab-pane>

    </el-tabs>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const activeTab = ref('basic')
const saving = ref(false)
const settings = ref({})
const testing = ref({ ai: false, tg: false })
const showKey = ref({ ai: false, gsc: false, yandex: false, tg: false })

const scheduleMode = ref('daily')
const intervalHours = ref(24)
const dailyHour = ref(9)

const autoAudit = computed({
  get: () => settings.value.auto_audit_enabled === 'true',
  set: (v) => { settings.value.auto_audit_enabled = v ? 'true' : 'false' },
})

onMounted(async () => {
  try {
    const res = await api.getSettings()
    settings.value = res.data
    // Parse schedule settings
    const cron = settings.value.auto_audit_cron || '0 9 * * *'
    // Try to detect mode from cron
    const parts = cron.trim().split(/\s+/)
    if (parts.length === 5) {
      const h = parts[1]
      if (h !== '*' && parts[0] === '0') {
        scheduleMode.value = 'daily'
        dailyHour.value = parseInt(h) || 9
      } else {
        scheduleMode.value = 'cron'
      }
    }
    // interval hours
    if (settings.value.auto_audit_interval_hours) {
      intervalHours.value = parseInt(settings.value.auto_audit_interval_hours) || 24
      scheduleMode.value = 'interval'
    }
  } catch {}
})

// Sync cron expression based on mode
watch([scheduleMode, intervalHours, dailyHour], () => {
  if (scheduleMode.value === 'daily') {
    settings.value.auto_audit_cron = `0 ${dailyHour.value} * * *`
    settings.value.auto_audit_interval_hours = '0'
  } else if (scheduleMode.value === 'interval') {
    settings.value.auto_audit_cron = `0 */${intervalHours.value} * * *`
    settings.value.auto_audit_interval_hours = String(intervalHours.value)
  }
})

async function save() {
  saving.value = true
  try { await api.saveSettings(settings.value); ElMessage.success('保存成功') } catch {}
  finally { saving.value = false }
}

async function saveSchedule() {
  if (scheduleMode.value === 'daily') {
    settings.value.auto_audit_cron = `0 ${dailyHour.value} * * *`
    settings.value.auto_audit_interval_hours = '0'
  } else if (scheduleMode.value === 'interval') {
    settings.value.auto_audit_cron = `0 */${intervalHours.value} * * *`
    settings.value.auto_audit_interval_hours = String(intervalHours.value)
  }
  await save()
}

async function testAI() { testing.value.ai = true; try { const res = await api.testAI(); ElMessage.success(res.data) } catch {} finally { testing.value.ai = false } }
async function testTG() { testing.value.tg = true; try { const res = await api.testTelegram(); ElMessage.success(res.data) } catch {} finally { testing.value.tg = false } }
async function authGSC() { try { const res = await api.getGSCAuthURL(); window.open(res.data.url, '_blank') } catch {} }
async function authGA() { try { const res = await api.getGAAuthURL(); window.open(res.data.url, '_blank') } catch {} }
async function authYandex() { try { const res = await api.getYandexAuthURL(); window.open(res.data.url, '_blank') } catch {} }
</script>

<style scoped>
.settings-page { max-width: 860px; }

/* Tabs override */
:deep(.el-tabs__header) { margin-bottom: 0; }
:deep(.el-tabs__nav-wrap::after) { height: 1.5px; background: var(--border-light); }
:deep(.el-tabs__item) { font-size: 14px; font-weight: 600; padding: 0 22px; height: 46px; line-height: 46px; color: var(--ink-soft); }
:deep(.el-tabs__item.is-active) { color: var(--sun-deep) !important; font-weight: 700 !important; }
:deep(.el-tabs__active-bar) { background: var(--sun) !important; height: 2.5px; border-radius: 2px; }
:deep(.el-tabs__content) { padding: 0; }

.tab-content { padding: 22px 0; display: flex; flex-direction: column; gap: 18px; }

/* Section Cards */
.section-card {
  background: var(--surface);
  border: 1.5px solid var(--border-light);
  border-radius: var(--r);
  box-shadow: var(--s1);
  padding: 24px 28px;
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.section-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--ink);
  display: flex;
  align-items: center;
  gap: 8px;
  padding-bottom: 14px;
  border-bottom: 1.5px solid var(--border-light);
}
.section-desc {
  font-size: 13px;
  color: var(--ink-soft);
  line-height: 1.6;
  margin-top: -8px;
}

/* Platform icons */
.platform-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px; height: 22px;
  background: #4285F4;
  color: #fff;
  font-size: 11px;
  font-weight: 900;
  border-radius: 5px;
}

/* Fields */
.field-row { display: flex; flex-direction: column; gap: 6px; }
.field-label { font-size: 13px; font-weight: 600; color: var(--ink-mid); }
.field-input {
  width: 100%;
  padding: 10px 14px;
  border: 1.5px solid var(--border);
  border-radius: var(--r-sm);
  font-size: 14px;
  font-family: inherit;
  background: var(--surface);
  color: var(--ink);
  transition: border-color .18s, box-shadow .18s;
  outline: none;
}
.field-input:focus { border-color: var(--sun); box-shadow: 0 0 0 3px rgba(249,115,22,.12); background: #FFFDF9; }
.field-hint { font-size: 12px; color: var(--ink-hint); }
.field-hint code { background: var(--surface-3); padding: 1px 5px; border-radius: 4px; font-family: 'JetBrains Mono', monospace; font-size: 11px; color: var(--sun-deep); }

.input-with-eye { position: relative; }
.input-with-eye .field-input { padding-right: 42px; }
.eye-btn { position: absolute; right: 12px; top: 50%; transform: translateY(-50%); background: none; border: none; cursor: pointer; font-size: 16px; line-height: 1; padding: 0; }

.action-row { display: flex; gap: 10px; flex-wrap: wrap; padding-top: 4px; }

/* Buttons */
.btn { display: inline-flex; align-items: center; gap: 7px; padding: 10px 22px; border-radius: var(--r-sm); border: 1.5px solid var(--border); background: var(--surface); cursor: pointer; font-family: inherit; font-size: 13.5px; font-weight: 600; transition: all .18s; color: var(--ink-mid); }
.btn:hover { background: var(--sun-light); border-color: var(--peach); color: var(--sun-deep); transform: translateY(-1px); }
.btn-primary { background: linear-gradient(135deg,var(--sun),var(--coral)); color: #fff; border-color: transparent; box-shadow: var(--s-amber); }
.btn-primary:hover { background: linear-gradient(135deg,var(--sun-deep),var(--sun)); color: #fff; box-shadow: 0 8px 24px rgba(249,115,22,.4); transform: translateY(-1px); }
.btn-primary:disabled { opacity: .6; cursor: not-allowed; transform: none; }
.btn-test { background: var(--surface-3); border-color: var(--border); color: var(--ink-mid); }
.btn-test:hover { background: var(--sun-light); border-color: var(--sun); color: var(--sun-deep); }
.btn-test:disabled { opacity: .6; cursor: not-allowed; }
.btn-google { background: #4285F4; color: #fff; border-color: transparent; }
.btn-google:hover { background: #3367D6; color: #fff; }
.btn-yandex { background: #FF0000; color: #fff; border-color: transparent; }
.btn-yandex:hover { background: #CC0000; color: #fff; }

.save-bar { display: flex; justify-content: flex-end; padding-top: 6px; }

/* Toggle */
.toggle-wrap { padding: 4px 0; }
.toggle-label { display: inline-flex; align-items: center; gap: 9px; cursor: pointer; user-select: none; }
.toggle-switch { width: 42px; height: 24px; border-radius: 12px; position: relative; flex-shrink: 0; transition: background .2s; }
.toggle-on .toggle-switch { background: linear-gradient(135deg, #16A34A, #22C55E); box-shadow: 0 2px 8px rgba(22,163,74,.3); }
.toggle-off .toggle-switch { background: #D1C4BA; }
.toggle-switch::after { content: ''; position: absolute; top: 3px; width: 18px; height: 18px; border-radius: 50%; background: #fff; box-shadow: 0 1px 4px rgba(0,0,0,.2); transition: left .2s; }
.toggle-on .toggle-switch::after { left: 21px; }
.toggle-off .toggle-switch::after { left: 3px; }
.toggle-text { font-weight: 700; font-size: 13px; }
.toggle-on .toggle-text { color: var(--ok); }
.toggle-off .toggle-text { color: var(--ink-soft); }

/* Schedule mode cards */
.schedule-mode-row { display: flex; flex-direction: column; gap: 8px; }
.mode-cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; }
.mode-card {
  border: 2px solid var(--border-light);
  border-radius: var(--r-sm);
  padding: 16px;
  cursor: pointer;
  text-align: center;
  transition: all .18s;
  background: var(--surface-2);
}
.mode-card:hover { border-color: var(--peach); background: var(--sun-light); }
.mode-active { border-color: var(--sun) !important; background: var(--sun-light) !important; box-shadow: 0 0 0 3px rgba(249,115,22,.12); }
.mode-icon { font-size: 24px; margin-bottom: 6px; }
.mode-name { font-weight: 700; font-size: 14px; color: var(--ink); margin-bottom: 4px; }
.mode-desc { font-size: 12px; color: var(--ink-soft); }

/* Number input */
.number-input-row { display: flex; align-items: center; gap: 10px; }
.num-btn { width: 36px; height: 36px; border-radius: 8px; border: 1.5px solid var(--border); background: var(--surface); color: var(--ink); font-size: 18px; font-weight: 700; cursor: pointer; display: flex; align-items: center; justify-content: center; transition: all .15s; flex-shrink: 0; }
.num-btn:hover { background: var(--sun-light); border-color: var(--sun); color: var(--sun); }
.num-input { width: 80px !important; text-align: center; font-weight: 700; }
.num-unit { font-size: 13px; color: var(--ink-soft); white-space: nowrap; }

/* Hour selector */
.hour-selector { display: flex; flex-wrap: wrap; gap: 7px; }
.hour-chip {
  padding: 5px 10px;
  border-radius: 7px;
  border: 1.5px solid var(--border-light);
  background: var(--surface-2);
  font-size: 12px;
  font-weight: 600;
  color: var(--ink-soft);
  cursor: pointer;
  transition: all .15s;
  font-family: 'JetBrains Mono', monospace;
}
.hour-chip:hover { background: var(--sun-light); border-color: var(--peach); color: var(--sun); }
.hour-active { background: var(--sun) !important; color: #fff !important; border-color: var(--sun) !important; box-shadow: 0 2px 8px rgba(249,115,22,.3); }
</style>
