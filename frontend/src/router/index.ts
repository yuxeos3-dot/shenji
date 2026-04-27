import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useUserStore } from '../stores/user'

const routes: RouteRecordRaw[] = [
  { path: '/login', name: 'Login', component: () => import('../views/Login.vue') },
  {
    path: '/',
    component: () => import('../views/Layout.vue'),
    redirect: '/sites',
    children: [
      { path: 'sites', name: 'Sites', component: () => import('../views/Sites.vue') },
      { path: 'sites/:id', name: 'SiteDetail', component: () => import('../views/SiteDetail.vue') },
      { path: 'sites/:id/audit/:taskId', name: 'AuditDetail', component: () => import('../views/AuditDetail.vue') },
      { path: 'data-management', name: 'DataManagement', component: () => import('../views/DataManagement.vue') },
      { path: 'dead-links', name: 'DeadLinks', component: () => import('../views/DeadLinks.vue') },
      { path: 'settings', name: 'Settings', component: () => import('../views/Settings.vue') },
    ]
  },
  { path: '/:pathMatch(.*)*', name: 'NotFound', component: () => import('../views/NotFound.vue') },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(to, from, savedPosition) {
    if (savedPosition) return savedPosition
    return { top: 0 }
  },
})

function safeRedirect(fullPath: string): string {
  if (!fullPath || fullPath === '/') return ''
  if (!fullPath.startsWith('/')) return ''
  if (fullPath.startsWith('//')) return ''
  if (fullPath.startsWith('/\\')) return ''
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
  if (to.name === 'NotFound') {
    next()
    return
  }

  if (to.path === '/login') {
    const userStore = useUserStore()
    if (userStore.authed) {
      next({ path: '/sites' })
      return
    }
    next()
    if (!authProbed) probeAuth()
    return
  }

  const userStore = useUserStore()
  if (userStore.authed) {
    next()
    return
  }

  if (!authProbed) {
    probeAuth()
    const target = safeRedirect(to.fullPath)
    next({ path: '/login', query: target ? { redirect: target } : {} })
    return
  }

  const target = safeRedirect(to.fullPath)
  next({ path: '/login', query: target ? { redirect: target } : {} })
})

export default router
