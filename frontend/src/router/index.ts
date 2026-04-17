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
      { path: 'dead-links', name: 'DeadLinks', component: () => import('../views/DeadLinks.vue') },
      { path: 'content-rewrite', name: 'ContentRewrite', component: () => import('../views/ContentRewrite.vue') },
      { path: 'external-links', name: 'ExternalLinks', component: () => import('../views/ExternalLinks.vue') },
      { path: 'reports', name: 'Reports', component: () => import('../views/Reports.vue') },
      { path: 'reports/write', name: 'WriteReport', component: () => import('../views/WriteReport.vue') },
      { path: 'settings', name: 'Settings', component: () => import('../views/Settings.vue') },
      { path: 'users', name: 'Users', component: () => import('../views/Users.vue') },
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
    next({ path: '/login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : {} })
  }
})

export default router
