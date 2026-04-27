<template>
  <div class="login-container">
    <div class="login-wrap">
      <div class="login-badge"><span class="lbdot"></span>SEO INTELLIGENCE</div>
      <h1 class="login-h1">{{ siteConfig.title }}</h1>
      <p class="login-sub">站点管理 · 数据聚合</p>

      <div class="login-card">
        <div class="form-group">
          <label>访问密码</label>
          <input
            v-model="password"
            type="password"
            placeholder="请输入访问密码"
            autocomplete="current-password"
            @keyup.enter="handleLogin"
          />
        </div>
        <button class="login-btn" @click="handleLogin" :disabled="loading">
          <span v-if="loading">登录中...</span>
          <span v-else>☀ 进入系统</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUserStore } from '../stores/user'
import { useSiteConfigStore } from '../stores/siteConfig'
import { ElMessage } from 'element-plus'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const siteConfig = useSiteConfigStore()

function safeRedirect(fullPath: string): string {
  if (!fullPath || fullPath === '/') return ''
  if (!fullPath.startsWith('/')) return ''
  if (fullPath.startsWith('//')) return ''
  if (fullPath.startsWith('/\\')) return ''
  if (/^\s*(javascript|data|vbscript):/i.test(fullPath)) return ''
  return fullPath
}

watch(() => userStore.authed, (now) => {
  if (now) {
    const raw = String(route.query.redirect || '/sites')
    const dest = safeRedirect(raw)
    router.replace(dest || '/sites')
  }
})

const password = ref('')
const loading = ref(false)

async function handleLogin() {
  if (!password.value) {
    ElMessage.warning('请输入访问密码')
    return
  }
  loading.value = true
  try {
    await userStore.login(password.value)
    ElMessage.success('登录成功')
    router.push('/sites')
  } catch (e: any) {
    if (!e?.response) console.error('登录失败', e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (!siteConfig.title) siteConfig.refresh()
  if (userStore.authed) {
    const raw = String(route.query.redirect || '/sites')
    const dest = safeRedirect(raw)
    router.replace(dest || '/sites')
  }
})
</script>

<style scoped>
.login-container {
  position: fixed; inset: 0; z-index: 500;
  background: var(--surface);
  display: flex; align-items: center; justify-content: center;
  background-image: radial-gradient(ellipse 70% 50% at 50% 0%, rgba(255,140,0,.07) 0%, transparent 60%);
  overflow-y: auto;
  padding: 40px 0;
}
.login-wrap { width: 420px; max-width: 92vw; text-align: center; }
.login-badge {
  display: inline-flex; align-items: center; gap: 6px;
  font-size: 12px; font-weight: 600;
  color: var(--sun); background: var(--sun-light);
  border-radius: 20px; padding: 5px 14px; margin-bottom: 24px;
  border: 1px solid var(--peach);
}
.lbdot {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--sun);
  animation: blink 1.4s infinite;
}
@keyframes blink { 0%,100%{opacity:1} 50%{opacity:.3} }
.login-h1 {
  font-size: 36px; font-weight: 800;
  letter-spacing: -1.5px; line-height: 1.08;
  margin-bottom: 12px; color: var(--ink);
}
.login-sub { font-size: 14px; color: var(--ink-hint); line-height: 1.7; margin-bottom: 28px; }
.login-card {
  background: var(--surface);
  border: 1.5px solid var(--border-light);
  border-radius: var(--r);
  padding: 28px;
  box-shadow: var(--s2);
  text-align: left;
}
.form-group { margin-bottom: 14px; }
.form-group label {
  display: block; margin-bottom: 6px;
  font-weight: 700; color: var(--ink-mid); font-size: 12.5px;
}
.form-group input {
  width: 100%; padding: 9px 12px;
  border: 1.5px solid var(--border); border-radius: 8px;
  font-size: 14px; font-family: inherit;
  background: var(--surface); color: var(--ink);
  transition: border-color .18s, box-shadow .18s;
  outline: none;
  box-sizing: border-box;
}
.form-group input:focus {
  border-color: var(--sun);
  box-shadow: 0 0 0 3px rgba(249,115,22,.14);
  background: #FFFDF9;
}
.login-btn {
  width: 100%; padding: 13px 24px;
  background: linear-gradient(135deg, var(--sun) 0%, var(--coral) 100%);
  color: #fff; font-family: inherit;
  font-size: 15px; font-weight: 700;
  border: none; border-radius: 9px;
  cursor: pointer; transition: all .2s;
  display: flex; align-items: center; justify-content: center; gap: 10px;
  box-shadow: var(--s-amber);
  margin-top: 4px;
}
.login-btn:hover {
  background: linear-gradient(135deg, var(--sun-deep) 0%, var(--sun) 100%);
  transform: translateY(-1px);
  box-shadow: 0 8px 24px rgba(249,115,22,.48);
}
.login-btn:disabled { opacity: .7; cursor: not-allowed; transform: none; }
</style>
