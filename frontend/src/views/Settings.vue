<template>
  <div>
    <h2 style="margin-top:0">系统设置</h2>
    <el-card v-loading="loading">
      <el-form :model="settings" label-width="180px">
        <el-divider content-position="left">基本设置</el-divider>
        <el-form-item label="系统标题"><el-input v-model="settings.site_title" /></el-form-item>

        <el-divider content-position="left">AI 配置</el-divider>
        <el-form-item label="AI API地址"><el-input v-model="settings.ai_api_url" /></el-form-item>
        <el-form-item label="AI API Key"><el-input v-model="settings.ai_api_key" type="password" show-password /></el-form-item>
        <el-form-item label="AI 模型"><el-input v-model="settings.ai_model" /></el-form-item>
        <el-form-item>
          <el-button @click="testAI" :loading="testing.ai">测试AI连接</el-button>
        </el-form-item>

        <el-divider content-position="left">Telegram 通知</el-divider>
        <el-form-item label="Bot Token"><el-input v-model="settings.telegram_bot_token" type="password" show-password /></el-form-item>
        <el-form-item label="Chat ID"><el-input v-model="settings.telegram_chat_id" /></el-form-item>
        <el-form-item>
          <el-button @click="testTelegram" :loading="testing.tg">测试推送</el-button>
        </el-form-item>

        <el-divider content-position="left">Google OAuth</el-divider>
        <el-form-item label="Client ID"><el-input v-model="settings.gsc_client_id" /></el-form-item>
        <el-form-item label="Client Secret"><el-input v-model="settings.gsc_client_secret" type="password" show-password /></el-form-item>
        <el-form-item label="Redirect URI"><el-input v-model="settings.gsc_redirect_uri" /></el-form-item>
        <el-form-item>
          <el-button @click="authGSC">授权GSC</el-button>
          <el-button @click="authGA" style="margin-left:8px">授权GA</el-button>
        </el-form-item>

        <el-divider content-position="left">Yandex OAuth</el-divider>
        <el-form-item label="Client ID"><el-input v-model="settings.yandex_client_id" /></el-form-item>
        <el-form-item label="Client Secret"><el-input v-model="settings.yandex_client_secret" type="password" show-password /></el-form-item>
        <el-form-item>
          <el-button @click="authYandex">授权Yandex</el-button>
        </el-form-item>

        <el-divider content-position="left">定时巡检</el-divider>
        <el-form-item label="启用自动巡检">
          <el-switch v-model="autoAuditEnabled" />
        </el-form-item>
        <el-form-item label="Cron表达式"><el-input v-model="settings.auto_audit_cron" placeholder="0 9 * * *" /></el-form-item>
        <el-form-item label="最大并发"><el-input v-model="settings.max_concurrent_audit" type="number" /></el-form-item>

        <el-form-item>
          <el-button type="primary" @click="save" :loading="saving">保存设置</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const loading = ref(false)
const saving = ref(false)
const settings = ref({})
const testing = ref({ ai: false, tg: false })

const autoAuditEnabled = computed({
  get: () => settings.value.auto_audit_enabled === 'true',
  set: (v) => { settings.value.auto_audit_enabled = v ? 'true' : 'false' },
})

async function loadSettings() {
  loading.value = true
  try { const res = await api.getSettings(); settings.value = res.data } catch {} finally { loading.value = false }
}

async function save() {
  saving.value = true
  try { await api.saveSettings(settings.value); ElMessage.success('保存成功') } catch {} finally { saving.value = false }
}

async function testAI() {
  testing.value.ai = true
  try { const res = await api.testAI(); ElMessage.success(res.data) } catch {} finally { testing.value.ai = false }
}

async function testTelegram() {
  testing.value.tg = true
  try { const res = await api.testTelegram(); ElMessage.success(res.data) } catch {} finally { testing.value.tg = false }
}

async function authGSC() {
  try { const res = await api.getGSCAuthURL(); window.open(res.data.url, '_blank') } catch {}
}
async function authGA() {
  try { const res = await api.getGAAuthURL(); window.open(res.data.url, '_blank') } catch {}
}
async function authYandex() {
  try { const res = await api.getYandexAuthURL(); window.open(res.data.url, '_blank') } catch {}
}

onMounted(loadSettings)
</script>
