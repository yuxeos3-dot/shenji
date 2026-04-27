<template>
  <div class="app-layout">
    <nav class="sidebar" :class="{ open: sidebarOpen }">
      <div class="logo">
        <div class="logo-main">☀ {{ siteConfig.title }}</div>
        <div class="logo-sub">管理后台 · <span class="logo-ver">{{ appVersion }}</span></div>
      </div>
      <ul class="nav-list">
        <li v-for="item in menuItems" :key="item.path">
          <router-link :to="item.path" :class="{active: isActive(item)}">
            <span class="nav-icon">{{ item.icon }}</span>{{ item.label }}
          </router-link>
        </li>
      </ul>
      <div class="sidebar-footer">
        <a href="#" @click.prevent="handleLogout">🚪 退出登录</a>
      </div>
    </nav>
    <div v-if="isMobile && sidebarOpen" class="sidebar-backdrop" @click="sidebarOpen = false" />
    <div class="content">
      <header class="page-header">
        <button v-if="isMobile" class="hamburger-btn" @click="sidebarOpen = !sidebarOpen">☰</button>
        <h1 id="page-title">{{ pageTitle }}</h1>
      </header>
      <div class="page-body">
        <router-view v-slot="{ Component }">
          <keep-alive include="DataManagement">
            <component :is="Component" :key="$route.fullPath" />
          </keep-alive>
        </router-view>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { useSiteConfigStore, APP_VERSION } from '../stores/siteConfig'
import { ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const siteConfig = useSiteConfigStore()
const appVersion = APP_VERSION
const isMobile = ref(window.innerWidth <= 768)
const sidebarOpen = ref(!isMobile.value)

const handleResize = () => {
  isMobile.value = window.innerWidth <= 768
  if (!isMobile.value) sidebarOpen.value = true
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
  const q = route.query
  if (q && q.oauth) {
    if (q.status === 'success') {
      ElMessage.success(`✅ ${String(q.oauth).toUpperCase()} 授权成功`)
    } else if (q.status === 'denied') {
      ElMessage.error({
        message: `❌ ${String(q.oauth).toUpperCase()} 授权失败：${q.reason || '未知原因'}`,
        duration: 10000,
        showClose: true,
      })
    }
    router.replace({ path: route.path, query: {} })
  }
})

onBeforeUnmount(() => {
  window.removeEventListener("resize", handleResize)
})

watch(() => route.path, () => {
  if (isMobile.value) sidebarOpen.value = false
})

const menuItems = [
  { path: '/sites',           icon: '🌐', label: '站点管理', activePrefix: '/sites' },
  { path: '/data-management', icon: '📡', label: '数据管理' },
  { path: '/dead-links',      icon: '💀', label: '死链管理' },
  { path: '/settings',        icon: '🛠️', label: '系统设置' },
]

function isActive(item: { path: string; activePrefix?: string }) {
  if (item.activePrefix) return route.path.startsWith(item.activePrefix)
  return route.path === item.path
}

const pageTitles: Record<string, string> = {
  '/sites': '站点管理',
  '/data-management': '数据管理',
  '/dead-links': '死链管理',
  '/settings': '系统设置',
}
const pageTitle = computed(() => {
  for (const [path, title] of Object.entries(pageTitles)) {
    if (route.path === path) return title
  }
  if (route.path.includes('/audit/')) return '审计详情'
  if (route.path.match(/\/sites\/\d+$/)) return '站点详情'
  return siteConfig.title
})

watch([pageTitle, () => siteConfig.title], ([section, _base]) => {
  siteConfig.applyDocumentTitle(section)
}, { immediate: true })

async function handleLogout() {
  await userStore.logout()
  router.push('/login')
}
</script>

<style scoped>
.app-layout { display: flex; min-height: 100vh; }

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
  padding: 22px 14px 20px;
  border-bottom: 1.5px solid var(--border-light);
  background: linear-gradient(135deg, var(--sun-light) 0%, #FFF 100%);
  text-align: center;
}
.sidebar .logo .logo-main {
  font-size: 17px;
  font-weight: 800;
  letter-spacing: -.4px;
  color: var(--sun-deep);
  line-height: 1.25;
  word-break: break-word;
}
.sidebar .logo .logo-sub {
  margin-top: 4px;
  font-size: 10.5px;
  font-weight: 600;
  color: var(--ink-hint);
  letter-spacing: .06em;
  text-transform: uppercase;
}
.sidebar .logo .logo-ver {
  color: var(--sun);
  font-family: 'JetBrains Mono', monospace;
  font-weight: 700;
}
.sidebar ul { list-style: none; padding: 10px; flex: 1; }
.sidebar ul li a {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 10px 14px;
  color: var(--ink-mid);
  font-size: 14px;
  font-weight: 600;
  border-radius: 9px;
  margin-bottom: 4px;
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
.page-body {
  flex: 1;
  padding: 28px 32px;
  overflow-y: auto;
  background: var(--surface-2);
}

.hamburger-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: 1.5px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  cursor: pointer;
  font-size: 18px;
  color: var(--ink-mid);
  transition: all 0.18s;
  flex-shrink: 0;
}
.hamburger-btn:hover {
  background: var(--sun-light);
  color: var(--sun-deep);
  border-color: var(--sun);
}
.sidebar-backdrop { display: none; }
</style>
