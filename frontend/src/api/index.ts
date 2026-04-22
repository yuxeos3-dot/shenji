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
      // Opt-out: a caller that wants to render field-level errors itself
      // can pass `{ _silent: true }` (see AxiosRequestConfig extension).
      // The global toast would otherwise double-announce the failure,
      // which is the "3 red banners stack on top" problem users saw
      // when email + password both failed validation.
      if (!(response.config as any)._silent) {
        ElMessage.error(data.error || '请求失败')
      }
      return Promise.reject(Object.assign(new Error(data.error), { response }))
    }
    return data as any
  },
  error => {
    if (error.response?.status === 401) {
      // Scrub any lingering legacy token — harmless if absent.
      localStorage.removeItem('token')
      router.push('/login')
    }
    // #43: CSRF token mismatch — session likely expired or cookie cleared.
    // Redirect to login so the user gets a fresh session + CSRF pair.
    if (error.response?.status === 403 && error.response?.data?.error?.includes?.('CSRF')) {
      localStorage.removeItem('token')
      router.push('/login')
    }
    if (!(error.config as any)?._silent) {
      ElMessage.error(error.response?.data?.error || '网络错误')
    }
    return Promise.reject(error)
  }
)

type Id = string | number

const api = {
  // Auth
  login: (data: { email: string; password: string }) => instance.post('/auth/login', data),
  // Self-registration. Public — invite code in the body is the only auth.
  // Backend lands the row in register_status='pending' and returns code:0
  // with a "等待审核" message, OR code:-1 with a friendly error.
  register: (data: Record<string, any>) => instance.post('/auth/register', data),
  publicListDepartments: () => instance.get('/auth/departments-public'),
  publicListGroups: (departmentID: number | string) =>
    instance.get('/auth/groups-public', { params: { department_id: departmentID } }),
  // Pending registration review (admin only — under /v1/users).
  // Global Analytics / Strategy Intelligence
  analyticsSnapshot: () => instance.get('/v1/admin/analytics/snapshot'),
  analyticsAdvise: () => instance.post('/v1/admin/analytics/ai-advise', {}, { timeout: 180000 } as any),

  listPendingUsers: () => instance.get('/v1/users/pending'),
  approvePendingUser: (id: Id, data: Record<string, any>) =>
    instance.post(`/v1/users/${id}/approve`, data),
  rejectPendingUser: (id: Id) => instance.post(`/v1/users/${id}/reject`),
  logout: () => instance.post('/v1/auth/logout'),
  changePassword: (data: { old_password: string; new_password: string }) => instance.post('/v1/auth/change-password', data),
  // Auth probe — used by the router guard on every fresh page load to
  // decide between /login and the requested route. 401 here means
  // "anonymous", not "session expired", so suppress the global error
  // toast that the interceptor would otherwise fire ("未登录").
  getMe: () => instance.get('/v1/auth/me', { _silent: true } as any),
  getCSRFToken: () => instance.get('/v1/auth/csrf'),

  // Users
  listUsers: () => instance.get('/v1/users'),
  // Member-safe team directory — id + name + dept + group, no email or role.
  // Used by the coord assignee / reviewer pickers from a member context.
  listTeamMembers: () => instance.get('/v1/team/members'),

  // Per-user data-platform site-hide list. The × button on GSC / GA4 /
  // Bing / Yandex overview cards calls hidePlatformSite; the list is
  // fetched once on overview mount and applied as a client-side filter.
  listPlatformHides: () => instance.get('/v1/platform-hides'),
  hidePlatformSite: (platform: string, site_key: string) =>
    instance.post('/v1/platform-hides', { platform, site_key }),
  unhidePlatformSite: (platform: string, site_key: string) =>
    instance.delete('/v1/platform-hides', { data: { platform, site_key } }),
  createUser: (data: Record<string, any>, config?: Record<string, any>) => instance.post('/v1/users', data, config),
  updateUser: (id: Id, data: Record<string, any>, config?: Record<string, any>) => instance.put(`/v1/users/${id}`, data, config),
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
  // params: optional { user_id } for admin monitoring — scopes the list
  // to a specific owner. Without params admin sees everything; non-admin
  // always sees only their own (backend ignores the filter for them).
  listSites: (params?: Record<string, any>) => instance.get('/v1/sites', { params }),
  createSite: (data: Record<string, any>) => instance.post('/v1/sites', data),
  getSite: (id: Id) => instance.get(`/v1/sites/${id}`),
  updateSite: (id: Id, data: Record<string, any>) => instance.put(`/v1/sites/${id}`, data),
  deleteSite: (id: Id) => instance.delete(`/v1/sites/${id}`),
  exportSites: () => instance.get('/v1/site-bundle/export'),
  importSites: (data: string) => instance.post('/v1/site-bundle/import', { data }),

  // Audit
  triggerAudit: (siteId: Id) => instance.post(`/v1/sites/${siteId}/audit`),

  // Site history (combined coord-completed + manual incidents)
  siteHistory: (siteId: Id) => instance.get(`/v1/sites/${siteId}/history`),
  createSiteIncident: (siteId: Id, data: Record<string, any>) =>
    instance.post(`/v1/sites/${siteId}/incidents`, data),
  deleteSiteIncident: (siteId: Id, incidentId: Id) =>
    instance.delete(`/v1/sites/${siteId}/incidents/${incidentId}`),
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
  //
  // _silent: true 让 axios 响应拦截器在这里 *不* 弹全局 ElMessage.error
  // toast。原因：一个用户常见的是绑了 GSC 但没绑 Yandex/Bing（反之亦然），
  // overview 请求会在未绑平台上返回 400；页面本身已经有 error.value
  // 状态会在卡片区域内联显示"❌ 未授权..."等提示，不需要屏幕顶部
  // 再叠 4 个 toast 遮挡侧边栏 / 主内容区的点击。之前"有时点不动"
  // 就是叠的 ElMessage 罩层尚未 dismiss。
  dashGSCListSites: () => instance.get('/v1/gsc/sites', { _silent: true } as any),
  dashGSCOverview: (params: Record<string, any>) => instance.get('/v1/gsc/overview', { params, _silent: true } as any),
  dashGSCSiteDetail: (params: Record<string, any>) => instance.get('/v1/gsc/site-detail', { params, _silent: true } as any),

  dashGAListProperties: () => instance.get('/v1/ga4/properties', { _silent: true } as any),
  dashGAOverview: (params: Record<string, any>) => instance.get('/v1/ga4/overview', { params, _silent: true } as any),
  dashGAPropertyDetail: (params: Record<string, any>) => instance.get('/v1/ga4/property-detail', { params, _silent: true } as any),

  dashYandexListHosts: () => instance.get('/v1/yandex/hosts', { _silent: true } as any),
  dashYandexOverview: (params: Record<string, any>) => instance.get('/v1/yandex/overview', { params, _silent: true } as any),
  dashYandexHostDetail: (params: Record<string, any>) => instance.get('/v1/yandex/host-detail', { params, _silent: true } as any),

  dashBingListSites: () => instance.get('/v1/bing/sites', { _silent: true } as any),
  dashBingOverview: (params: Record<string, any>) => instance.get('/v1/bing/overview', { params, _silent: true } as any),
  dashBingSiteDetail: (params: Record<string, any>) => instance.get('/v1/bing/site-detail', { params, _silent: true } as any),

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

  // Personal platform credentials (per-user OAuth client + tokens,
  // Telegram bot, Bing key, FRW apikey). Everyone can manage their
  // own; admin uses /users/:id/platform-status for read-only monitoring.
  getMyOAuthConfigs: () => instance.get('/v1/auth/my-oauth-config'),
  saveMyOAuthConfig: (data: Record<string, any>) => instance.put('/v1/auth/my-oauth-config', data),
  unbindMyOAuthToken: (service: string) => instance.delete('/v1/auth/my-oauth-token', { params: { service } }),
  getMyTelegram: () => instance.get('/v1/auth/my-telegram'),
  saveMyTelegram: (data: Record<string, any>) => instance.put('/v1/auth/my-telegram', data),
  testMyTelegram: () => instance.post('/v1/auth/my-telegram/test'),
  getMyBing: () => instance.get('/v1/auth/my-bing'),
  saveMyBing: (data: Record<string, any>) => instance.put('/v1/auth/my-bing', data),
  getMyFrwApiKey: () => instance.get('/v1/auth/my-frw-apikey'),
  saveMyFrwApiKey: (data: Record<string, any>) => instance.put('/v1/auth/my-frw-apikey', data),
  getUserPlatformStatus: (userId: Id) => instance.get(`/v1/users/${userId}/platform-status`),
  getUserOverview: (userId: Id) => instance.get(`/v1/users/${userId}/overview`),

  // Admin-only global overview: all sites across all users + latest
  // 4-platform snapshot. Params: { user_id? } to narrow by owner.
  getGlobalData: (params?: Record<string, any>) => instance.get('/v1/admin/global-data', { params }),
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
  coordListScopes: () => instance.get('/v1/coord/scopes'),
  coordCreateTask: (data: Record<string, any>) => instance.post('/v1/coord/tasks', data),
  coordGetTask: (id: Id) => instance.get(`/v1/coord/tasks/${id}`),
  coordUpdateTask: (id: Id, data: Record<string, any>) => instance.put(`/v1/coord/tasks/${id}`, data),
  coordDeleteTask: (id: Id) => instance.delete(`/v1/coord/tasks/${id}`),
  coordAssignTask: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/assign`, data),
  // 组长 / 经理审核 — approve or reject a pending_review task.
  coordReviewDecision: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/review`, data),
  coordSubmitTask: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/submit`, data),
  coordVerifyTask: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/verify`, data),
  coordAddComment: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/comments`, data),
  // Developer ↔ creator Q&A: ask pauses dev, answer resumes it.
  coordAskTask: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/ask`, data),
  coordAnswerTask: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/answer`, data),
  coordPingDev: (id: Id, data: Record<string, any>) => instance.post(`/v1/coord/tasks/${id}/ping-dev`, data),
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

  // Upload local file as media asset
  uploadFRWMedia: (formData: FormData) =>
    instance.post("/v1/frw/upload", formData, {
      headers: { "Content-Type": "multipart/form-data" },
    }),

  // Push Management
  listPushLogs: (siteId: Id, params?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/push/logs`, { params }),
  submitSitemap: (siteId: Id, data: { sitemap_url: string; platforms: string[] }) =>
    instance.post(`/v1/sites/${siteId}/push/sitemap`, data),
  submitSitemapAll: (data: { sitemap_url_suffix: string; platforms: string[] }) =>
    instance.post('/v1/push/sitemap-all', data),
  submitURLs: (siteId: Id, data: { platform: string; urls: string[] }) =>
    instance.post(`/v1/sites/${siteId}/push/urls`, data),
  listPushSchedules: (siteId: Id) =>
    instance.get(`/v1/sites/${siteId}/push/schedules`),
  savePushSchedule: (siteId: Id, data: Record<string, any>) =>
    instance.post(`/v1/sites/${siteId}/push/schedules`, data),
  deletePushSchedule: (id: Id) =>
    instance.delete(`/v1/push/schedules/${id}`),
  togglePushSchedule: (id: Id) =>
    instance.put(`/v1/push/schedules/${id}/toggle`),

  // ── Platform Data Deep ─────────────────────────────────────────
  // Site Analysis (全站分析)
  getSiteHealthScore: (siteId: Id) => instance.get(`/v1/sites/${siteId}/health-score`),
  computeHealthScore: (siteId: Id) => instance.post(`/v1/sites/${siteId}/health-score/compute`),
  getSiteAnalysisSummary: (siteId: Id) => instance.get(`/v1/sites/${siteId}/analysis-summary`),
  runAnalysisPipeline: (siteId: Id) => instance.post(`/v1/sites/${siteId}/run-analysis`, {}, { timeout: 120000 } as any),
  generateReport: (siteId: Id, type?: string) => instance.post(`/v1/sites/${siteId}/generate-report`, {}, { params: { type }, timeout: 300000 } as any),
  listSiteReports: (siteId: Id) => instance.get(`/v1/sites/${siteId}/analysis-reports`),
  getSiteReport: (reportId: Id) => instance.get(`/v1/analysis-reports/${reportId}`),
  // Management CRUD
  listCompetitors: (siteId: Id) => instance.get(`/v1/sites/${siteId}/competitors`),
  createCompetitor: (siteId: Id, data: Record<string, any>) => instance.post(`/v1/sites/${siteId}/competitors`, data),
  deleteCompetitor: (id: Id) => instance.delete(`/v1/competitors/${id}`),
  listAlgorithmUpdates: () => instance.get('/v1/algorithm-updates'),
  createAlgorithmUpdate: (data: Record<string, any>) => instance.post('/v1/algorithm-updates', data),
  listSERPObservations: (siteId: Id) => instance.get(`/v1/sites/${siteId}/serp-observations`),
  createSERPObservation: (siteId: Id, data: Record<string, any>) => instance.post(`/v1/sites/${siteId}/serp-observations`, data),
  listAIOObservations: (siteId: Id) => instance.get(`/v1/sites/${siteId}/aio-observations`),
  createAIOObservation: (siteId: Id, data: Record<string, any>) => instance.post(`/v1/sites/${siteId}/aio-observations`, data),
  listTimelineEvents: (siteId: Id) => instance.get(`/v1/sites/${siteId}/timeline`),
  createTimelineEvent: (siteId: Id, data: Record<string, any>) => instance.post(`/v1/sites/${siteId}/timeline`, data),
  getContentStats: (siteId: Id) => instance.get(`/v1/sites/${siteId}/content-stats`),
  getRuleFindings: (siteId: Id) => instance.get(`/v1/sites/${siteId}/rule-findings`),
  getBacklinkProfile: (siteId: Id) => instance.get(`/v1/sites/${siteId}/backlink-profile`),

  // Unified platform data (single endpoint for everything)
  getUnifiedData: (siteId: Id, platform: string, days?: number) =>
    instance.get(`/v1/sites/${siteId}/platform-data`, { params: { platform, days } }),
  syncAndLoad: (siteId: Id, platform: string, days?: number) =>
    instance.post(`/v1/sites/${siteId}/sync-and-load`, {}, { params: { platform, days }, timeout: 300000 } as any),

  // Site ID resolution (domain → site_id for deep data)
  resolveSiteId: (domain: string) =>
    instance.get('/v1/resolve-site-id', { params: { domain }, _silent: true } as any),

  // GSC Deep
  syncGSCDeep: (siteId: Id, date?: string) =>
    instance.post(`/v1/sites/${siteId}/gsc/deep-sync`, date ? { date } : {}, { timeout: 300000 } as any),
  syncGSCURLInspection: (siteId: Id) =>
    instance.post(`/v1/sites/${siteId}/gsc/url-inspection`, {}, { timeout: 300000 } as any),
  getGSCOverviewDeep: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/overview`, { params: { days } }),
  getGSCRankBuckets: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/buckets`, { params: { days } }),
  getGSCKeywords: (siteId: Id, params?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/keywords`, { params }),
  getGSCKeywordDetail: (siteId: Id, query: string) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/keyword-detail`, { params: { q: query } }),
  getGSCDeepPages: (siteId: Id, params?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/pages`, { params }),
  getGSCAppearance: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/appearance`, { params: { days } }),
  getGSCUrlInspection: (siteId: Id, params?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/url-inspection`, { params }),
  getGSCSitemaps: (siteId: Id) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/sitemaps`),
  getGSCOpportunities: (siteId: Id, params?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/opportunities`, { params }),
  getGSCCannibalization: (siteId: Id) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/cannibalization`),
  getGSCQueryDiff: (siteId: Id, type?: string) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/query-diff`, { params: { type } }),

  getGSCKeywordsExport: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/gsc/keywords/export`, { params: { days }, responseType: 'blob' } as any),

  // GA4 Deep
  syncGA4Deep: (siteId: Id) =>
    instance.post(`/v1/sites/${siteId}/ga/deep-sync`, {}, { timeout: 300000 } as any),
  getGA4Extended: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/ga/extended`, { params: { days } }),
  getGA4LandingPages: (siteId: Id, params?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/deep/ga/landing-pages`, { params }),
  getGA4SourceMedium: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/ga/source-medium`, { params: { days } }),
  getGA4Events: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/ga/events`, { params: { days } }),
  getGA4Tech: (siteId: Id, dimension?: string, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/ga/tech`, { params: { dimension, days } }),
  getGA4Geo: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/ga/geo`, { params: { days } }),
  getGA4Hourly: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/ga/hourly`, { params: { days } }),
  getGA4DayOfWeek: (siteId: Id) =>
    instance.get(`/v1/sites/${siteId}/deep/ga/day-of-week`),
  getGA4ScrollDepth: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/ga/scroll-depth`, { params: { days } }),
  getGA4OutboundClicks: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/ga/outbound-clicks`, { params: { days } }),
  getGA4ConversionsDaily: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/ga/conversions`, { params: { days } }),

  // Bing Deep
  syncBingDeep: (siteId: Id) =>
    instance.post(`/v1/sites/${siteId}/bing/deep-sync`, {}, { timeout: 300000 } as any),
  getBingKeywords: (siteId: Id, params?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/deep/bing/keywords`, { params }),
  getBingDeepPages: (siteId: Id, params?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/deep/bing/pages`, { params }),
  getBingInboundLinks: (siteId: Id) =>
    instance.get(`/v1/sites/${siteId}/deep/bing/inbound-links`),
  getBingSitemaps: (siteId: Id) =>
    instance.get(`/v1/sites/${siteId}/deep/bing/sitemaps`),

  // Yandex Deep
  syncYandexDeep: (siteId: Id) =>
    instance.post(`/v1/sites/${siteId}/yandex/deep-sync`, {}, { timeout: 300000 } as any),
  getYandexKeywords: (siteId: Id, params?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/deep/yandex/keywords`, { params }),
  getYandexIndexEvents: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/yandex/index-events`, { params: { days } }),
  getYandexHTTPStatus: (siteId: Id, days?: number) =>
    instance.get(`/v1/sites/${siteId}/deep/yandex/http-status`, { params: { days } }),
  getYandexExternalLinks: (siteId: Id) =>
    instance.get(`/v1/sites/${siteId}/deep/yandex/external-links`),
  getYandexIndexedUrls: (siteId: Id, params?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/deep/yandex/indexed-urls`, { params }),
}

export default api
