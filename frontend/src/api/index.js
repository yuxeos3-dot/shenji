import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

const api = axios.create({
  baseURL: '/api',
  timeout: 120000,
})

api.interceptors.request.use(config => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  response => {
    const data = response.data
    if (data.code !== 0) {
      ElMessage.error(data.error || '请求失败')
      return Promise.reject(new Error(data.error))
    }
    return data
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

export default {
  // Auth
  login: (data) => api.post('/auth/login', data),
  logout: () => api.post('/v1/auth/logout'),
  changePassword: (data) => api.post('/v1/auth/change-password', data),
  getMe: () => api.get('/v1/auth/me'),

  // Users
  listUsers: () => api.get('/v1/users'),
  createUser: (data) => api.post('/v1/users', data),
  updateUser: (id, data) => api.put(`/v1/users/${id}`, data),
  deleteUser: (id) => api.delete(`/v1/users/${id}`),

  // Sites
  listSites: () => api.get('/v1/sites'),
  createSite: (data) => api.post('/v1/sites', data),
  getSite: (id) => api.get(`/v1/sites/${id}`),
  updateSite: (id, data) => api.put(`/v1/sites/${id}`, data),
  deleteSite: (id) => api.delete(`/v1/sites/${id}`),

  // Audit
  triggerAudit: (siteId) => api.post(`/v1/sites/${siteId}/audit`),
  listAudits: (siteId) => api.get(`/v1/sites/${siteId}/audits`),
  deleteAudit: (taskId) => api.delete(`/v1/audits/${taskId}`),
  getAudit: (taskId) => api.get(`/v1/audits/${taskId}`),
  getAuditPages: (taskId) => api.get(`/v1/audits/${taskId}/pages`),
  downloadAuditReport: (taskId) => `/api/v1/audits/${taskId}/report/download`,

  // Dead Link Check
  getDeadLinks: (siteId) => api.get(`/v1/sites/${siteId}/deadlinks`),
  getDeadLinkProgress: (siteId) => api.get(`/v1/sites/${siteId}/deadlinks/progress`),
  startDeadLinkCheck: (siteId) => api.post(`/v1/sites/${siteId}/deadlinks/check`),

  // Content Rewrite
  rewriteContent: (data) => api.post('/v1/rewrite', data),

  // External Links
  getExternalLinks: (siteId) => api.get(`/v1/sites/${siteId}/extlinks`),
  createExternalLink: (data) => api.post('/v1/extlinks', data),
  updateExternalLink: (id, data) => api.put(`/v1/extlinks/${id}`, data),
  deleteExternalLink: (id) => api.delete(`/v1/extlinks/${id}`),
  checkExternalLink: (id) => api.post(`/v1/extlinks/${id}/check`),

  // Site Pages
  listSitePages: (siteId) => api.get(`/v1/sites/${siteId}/pages`),
  createSitePage: (siteId, data) => api.post(`/v1/sites/${siteId}/pages`, data),
  deleteSitePage: (siteId, pageId) => api.delete(`/v1/sites/${siteId}/pages/${pageId}`),
  fetchSitePage: (siteId, pageId) => api.post(`/v1/sites/${siteId}/pages/${pageId}/fetch`),
  getSitePageSource: (siteId, pageId) => api.get(`/v1/sites/${siteId}/pages/${pageId}/source`),

  // GSC
  getGSCAuthURL: () => api.get('/v1/oauth/gsc'),
  syncGSC: (siteId) => api.post(`/v1/sites/${siteId}/gsc/sync`),
  getGSCData: (siteId, days) => api.get(`/v1/sites/${siteId}/gsc`, { params: { days } }),

  // GA
  getGAAuthURL: () => api.get('/v1/oauth/ga'),
  syncGA: (siteId) => api.post(`/v1/sites/${siteId}/ga/sync`),
  getGAData: (siteId) => api.get(`/v1/sites/${siteId}/ga`),

  // Yandex
  getYandexAuthURL: () => api.get('/v1/oauth/yandex'),
  syncYandex: (siteId) => api.post(`/v1/sites/${siteId}/yandex/sync`),
  getYandexData: (siteId) => api.get(`/v1/sites/${siteId}/yandex`),

  // Reports
  listReports: (params) => api.get('/v1/reports', { params }),
  submitReport: (data) => api.post('/v1/reports', data),
  getReport: (date) => api.get(`/v1/reports/${date}`),
  exportReports: (params) => api.get('/v1/reports/export', { params }),

  // Settings
  getSettings: () => api.get('/v1/settings'),
  saveSettings: (data) => api.put('/v1/settings', data),
  testAI: () => api.post('/v1/settings/test-ai'),
  testTelegram: () => api.post('/v1/settings/test-telegram'),
}
