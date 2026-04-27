import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

export interface ApiResponse<T = any> {
  code: number
  data: T
  error?: string
}

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
  withCredentials: true,
})

instance.interceptors.request.use(config => {
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
      if (!(response.config as any)._silent) {
        ElMessage.error(data.error || '请求失败')
      }
      return Promise.reject(Object.assign(new Error(data.error), { response }))
    }
    return data as any
  },
  error => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      if (router.currentRoute.value.path !== '/login') {
        router.push('/login')
      }
    }
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
  // Auth — single password gate
  login: (data: { password: string }) => instance.post('/auth/login', data),
  logout: () => instance.post('/v1/auth/logout'),
  getMe: () => instance.get('/v1/auth/me', { _silent: true } as any),
  getCSRFToken: () => instance.get('/v1/auth/csrf'),

  // Sites
  listSites: (params?: Record<string, any>) => instance.get('/v1/sites', { params }),
  createSite: (data: Record<string, any>) => instance.post('/v1/sites', data),
  getSite: (id: Id) => instance.get(`/v1/sites/${id}`),
  updateSite: (id: Id, data: Record<string, any>) => instance.put(`/v1/sites/${id}`, data),
  deleteSite: (id: Id) => instance.delete(`/v1/sites/${id}`),
  exportSites: () => instance.get('/v1/site-bundle/export'),
  importSites: (data: string) => instance.post('/v1/site-bundle/import', { data }),

  // Site history (manual incidents)
  siteHistory: (siteId: Id) => instance.get(`/v1/sites/${siteId}/history`),
  createSiteIncident: (siteId: Id, data: Record<string, any>) =>
    instance.post(`/v1/sites/${siteId}/incidents`, data),
  deleteSiteIncident: (siteId: Id, incidentId: Id) =>
    instance.delete(`/v1/sites/${siteId}/incidents/${incidentId}`),

  // Audit
  triggerAudit: (siteId: Id) => instance.post(`/v1/sites/${siteId}/audit`),
  listAudits: (siteId: Id) => instance.get(`/v1/sites/${siteId}/audits`),
  deleteAudit: (taskId: Id) => instance.delete(`/v1/audits/${taskId}`),
  getAudit: (taskId: Id) => instance.get(`/v1/audits/${taskId}`),
  getAuditPages: (taskId: Id) => instance.get(`/v1/audits/${taskId}/pages`),
  downloadAuditReport: (taskId: Id): string => `/api/v1/audits/${taskId}/report/download`,
  fetchAuditReportBlob: async (taskId: Id): Promise<{ blob: Blob; filename: string }> => {
    const resp: any = await instance.get(`/v1/audits/${taskId}/report/download`, { responseType: 'blob' })
    const disp = resp.headers?.['content-disposition'] || ''
    const match = /filename="([^"]+)"/.exec(disp)
    return { blob: resp.data as Blob, filename: match ? match[1] : `SEO审计报告_${taskId}.docx` }
  },

  // Site pages (used by audit / deadlinks workflows)
  listSitePages: (siteId: Id) => instance.get(`/v1/sites/${siteId}/pages`),
  createSitePage: (siteId: Id, data: Record<string, any>) => instance.post(`/v1/sites/${siteId}/pages`, data),
  deleteSitePage: (siteId: Id, pageId: Id) => instance.delete(`/v1/sites/${siteId}/pages/${pageId}`),
  fetchSitePage: (siteId: Id, pageId: Id) => instance.post(`/v1/sites/${siteId}/pages/${pageId}/fetch`),
  getSitePageSource: (siteId: Id, pageId: Id) => instance.get(`/v1/sites/${siteId}/pages/${pageId}/source`),

  // Dead links
  getDeadLinks: (siteId: Id, params?: Record<string, any>) => instance.get(`/v1/sites/${siteId}/deadlinks`, { params }),
  retryDeadLinks: (siteId: Id) => instance.post(`/v1/sites/${siteId}/deadlinks/retry`),
  getDeadLinkProgress: (siteId: Id) => instance.get(`/v1/sites/${siteId}/deadlinks/progress`),
  startDeadLinkCheck: (siteId: Id) => instance.post(`/v1/sites/${siteId}/deadlinks/check`),

  // ── 4-platform OAuth + sync (data management) ──────────────────
  // GSC
  getGSCAuthURL: () => instance.get('/v1/oauth/gsc'),
  syncGSC: (siteId: Id) => instance.post(`/v1/sites/${siteId}/gsc/sync`),
  getGSCData: (siteId: Id, days?: number, config?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/gsc`, { params: { days }, ...(config || {}) }),

  // GA4
  getGAAuthURL: () => instance.get('/v1/oauth/ga'),
  syncGA: (siteId: Id) => instance.post(`/v1/sites/${siteId}/ga/sync`),
  getGAData: (siteId: Id, config?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/ga`, config),

  // Yandex
  getYandexAuthURL: () => instance.get('/v1/oauth/yandex'),
  syncYandex: (siteId: Id) => instance.post(`/v1/sites/${siteId}/yandex/sync`),
  getYandexData: (siteId: Id, config?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/yandex`, config),

  // Bing
  syncBing: (siteId: Id) => instance.post(`/v1/sites/${siteId}/bing/sync`),
  getBingData: (siteId: Id, config?: Record<string, any>) =>
    instance.get(`/v1/sites/${siteId}/bing`, config),

  // Data-management page dashboards
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

  // Settings (global)
  getSettings: () => instance.get('/v1/settings'),
  getPublicSettings: () => instance.get('/v1/settings/public'),
  saveSettings: (data: Record<string, string>) => instance.put('/v1/settings', data),

  // Personal config (singleton — backend hardcodes user_id=1)
  getMyAIConfig: () => instance.get('/v1/auth/my-ai-config'),
  saveMyAIConfig: (data: Record<string, any>) => instance.put('/v1/auth/my-ai-config', data),
  testMyAI: () => instance.post('/v1/auth/test-my-ai'),

  getMyOAuthConfigs: () => instance.get('/v1/auth/my-oauth-config'),
  saveMyOAuthConfig: (data: Record<string, any>) => instance.put('/v1/auth/my-oauth-config', data),
  unbindMyOAuthToken: (service: string) => instance.delete('/v1/auth/my-oauth-token', { params: { service } }),

  getMyBing: () => instance.get('/v1/auth/my-bing'),
  saveMyBing: (data: Record<string, any>) => instance.put('/v1/auth/my-bing', data),
}

export default api
