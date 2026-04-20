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
      { path: 'users', name: 'Users', component: () => import('../views/Users.vue') },
      { path: 'users-monitor', name: 'UserMonitor', component: () => import('../views/UserMonitor.vue') },
      { path: 'global-data', name: 'GlobalData', component: () => import('../views/GlobalData.vue') },
      { path: 'global-analytics', name: 'GlobalAnalytics', component: () => import('../views/GlobalAnalytics.vue') },
      { path: 'cache-management', name: 'CacheManagement', component: () => import('../views/CacheManagement.vue') },
      { path: 'spider-management', name: 'SpiderManagement', component: () => import('../views/SpiderManagement.vue') },
      { path: 'push-management', name: 'PushManagement', component: () => import('../views/PushManagement.vue') },
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

// authProbed flips true the first time fetchUser resolves, regardless
// of outcome. Used to distinguish "we haven't checked yet" from
// "checked, definitely anonymous" so cold-load can route to /login
// optimistically without re-probing on every step.
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
  if (to.path === '/login') {
    // Visiting /login while already authed (back button, manual URL)
    // should bounce straight to the dashboard rather than show a stale
    // form. Otherwise the user clicks 登录 and gets a "登录成功"
    // toast for what was already a valid session.
    const userStore = useUserStore()
    if (userStore.authed) {
      next({ path: '/dashboard' })
      return
    }
    next()
    // Probe in the background so if the cookie is actually valid,
    // Login.vue's mounted hook can pick up authed=true and redirect.
    if (!authProbed) probeAuth()
    return
  }

  const userStore = useUserStore()
  if (userStore.authed) {
    next()
    return
  }

  // Cold-load fast path: we've never probed and the cookie state is
  // unknown. The vast majority of cold loads are anonymous (a session
  // exists for at most a single browser tab at a time, and the typing
  // a-domain-into-the-bar gesture usually means "I want to log in").
  // Route to /login synchronously to skip the blank-screen wait — if
  // the probe comes back authed, the redirect query in /login will
  // bounce them to their original target.
  if (!authProbed) {
    probeAuth().then(() => {
      // No-op here; the awaiting Login page will react to authed.value
      // turning true and route forward via its own guard.
    })
    const target = safeRedirect(to.fullPath)
    next({ path: '/login', query: target ? { redirect: target } : {} })
    return
  }

  // We've probed and the user is definitely anonymous (otherwise we
  // would have hit the userStore.authed branch above). Send them to
  // login proper.
  const target = safeRedirect(to.fullPath)
  next({ path: '/login', query: target ? { redirect: target } : {} })
})

export default router
