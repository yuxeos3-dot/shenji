<template>
  <div class="app-layout">
    <nav class="sidebar">
      <div class="logo">☀ SEO巡站<span>管理后台</span></div>
      <ul>
        <li><router-link to="/dashboard" :class="{active: $route.path==='/dashboard'}"><span class="nav-icon">📊</span>仪表盘</router-link></li>
        <li><router-link to="/data-management" :class="{active: $route.path==='/data-management'}"><span class="nav-icon">📡</span>数据管理</router-link></li>
        <li><router-link to="/sites" :class="{active: $route.path.startsWith('/sites')}"><span class="nav-icon">🌐</span>站点管理</router-link></li>
        <li><router-link to="/geo-management" :class="{active: $route.path==='/geo-management'}"><span class="nav-icon">🤖</span>GEO管理</router-link></li>
        <li><router-link to="/dead-links" :class="{active: $route.path==='/dead-links'}"><span class="nav-icon">💀</span>死链检查</router-link></li>
        <li><router-link to="/content-rewrite" :class="{active: $route.path==='/content-rewrite'}"><span class="nav-icon">✍️</span>内容二创</router-link></li>
        <li><router-link to="/external-links" :class="{active: $route.path==='/external-links'}"><span class="nav-icon">🔗</span>外链管理</router-link></li>
        <li><router-link to="/reports" :class="{active: $route.path.startsWith('/reports')}"><span class="nav-icon">📋</span>每日汇报</router-link></li>
        <li v-if="userStore.isAdmin()"><router-link to="/users" :class="{active: $route.path==='/users'}"><span class="nav-icon">👥</span>用户管理</router-link></li>
        <li v-if="userStore.isAdmin()"><router-link to="/settings" :class="{active: $route.path==='/settings'}"><span class="nav-icon">⚙️</span>系统设置</router-link></li>
      </ul>
      <div class="sidebar-footer">
        <a href="#" @click.prevent="showAIConfig=true">🤖 我的 AI 设置</a>
        <a href="#" @click.prevent="showPasswordDialog=true">🔑 修改密码</a>
        <a href="#" @click.prevent="handleLogout">🚪 退出登录</a>
      </div>
    </nav>
    <div class="content">
      <header class="page-header">
        <h1 id="page-title">{{ pageTitle }}</h1>
        <div class="header-right">
          <span class="user-chip">{{ userStore.user?.name || '' }}</span>
        </div>
      </header>
      <div class="page-body">
        <router-view />
      </div>
    </div>
  </div>

  <el-dialog v-model="showPasswordDialog" title="修改密码" width="420px">
    <div class="form-group">
      <label>旧密码</label>
      <el-input v-model="pwdForm.old_password" type="password" show-password />
    </div>
    <div class="form-group">
      <label>新密码</label>
      <el-input v-model="pwdForm.new_password" type="password" show-password />
    </div>
    <template #footer>
      <el-button @click="showPasswordDialog=false">取消</el-button>
      <el-button type="primary" @click="changePassword">确定</el-button>
    </template>
  </el-dialog>

  <!-- AI Config Dialog -->
  <el-dialog v-model="showAIConfig" title="🤖 我的 AI 设置" width="620px">
    <div style="font-size:13px;color:#78350F;background:#FFFBEB;border:1px solid #FDE68A;border-radius:8px;padding:10px 14px;margin-bottom:16px">
      每个用户使用自己的 API Key，最多配 3 组。第一组不可用时自动切换到第二组。
    </div>
    <div v-for="(slot, idx) in aiSlots" :key="idx" style="border:1px solid #e5e7eb;border-radius:10px;padding:14px;margin-bottom:12px;background:#f9fafb">
      <div style="font-weight:700;font-size:13px;color:#4338CA;margin-bottom:10px">API {{ idx + 1 }}</div>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:8px">
        <div><div style="font-size:11px;font-weight:600;color:#6b7280;margin-bottom:4px">接口地址</div><el-input v-model="slot.api_url" placeholder="https://api.openai.com/v1" size="small" /></div>
        <div><div style="font-size:11px;font-weight:600;color:#6b7280;margin-bottom:4px">模型名称</div><el-input v-model="slot.model" placeholder="claude-sonnet-4-6" size="small" /></div>
      </div>
      <div><div style="font-size:11px;font-weight:600;color:#6b7280;margin-bottom:4px">API Key</div><el-input v-model="slot.api_key" placeholder="sk-..." type="password" show-password size="small" /></div>
    </div>
    <template #footer>
      <el-button @click="showAIConfig=false">取消</el-button>
      <el-button type="warning" @click="testAI" :loading="testingAI">测试连通性</el-button>
      <el-button type="primary" @click="saveAIConfig" :loading="savingAI">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { ElMessage } from 'element-plus'
import apiClient from '../api'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const showPasswordDialog = ref(false)
const pwdForm = ref({ old_password: '', new_password: '' })

// AI Config
const showAIConfig = ref(false)
const aiSlots = ref([{api_url:'',api_key:'',model:''},{api_url:'',api_key:'',model:''},{api_url:'',api_key:'',model:''}])
const savingAI = ref(false)
const testingAI = ref(false)

async function loadAIConfig() {
  try {
    const res = await apiClient.getMyAIConfig()
    const slots = res.data?.slots || []
    for (let i = 0; i < 3; i++) {
      aiSlots.value[i] = slots[i] || {api_url:'',api_key:'',model:''}
    }
  } catch {}
}
async function saveAIConfig() {
  savingAI.value = true
  try {
    await apiClient.saveMyAIConfig({ slots: aiSlots.value })
    ElMessage.success('AI 配置已保存')
    showAIConfig.value = false
  } catch { ElMessage.error('保存失败') }
  finally { savingAI.value = false }
}
async function testAI() {
  testingAI.value = true
  try {
    const res = await apiClient.testMyAI()
    const results = res.data || []
    const msgs = results.map(r => `API${r.slot}: ${r.status==='ok'?'✅ 连通':'❌ '+r.error}`).join('\n')
    ElMessage({ message: msgs.replace(/\n/g, '<br>'), dangerouslyUseHTMLString: true, duration: 5000, type: results.some(r=>r.status==='ok')?'success':'error' })
  } catch { ElMessage.error('测试失败') }
  finally { testingAI.value = false }
}

const pageTitles = {
  '/dashboard': '仪表盘',
  '/data-management': '数据管理',
  '/sites': '站点管理',
  '/geo-management': 'Geo管理',
  '/dead-links': '死链检查',
  '/content-rewrite': '内容二创',
  '/external-links': '外链管理',
  '/reports': '每日汇报',
  '/reports/write': '写汇报',
  '/users': '用户管理',
  '/settings': '系统设置',
}
const pageTitle = computed(() => {
  for (const [path, title] of Object.entries(pageTitles)) {
    if (route.path === path) return title
  }
  if (route.path.includes('/audit/')) return '审计详情'
  if (route.path.includes('/data')) return '数据中心'
  if (route.path.match(/\/sites\/\d+$/)) return '站点详情'
  return 'SEO巡站系统'
})

onMounted(() => { userStore.fetchUser(); loadAIConfig() })

function handleLogout() {
  userStore.logout()
  router.push('/login')
}

async function changePassword() {
  try {
    await apiClient.changePassword(pwdForm.value)
    ElMessage.success('密码修改成功')
    showPasswordDialog.value = false
    pwdForm.value = { old_password: '', new_password: '' }
  } catch {}
}
</script>

<style scoped>
.app-layout { display: flex; min-height: 100vh; }

/* ── Sidebar ── */
.sidebar {
  width: 220px;
  background: var(--surface);
  border-right: 1.5px solid var(--border-light);
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  position: sticky;
  top: 0;
  height: 100vh;
  overflow-y: auto;
  flex-shrink: 0;
  box-shadow: 2px 0 20px rgba(200,100,10,.06);
}
.sidebar .logo {
  padding: 24px 20px 22px;
  font-size: 17px;
  font-weight: 800;
  letter-spacing: -.4px;
  border-bottom: 1.5px solid var(--border-light);
  background: linear-gradient(135deg, var(--sun-light) 0%, #FFF 100%);
  color: var(--sun-deep);
}
.sidebar .logo span {
  display: block;
  font-size: 10.5px;
  font-weight: 600;
  color: var(--ink-hint);
  letter-spacing: .06em;
  text-transform: uppercase;
  margin-top: 2px;
}
.sidebar ul { list-style: none; padding: 10px; flex: 1; }
.sidebar ul li a {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 9px 12px;
  color: var(--ink-mid);
  font-size: 13.5px;
  font-weight: 600;
  border-radius: 9px;
  margin-bottom: 2px;
  transition: background .18s, color .18s, transform .18s;
  text-decoration: none;
}
.sidebar ul li a .nav-icon {
  width: 18px;
  text-align: center;
  font-size: 15px;
  flex-shrink: 0;
  opacity: .75;
}
.sidebar ul li a:hover {
  background: var(--sun-light);
  color: var(--sun-deep);
  transform: translateX(3px);
}
.sidebar ul li a:hover .nav-icon { opacity: 1; }
.sidebar ul li a.active {
  background: linear-gradient(135deg, var(--sun) 0%, var(--coral) 100%);
  color: #fff;
  box-shadow: var(--s-amber);
}
.sidebar ul li a.active .nav-icon { opacity: 1; }
.sidebar ul li a.active:hover { transform: none; }

.sidebar-footer {
  padding: 14px 12px;
  border-top: 1.5px solid var(--border-light);
  background: var(--surface-2);
}
.sidebar-footer a {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  color: var(--ink-soft);
  font-size: 13px;
  font-weight: 600;
  border-radius: 9px;
  transition: background .18s, color .18s;
  text-decoration: none;
}
.sidebar-footer a:hover { background: var(--sun-light); color: var(--sun-deep); }

/* ── Content ── */
.content {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
}
.page-header {
  background: var(--surface);
  border-bottom: 1.5px solid var(--border-light);
  padding: 18px 32px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-shadow: 0 2px 8px rgba(150,60,0,.05);
}
.page-header h1 {
  font-size: 20px;
  font-weight: 800;
  color: var(--ink);
  letter-spacing: -.3px;
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0;
}
.page-header h1::before {
  content: '';
  display: block;
  width: 4px;
  height: 22px;
  background: linear-gradient(180deg, var(--sun) 0%, var(--gold) 100%);
  border-radius: 4px;
  flex-shrink: 0;
}
.header-right { display: flex; align-items: center; gap: 12px; }
.user-chip {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-mid);
  background: var(--sun-light);
  padding: 5px 14px;
  border-radius: 20px;
  border: 1px solid var(--peach);
}
.page-body {
  flex: 1;
  padding: 28px 32px;
  overflow-y: auto;
  background: var(--surface-2);
}

.form-group { margin-bottom: 18px; }
.form-group label {
  display: block;
  margin-bottom: 7px;
  font-weight: 700;
  color: var(--ink-mid);
  font-size: 13px;
}
</style>
