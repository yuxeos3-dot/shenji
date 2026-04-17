import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '../api'

// Version bumps: update this whenever a release is cut. Shown in the sidebar
// footer + login page so operators can confirm what's deployed.
export const APP_VERSION = 'v12.8'

/**
 * siteConfig holds globally-visible UI strings (window title, sidebar logo,
 * login banner, etc.) that come from the backend /settings endpoint.
 *
 * Load strategy: the public login page calls /settings/public (no auth) to
 * get the title; authenticated pages reuse the cached value. On save the
 * settings page calls refresh() so the new title propagates without a reload.
 */
export const useSiteConfigStore = defineStore('siteConfig', () => {
  const title = ref<string>('SEO 巡站管理系统') // safe fallback until first load
  const loaded = ref(false)

  async function refresh() {
    try {
      // Try the public endpoint first (works for logged-out users on /login).
      // Falls back to the admin endpoint for authed users.
      let res: any
      let src = 'public'
      try {
        res = await api.getPublicSettings()
      } catch {
        src = 'admin'
        res = await api.getSettings()
      }
      const t = (res?.data?.site_title || '').trim()
      if (t) {
        title.value = t
        // Diagnostic: make it obvious in devtools where the title came from
        // so we can tell whether a failed save is a read or write issue.
        console.debug(`[siteConfig] refreshed from ${src}: title=%o`, t)
      } else {
        console.warn(`[siteConfig] refresh via ${src} returned no site_title`, res?.data)
      }
      loaded.value = true
      // Keep document.title in sync — placeholder pageTitle handling in
      // Layout still shows the section name before the " — <site>" suffix.
      applyDocumentTitle()
    } catch (e) {
      console.warn('[siteConfig] refresh failed; keeping fallback title', e)
    }
  }

  function applyDocumentTitle(section?: string) {
    const base = title.value || 'SEO 巡站管理系统'
    document.title = section ? `${section} · ${base}` : base
  }

  return { title, loaded, refresh, applyDocumentTitle }
})
