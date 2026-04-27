<template>
  <div class="settings-page">
    <div class="hdr">
      <h1>⚙️ 系统设置</h1>
      <p class="hdr-sub">管理站点的全局配置、AI 接口和 4 大数据平台的 OAuth 凭据。</p>
    </div>

    <el-tabs v-model="activeTab" type="border-card">
      <el-tab-pane label="🏠 基础" name="basic">
        <div class="tab-body">
          <div class="field-row">
            <label>系统标题</label>
            <el-input v-model="adminSettings.site_title" placeholder="站点操作管理系统" />
            <div class="field-hint">显示在浏览器标签和页面顶部</div>
          </div>
          <div class="field-row">
            <label>启用自动巡检</label>
            <el-switch v-model="autoAudit" />
          </div>
          <div class="field-row" v-if="autoAudit">
            <label>每天巡检时间（小时 0-23）</label>
            <el-input-number v-model="dailyHour" :min="0" :max="23" />
          </div>
          <div class="field-row">
            <label>最大并发巡检数</label>
            <el-input-number v-model.number="adminSettings.max_concurrent_audit" :min="1" :max="20" />
          </div>
          <div class="row-actions">
            <el-button type="primary" :loading="adminSaving" @click="saveAdmin">💾 保存</el-button>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="🤖 AI 接口" name="ai">
        <div class="tab-body">
          <p class="tab-desc">最多 3 组备用配置。第一组失败时自动切到下一组。</p>
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

      <el-tab-pane label="🟦 GSC" name="gsc">
        <div class="tab-body">
          <p class="tab-desc">
            在 <a href="https://console.cloud.google.com/apis/credentials" target="_blank" rel="noopener">Google Cloud Console</a>
            创建 OAuth 2.0 Client ID（Web application）。
          </p>
          <OAuthConfigForm service="gsc" :cfg="oauthCfg.gsc" :saving="saving.gsc"
            @save="(d) => saveOAuth('gsc', d)" @authorize="authorize('gsc')" @unbind="unbindToken('gsc')" />
        </div>
      </el-tab-pane>

      <el-tab-pane label="🟩 GA4" name="ga">
        <div class="tab-body">
          <p class="tab-desc">可以复用 GSC 的 OAuth 应用（启用 Analytics API），Redirect URI 单独指向 /api/v1/oauth/ga/callback。</p>
          <OAuthConfigForm service="ga" :cfg="oauthCfg.ga" :saving="saving.ga"
            @save="(d) => saveOAuth('ga', d)" @authorize="authorize('ga')" @unbind="unbindToken('ga')" />
        </div>
      </el-tab-pane>

      <el-tab-pane label="🟥 Yandex" name="yandex">
        <div class="tab-body">
          <p class="tab-desc">在 <a href="https://oauth.yandex.com/" target="_blank" rel="noopener">Yandex OAuth</a> 创建应用，勾选 Webmaster 只读权限。</p>
          <OAuthConfigForm service="yandex" :cfg="oauthCfg.yandex" :saving="saving.yandex"
            @save="(d) => saveOAuth('yandex', d)" @authorize="authorize('yandex')" @unbind="unbindToken('yandex')" />
        </div>
      </el-tab-pane>

      <el-tab-pane label="🟦 Bing" name="bing">
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
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, h } from 'vue'
import { ElMessage, ElInput, ElButton } from 'element-plus'
import api from '../api'
import { useSiteConfigStore } from '../stores/siteConfig'

const siteConfig = useSiteConfigStore()

const OAuthConfigForm = {
  props: ['service', 'cfg', 'saving'],
  emits: ['save', 'authorize', 'unbind'],
  setup(props: any, { emit }: any) {
    const form = ref({ client_id: '', client_secret: '', redirect_uri: '' })
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
      (now: any) => {
        if (!now) return
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
          'onUpdate:modelValue': (v: string) => (form.value.client_id = v),
          placeholder: 'xxxx.apps.googleusercontent.com',
        }),
      ]),
      h('div', { class: 'field-row' }, [
        h('label', null, 'Client Secret'),
        h(ElInput, {
          modelValue: form.value.client_secret,
          'onUpdate:modelValue': (v: string) => (form.value.client_secret = v),
          type: 'password',
          autocomplete: 'new-password',
          placeholder: props.cfg?.client_secret ? '●●●●●● (已配置，留空不改动)' : '',
        }),
      ]),
      h('div', { class: 'field-row' }, [
        h('label', null, 'Redirect URI'),
        h(ElInput, {
          modelValue: form.value.redirect_uri,
          'onUpdate:modelValue': (v: string) => (form.value.redirect_uri = v),
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

const activeTab = ref('basic')

const aiSlots = ref<any[]>([
  { api_url: '', api_key: '', model: '' },
  { api_url: '', api_key: '', model: '' },
  { api_url: '', api_key: '', model: '' },
])
const aiTestResults = ref<any[]>([])

const oauthCfg = ref<any>({
  gsc: { client_id: '', client_secret: '', redirect_uri: '', bound: false },
  ga: { client_id: '', client_secret: '', redirect_uri: '', bound: false },
  yandex: { client_id: '', client_secret: '', redirect_uri: '', bound: false },
})

const bing = ref<any>({ bound: false })
const bingForm = ref({ api_key: '' })

const saving = ref<any>({ ai: false, gsc: false, ga: false, yandex: false, bing: false })
const testing = ref({ ai: false })

const adminSaving = ref(false)
const adminSettings = ref<any>({})
const dailyHour = ref(9)

const autoAudit = computed({
  get: () => adminSettings.value.auto_audit_enabled === 'true',
  set: (v: boolean) => { adminSettings.value.auto_audit_enabled = v ? 'true' : 'false' },
})

onMounted(async () => {
  await Promise.all([loadAI(), loadOAuth(), loadBing(), loadAdminSettings()])
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

async function loadBing() {
  try {
    const res = await api.getMyBing()
    bing.value = res.data
  } catch (e) { console.error(e) }
}

async function saveAI() {
  saving.value.ai = true
  try {
    await api.saveMyAIConfig({ slots: aiSlots.value })
    ElMessage.success('已保存')
  } catch (e: any) {
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
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '测试失败')
  } finally {
    testing.value.ai = false
  }
}

async function saveOAuth(service: string, data: any) {
  saving.value[service] = true
  try {
    await api.saveMyOAuthConfig(data)
    ElMessage.success('已保存 ' + service.toUpperCase() + ' 应用凭据')
    await loadOAuth()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  } finally {
    saving.value[service] = false
  }
}

async function authorize(service: string) {
  try {
    let res: any
    if (service === 'gsc') res = await api.getGSCAuthURL()
    else if (service === 'ga') res = await api.getGAAuthURL()
    else if (service === 'yandex') res = await api.getYandexAuthURL()
    if (res?.data?.url) window.location.href = res.data.url
    else ElMessage.error('授权 URL 为空')
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '授权发起失败')
  }
}

async function unbindToken(service: string) {
  try {
    await api.unbindMyOAuthToken(service)
    ElMessage.success('已解绑')
    await loadOAuth()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '解绑失败')
  }
}

async function saveBing() {
  saving.value.bing = true
  try {
    await api.saveMyBing(bingForm.value)
    ElMessage.success('已保存')
    bingForm.value.api_key = ''
    await loadBing()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  } finally {
    saving.value.bing = false
  }
}

async function loadAdminSettings() {
  try {
    const res = await api.getSettings()
    adminSettings.value = res.data || {}
    const cron = adminSettings.value.auto_audit_cron || '0 9 * * *'
    const parts = cron.trim().split(/\s+/)
    if (parts.length === 5 && parts[0] === '0') {
      dailyHour.value = parseInt(parts[1]) || 9
    }
  } catch (e) { console.error(e) }
}

async function saveAdmin() {
  adminSaving.value = true
  adminSettings.value.auto_audit_cron = `0 ${dailyHour.value} * * *`
  try {
    await api.saveSettings(adminSettings.value)
    siteConfig.refresh()
    ElMessage.success('保存成功')
  } catch (e) { console.error(e) }
  finally { adminSaving.value = false }
}
</script>

<style scoped>
.settings-page { max-width: 860px; }
.hdr { margin-bottom: 14px; }
.hdr h1 { font-size: 18px; font-weight: 800; color: var(--ink); }
.hdr-sub { font-size: 13px; color: var(--ink-soft); line-height: 1.7; margin-top: 6px; }
.tab-body { padding: 8px 4px; }
.tab-desc { font-size: 13px; color: var(--ink-soft); margin-bottom: 14px; line-height: 1.7; }
.tab-desc a { color: var(--sun-deep); }
.field-row { margin-bottom: 14px; }
.field-row label { display: block; font-size: 12.5px; font-weight: 700; color: var(--ink-mid); margin-bottom: 6px; }
.field-hint { font-size: 11.5px; color: var(--ink-soft); margin-top: 4px; }
.slot-card { background: var(--surface-2); border: 1px solid var(--border-light); border-radius: 8px; padding: 14px; margin-bottom: 12px; }
.slot-head { font-weight: 700; margin-bottom: 10px; display: flex; align-items: center; gap: 8px; }
.tag-primary { font-size: 11px; padding: 2px 8px; border-radius: 10px; background: var(--sun); color: #fff; font-weight: 700; }
.tag-muted { font-size: 11px; padding: 2px 8px; border-radius: 10px; background: var(--border-light); color: var(--ink-soft); font-weight: 700; }
.row-actions { display: flex; gap: 10px; margin-top: 14px; }
.test-results { margin-top: 12px; }
.test-item { font-size: 12.5px; padding: 6px 10px; border-radius: 6px; margin-bottom: 4px; }
.test-item.ok { background: #f0fdf4; color: #166534; }
.test-item.fail { background: #fef2f2; color: #991b1b; }
.state-ok { color: #166534; font-size: 13px; margin-top: 8px; font-weight: 600; }
.state-warn { color: #ca8a04; font-size: 13px; margin-top: 8px; font-weight: 600; }
</style>
