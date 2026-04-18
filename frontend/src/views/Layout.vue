<template>
  <div class="app-layout">
    <nav class="sidebar" :class="{ open: sidebarOpen }">
      <div class="logo">
        <div class="logo-main">☀ {{ siteConfig.title }}</div>
        <div class="logo-sub">管理后台 · <span class="logo-ver">{{ appVersion }}</span></div>
      </div>
      <ul class="nav-list">
        <!-- Top-level single items go here (no group header). -->
        <template v-for="item in flatTop" :key="item.path">
          <li>
            <router-link
              :to="item.path"
              :class="{active: isActive(item)}"
            >
              <span class="nav-icon">{{ item.icon }}</span>{{ item.label }}
            </router-link>
          </li>
        </template>

        <!-- Groups -->
        <template v-for="g in visibleGroups" :key="g.key">
          <li class="nav-group-header" @click="toggleGroup(g.key)">
            <span class="grp-title">
              <span class="grp-icon">{{ g.icon }}</span>{{ g.label }}
            </span>
            <span class="grp-chev" :class="{ open: !isCollapsed(g.key) }">▸</span>
          </li>
          <ul v-show="!isCollapsed(g.key)" class="nav-sub">
            <li v-for="item in g.items" :key="item.path">
              <router-link
                :to="item.path"
                :class="{active: isActive(item)}"
              >
                <span class="nav-icon">{{ item.icon }}</span>{{ item.label }}
              </router-link>
            </li>
          </ul>
        </template>
      </ul>
      <div class="sidebar-footer">
        <a href="#" @click.prevent="showPasswordDialog=true">🔑 修改密码</a>
        <a href="#" @click.prevent="handleLogout">🚪 退出登录</a>
      </div>
    </nav>
    <div v-if="isMobile && sidebarOpen" class="sidebar-backdrop" @click="sidebarOpen = false" />
    <div class="content">
      <header class="page-header">
        <button v-if="isMobile" class="hamburger-btn" @click="sidebarOpen = !sidebarOpen">☰</button>
        <h1 id="page-title">{{ pageTitle }}</h1>
        <div class="header-right">
          <span class="user-chip">{{ userStore.user?.name || '' }}</span>
        </div>
      </header>
      <div class="page-body">
        <router-view v-slot="{ Component }">
          <keep-alive include="DataManagement">
            <component :is="Component" />
          </keep-alive>
        </router-view>
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

</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { useSiteConfigStore, APP_VERSION } from '../stores/siteConfig'
import { ElMessage } from 'element-plus'
import apiClient from '../api'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const siteConfig = useSiteConfigStore()
const appVersion = APP_VERSION
const isMobile = ref(window.innerWidth <= 768)
const sidebarOpen = ref(!isMobile.value)

onMounted(() => {
  window.addEventListener('resize', () => {
    isMobile.value = window.innerWidth <= 768
    if (!isMobile.value) sidebarOpen.value = true
  })
})

// Close sidebar on mobile route change
watch(() => route.path, () => {
  if (isMobile.value) sidebarOpen.value = false
})

const showPasswordDialog = ref(false)
const pwdForm = ref({ old_password: '', new_password: '' })

// ─── Sidebar navigation structure ────────────────────────────────────────
//
// Single source of truth. To reshape the menu:
//   - move items between groups
//   - rename a group's `label`
//   - add/remove groups
//   - move an item up to `topItems` to remove it from any group
//
// An item is active when the current route.path matches its path or
// (for items with children) starts with it — see isActive() below.
//
// Groups are collapsible. Collapse state persists in localStorage so a
// collapsed group stays collapsed across reloads. Groups whose items
// contain the current route auto-expand on first visit.

const topItems = [
  { path: '/dashboard', icon: '📊', label: '仪表盘' },
]

const groups = [
  {
    key: 'sites-data', icon: '🌐', label: '站点与数据',
    items: [
      { path: '/sites',             icon: '🌐', label: '站点管理',   activePrefix: '/sites' },
      { path: '/data-management',   icon: '📡', label: '数据管理' },
      { path: '/spider-management', icon: '🕷️', label: '蜘蛛管理' },
      { path: '/dead-links',        icon: '💀', label: '死链管理' },
      { path: '/push-management',  icon: '📤', label: '推送管理' },
    ],
  },
  {
    key: 'content', icon: '🎨', label: '内容生产',
    items: [
      { path: '/geo-management',   icon: '🤖', label: 'GEO管理' },
      { path: '/image-management', icon: '🎨', label: '图片管理' },
      { path: '/video-management', icon: '🎬', label: '视频管理' },
      { path: '/content-rewrite',  icon: '✍️', label: '二创管理' },
      { path: '/external-links',   icon: '🔗', label: '外链管理' },
    ],
  },
  {
    key: 'ops', icon: '🚀', label: '运营与协作',
    items: [
      { path: '/coordination', icon: '🤝', label: '协同管理', activePrefix: '/coordination' },
      { path: '/reports',      icon: '📋', label: '汇报管理', activePrefix: '/reports' },
    ],
  },
  {
    // 用户管理 top-level group (admin only). Keeps "who can do what" knobs
    // next to "what are they actually doing" — avoids stuffing monitoring
    // under the generic 系统管理 bucket.
    key: 'user-mgmt', icon: '👥', label: '用户管理', adminOnly: true,
    items: [
      { path: '/users',         icon: '👤', label: '用户管理' },
      { path: '/users-monitor', icon: '📊', label: '用户数据' },
    ],
  },
  {
    // 全局数据 (admin only). Cross-user total overview of all sites and
    // their 4-platform data — the bird's-eye view.
    key: 'global-data', icon: '🌐', label: '全局数据', adminOnly: true,
    items: [
      { path: '/global-data', icon: '📈', label: '全局数据总览' },
    ],
  },
  {
    // 系统管理 group. Non-admin members only see 个人中心 here; admins see
    // the remaining management knobs. 用户管理 / 用户数据 moved into their
    // own top-level group above.
    key: 'system', icon: '⚙️', label: '系统管理',
    items: [
      { path: '/personal-center',  icon: '🧑', label: '个人中心' },
      { path: '/cache-management', icon: '💾', label: '缓存管理',   adminOnly: true },
      { path: '/settings',         icon: '⚙️', label: '系统设置',   adminOnly: true },
    ],
  },
]

const flatTop = computed(() => topItems)

// Per-role filtering. A group is visible when:
//   - the group isn't admin-only, OR the user is admin, AND
//   - at least one of its items is visible under the user's role.
// Items can also be marked adminOnly individually. Non-admins see only
// their own-scoped items (e.g. 个人中心) inside a mostly-admin group.
const visibleGroups = computed(() => {
  const isAdmin = userStore.isAdmin()
  return groups
    .filter(g => !g.adminOnly || isAdmin)
    .map(g => ({
      ...g,
      items: g.items.filter(it => !it.adminOnly || isAdmin),
    }))
    .filter(g => g.items.length > 0)
})

// Collapse state: keyed by group key. Default = ALL COLLAPSED (per
// product decision — menu is dense, user prefers to expand on demand).
// Persisted in localStorage under 'nav-collapsed' so a user's explicit
// expand/collapse choices stick across reloads.
//
// On first-ever load (no localStorage entry) we seed with every group
// collapsed; ensureCurrentGroupOpen() will then pop open whichever
// group contains the current route so the user always sees their
// current location.
// Bumped to v2 alongside the "no auto-expand + collapsed-by-default"
// decision; existing users (who had every group expanded by habit)
// reset to the clean collapsed state on next load.
const COLLAPSE_KEY = 'xunzhan:nav-collapsed-v2'
const collapsed = ref(new Set())
const hasSaved = (() => {
  try { return localStorage.getItem(COLLAPSE_KEY) != null } catch { return false }
})()
if (hasSaved) {
  try {
    const saved = JSON.parse(localStorage.getItem(COLLAPSE_KEY) || '[]')
    if (Array.isArray(saved)) collapsed.value = new Set(saved)
  } catch { /* corrupt entry, ignore */ }
} else {
  // First-time visitor — seed all group keys as collapsed.
  collapsed.value = new Set(groups.map(g => g.key))
}

function isCollapsed(key) { return collapsed.value.has(key) }
function toggleGroup(key) {
  // Mutate via set-replacement so Vue reactivity fires.
  const next = new Set(collapsed.value)
  if (next.has(key)) next.delete(key); else next.add(key)
  collapsed.value = next
  try {
    localStorage.setItem(COLLAPSE_KEY, JSON.stringify([...next]))
  } catch { /* quota / privacy mode, lose state silently */ }
}

// Per product decision: NO auto-expand on navigation. The sidebar stays
// fully collapsed until the user manually clicks a group header. This is
// a deliberate minimalism choice — the menu is dense and most sessions
// only touch 1-2 groups, so popping open three or four on every route
// change was noisy. Users who want a group to stay open can click it
// once and the state sticks via localStorage.

function isActive(item) {
  if (item.activePrefix) return route.path.startsWith(item.activePrefix)
  return route.path === item.path
}


const pageTitles = {
  '/dashboard': '仪表盘',
  '/sites': '站点管理',
  '/data-management': '数据管理',
  '/spider-management': '蜘蛛管理',
  '/geo-management': 'GEO管理',
  '/dead-links': '死链管理',
  '/push-management': '推送管理',
  '/content-rewrite': '二创管理',
  '/external-links': '外链管理',
  '/coordination': '协同管理',
  '/reports': '汇报管理',
  '/reports/write': '写汇报',
  '/reports/weekly': '周汇报',
  '/reports/weekly/new': '生成周汇报',
  '/reports/monthly': '月汇报',
  '/reports/monthly/new': '生成月汇报',
  '/users': '用户管理',
  '/users-monitor': '用户数据',
  '/global-data': '全局数据总览',
  '/cache-management': '缓存管理',
  '/settings': '系统设置',
  '/personal-center': '个人中心',
}
const pageTitle = computed(() => {
  for (const [path, title] of Object.entries(pageTitles)) {
    if (route.path === path) return title
  }
  if (route.path.includes('/audit/')) return '审计详情'
  if (route.path.includes('/data')) return '数据中心'
  if (route.path.match(/\/sites\/\d+$/)) return '站点详情'
  if (route.path.match(/\/coordination\/\d+$/)) return '协同任务详情'
  if (route.path.match(/\/reports\/weekly\/\d+$/)) return '周汇报详情'
  if (route.path.match(/\/reports\/monthly\/\d+$/)) return '月汇报详情'
  return siteConfig.title
})

// Sync the browser tab title with `<section> · <site title>` whenever
// either the route or the configured site title changes. Using a watch
// (instead of onMounted) means a live edit in settings reflects in the
// tab title immediately after siteConfig.refresh() is called.
watch([pageTitle, () => siteConfig.title], ([section, _base]) => {
  siteConfig.applyDocumentTitle(section)
}, { immediate: true })

// router.beforeEach in router/index.ts already calls fetchUser() before
// rendering any authed route, so there's no need to call it again here —
// doing so was causing two /auth/me requests on every first-load.

async function handleLogout() {
  await userStore.logout()
  router.push('/login')
}

async function changePassword() {
  try {
    await apiClient.changePassword(pwdForm.value)
    ElMessage.success('密码修改成功')
    showPasswordDialog.value = false
    pwdForm.value = { old_password: '', new_password: '' }
  } catch (e) { console.error(e); ElMessage.error('密码修改失败') }
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
.sidebar ul.nav-list { flex: 1; }
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

/* Group headers: click to collapse/expand */
.sidebar .nav-group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 13px 12px 8px;
  margin-top: 10px;
  font-size: 14px;
  font-weight: 800;
  letter-spacing: .02em;
  color: var(--ink);
  cursor: pointer;
  user-select: none;
  border-radius: 6px;
  transition: color .15s, background .15s;
}
.sidebar .nav-group-header:hover {
  color: var(--sun-deep);
  background: var(--sun-light);
}
.sidebar .nav-group-header .grp-title {
  display: flex; align-items: center; gap: 8px;
}
.sidebar .nav-group-header .grp-icon {
  font-size: 16px; opacity: .95;
}
.sidebar .nav-group-header .grp-chev {
  font-size: 12px;
  color: var(--ink-soft);
  transition: transform .18s;
  display: inline-block;
}
.sidebar .nav-group-header .grp-chev.open {
  transform: rotate(90deg);
}
/* Sub-list: no extra padding beyond the parent list's. Items sit
   flush with top-level items so indent depth feels consistent. */
.sidebar ul.nav-sub {
  list-style: none;
  padding: 0;
  margin: 0;
}

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
.sidebar-backdrop {
  display: none;
}
</style>
