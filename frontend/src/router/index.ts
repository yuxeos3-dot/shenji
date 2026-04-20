import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useUserStore } from '../stores/user'

const routes: RouteRecordRaw[] = [
  { path: '/login', name: 'Login', component: () => import('../views/Login.vue') },
  {
    path: '/',
    component: () => import('../views/Layout.vue'),
    redirect: '/dashboard',
    children: [
      { path: 'dashboard', name: 'Dashboard', component: () => import('../views/Dashboard.vue') },
      { path: 'data-management', name: 'DataManagement', component: () => import('../views/DataManagement.vue') },
      { path: 'sites', name: 'Sites', component: () => import('../views/Sites.vue') },
      { path: 'sites/:id', name: 'SiteDetail', component: () => import('../views/SiteDetail.vue') },
      { path: 'sites/:id/audit/:taskId', name: 'AuditDetail', component: () => import('../views/AuditDetail.vue') },
      { path: 'sites/:id/data', name: 'DataCenter', component: () => import('../views/DataCenter.vue') },
      { path: 'geo-management', name: 'GeoManagement', component: () => import('../views/GeoContent.vue') },
      { path: 'image-management', name: 'ImageManagement', component: () => import('../views/MediaGen.vue'), props: { kind: 'image' } },
      { path: 'video-management', name: 'VideoManagement', component: () => import('../views/MediaGen.vue'), props: { kind: 'video' } },
      { path: 'dead-links', name: 'DeadLinks', component: () => import('../views/DeadLinks.vue') },
      { path: 'content-rewrite', name: 'ContentRewrite', component: () => import('../views/ContentRewrite.vue') },
      { path: 'external-links', name: 'ExternalLinks', component: () => import('../views/ExternalLinks.vue') },
      { path: 'reports', name: 'Reports', component: () => import('../views/Reports.vue') },
      { path: 'reports/write', name: 'WriteReport', component: () => import('../views/WriteReport.vue') },
      { path: 'reports/weekly', name: 'WeeklyReports', component: () => import('../views/WeeklyReports.vue') },
      { path: 'reports/weekly/new', name: 'NewWeeklyReport', component: () => import('../views/NewWeeklyReport.vue') },
      { path: 'reports/weekly/:id', name: 'WeeklyReportDetail', component: () => import('../views/WeeklyReportDetail.vue') },
      { path: 'reports/monthly', name: 'MonthlyReports', component: () => import('../views/MonthlyReports.vue') },
      { path: 'reports/monthly/new', name: 'NewMonthlyReport', component: () => import('../views/NewMonthlyReport.vue') },
      { path: 'reports/monthly/:id', name: 'MonthlyReportDetail', component: () => import('../views/MonthlyReportDetail.vue') },
      { path: 'settings', name: 'Settings', component: () => import('../views/Settings.vue') },
      { path: 'coordination', name: 'Coordination', component: () => import('../views/Coordination.vue') },
      { path: 'coordination/:id', name: 'CoordinationDetail', component: () => import('../views/CoordinationDetail.vue') },
      { path: 'push-management', name: 'PushManagement', component: () => import('../views/PushManagement.vue') },

      // ── Admin-only routes ─────────────────────────────────────────
      // These routes require admin role. The backend also enforces 403
      // for non-admin users, so this is defense-in-depth, not the only
      // access control layer.
      { path: 'users', name: 'Users', component: () => import('../views/Users.vue'), meta: { requireAdmin: true } },
      { path: 'users-monitor', name: 'UserMonitor', component: () => import('../views/UserMonitor.vue'), meta: { requireAdmin: true } },
      { path: 'global-data', name: 'GlobalData', component: () => import('../views/GlobalData.vue'), meta: { requireAdmin: true } },
      { path: 'global-analytics', name: 'GlobalAnalytics', component: () => import('../views/GlobalAnalytics.vue'), meta: { requireAdmin: true } },
      { path: 'cache-management', name: 'CacheManagement', component: () => import('../views/CacheManagement.vue'), meta: { requireAdmin: true } },
      { path: 'spider-management', name: 'SpiderManagement', component: () => import('../views/SpiderManagement.vue'), meta: { requireAdmin: true } },
    ]
  },
  // 404 catch-all — must be last
  { path: '/:pathMatch(.*)*', name: 'NotFound', component: () => import('../views/NotFound.vue') },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  // Restore scroll position on back/forward; scroll to top on new navigation
  scrollBehavior(to, from, savedPosition) {
    if (savedPosition) return savedPosition
    return { top: 0 }
  },
})

// ── Auth & permission guards ────────────────────────────────────────────────

// safeRedirect returns `to.fullPath` only when it's a safe same-origin path.
function safeRedirect(fullPath: string): string {
  if (!fullPath || fullPath === '/') return ''
  if (!fullPath.startsWith('/')) return ''
  if (fullPath.startsWith('//')) return ''
  if (fullPath.startsWith('/\')) return ''
  if (/^\s*(javascript|data|vbscript):/i.test(fullPath)) return ''
  return fullPath
}

let authProbed = false
let inflightProbe: Promise<void> | null = null

function probeAuth() {
  if (inflightProbe) return inflightProbe
  const userStore = useUserStore()
  inflightProbe = userStore.fetchUser().finally(() => {
    authProbed = true
    inflightProbe = null
  })
  return inflightProbe
}

router.beforeEach(async (to, _from, next) => {
  // Allow 404 and login without auth
  if (to.name === 'NotFound') {
    next()
    return
  }

  if (to.path === '/login') {
    const userStore = useUserStore()
    if (userStore.authed) {
      next({ path: '/dashboard' })
      return
    }
    next()
    if (!authProbed) probeAuth()
    return
  }

  const userStore = useUserStore()
  if (userStore.authed) {
    // Admin route guard: redirect non-admin users to dashboard
    if (to.meta.requireAdmin && !userStore.isAdmin()) {
      next({ path: '/dashboard' })
      return
    }
    next()
    return
  }

  // Cold-load fast path
  if (!authProbed) {
    probeAuth().then(() => {
      // Login page reacts to authed.value turning true
    })
    const target = safeRedirect(to.fullPath)
    next({ path: '/login', query: target ? { redirect: target } : {} })
    return
  }

  const target = safeRedirect(to.fullPath)
  next({ path: '/login', query: target ? { redirect: target } : {} })
})

export default router
