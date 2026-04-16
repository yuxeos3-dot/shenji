import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

export interface ApiResponse<T = any> {
  code: number
  data: T
  error?: string
}

const instance = axios.create({
  baseURL: '/api',
  timeout: 120000,
})

instance.interceptors.request.use(config => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

instance.interceptors.response.use(
  response => {
    const data = response.data as ApiResponse
    if (data.code !== 0) {
      ElMessage.error(data.error || '请求失败')
      return Promise.reject(new Error(data.error))
    }
    return data as any
  },
  error => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      router.push('/login')
    }
    ElMessage.error(error.response?.data?.error || '网络错误')
    return Promise.reject(error)
  }
)

type Id = string | number

const api = {
  // Auth
  login: (data: { email: string; password: string }) => instance.post('/auth/login', data),
  logout: () => instance.post('/v1/auth/logout'),
  changePassword: (data: { old_password: string; new_password: string }) => instance.post('/v1/auth/change-password', data),
  getMe: () => instance.get('/v1/auth/me'),

  // Users
  listUsers: () => instance.get('/v1/users'),
  createUser: (data: Record<string, any>) => instance.post('/v1/users', data),
  updateUser: (id: Id, data: Record<string, any>) => instance.put(`/v1/users/${id}`, data),
  deleteUser: (id: Id) => instance.delete(`/v1/users/${id}`),

  // Sites
  listSites: () => instance.get('/v1/sites'),
  createSite: (data: Record<string, any>) => instance.post('/v1/sites', data),
  getSite: (id: Id) => instance.get(`/v1/sites/${id}`),
  updateSite: (id: Id, data: Record<string, any>) => instance.put(`/v1/sites/${id}`, data),
  deleteSite: (id: Id) => instance.delete(`/v1/sites/${id}`),

  // Audit
  triggerAudit: (siteId: Id) => instance.post(`/v1/sites/${siteId}/audit`),
  listAudits: (siteId: Id) => instance.get(`/v1/sites/${siteId}/audits`),
  deleteAudit: (taskId: Id) => instance.delete(`/v1/audits/${taskId}`),
  getAudit: (taskId: Id) => instance.get(`/v1/audits/${taskId}`),
  getAuditPages: (taskId: Id) => instance.get(`/v1/audits/${taskId}/pages`),
  downloadAuditReport: (taskId: Id): string => `/api/v1/audits/${taskId}/report/download`,

  // Dead Link Check
  getDeadLinks: (siteId: Id, params?: Record<string, any>) => instance.get(`/v1/sites/${siteId}/deadlinks`, { params }),
  retryDeadLinks: (siteId: Id) => instance.post(`/v1/sites/${siteId}/deadlinks/retry`),
  getDeadLinkProgress: (siteId: Id) => instance.get(`/v1/sites/${siteId}/deadlinks/progress`),
  startDeadLinkCheck: (siteId: Id) => instance.post(`/v1/sites/${siteId}/deadlinks/check`),

  // Content Rewrite
  rewriteContent: (data: Record<string, any>) => instance.post('/v1/rewrite', data),

  // External Links
  getExternalLinks: (siteId: Id) => instance.get(`/v1/sites/${siteId}/extlinks`),
  createExternalLink: (data: Record<string, any>) => instance.post('/v1/extlinks', data),
  updateExternalLink: (id: Id, data: Record<string, any>) => instance.put(`/v1/extlinks/${id}`, data),
  deleteExternalLink: (id: Id) => instance.delete(`/v1/extlinks/${id}`),
  checkExternalLink: (id: Id) => instance.post(`/v1/extlinks/${id}/check`),

  // Site Pages
  listSitePages: (siteId: Id) => instance.get(`/v1/sites/${siteId}/pages`),
  createSitePage: (siteId: Id, data: Record<string, any>) => instance.post(`/v1/sites/${siteId}/pages`, data),
  deleteSitePage: (siteId: Id, pageId: Id) => instance.delete(`/v1/sites/${siteId}/pages/${pageId}`),
  fetchSitePage: (siteId: Id, pageId: Id) => instance.post(`/v1/sites/${siteId}/pages/${pageId}/fetch`),
  getSitePageSource: (siteId: Id, pageId: Id) => instance.get(`/v1/sites/${siteId}/pages/${pageId}/source`),

  // GSC
  getGSCAuthURL: () => instance.get('/v1/oauth/gsc'),
  syncGSC: (siteId: Id) => instance.post(`/v1/sites/${siteId}/gsc/sync`),
  getGSCData: (siteId: Id, days?: number) => instance.get(`/v1/sites/${siteId}/gsc`, { params: { days } }),

  // GA
  getGAAuthURL: () => instance.get('/v1/oauth/ga'),
  syncGA: (siteId: Id) => instance.post(`/v1/sites/${siteId}/ga/sync`),
  getGAData: (siteId: Id) => instance.get(`/v1/sites/${siteId}/ga`),

  // Yandex
  getYandexAuthURL: () => instance.get('/v1/oauth/yandex'),
  syncYandex: (siteId: Id) => instance.post(`/v1/sites/${siteId}/yandex/sync`),
  getYandexData: (siteId: Id) => instance.get(`/v1/sites/${siteId}/yandex`),

  // Reports
  listReports: (params?: Record<string, any>) => instance.get('/v1/reports', { params }),
  submitReport: (data: Record<string, any>) => instance.post('/v1/reports', data),
  polishReport: (data: Record<string, any>) => instance.post('/v1/reports/polish', data),
  getReport: (date: string) => instance.get(`/v1/reports/${date}`),
  exportReports: (params?: Record<string, any>) => instance.get('/v1/reports/export', { params }),

  // Bing Webmaster
  syncBing: (siteId: Id) => instance.post(`/v1/sites/${siteId}/bing/sync`),
  getBingData: (siteId: Id) => instance.get(`/v1/sites/${siteId}/bing`),

  // Settings & Permissions
  getOnlineUsers: () => instance.get('/v1/auth/online'),
  getMyPermissions: () => instance.get('/v1/auth/my-permissions'),
  getPermissionTree: () => instance.get('/v1/users/permission-tree'),
  getUserPermissions: (id: Id) => instance.get(`/v1/users/${id}/permissions`),
  saveUserPermissions: (id: Id, data: { permissions: string[] }) => instance.put(`/v1/users/${id}/permissions`, data),

  getMyAIConfig: () => instance.get('/v1/auth/my-ai-config'),
  saveMyAIConfig: (data: Record<string, any>) => instance.put('/v1/auth/my-ai-config', data),
  testMyAI: () => instance.post('/v1/auth/test-my-ai'),

  listExtlinkPool: () => instance.get('/v1/extlink-pool'),
  importExtlinkPool: (data: Record<string, any>) => instance.post('/v1/extlink-pool/import', data),
  deleteExtlinkPoolItem: (id: Id) => instance.delete(`/v1/extlink-pool/${id}`),

  getSettings: () => instance.get('/v1/settings'),
  saveSettings: (data: Record<string, string>) => instance.put('/v1/settings', data),
  getMyIP: () => instance.get('/v1/settings/my-ip'),
  testAI: () => instance.post('/v1/settings/test-ai'),
  testTelegram: () => instance.post('/v1/settings/test-telegram'),

  // Audit Logs
  listAuditLogs: () => instance.get('/v1/audit-logs'),

  // GEO Content Production
  geoStats: (params?: Record<string, any>) => instance.get('/v1/geo/stats', { params }),
  geoListTitleLibs: (params?: Record<string, any>) => instance.get('/v1/geo/title-libs', { params }),
  geoCreateTitleLib: (data: Record<string, any>) => instance.post('/v1/geo/title-libs', data),
  geoDeleteTitleLib: (id: Id, ct?: string) => instance.delete(`/v1/geo/title-libs/${id}`, { params: { content_type: ct || 'geo' } }),
  geoListTitles: (libId: Id, params?: Record<string, any>) => instance.get(`/v1/geo/title-libs/${libId}/titles`, { params }),
  geoAddTitles: (libId: Id, data: Record<string, any>) => instance.post(`/v1/geo/title-libs/${libId}/titles`, data),
  geoImportTitles: (libId: Id, data: Record<string, any>) => instance.post(`/v1/geo/title-libs/${libId}/import`, data),
  geoImportFromGSC: (libId: Id, data: Record<string, any>) => instance.post(`/v1/geo/title-libs/${libId}/import-gsc`, data),
  geoDeleteTitle: (id: Id) => instance.delete(`/v1/geo/titles/${id}`),
  geoListPrompts: (params?: Record<string, any>) => instance.get('/v1/geo/prompts', { params }),
  geoCreatePrompt: (data: Record<string, any>) => instance.post('/v1/geo/prompts', data),
  geoUpdatePrompt: (id: Id, data: Record<string, any>) => instance.put(`/v1/geo/prompts/${id}`, data),
  geoDeletePrompt: (id: Id, ct?: string) => instance.delete(`/v1/geo/prompts/${id}`, { params: { content_type: ct || 'geo' } }),
  geoListKnowledge: (params?: Record<string, any>) => instance.get('/v1/geo/knowledge', { params }),
  geoCreateKnowledge: (data: Record<string, any>) => instance.post('/v1/geo/knowledge', data),
  geoUpdateKnowledge: (id: Id, data: Record<string, any>) => instance.put(`/v1/geo/knowledge/${id}`, data),
  geoDeleteKnowledge: (id: Id, ct?: string) => instance.delete(`/v1/geo/knowledge/${id}`, { params: { content_type: ct || 'geo' } }),
  geoListTasks: (params?: Record<string, any>) => instance.get('/v1/geo/tasks', { params }),
  geoCreateTask: (data: Record<string, any>) => instance.post('/v1/geo/tasks', data),
  geoUpdateTask: (id: Id, data: Record<string, any>) => instance.put(`/v1/geo/tasks/${id}`, data),
  geoDeleteTask: (id: Id, ct?: string) => instance.delete(`/v1/geo/tasks/${id}`, { params: { content_type: ct || 'geo' } }),
  geoStartTask: (id: Id) => instance.post(`/v1/geo/tasks/${id}/start`),
  geoStopTask: (id: Id) => instance.post(`/v1/geo/tasks/${id}/stop`),
  geoRunTask: (id: Id) => instance.post(`/v1/geo/tasks/${id}/run`),
  geoListArticles: (params?: Record<string, any>) => instance.get('/v1/geo/articles', { params }),
  geoGetArticle: (id: Id) => instance.get(`/v1/geo/articles/${id}`),
  geoUpdateArticle: (id: Id, data: Record<string, any>) => instance.put(`/v1/geo/articles/${id}`, data),
  geoDeleteArticle: (id: Id, ct?: string) => instance.delete(`/v1/geo/articles/${id}`, { params: { content_type: ct || 'geo' } }),
  geoReviewArticle: (id: Id, data: Record<string, any>) => instance.post(`/v1/geo/articles/${id}/review`, data),
  geoPublishArticle: (id: Id) => instance.post(`/v1/geo/articles/${id}/publish`),
}

export default api
