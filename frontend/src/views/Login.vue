<template>
  <div class="login-container">
    <div class="login-wrap">
      <div class="login-badge"><span class="lbdot"></span>SEO INTELLIGENCE</div>
      <h1 class="login-h1">SEO<em>巡站</em>系统</h1>
      <p class="login-sub">多站点SEO健康监控 · AI驱动分析 · 数据聚合</p>
      <div class="login-card">
        <div class="form-group">
          <label>邮箱</label>
          <input v-model="form.email" type="email" placeholder="admin@seo-system.local" @keyup.enter="handleLogin" />
        </div>
        <div class="form-group">
          <label>密码</label>
          <input v-model="form.password" type="password" placeholder="默认：Admin@123456" @keyup.enter="handleLogin" />
        </div>
        <button class="login-btn" @click="handleLogin" :disabled="loading">
          <span v-if="loading">登录中...</span>
          <span v-else>☀ 登录系统</span>
        </button>
      </div>
      <p class="login-note">Sunrise Design · Powered by Go + Vue3</p>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { ElMessage } from 'element-plus'

const router = useRouter()
const userStore = useUserStore()
const loading = ref(false)
const form = ref({ email: '', password: '' })

async function handleLogin() {
  if (!form.value.email || !form.value.password) {
    ElMessage.warning('请输入邮箱和密码')
    return
  }
  loading.value = true
  try {
    await userStore.login(form.value.email, form.value.password)
    ElMessage.success('登录成功')
    router.push('/dashboard')
  } catch (e) {
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  position: fixed; inset: 0; z-index: 500;
  background: var(--surface);
  display: flex; align-items: center; justify-content: center;
  background-image: radial-gradient(ellipse 70% 50% at 50% 0%, rgba(255,140,0,.07) 0%, transparent 60%);
}
.login-wrap { width: 440px; max-width: 92vw; text-align: center; }
.login-badge {
  display: inline-flex; align-items: center; gap: 6px;
  font-size: 12px; font-weight: 600;
  color: var(--sun); background: var(--sun-light);
  border-radius: 20px; padding: 5px 14px; margin-bottom: 28px;
  border: 1px solid var(--peach);
}
.lbdot {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--sun);
  animation: blink 1.4s infinite;
}
@keyframes blink { 0%,100%{opacity:1} 50%{opacity:.3} }
.login-h1 {
  font-size: 40px; font-weight: 800;
  letter-spacing: -1.5px; line-height: 1.08;
  margin-bottom: 14px; color: var(--ink);
}
.login-h1 em { font-style: normal; color: var(--sun); }
.login-sub { font-size: 16px; color: var(--ink-hint); line-height: 1.7; margin-bottom: 40px; }
.login-card {
  background: var(--surface);
  border: 1.5px solid var(--border-light);
  border-radius: var(--r);
  padding: 32px;
  box-shadow: var(--s2);
  text-align: left;
}
.form-group { margin-bottom: 18px; }
.form-group label {
  display: block; margin-bottom: 7px;
  font-weight: 700; color: var(--ink-mid); font-size: 13px;
}
.form-group input {
  width: 100%; padding: 10px 13px;
  border: 1.5px solid var(--border); border-radius: 9px;
  font-size: 14px; font-family: inherit;
  background: var(--surface); color: var(--ink);
  transition: border-color .18s, box-shadow .18s;
  outline: none;
}
.form-group input:focus {
  border-color: var(--sun);
  box-shadow: 0 0 0 3px rgba(249,115,22,.14);
  background: #FFFDF9;
}
.login-btn {
  width: 100%; padding: 14px 24px;
  background: linear-gradient(135deg, var(--sun) 0%, var(--coral) 100%);
  color: #fff; font-family: inherit;
  font-size: 15px; font-weight: 700;
  border: none; border-radius: 9px;
  cursor: pointer; transition: all .2s;
  display: flex; align-items: center; justify-content: center; gap: 10px;
  box-shadow: var(--s-amber);
}
.login-btn:hover {
  background: linear-gradient(135deg, var(--sun-deep) 0%, var(--sun) 100%);
  transform: translateY(-1px);
  box-shadow: 0 8px 24px rgba(249,115,22,.48);
}
.login-btn:disabled { opacity: .7; cursor: not-allowed; transform: none; }
.login-note { margin-top: 20px; font-size: 12px; color: var(--ink-hint); }
</style>
