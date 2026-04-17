import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

export interface ApiResponse<T = any> {
  code: number
  data: T
  error?: string
}

// readCookie reads a non-httpOnly cookie by name. The auth cookie IS httpOnly
// and intentionally not readable here — only the CSRF cookie is needed from JS.
function readCookie(name: string): string {
  const prefix = name + '='
  for (const raw of document.cookie.split(';')) {
    const c = raw.trim()
    if (c.startsWith(prefix)) return decodeURIComponent(c.slice(prefix.length))
  }
  return ''
}

const instance = axios.create({
  baseURL: '/api',
  timeout: 120000,
  // Send cookies on same-origin and approved cross-origin (CORS credentials).
  withCredentials: true,
})

instance.interceptors.request.use(config => {
  // CSRF double-submit: mirror the csrf_token cookie into X-CSRF-Token for
  // state-changing methods. The auth cookie is httpOnly and handled by the
  // browser automatically (via withCredentials). We deliberately do NOT
  // read localStorage here: an older build used to stash the JWT there,
  // and a stale or attacker-controlled value would otherwise get replayed
  // on every request AND bypass CSRF (Bearer clients are exempt server-side).
  const method = (config.method || 'get').toLowerCase()
  if (method !== 'get' && method !== 'head' && method !== 'options') {
    const csrf = readCookie('csrf_token')
    if (csrf) config.headers['X-CSRF-Token'] = csrf
  }
  return config
})

instance.interceptors.response.use(
  response => {
    if (response.config.responseType === 'blob' || response.config.responseType === 'arraybuffer') {
      return response as any
    }
    const data = response.data as ApiResponse
    if (data.code !== 0) {
      ElMessage.error(data.error || '请求失败')
      return Promise.reject(new Error(data.error))
    }
    return data as any
  },
  error => {
    if (error.response?.status === 401) {
      // Scrub any lingering legacy token — harmless if absent.
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
  getCSRFToken: () => instance.get('/v1/auth/csrf'),

  // Users
  listUsers: () => instance.get('/v1/users'),
  createUser: (data: Record<string, any>) => instance.post('/v1/users', data),
  updateUser: (id: Id, data: Record<string, any>) => instance.put(`/v1/users/${id}`, data),
  deleteUser: (id: Id) => instance.delete(`/v1/users/${id}`),

  // Organization (departments + groups). Admin-only on the backend;
  // the frontend shows the CRUD panel on the Users page.
  listDepartments: () => instance.get('/v1/org/departments'),
  createDepartment: (data: Record<string, any>) => instance.post('/v1/org/departments', data),
  updateDepartment: (id: Id, data: Record<string, any>) => instance.put(`/v1/org/departments/${id}`, data),
  deleteDepartment: (id: Id) => instance.delete(`/v1/org/departments/${id}`),
  listGroups: (params?: Record<string, any>) => instance.get('/v1/org/groups', { params }),
  createGroup: (data: Record<string, any>) => instance.post('/v1/org/groups', data),
  updateGroup: (id: Id, data: Record<string, any>) => instance.put(`/v1/org/groups/${id}`, data),
  deleteGroup: (id: Id) => instance.delete(`/v1/org/groups/${id}`),

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
  // fetchAuditReportBlob reuses the axios instance so the Authorization header
  // and 401-redirect logic from the interceptor apply.
  fetchAuditReportBlob: async (taskId: Id): Promise<{ blob: Blob; filename: string }> => {
    const resp: any = await instance.get(`/v1/audits/${taskId}/report/download`, { responseType: 'blob' })
    const disp = resp.headers?.['content-disposition'] || ''
    const match = /filename="([^"]+)"/.exec(disp)
    return { blob: resp.data as Blob, filename: match ? match[1] : `SEO审计报告_${taskId}.docx` }
  },

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
  getGSCData: (siteId: Id, days?: number, config?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/gsc`, { params: { days }, ...(config || {}) }),

  // GA
  getGAAuthURL: () => instance.get('/v1/oauth/ga'),
  syncGA: (siteId: Id) => instance.post(`/v1/sites/${siteId}/ga/sync`),
  getGAData: (siteId: Id, config?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/ga`, config),

  // Yandex
  getYandexAuthURL: () => instance.get('/v1/oauth/yandex'),
  syncYandex: (siteId: Id) => instance.post(`/v1/sites/${siteId}/yandex/sync`),
  getYandexData: (siteId: Id, config?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/yandex`, config),

  // ── Dashboard (data-management page): account-level overview + per-site
  // detail, date-range aware. No DB persistence — live view from upstream.
  dashGSCListSites: () => instance.get('/v1/gsc/sites'),
  dashGSCOverview: (params: Record<string, any>) => instance.get('/v1/gsc/overview', { params }),
  dashGSCSiteDetail: (params: Record<string, any>) => instance.get('/v1/gsc/site-detail', { params }),

  dashGAListProperties: () => instance.get('/v1/ga4/properties'),
  dashGAOverview: (params: Record<string, any>) => instance.get('/v1/ga4/overview', { params }),
  dashGAPropertyDetail: (params: Record<string, any>) => instance.get('/v1/ga4/property-detail', { params }),

  dashYandexListHosts: () => instance.get('/v1/yandex/hosts'),
  dashYandexOverview: (params: Record<string, any>) => instance.get('/v1/yandex/overview', { params }),
  dashYandexHostDetail: (params: Record<string, any>) => instance.get('/v1/yandex/host-detail', { params }),

  dashBingListSites: () => instance.get('/v1/bing/sites'),
  dashBingOverview: (params: Record<string, any>) => instance.get('/v1/bing/overview', { params }),
  dashBingSiteDetail: (params: Record<string, any>) => instance.get('/v1/bing/site-detail', { params }),

  // Reports
  listReports: (params?: Record<string, any>) => instance.get('/v1/reports', { params }),
  submitReport: (data: Record<string, any>) => instance.post('/v1/reports', data),
  polishReport: (data: Record<string, any>) => instance.post('/v1/reports/polish', data),
  getReport: (date: string) => instance.get(`/v1/reports/${date}`),
  exportReports: (params?: Record<string, any>) => instance.get('/v1/reports/export', { params }),

  // Weekly reports (周汇报)
  listWeeklyReports: (params?: Record<string, any>) => instance.get('/v1/reports/weekly', { params }),
  listAllWeeklyReports: (params?: Record<string, any>) => instance.get('/v1/reports/weekly/all', { params }),
  generateWeeklyReport: (data: Record<string, any>) => instance.post('/v1/reports/weekly/generate', data),
  getWeeklyReport: (id: Id) => instance.get(`/v1/reports/weekly/${id}`),
  updateWeeklyReport: (id: Id, data: Record<string, any>) => instance.put(`/v1/reports/weekly/${id}`, data),
  submitWeeklyReport: (id: Id) => instance.post(`/v1/reports/weekly/${id}/submit`),
  polishWeeklyReport: (id: Id) => instance.post(`/v1/reports/weekly/${id}/polish`),
  deleteWeeklyReport: (id: Id) => instance.delete(`/v1/reports/weekly/${id}`),

  // Monthly reports (月汇报)
  listMonthlyReports: (params?: Record<string, any>) => instance.get('/v1/reports/monthly', { params }),
  listAllMonthlyReports: (params?: Record<string, any>) => instance.get('/v1/reports/monthly/all', { params }),
  generateMonthlyReport: (data: Record<string, any>) => instance.post('/v1/reports/monthly/generate', data),
  getMonthlyReport: (id: Id) => instance.get(`/v1/reports/monthly/${id}`),
  updateMonthlyReport: (id: Id, data: Record<string, any>) => instance.put(`/v1/reports/monthly/${id}`, data),
  submitMonthlyReport: (id: Id) => instance.post(`/v1/reports/monthly/${id}/submit`),
  polishMonthlyReport: (id: Id) => instance.post(`/v1/reports/monthly/${id}/polish`),
  deleteMonthlyReport: (id: Id) => instance.delete(`/v1/reports/monthly/${id}`),

  // Report evaluations (admin 写评价 / 作者查看)
  listEvaluations: (params: Record<string, any>) => instance.get('/v1/evaluations', { params }),
  createEvaluation: (data: Record<string, any>) => instance.post('/v1/evaluations', data),
  polishEvaluation: (data: Record<string, any>) => instance.post('/v1/evaluations/polish', data),
  deleteEvaluation: (id: Id) => instance.delete(`/v1/evaluations/${id}`),

  // Bing Webmaster
  syncBing: (siteId: Id) => instance.post(`/v1/sites/${siteId}/bing/sync`),
  getBingData: (siteId: Id, config?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/bing`, config),

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
  getPublicSettings: () => instance.get('/v1/settings/public'),
  saveSettings: (data: Record<string, string>) => instance.put('/v1/settings', data),
  getMyIP: () => instance.get('/v1/settings/my-ip'),
  testAI: () => instance.post('/v1/settings/test-ai'),
  testTelegram: () => instance.post('/v1/settings/test-telegram'),

  // Audit Logs
  listAuditLogs: () => instance.get('/v1/audit-logs'),

  // Cache management
  listCaches: () => instance.get('/v1/cache'),
  clearCache: (key: string) => instance.delete(`/v1/cache/${key}`),

  // Spider analytics (site-scoped)
  spiderSiteSummary: () => instance.get('/v1/spider/sites'),
  spiderOverview: (siteId: Id) => instance.get('/v1/spider/overview', { params: { site_id: siteId } }),
  spiderTrend: (siteId: Id, params?: Record<string, any>) => instance.get('/v1/spider/trend', { params: { site_id: siteId, ...(params || {}) } }),
  spiderLogs: (siteId: Id, params?: Record<string, any>) => instance.get('/v1/spider/logs', { params: { site_id: siteId, ...(params || {}) } }),

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
  // (Prompt 模板 / 知识库 / 生成任务 endpoints were removed in the
  // 4-step wizard refactor. Use geoStartKeywordWorkflow + geoGetDefaults.)
  geoListArticles: (params?: Record<string, any>) => instance.get('/v1/geo/articles', { params }),
  geoGetArticle: (id: Id) => instance.get(`/v1/geo/articles/${id}`),
  geoUpdateArticle: (id: Id, data: Record<string, any>) => instance.put(`/v1/geo/articles/${id}`, data),
  geoDeleteArticle: (id: Id, ct?: string) => instance.delete(`/v1/geo/articles/${id}`, { params: { content_type: ct || 'geo' } }),
  geoReviewArticle: (id: Id, data: Record<string, any>) => instance.post(`/v1/geo/articles/${id}/review`, data),
  geoPublishArticle: (id: Id) => instance.post(`/v1/geo/articles/${id}/publish`),

  // Per-keyword AI writing workflow (GEO + 外链 共用)
  geoStartKeywordWorkflow: (data: Record<string, any>) => instance.post('/v1/geo/keyword/workflow/start', data),
  geoListWorkflowJobs: (params?: Record<string, any>) => instance.get('/v1/geo/keyword/workflow/jobs', { params }),
  geoGetWorkflowJob: (id: Id) => instance.get(`/v1/geo/keyword/workflow/${id}`),
  geoApplyWorkflowJob: (id: Id, data?: Record<string, any>) => instance.post(`/v1/geo/keyword/workflow/${id}/apply`, data || {}),
  geoDiscardWorkflowJob: (id: Id) => instance.delete(`/v1/geo/keyword/workflow/${id}`),
  geoGetDefaults: (contentType?: string) => instance.get('/v1/geo/defaults', { params: { content_type: contentType || 'geo' } }),

  // Coordination (协同管理)
  coordStats: (params?: Record<string, any>) => instance.get('/v1/coord/stats', { params }),
  coordListTasks: (params?: Record<string, any>) => instance.get('/v1/coord/tasks', { params }),
  coordCreateTask: (data: Record<string, any>) => instance.post('/v1/coord/tasks', data),
  coordGetTask: (id: Id) => instance.get(`/v1/coord/tasks/${id}`),
  coordUpdateTask: (id: Id, data: Record<string, any>) => instance.put(`/v1/coord/tasks/${id}`, data),
  coordDeleteTask: (id: Id) => instance.delete(`/v1/coord/tasks/${id}`),
  coordAssignTask: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/assign`, data),
  coordSubmitTask: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/submit`, data),
  coordVerifyTask: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/verify`, data),
  coordAddComment: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/comments`, data),
  // Developer ↔ creator Q&A: ask pauses dev, answer resumes it.
  coordAskTask: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/ask`, data),
  coordAnswerTask: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/answer`, data),
  // Attachments. Upload uses multipart; we override the Content-Type
  // so axios lets the browser set the multipart boundary correctly.
  coordUploadAttachment: (id: Id, formData: FormData) =>
    instance.post(`/v1/coord/tasks/${id}/attachments`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    }),
  coordDeleteAttachment: (id: Id) => instance.delete(`/v1/coord/attachments/${id}`),
  // Absolute URL for <img src=...> / <a href=...>. The Content-Disposition
  // from the server decides inline vs download.
  coordAttachmentUrl: (id: Id) => `/api/v1/coord/attachments/${id}/file`,

  // Pre-audit (待验收): test-server pages + AI verdict + reject flow
  coordListTestPages: (id: Id) => instance.get(`/v1/coord/tasks/${id}/test-pages`),
  coordAddTestPage: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/test-pages`, data),
  coordDeleteTestPage: (id: Id, pageId: Id) => instance.delete(`/v1/coord/tasks/${id}/test-pages/${pageId}`),
  coordFetchTestPage: (id: Id, pageId: Id) => instance.post(`/v1/coord/tasks/${id}/test-pages/${pageId}/fetch`),
  coordGetTestPageSource: (id: Id, pageId: Id) => instance.get(`/v1/coord/tasks/${id}/test-pages/${pageId}/source`),
  coordPreAudit: (id: Id) => instance.post(`/v1/coord/tasks/${id}/pre-audit`, {}),
  coordRejectTask: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/reject`, data),

  // FRW image / video generation
  getMyFRWConfig: () => instance.get('/v1/frw/config'),
  saveMyFRWConfig: (data: Record<string, any>) => instance.put('/v1/frw/config', data),
  listFRWGenerations: (params?: Record<string, any>) => instance.get('/v1/frw/generations', { params }),
  submitFRWGeneration: (data: Record<string, any>) => instance.post('/v1/frw/generations', data),
  getFRWGeneration: (id: Id) => instance.get(`/v1/frw/generations/${id}`),
  publishFRWGeneration: (id: Id) => instance.post(`/v1/frw/generations/${id}/publish`, {}),
  deleteFRWGeneration: (id: Id) => instance.delete(`/v1/frw/generations/${id}`),
}

export default api
