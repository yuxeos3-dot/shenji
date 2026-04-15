<template>
  <div class="dm-page">
    <!-- 配置面板 -->
    <div class="config-panel panel" :class="showConfig ? 'config-open' : ''">
      <div class="config-header" @click="showConfig=!showConfig">
        <span class="config-title">⚙️ 数据源配置</span>
        <span class="config-sub">Google Search Console + Yandex Webmaster 账号级接入</span>
        <span class="config-toggle">{{ showConfig ? '▲ 收起' : '▼ 展开配置' }}</span>
      </div>

      <div v-show="showConfig" class="config-body">
        <div class="config-grid">
          <div class="config-field">
            <label class="config-label">
              Google OAuth Client ID
              <a href="https://console.cloud.google.com/apis/credentials" target="_blank" class="config-help">如何获取 →</a>
            </label>
            <input class="config-input" v-model="config.clientId" placeholder="264012835933-xxx.apps.googleusercontent.com" />
            <div class="config-hint">在 Google Cloud Console 创建 OAuth 2.0 客户端，授权来源填写本站域名</div>
          </div>
          <div class="config-field">
            <label class="config-label">
              Yandex OAuth Token
              <a href="https://oauth.yandex.com/" target="_blank" class="config-help">获取 Token →</a>
            </label>
            <input class="config-input" v-model="config.yandexToken" type="password" placeholder="y0__xxxx..." />
            <div class="config-hint">Yandex Webmaster OAuth Token，已预填默认值，如需更换请修改</div>
          </div>
        </div>
        <div class="config-actions">
          <button class="btn btn-primary btn-sm" @click="saveAndOpen">💾 保存并打开数据面板</button>
          <span class="config-note">配置保存在浏览器本地，不上传服务器</span>
        </div>
      </div>
    </div>

    <!-- 数据面板 iframe -->
    <div class="iframe-wrap" v-if="iframeSrc">
      <iframe
        :src="iframeSrc"
        class="gsc-iframe"
        ref="iframeRef"
        frameborder="0"
        allowfullscreen
      ></iframe>
    </div>

    <!-- 未配置提示 -->
    <div v-else class="empty-state panel">
      <div class="empty-icon">📡</div>
      <div class="empty-title">配置数据源，开始分析所有站点</div>
      <div class="empty-desc">点击上方「展开配置」，填写 Google Client ID 后即可一次查看所有绑定站点的 GSC 数据</div>
      <button class="btn btn-primary" @click="showConfig=true">⚙️ 立即配置</button>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const showConfig = ref(false)
const iframeSrc = ref('')
const iframeRef = ref(null)

const config = ref({
  clientId: '',
  yandexToken: 'y0__xDKxafgCBiX0z8g7tjA9RY6_2WTWgBQ78k2YJBOmtrgozXq9w',
})

onMounted(() => {
  // Load saved config
  const saved = localStorage.getItem('gsc_dashboard_config')
  if (saved) {
    try {
      const parsed = JSON.parse(saved)
      config.value = { ...config.value, ...parsed }
      // Auto-open if client ID is configured
      if (config.value.clientId) {
        buildIframeSrc()
      }
    } catch {}
  }
})

function saveAndOpen() {
  localStorage.setItem('gsc_dashboard_config', JSON.stringify(config.value))
  buildIframeSrc()
  showConfig.value = false
}

function buildIframeSrc() {
  const params = new URLSearchParams()
  if (config.value.clientId) params.set('cid', config.value.clientId)
  if (config.value.yandexToken) params.set('ytk', config.value.yandexToken)
  iframeSrc.value = `/gsc-dashboard.html?${params.toString()}`
}
</script>

<style scoped>
.dm-page { display:flex; flex-direction:column; gap:16px; height:calc(100vh - 120px); }

.panel { background:var(--surface); border-radius:var(--r); box-shadow:var(--s1); border:1.5px solid var(--border-light); }

/* 配置面板 */
.config-panel { overflow:hidden; flex-shrink:0; }
.config-header { display:flex; align-items:center; gap:12px; padding:14px 20px; cursor:pointer; user-select:none; transition:background .15s; }
.config-header:hover { background:var(--surface-2); }
.config-title { font-size:14px; font-weight:700; color:var(--ink); }
.config-sub { font-size:12.5px; color:var(--ink-soft); flex:1; }
.config-toggle { font-size:12px; color:var(--sun); font-weight:600; white-space:nowrap; }

.config-body { padding:16px 20px 20px; border-top:1.5px solid var(--border-light); }
.config-grid { display:grid; grid-template-columns:1fr 1fr; gap:16px; margin-bottom:16px; }
.config-field { display:flex; flex-direction:column; gap:6px; }
.config-label { font-size:12.5px; font-weight:700; color:var(--ink-mid); display:flex; align-items:center; justify-content:space-between; }
.config-help { font-size:11.5px; font-weight:600; color:var(--info); text-decoration:none; }
.config-help:hover { text-decoration:underline; }
.config-input { width:100%; padding:9px 12px; border:1.5px solid var(--border); border-radius:var(--r-sm); font-size:13px; font-family:inherit; background:var(--surface); color:var(--ink); outline:none; transition:border-color .18s; }
.config-input:focus { border-color:var(--sun); box-shadow:0 0 0 3px rgba(249,115,22,.12); }
.config-hint { font-size:11.5px; color:var(--ink-hint); line-height:1.5; }
.config-actions { display:flex; align-items:center; gap:14px; }
.config-note { font-size:12px; color:var(--ink-hint); }

/* iframe */
.iframe-wrap { flex:1; border-radius:var(--r); overflow:hidden; box-shadow:var(--s2); border:1.5px solid var(--border-light); min-height:600px; }
.gsc-iframe { width:100%; height:100%; border:none; display:block; }

/* empty */
.empty-state { display:flex; flex-direction:column; align-items:center; justify-content:center; gap:14px; padding:80px 40px; flex:1; }
.empty-icon { font-size:56px; }
.empty-title { font-size:18px; font-weight:700; color:var(--ink); }
.empty-desc { font-size:14px; color:var(--ink-soft); text-align:center; max-width:500px; line-height:1.7; }

.btn { display:inline-flex; align-items:center; gap:7px; padding:10px 22px; border-radius:var(--r-sm); border:1.5px solid var(--border); background:var(--surface); cursor:pointer; font-family:inherit; font-size:13.5px; font-weight:700; transition:all .18s; color:var(--ink-mid); }
.btn-primary { background:linear-gradient(135deg,var(--sun),var(--coral)); color:#fff; border-color:transparent; box-shadow:var(--s-amber); }
.btn-primary:hover { filter:brightness(1.06); transform:translateY(-1px); }
.btn-sm { padding:7px 16px; font-size:12.5px; }
</style>
