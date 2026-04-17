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
      // Image/Video gen share a single view with a kind prop. The `props` route
      // option turns route params into component props but we need a static
      // value, hence the render-function wrapper.
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
      { path: 'personal-center', name: 'PersonalCenter', component: () => import('../views/PersonalCenter.vue') },
      { path: 'users', name: 'Users', component: () => import('../views/Users.vue') },
      { path: 'users-monitor', name: 'UserMonitor', component: () => import('../views/UserMonitor.vue') },
      { path: 'global-data', name: 'GlobalData', component: () => import('../views/GlobalData.vue') },
      { path: 'cache-management', name: 'CacheManagement', component: () => import('../views/CacheManagement.vue') },
      { path: 'spider-management', name: 'SpiderManagement', component: () => import('../views/SpiderManagement.vue') },
      { path: 'coordination', name: 'Coordination', component: () => import('../views/Coordination.vue') },
      { path: 'coordination/:id', name: 'CoordinationDetail', component: () => import('../views/CoordinationDetail.vue') },
    ]
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// With cookie-based auth we can't see the token from JS, so the guard asks
// the server once via /auth/me. Result is cached in the pinia store so
// subsequent navigations are instant.
// safeRedirect returns `to.fullPath` only when it's a safe same-origin path,
// i.e. starts with exactly one "/" and isn't a protocol-relative URL like
// "//evil.com/..." or a javascript: scheme. Anything else becomes "" and
// the /login redirect falls back to its default landing page.
function safeRedirect(fullPath: string): string {
  if (!fullPath || fullPath === '/') return ''
  if (!fullPath.startsWith('/')) return ''
  if (fullPath.startsWith('//')) return '' // protocol-relative
  if (fullPath.startsWith('/\\')) return ''
  if (/^\s*(javascript|data|vbscript):/i.test(fullPath)) return ''
  return fullPath
}

router.beforeEach(async (to, _from, next) => {
  if (to.path === '/login') {
    next()
    return
  }
  const userStore = useUserStore()
  if (userStore.authed) {
    next()
    return
  }
  await userStore.fetchUser()
  if (userStore.authed) {
    next()
  } else {
    const target = safeRedirect(to.fullPath)
    next({ path: '/login', query: target ? { redirect: target } : {} })
  }
})

export default router
