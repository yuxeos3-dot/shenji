<template>
  <div class="login-container">
    <div class="login-wrap">
      <div class="login-badge"><span class="lbdot"></span>SEO INTELLIGENCE</div>
      <h1 class="login-h1">{{ siteConfig.title }}</h1>
      <p class="login-sub">多站点SEO健康监控 · AI驱动分析 · 数据聚合</p>

      <div class="login-card">
        <!-- Login / Register tab switch. -->
        <div class="tabs">
          <button
            class="tab"
            :class="{ active: mode === 'login' }"
            @click="mode = 'login'"
          >登录</button>
          <button
            class="tab"
            :class="{ active: mode === 'register' }"
            @click="mode = 'register'"
          >注册</button>
        </div>

        <!-- ── Login form ────────────────────────────────────────── -->
        <template v-if="mode === 'login'">
          <div class="form-group">
            <label>邮箱 / 工号</label>
            <input
              v-model="loginForm.email"
              type="text"
              placeholder="请输入邮箱或工号"
              autocomplete="username"
              @keyup.enter="handleLogin"
            />
          </div>
          <div class="form-group">
            <label>密码</label>
            <input
              v-model="loginForm.password"
              type="password"
              placeholder="请输入密码"
              autocomplete="current-password"
              @keyup.enter="handleLogin"
            />
          </div>
          <button class="login-btn" @click="handleLogin" :disabled="loading">
            <span v-if="loading">登录中...</span>
            <span v-else>☀ 登录系统</span>
          </button>
        </template>

        <!-- ── Register form ─────────────────────────────────────── -->
        <template v-else>
          <div class="form-row">
            <div class="form-group">
              <label>TG名称 <span class="req">*</span></label>
              <input v-model="regForm.tg_name" placeholder="请输入TG名称" />
            </div>
            <div class="form-group">
              <label>TG号（选填）</label>
              <input v-model="regForm.tg_id" placeholder="@username" />
            </div>
          </div>

          <div class="form-row">
            <div class="form-group">
              <label>花名 <span class="req">*</span></label>
              <input v-model="regForm.nickname" placeholder="请输入花名" />
            </div>
            <div class="form-group">
              <label>工号 <span class="req">*</span></label>
              <input v-model="regForm.employee_id" placeholder="如 HF001" />
            </div>
          </div>

          <div class="form-group">
            <label>邮箱 <span class="req">*</span></label>
            <input
              v-model="regForm.email"
              type="email"
              placeholder="登录用的邮箱，请确保能收到邮件"
              autocomplete="email"
            />
          </div>

          <div class="form-group">
            <label>邀请码（选填）</label>
            <input v-model="regForm.invite_code" placeholder="没有可留空，管理员会统一审核" />
          </div>

          <div class="form-row">
            <div class="form-group">
              <label>一级标签 <span class="req">*</span></label>
              <select v-model="regForm.department_id" @change="onDeptChange">
                <option :value="null" disabled>请选择一级标签</option>
                <option v-for="d in departments" :key="d.id" :value="d.id">{{ d.name }}</option>
              </select>
            </div>
            <div class="form-group">
              <label>二级标签</label>
              <select v-model="regForm.group_id" :disabled="!regForm.department_id">
                <option :value="null">请选择二级标签</option>
                <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
              </select>
            </div>
          </div>

          <div class="form-group">
            <label>密码 <span class="req">*</span></label>
            <input
              v-model="regForm.password"
              type="password"
              placeholder="至少 8 位，包含字母和数字"
              autocomplete="new-password"
            />
          </div>
          <div class="form-group">
            <label>确认密码 <span class="req">*</span></label>
            <input
              v-model="regForm.confirm"
              type="password"
              placeholder="再次输入密码"
              autocomplete="new-password"
              @keyup.enter="handleRegister"
            />
          </div>

          <button class="login-btn" @click="handleRegister" :disabled="loading">
            <span v-if="loading">提交中...</span>
            <span v-else>📝 提交注册申请</span>
          </button>
          <div class="reg-hint">
            提交后需等待管理员审核通过，方可登录。
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, watch, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUserStore } from '../stores/user'
import { useSiteConfigStore } from '../stores/siteConfig'
import { ElMessage } from 'element-plus'
import api from '../api'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const siteConfig = useSiteConfigStore()

// Cold-load optimisation: the router guard now sends new tabs to
// /login synchronously without waiting for the auth probe, then runs
// the probe in the background. If the probe comes back as "actually
// authed" (cookie was valid), bounce to the originally requested
// route rather than make the user log in again.
watch(() => userStore.authed, (now) => {
  if (now) {
    const dest = (route.query.redirect as string) || '/dashboard'
    router.replace(dest)
  }
})

const mode = ref('login') // 'login' | 'register'
const loading = ref(false)

const loginForm = reactive({ email: '', password: '' })

const regForm = reactive({
  tg_name: '',
  tg_id: '',
  nickname: '',
  employee_id: '',
  email: '',
  invite_code: '',
  department_id: null,
  group_id: null,
  password: '',
  confirm: '',
})

const departments = ref([])
const groups = ref([])

// Lazy-load the dept list the first time the user switches to the
// register tab — we don't want to spam the public endpoints from every
// hit on the login page.
watch(mode, async (m) => {
  if (m === 'register' && departments.value.length === 0) {
    try {
      const res = await api.publicListDepartments()
      departments.value = res.data || []
    } catch (e) {
      console.error('加载一级标签失败', e)
    }
  }
})

async function onDeptChange() {
  regForm.group_id = null
  groups.value = []
  if (!regForm.department_id) return
  try {
    const res = await api.publicListGroups(regForm.department_id)
    groups.value = res.data || []
  } catch (e) {
    console.error('加载二级标签失败', e)
  }
}

async function handleLogin() {
  if (!loginForm.email.trim() || !loginForm.password) {
    ElMessage.warning('请填写邮箱/工号和密码')
    return
  }
  loading.value = true
  try {
    await userStore.login(loginForm.email.trim(), loginForm.password)
    ElMessage.success('登录成功')
    router.push('/dashboard')
  } catch (e) {
    if (!e?.response) console.error('登录失败', e)
  } finally {
    loading.value = false
  }
}

async function handleRegister() {
  // Client-side sanity checks. Backend re-validates everything; we just
  // catch the obvious so the user doesn't waste a round-trip.
  // invite_code is intentionally NOT here — admin review is the real
  // gate, so users without a code can still apply.
  const required = [
    ['tg_name', 'TG名称'],
    ['nickname', '花名'],
    ['employee_id', '工号'],
    ['email', '邮箱'],
    ['password', '密码'],
    ['confirm', '确认密码'],
  ]
  for (const [k, label] of required) {
    if (!String(regForm[k] || '').trim()) {
      ElMessage.warning('请填写' + label)
      return
    }
  }
  // Basic email shape — backend re-validates via binding:"email", this
  // just saves a round-trip for obvious typos.
  if (!/^\S+@\S+\.\S+$/.test(regForm.email.trim())) {
    ElMessage.warning('邮箱格式不正确')
    return
  }
  if (!regForm.department_id) {
    ElMessage.warning('请选择一级标签')
    return
  }
  if (regForm.password !== regForm.confirm) {
    ElMessage.warning('两次输入的密码不一致')
    return
  }
  if (regForm.password.length < 8) {
    ElMessage.warning('密码至少 8 位')
    return
  }

  loading.value = true
  try {
    const res = await api.register({
      tg_name: regForm.tg_name.trim(),
      tg_id: regForm.tg_id.trim(),
      nickname: regForm.nickname.trim(),
      employee_id: regForm.employee_id.trim(),
      email: regForm.email.trim(),
      invite_code: regForm.invite_code.trim(),
      department_id: regForm.department_id,
      group_id: regForm.group_id || 0,
      password: regForm.password,
      confirm: regForm.confirm,
    })
    ElMessage.success(res.data?.message || '注册申请已提交，请等待管理员审核')
    // Reset + flip back to login so the user sees the right tab when
    // they come back after approval.
    Object.assign(regForm, {
      tg_name: '', tg_id: '', nickname: '', employee_id: '', email: '',
      invite_code: '', department_id: null, group_id: null,
      password: '', confirm: '',
    })
    mode.value = 'login'
  } catch (e) {
    if (!e?.response) console.error('注册失败', e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (!siteConfig.title) siteConfig.fetch?.()
  // Belt + braces with the watcher above: if the auth probe already
  // resolved to authed=true before this component finished mounting
  // (rare but possible on a cached cookie), the watch won't fire for
  // the initial value. Check once on mount too.
  if (userStore.authed) {
    const dest = (route.query.redirect as string) || '/dashboard'
    router.replace(dest)
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
.login-wrap { width: 480px; max-width: 92vw; text-align: center; }
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
.tabs { display: flex; margin: -8px -8px 18px; border-bottom: 1.5px solid var(--border-light); }
.tab {
  flex: 1; padding: 12px 0;
  background: none; border: none; cursor: pointer;
  font-family: inherit; font-size: 15px; font-weight: 700;
  color: var(--ink-soft);
  border-bottom: 2.5px solid transparent;
  margin-bottom: -1.5px;
  transition: all .15s;
}
.tab:hover { color: var(--ink-mid); }
.tab.active { color: var(--sun-deep); border-bottom-color: var(--sun); }

.form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.form-group { margin-bottom: 14px; }
.form-group label {
  display: block; margin-bottom: 6px;
  font-weight: 700; color: var(--ink-mid); font-size: 12.5px;
}
.req { color: var(--err, #ef4444); margin-left: 2px; }
.form-group input,
.form-group select {
  width: 100%; padding: 9px 12px;
  border: 1.5px solid var(--border); border-radius: 8px;
  font-size: 14px; font-family: inherit;
  background: var(--surface); color: var(--ink);
  transition: border-color .18s, box-shadow .18s;
  outline: none;
  box-sizing: border-box;
}
.form-group input:focus,
.form-group select:focus {
  border-color: var(--sun);
  box-shadow: 0 0 0 3px rgba(249,115,22,.14);
  background: #FFFDF9;
}
.form-group select:disabled { background: var(--surface-2); color: var(--ink-soft); cursor: not-allowed; }

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

.reg-hint {
  margin-top: 12px;
  font-size: 12.5px; color: var(--ink-soft);
  text-align: center; line-height: 1.5;
}
</style>
