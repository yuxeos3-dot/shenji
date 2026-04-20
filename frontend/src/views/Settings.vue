<template>
  <div class="settings-page">
    <div class="hdr">
      <h1>⚙️ 设置</h1>
      <p class="hdr-sub">
        管理你的平台凭据和系统配置。个人凭据（API Key、OAuth、Telegram 等）只有你自己能看到和使用。
      </p>
    </div>

    <el-tabs v-model="activeTab" type="border-card">

      <!-- Per-user credential tabs (visible to ALL users) -->

      <!-- ─── AI ─── -->
      <el-tab-pane label="🤖 AI 接口" name="ai">
        <div class="tab-body">
          <p class="tab-desc">每个用户独立配置自己的 AI Key，最多 3 组。第一组不可用时自动切到第二组。</p>
          <div v-for="(slot, idx) in aiSlots" :key="idx" class="slot-card">
            <div class="slot-head">
              API {{ idx + 1 }}
              <span v-if="idx === 0" class="tag-primary">主用</span>
              <span v-else class="tag-muted">备用</span>
            </div>
            <div class="field-row">
              <label>接口地址</label>
              <el-input v-model="slot.api_url" placeholder="https://api.openai.com/v1" />
            </div>
            <div class="field-row">
              <label>API Key</label>
              <el-input v-model="slot.api_key" type="password" autocomplete="new-password" placeholder="sk-..." />
            </div>
            <div class="field-row">
              <label>模型</label>
              <el-input v-model="slot.model" placeholder="claude-sonnet-4-6 / gpt-4o" />
            </div>
          </div>
          <div class="row-actions">
            <el-button @click="testAI" :loading="testing.ai">🔌 测试连通性</el-button>
            <el-button type="primary" @click="saveAI" :loading="saving.ai">💾 保存</el-button>
          </div>
          <div v-if="aiTestResults.length" class="test-results">
            <div v-for="r in aiTestResults" :key="r.slot" :class="['test-item', r.status === 'ok' ? 'ok' : 'fail']">
              API {{ r.slot }}: {{ r.status === 'ok' ? '✅ 连通 (' + r.model + ')' : r.status === 'skip' ? '⏭ 未填写' : '❌ ' + r.error }}
            </div>
          </div>
        </div>
      </el-tab-pane>

      <!-- ─── GSC (Google Search Console) ─── -->
      <el-tab-pane label="🟦 Google Search Console" name="gsc">
        <div class="tab-body">
          <p class="tab-desc">
            在 <a href="https://console.cloud.google.com/apis/credentials" target="_blank" rel="noopener">Google Cloud Console</a>
            创建 OAuth 2.0 Client ID（Web application），把 Redirect URI 填到下面。
          </p>
          <OAuthConfigForm
            service="gsc"
            :cfg="oauthCfg.gsc"
            :saving="saving.gsc"
            @save="(d) => saveOAuth('gsc', d)"
            @authorize="authorize('gsc')"
            @unbind="unbindToken('gsc')"
          />
        </div>
      </el-tab-pane>

      <!-- ─── GA (Google Analytics) ─── -->
      <el-tab-pane label="🟩 Google Analytics" name="ga">
        <div class="tab-body">
          <p class="tab-desc">
            可以复用 GSC 那套 OAuth 应用的 Client ID / Secret（如果启用了 Analytics API），Redirect URI 需要单独配一个指向 /api/v1/oauth/ga/callback。
          </p>
          <OAuthConfigForm
            service="ga"
            :cfg="oauthCfg.ga"
            :saving="saving.ga"
            @save="(d) => saveOAuth('ga', d)"
            @authorize="authorize('ga')"
            @unbind="unbindToken('ga')"
          />
        </div>
      </el-tab-pane>

      <!-- ─── Yandex ─── -->
      <el-tab-pane label="🟥 Yandex Webmaster" name="yandex">
        <div class="tab-body">
          <p class="tab-desc">
            在 <a href="https://oauth.yandex.com/" target="_blank" rel="noopener">Yandex OAuth</a> 创建应用，勾选 Webmaster 只读权限。
          </p>
          <OAuthConfigForm
            service="yandex"
            :cfg="oauthCfg.yandex"
            :saving="saving.yandex"
            @save="(d) => saveOAuth('yandex', d)"
            @authorize="authorize('yandex')"
            @unbind="unbindToken('yandex')"
          />
        </div>
      </el-tab-pane>

      <!-- ─── Bing ─── -->
      <el-tab-pane label="🟦 Bing Webmaster" name="bing">
        <div class="tab-body">
          <p class="tab-desc">
            在 <a href="https://www.bing.com/webmasters/home" target="_blank" rel="noopener">Bing Webmaster Tools</a>
            → 设置 → API 访问里生成 API Key。
          </p>
          <div class="field-row">
            <label>Bing API Key</label>
            <el-input v-model="bingForm.api_key" type="password" autocomplete="new-password"
              :placeholder="bing.bound ? '●●●●●● (已配置，留空不改动)' : '粘贴 API Key'" />
          </div>
          <div class="row-actions">
            <el-button type="primary" @click="saveBing" :loading="saving.bing">💾 保存</el-button>
          </div>
          <p v-if="bing.bound" class="state-ok">✅ 已配置</p>
        </div>
      </el-tab-pane>

      <!-- ─── Telegram ─── -->
      <el-tab-pane label="📱 Telegram" name="telegram">
        <div class="tab-body">
          <p class="tab-desc">
            去 Telegram 找 @BotFather 创建自己的机器人，拿到 Bot Token；去 @userinfobot 查自己的 Chat ID。
            审计完成、汇报提交等通知只会发到你自己的机器人。
          </p>
          <div class="field-row">
            <label>Bot Token</label>
            <el-input v-model="tgForm.bot_token" type="password" autocomplete="new-password"
              :placeholder="tg.bound ? '●●●●●● (已配置，留空不改动)' : '123456:ABC...'" />
          </div>
          <div class="field-row">
            <label>Chat ID</label>
            <el-input v-model="tgForm.chat_id" placeholder="-100xxxxxxxxxx 或个人 ID" />
          </div>
          <div class="row-actions">
            <el-button @click="testTG" :loading="testing.tg">📨 发送测试消息</el-button>
            <el-button type="primary" @click="saveTG" :loading="saving.tg">💾 保存</el-button>
          </div>
          <p v-if="tg.bound" class="state-ok">✅ 已配置</p>
        </div>
      </el-tab-pane>

      <!-- ─── FRW 图/视频生成 ─── -->
      <el-tab-pane label="🎨 FRW (AI 图片 / 视频)" name="frw">
        <div class="tab-body">
          <p class="tab-desc">
            粘贴从 FRW 平台管理员获取的对接 KEY 即可，系统会自动识别 user_id 并复用为 Bearer token。
          </p>
          <div class="field-row">
            <label>FRW API Key</label>
            <el-input v-model="frwForm.apikey" type="password" autocomplete="new-password"
              :placeholder="frw.bound ? '●●●●●● (已配置)' : '粘贴 FRW 对接 KEY (JWT 格式)'" />
          </div>
          <div class="row-actions">
            <el-button type="primary" @click="saveFRW" :loading="saving.frw">💾 保存</el-button>
          </div>
          <p v-if="frw.bound" class="state-ok">✅ 已配置</p>
        </div>
      </el-tab-pane>
    

      <!-- Admin-only tabs -->


      <!-- ── 基础配置 ── -->
      <el-tab-pane v-if="isAdmin" label="🏠 基础配置" name="basic">
        <div class="tab-content">
          <div class="section-card">
            <div class="section-title">系统基本设置</div>
            <div class="field-row">
              <label class="field-label">系统标题</label>
              <input class="field-input" v-model="adminSettings.site_title" placeholder="SEO巡站管理系统" />
              <div class="field-hint">显示在浏览器标签和页面顶部</div>
            </div>
          </div>
          <div class="save-bar">
            <button class="btn btn-primary" @click="saveAdmin" :disabled="adminSaving">{{ adminSaving ? '保存中...' : '💾 保存配置' }}</button>
          </div>
        </div>
      </el-tab-pane>


      <!-- ── 安全设置 ── -->
      <el-tab-pane v-if="isAdmin" label="🔒 安全设置" name="security">
        <div class="tab-content">
          <div class="section-card">
            <div class="section-title">IP 白名单</div>
            <div class="section-desc">
              只有白名单中的 IP 才能访问系统。留空则不限制（允许所有 IP）。每行填写一个 IP 地址。
            </div>
            <div class="field-row">
              <label class="field-label">允许访问的 IP 地址</label>
              <textarea
                class="field-input ip-textarea"
                v-model="adminSettings.ip_whitelist"
                placeholder="每行一个 IP 地址，例如：&#10;192.168.1.100&#10;10.0.0.1&#10;留空则不限制访问"
                rows="8"
              ></textarea>
              <div class="field-hint">
                当前客户端 IP：<code>{{ clientIP || '获取中...' }}</code>
                　·　请确保将自己的 IP 加入白名单，否则保存后将无法访问系统
              </div>
            </div>
          </div>
          <div class="save-bar">
            <button class="btn btn-primary" @click="saveAdmin" :disabled="adminSaving">{{ adminSaving ? '保存中...' : '💾 保存配置' }}</button>
          </div>
        </div>
      </el-tab-pane>

      <!-- ── 巡检调度 ── -->
      <el-tab-pane v-if="isAdmin" label="⏰ 全局调度" name="schedule">
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
                <input class="field-input" v-model="adminSettings.auto_audit_cron" placeholder="0 9 * * *" style="font-family:'JetBrains Mono',monospace" />
                <div class="field-hint">格式：分 时 日 月 周。例：<code>0 9 * * *</code> 表示每天早上9点</div>
              </div>
            </template>

            <div class="field-row">
              <label class="field-label">最大并发巡检数</label>
              <div class="number-input-row">
                <button class="num-btn" @click="adminSettings.max_concurrent_audit=Math.max(1,Number(adminSettings.max_concurrent_audit)-1)">－</button>
                <input class="field-input num-input" type="number" v-model.number="adminSettings.max_concurrent_audit" min="1" max="20" />
                <button class="num-btn" @click="adminSettings.max_concurrent_audit=Math.min(20,Number(adminSettings.max_concurrent_audit)+1)">＋</button>
                <span class="num-unit">个站点同时巡检</span>
              </div>
              <div class="field-hint">建议不超过服务器 CPU 核心数，避免资源耗尽</div>
            </div>
          </div>

          <div class="save-bar">
            <button class="btn btn-primary" @click="saveSchedule" :disabled="adminSaving">{{ adminSaving ? '保存中...' : '💾 保存调度配置' }}</button>
          </div>
        </div>
      </el-tab-pane>

    
    </el-tabs>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, h } from 'vue'
import { ElMessage, ElInput, ElButton } from 'element-plus'
import api from '../api'
import { useUserStore } from '../stores/user'
import { useSiteConfigStore } from '../stores/siteConfig'

const userStore = useUserStore()
const siteConfig = useSiteConfigStore()
const isAdmin = computed(() => userStore.isAdmin())


// ─── 子组件：OAuth 表单（GSC / GA / Yandex 通用）─────────────────────
const OAuthConfigForm = {
  props: ['service', 'cfg', 'saving'],
  emits: ['save', 'authorize', 'unbind'],
  setup(props, { emit }) {
    const form = ref({ client_id: '', client_secret: '', redirect_uri: '' })
    // reset() re-syncs the editable form state from the latest props.cfg.
    // The bug we hit: setup() runs before loadOAuth() returns (it's
    // async, fired in parallel from the parent's onMounted). The initial
    // reset() call below picks up an empty cfg and populates form with
    // "", and because this form state is local the values never sync
    // back up when cfg later arrives. User saw an empty Client ID +
    // Redirect URI even after saving them, while the Secret placeholder
    // (which reads props.cfg directly, not form) correctly said
    // "已配置". Fix: watch the cfg prop and re-seed on every change.
    const reset = () => {
      form.value = {
        client_id: props.cfg?.client_id || '',
        client_secret: '',
        redirect_uri: props.cfg?.redirect_uri || '',
      }
    }
    reset()
    watch(
      () => props.cfg,
      (now) => {
        if (!now) return
        // Don't wipe a client_secret the user has started typing — only
        // re-sync the plain fields. Secret stays blank-to-preserve per
        // the existing UX contract.
        form.value.client_id = now.client_id || ''
        form.value.redirect_uri = now.redirect_uri || ''
      },
      { deep: true }
    )
    return () => h('div', { class: 'oauth-form' }, [
      h('div', { class: 'field-row' }, [
        h('label', null, 'Client ID'),
        h(ElInput, {
          modelValue: form.value.client_id,
          'onUpdate:modelValue': (v) => (form.value.client_id = v),
          placeholder: 'xxxx.apps.googleusercontent.com',
        }),
      ]),
      h('div', { class: 'field-row' }, [
        h('label', null, 'Client Secret'),
        h(ElInput, {
          modelValue: form.value.client_secret,
          'onUpdate:modelValue': (v) => (form.value.client_secret = v),
          type: 'password',
          autocomplete: 'new-password',
          placeholder: props.cfg?.client_secret ? '●●●●●● (已配置，留空不改动)' : '',
        }),
      ]),
      h('div', { class: 'field-row' }, [
        h('label', null, 'Redirect URI'),
        h(ElInput, {
          modelValue: form.value.redirect_uri,
          'onUpdate:modelValue': (v) => (form.value.redirect_uri = v),
          placeholder: `https://yourdomain.com/api/v1/oauth/${props.service}/callback`,
        }),
      ]),
      h('div', { class: 'row-actions' }, [
        h(ElButton, { onClick: () => emit('save', { ...form.value, service: props.service }), type: 'primary', loading: props.saving }, () => '💾 保存应用凭据'),
        h(ElButton, { onClick: () => emit('authorize'), disabled: !props.cfg?.client_id }, () => props.cfg?.bound ? '🔄 重新授权' : '🔐 发起授权'),
        props.cfg?.bound
          ? h(ElButton, { onClick: () => emit('unbind'), type: 'danger', plain: true }, () => '🗑 解绑')
          : null,
      ]),
      props.cfg?.bound
        ? h('p', { class: 'state-ok' }, `✅ 已绑定${props.cfg.expires_at ? '（Token 到期：' + props.cfg.expires_at.slice(0, 16) + '）' : ''}`)
        : h('p', { class: 'state-warn' }, '⚠ 未绑定 — 先保存应用凭据，然后点「发起授权」'),
    ])
  },
}

// ─── 状态 ────────────────────────────────────────────────────
const activeTab = ref('ai')

// AI
const aiSlots = ref([
  { api_url: '', api_key: '', model: '' },
  { api_url: '', api_key: '', model: '' },
  { api_url: '', api_key: '', model: '' },
])
const aiTestResults = ref([])

// OAuth configs keyed by service
const oauthCfg = ref({
  gsc: { client_id: '', client_secret: '', redirect_uri: '', bound: false },
  ga: { client_id: '', client_secret: '', redirect_uri: '', bound: false },
  yandex: { client_id: '', client_secret: '', redirect_uri: '', bound: false },
})

// Telegram
const tg = ref({ bound: false })
const tgForm = ref({ bot_token: '', chat_id: '' })

// Bing
const bing = ref({ bound: false })
const bingForm = ref({ api_key: '' })

// FRW
const frw = ref({ bound: false })
const frwForm = ref({ apikey: '' })

// Loading flags
const saving = ref({ ai: false, gsc: false, ga: false, yandex: false, tg: false, bing: false, frw: false })
const testing = ref({ ai: false, tg: false })

// ─── 加载 ────────────────────────────────────────────────────


// --- Admin settings state ---
const adminSaving = ref(false)
const adminSettings = ref({})
const clientIP = ref('')

const scheduleMode = ref('daily')
const intervalHours = ref(24)
const dailyHour = ref(9)

const autoAudit = computed({
  get: () => adminSettings.value.auto_audit_enabled === 'true',
  set: (v) => { adminSettings.value.auto_audit_enabled = v ? 'true' : 'false' },
})

// --- Loading ---
onMounted(async () => {
  // Personal credential data - always load
  await Promise.all([loadAI(), loadOAuth(), loadTG(), loadBing(), loadFRW()])

  // Admin settings - only load when user is admin
  if (isAdmin.value) {
    await loadAdminSettings()
  }
})
async function loadAI() {
  try {
    const res = await api.getMyAIConfig()
    const slots = res.data?.slots || []
    for (let i = 0; i < 3; i++) {
      aiSlots.value[i] = slots[i] ? { ...slots[i] } : { api_url: '', api_key: '', model: '' }
    }
  } catch (e) { console.error(e) }
}

async function loadOAuth() {
  try {
    const res = await api.getMyOAuthConfigs()
    for (const cfg of (res.data || [])) {
      if (oauthCfg.value[cfg.service]) {
        oauthCfg.value[cfg.service] = cfg
      }
    }
  } catch (e) { console.error(e) }
}

async function loadTG() {
  try {
    const res = await api.getMyTelegram()
    tg.value = res.data
    tgForm.value.chat_id = res.data.chat_id || ''
  } catch (e) { console.error(e) }
}

async function loadBing() {
  try {
    const res = await api.getMyBing()
    bing.value = res.data
  } catch (e) { console.error(e) }
}

async function loadFRW() {
  try {
    const apiKeyRes = await api.getMyFrwApiKey()
    frw.value.bound = !!apiKeyRes.data?.bound
  } catch (e) { console.error(e) }
}

// ─── 保存 ────────────────────────────────────────────────────
async function saveAI() {
  saving.value.ai = true
  try {
    await api.saveMyAIConfig({ slots: aiSlots.value })
    ElMessage.success('已保存')
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  } finally {
    saving.value.ai = false
  }
}

async function testAI() {
  testing.value.ai = true
  aiTestResults.value = []
  try {
    const res = await api.testMyAI()
    aiTestResults.value = res.data?.data || res.data || []
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '测试失败')
  } finally {
    testing.value.ai = false
  }
}

async function saveOAuth(service, data) {
  saving.value[service] = true
  try {
    await api.saveMyOAuthConfig(data)
    ElMessage.success('已保存 ' + service.toUpperCase() + ' 应用凭据')
    await loadOAuth()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  } finally {
    saving.value[service] = false
  }
}

async function authorize(service) {
  try {
    let res
    if (service === 'gsc') res = await api.getGSCAuthURL()
    else if (service === 'ga') res = await api.getGAAuthURL()
    else if (service === 'yandex') res = await api.getYandexAuthURL()
    if (res?.data?.url) window.location.href = res.data.url
    else ElMessage.error('授权 URL 为空')
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '授权发起失败')
  }
}

async function unbindToken(service) {
  try {
    await api.unbindMyOAuthToken(service)
    ElMessage.success('已解绑')
    await loadOAuth()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '解绑失败')
  }
}

async function saveTG() {
  saving.value.tg = true
  try {
    await api.saveMyTelegram(tgForm.value)
    ElMessage.success('已保存')
    tgForm.value.bot_token = ''
    await loadTG()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  } finally {
    saving.value.tg = false
  }
}

async function testTG() {
  testing.value.tg = true
  try {
    const res = await api.testMyTelegram()
    ElMessage.success(res.data || '已发送')
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '测试失败')
  } finally {
    testing.value.tg = false
  }
}

async function saveBing() {
  saving.value.bing = true
  try {
    await api.saveMyBing(bingForm.value)
    ElMessage.success('已保存')
    bingForm.value.api_key = ''
    await loadBing()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  } finally {
    saving.value.bing = false
  }
}

async function saveFRW() {
  if (!frwForm.value.apikey) {
    ElMessage.warning('请先粘贴 FRW API Key')
    return
  }
  saving.value.frw = true
  try {
    await api.saveMyFrwApiKey({ api_key: frwForm.value.apikey })
    ElMessage.success('已保存')
    frwForm.value.apikey = ''
    await loadFRW()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  } finally {
    saving.value.frw = false
  }
}


// --- Admin settings loader ---

async function loadAdminSettings() {
  try {
    const res = await api.getSettings()
    adminSettings.value = res.data
    // Parse schedule settings
    const cron = adminSettings.value.auto_audit_cron || '0 9 * * *'
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
    if (adminSettings.value.auto_audit_interval_hours) {
      intervalHours.value = parseInt(adminSettings.value.auto_audit_interval_hours) || 24
      scheduleMode.value = 'interval'
    }
  } catch (e) { console.error(e) }
  // Load client IP for whitelist hint
  try {
    const ipRes = await api.getMyIP()
    clientIP.value = ipRes.data || ''
  } catch (e) { console.error(e) }
}

// --- Admin settings savers ---

// Sync cron expression based on mode
watch([scheduleMode, intervalHours, dailyHour], () => {
  if (scheduleMode.value === 'daily') {
    adminSettings.value.auto_audit_cron = `0 ${dailyHour.value} * * *`
    adminSettings.value.auto_audit_interval_hours = '0'
  } else if (scheduleMode.value === 'interval') {
    adminSettings.value.auto_audit_cron = `0 */${intervalHours.value} * * *`
    adminSettings.value.auto_audit_interval_hours = String(intervalHours.value)
  }
})

async function saveAdmin() {
  adminSaving.value = true
  try {
    await api.saveSettings(adminSettings.value)
    siteConfig.refresh()
    ElMessage.success('保存成功')
  } catch (e) { console.error(e) }
  finally { adminSaving.value = false }
}

async function saveSchedule() {
  if (scheduleMode.value === 'daily') {
    adminSettings.value.auto_audit_cron = `0 ${dailyHour.value} * * *`
    adminSettings.value.auto_audit_interval_hours = '0'
  } else if (scheduleMode.value === 'interval') {
    adminSettings.value.auto_audit_cron = `0 */${intervalHours.value} * * *`
    adminSettings.value.auto_audit_interval_hours = String(intervalHours.value)
  }
  await saveAdmin()
}
</script>

<style scoped>
.personal-center { max-width: 860px; }
.hdr { margin-bottom: 14px; }
.hdr h1 { font-size: 18px; font-weight: 800; color: var(--ink); }
.hdr-sub { font-size: 13px; color: var(--ink-soft); line-height: 1.7; margin-top: 6px; }

.tab-body { padding: 6px 0; display: flex; flex-direction: column; gap: 14px; }
.tab-desc { font-size: 13px; color: var(--ink-soft); line-height: 1.7; }
.tab-desc a { color: var(--sun-deep); font-weight: 600; }

.slot-card {
  background: var(--surface-3);
  border: 1.5px solid var(--border-light);
  border-radius: 10px;
  padding: 14px 18px;
  display: flex; flex-direction: column; gap: 8px;
}
.slot-head { font-size: 13px; font-weight: 800; color: var(--ink); display: flex; align-items: center; gap: 8px; }
.tag-primary { font-size: 10px; font-weight: 700; padding: 2px 8px; border-radius: 4px; background: var(--sun); color: #fff; }
.tag-muted { font-size: 10px; font-weight: 700; padding: 2px 8px; border-radius: 4px; background: var(--surface); color: var(--ink-hint); border: 1px solid var(--border); }

.field-row { display: flex; flex-direction: column; gap: 4px; }
.field-row label { font-size: 12px; font-weight: 700; color: var(--ink-mid); }

.row-actions { display: flex; gap: 8px; margin-top: 6px; }

.state-ok { font-size: 13px; color: var(--ok); font-weight: 700; margin-top: 4px; }
.state-warn { font-size: 13px; color: var(--warn); font-weight: 700; margin-top: 4px; }

.test-results { margin-top: 8px; display: flex; flex-direction: column; gap: 4px; }
.test-item { padding: 6px 10px; border-radius: 6px; font-size: 12px; font-weight: 600; }
.test-item.ok { background: var(--ok-bg); color: var(--ok); border: 1px solid var(--ok-border); }
.test-item.fail { background: var(--err-bg); color: var(--err); border: 1px solid var(--err-border); }

:deep(.oauth-form) { display: flex; flex-direction: column; gap: 12px; }


/* --- Admin tab styles (from old Settings) --- */

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
.ai-slot-card { border:1.5px solid #E5E7EB; border-radius:10px; padding:14px 16px; margin-bottom:12px; background:#FAFBFC; }
.ai-slot-head { font-weight:700; font-size:13px; color:#4338CA; margin-bottom:10px; display:flex; align-items:center; gap:8px; }
.ai-slot-tag { font-size:11px; font-weight:600; padding:2px 8px; border-radius:4px; background:#EEF2FF; color:#4338CA; }
.ai-slot-tag.backup { background:#FEF3C7; color:#92400E; }
.ai-test-results { margin-top:12px; }
.ai-test-item { padding:6px 12px; border-radius:6px; margin-bottom:4px; font-size:13px; font-weight:500; }
.test-ok { background:#F0FDF4; color:#166534; }
.test-fail { background:#FEF2F2; color:#991B1B; }
.ip-textarea { resize:vertical; font-family:'JetBrains Mono',monospace; font-size:13px; line-height:1.7; min-height:120px; }

</style>
