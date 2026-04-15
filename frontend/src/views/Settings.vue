<template>
  <div>
    <div class="panel panel-amber" style="margin-bottom:18px">
      <div class="form-section">
        <h3>☀ 基本设置</h3>
        <div class="form-group"><label>系统标题</label><input v-model="settings.site_title" /></div>
      </div>
    </div>

    <div class="panel" style="margin-bottom:18px">
      <div class="form-section">
        <h3>🤖 AI 配置</h3>
        <div class="form-group"><label>API 地址</label><input v-model="settings.ai_api_url" /></div>
        <div class="form-group"><label>API Key</label><input v-model="settings.ai_api_key" type="password" /></div>
        <div class="form-group"><label>模型名称</label><input v-model="settings.ai_model" /></div>
        <button class="btn" @click="testAI" :disabled="testing.ai">{{ testing.ai ? '测试中...' : '🔌 测试AI连接' }}</button>
      </div>
    </div>

    <div class="panel" style="margin-bottom:18px">
      <div class="form-section">
        <h3>📱 Telegram 通知</h3>
        <div class="form-group"><label>Bot Token</label><input v-model="settings.telegram_bot_token" type="password" /></div>
        <div class="form-group"><label>Chat ID</label><input v-model="settings.telegram_chat_id" /></div>
        <button class="btn" @click="testTG" :disabled="testing.tg">{{ testing.tg ? '测试中...' : '📨 测试推送' }}</button>
      </div>
    </div>

    <div class="panel" style="margin-bottom:18px">
      <div class="form-section">
        <h3>🔑 Google OAuth</h3>
        <div class="form-group"><label>Client ID</label><input v-model="settings.gsc_client_id" /></div>
        <div class="form-group"><label>Client Secret</label><input v-model="settings.gsc_client_secret" type="password" /></div>
        <div class="form-group"><label>Redirect URI</label><input v-model="settings.gsc_redirect_uri" /></div>
        <div class="form-actions">
          <button class="btn" @click="authGSC">📊 授权GSC</button>
          <button class="btn" @click="authGA">📈 授权GA</button>
        </div>
      </div>
    </div>

    <div class="panel" style="margin-bottom:18px">
      <div class="form-section">
        <h3>🔍 Yandex OAuth</h3>
        <div class="form-group"><label>Client ID</label><input v-model="settings.yandex_client_id" /></div>
        <div class="form-group"><label>Client Secret</label><input v-model="settings.yandex_client_secret" type="password" /></div>
        <button class="btn" @click="authYandex">🔗 授权Yandex</button>
      </div>
    </div>

    <div class="panel" style="margin-bottom:18px">
      <div class="form-section">
        <h3>⏰ 定时巡检</h3>
        <div class="form-group">
          <label>启用自动巡检</label>
          <label class="toggle-label" :class="autoAudit?'cache-on':'cache-off'" @click="autoAudit=!autoAudit">
            <span class="toggle-switch"></span>
            <span class="toggle-text">{{ autoAudit ? '已开启' : '已关闭' }}</span>
          </label>
        </div>
        <div class="form-group"><label>Cron 表达式</label><input v-model="settings.auto_audit_cron" placeholder="0 9 * * *" /></div>
        <div class="form-group"><label>最大并发数</label><input v-model="settings.max_concurrent_audit" type="number" /></div>
      </div>
    </div>

    <div class="form-actions">
      <button class="btn btn-primary" @click="save" :disabled="saving">{{ saving ? '保存中...' : '☀ 保存所有设置' }}</button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const saving = ref(false)
const settings = ref({})
const testing = ref({ ai: false, tg: false })

const autoAudit = computed({
  get: () => settings.value.auto_audit_enabled === 'true',
  set: (v) => { settings.value.auto_audit_enabled = v ? 'true' : 'false' },
})

onMounted(async () => { try { const res = await api.getSettings(); settings.value = res.data } catch {} })

async function save() { saving.value = true; try { await api.saveSettings(settings.value); ElMessage.success('保存成功') } catch {} finally { saving.value = false } }
async function testAI() { testing.value.ai = true; try { const res = await api.testAI(); ElMessage.success(res.data) } catch {} finally { testing.value.ai = false } }
async function testTG() { testing.value.tg = true; try { const res = await api.testTelegram(); ElMessage.success(res.data) } catch {} finally { testing.value.tg = false } }
async function authGSC() { try { const res = await api.getGSCAuthURL(); window.open(res.data.url, '_blank') } catch {} }
async function authGA() { try { const res = await api.getGAAuthURL(); window.open(res.data.url, '_blank') } catch {} }
async function authYandex() { try { const res = await api.getYandexAuthURL(); window.open(res.data.url, '_blank') } catch {} }
</script>

<style scoped>
.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); padding:22px 26px; }
.panel-amber { background:linear-gradient(135deg,#FFFDF8 0%,#FFF8EC 60%,#FFF2D8 100%); border-color:var(--peach); }

.form-section h3 { font-size:15px; font-weight:700; margin-bottom:16px; display:flex; align-items:center; gap:6px; }
.form-group { margin-bottom:16px; }
.form-group label { display:block; margin-bottom:7px; font-weight:700; color:var(--ink-mid); font-size:13px; }
.form-group input {
  width:100%; padding:10px 13px; border:1.5px solid var(--border); border-radius:9px;
  font-size:14px; font-family:inherit; background:var(--surface); color:var(--ink);
  transition:border-color .18s, box-shadow .18s; outline:none;
}
.form-group input:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.14); background:#FFFDF9; }
.form-actions { display:flex; gap:10px; margin-top:12px; }

.btn { display:inline-flex; align-items:center; gap:6px; padding:9px 20px; border-radius:9px; border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn:hover { background:var(--sun-light); border-color:var(--peach); color:var(--sun-deep); transform:translateY(-1px); box-shadow:var(--s1); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { background:linear-gradient(135deg,var(--sun-deep),var(--sun)); color:#fff; box-shadow:0 8px 24px rgba(249,115,22,.48); }
.btn-primary:disabled { opacity:.6; cursor:not-allowed; transform:none; }

.toggle-label { display:inline-flex; align-items:center; gap:7px; cursor:pointer; user-select:none; font-size:12.5px; }
.toggle-switch {
  width:36px; height:20px; border-radius:10px; position:relative; flex-shrink:0;
  transition:background .18s, box-shadow .18s;
}
.cache-on .toggle-switch { background:linear-gradient(135deg,#16A34A,#22C55E); box-shadow:0 2px 8px rgba(22,163,74,.32); }
.cache-off .toggle-switch { background:#E2D4C4; }
.toggle-switch::after {
  content:''; position:absolute; top:3px; width:14px; height:14px; border-radius:50%;
  background:#fff; box-shadow:0 1px 4px rgba(0,0,0,.2); transition:left .18s;
}
.cache-on .toggle-switch::after { left:18px; }
.cache-off .toggle-switch::after { left:3px; }
.toggle-text { font-weight:700; font-size:12.5px; }
.cache-on .toggle-text { color:var(--ok); }
.cache-off .toggle-text { color:var(--ink-soft); }
</style>
