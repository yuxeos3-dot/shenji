<template>
  <div class="personal-center">
    <div class="hdr">
      <h1>🧑 个人中心</h1>
      <p class="hdr-sub">
        每个用户管理自己的平台凭据。这里填写的 API Key、OAuth 应用信息、Telegram 机器人
        只有你自己能用和看到（管理员只能看到绑定状态，看不到你的密钥本身）。
      </p>
    </div>

    <el-tabs v-model="activeTab" type="border-card">
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
            FRW API Key + 浏览器 Bearer Token 都由每个用户自己配置，没有全局回退。
          </p>
          <div class="field-row">
            <label>FRW API Key</label>
            <el-input v-model="frwForm.apikey" type="password" autocomplete="new-password"
              :placeholder="frw.bound ? '●●●●●● (已配置)' : '粘贴 apikey'" />
          </div>
          <div class="field-row">
            <label>Bearer Token（从 frw.aiaiartist 浏览器控制台复制，几小时会失效）</label>
            <el-input v-model="frwForm.token" type="password" autocomplete="new-password" placeholder="eyJh..." />
          </div>
          <div class="field-row">
            <label>FRW User ID</label>
            <el-input v-model="frwForm.user_id" placeholder="FRW 里的用户数字 ID" />
          </div>
          <div class="row-actions">
            <el-button type="primary" @click="saveFRW" :loading="saving.frw">💾 保存</el-button>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup>
import { ref, onMounted, h } from 'vue'
import { ElMessage, ElInput, ElButton } from 'element-plus'
import api from '../api'

// ─── 子组件：OAuth 表单（GSC / GA / Yandex 通用）─────────────────────
const OAuthConfigForm = {
  props: ['service', 'cfg', 'saving'],
  emits: ['save', 'authorize', 'unbind'],
  setup(props, { emit }) {
    const form = ref({ client_id: '', client_secret: '', redirect_uri: '' })
    const reset = () => {
      form.value = {
        client_id: props.cfg?.client_id || '',
        client_secret: '',
        redirect_uri: props.cfg?.redirect_uri || '',
      }
    }
    reset()
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
const frwForm = ref({ apikey: '', token: '', user_id: '' })

// Loading flags
const saving = ref({ ai: false, gsc: false, ga: false, yandex: false, tg: false, bing: false, frw: false })
const testing = ref({ ai: false, tg: false })

// ─── 加载 ────────────────────────────────────────────────────
onMounted(async () => {
  await Promise.all([loadAI(), loadOAuth(), loadTG(), loadBing(), loadFRW()])
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
    const cfgRes = await api.getMyFRWConfig()
    frw.value.bound = !!apiKeyRes.data?.bound || !!cfgRes.data?.has_apikey
    frwForm.value.user_id = cfgRes.data?.user_id || ''
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
    aiTestResults.value = res.data?.data || res.data?.results || []
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
  saving.value.frw = true
  try {
    // Save apikey via dedicated encrypted endpoint
    if (frwForm.value.apikey) {
      await api.saveMyFrwApiKey({ api_key: frwForm.value.apikey })
    }
    // Save bearer + user_id via the existing /frw/config endpoint
    await api.saveMyFRWConfig({
      apikey: '', // apikey already handled above; pass empty so we don't overwrite with ****
      token: frwForm.value.token,
      user_id: frwForm.value.user_id,
    })
    ElMessage.success('已保存')
    frwForm.value.apikey = ''
    frwForm.value.token = ''
    await loadFRW()
  } catch (e) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  } finally {
    saving.value.frw = false
  }
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
</style>
