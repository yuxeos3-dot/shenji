import { createRouter, createWebHistory } from 'vue-router'

const routes = [
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
      { path: 'geo-management', name: 'GeoManagement', component: () => import('../views/GeoManagement.vue') },
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

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('token')
  if (to.path !== '/login' && !token) {
    next('/login')
  } else {
    next()
  }
})

export default router
