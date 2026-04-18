import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '../api'

export interface User {
  id: number
  name: string
  email: string
  role: string
  telegram_id?: string
}

// Since moving to httpOnly auth cookies, the store no longer mirrors the JWT
// into localStorage. `authed` tracks whether a server session is known to
// exist (set after login, cleared on logout/401).
export const useUserStore = defineStore('user', () => {
  const user = ref<User | null>(null)
  const authed = ref(false)

  async function login(email: string, password: string) {
    const res = await api.login({ email, password })
    user.value = res.data.user
    authed.value = true
    // Clear legacy bearer token — the cookie is now the source of truth.
    localStorage.removeItem('token')
    // Product rule: every fresh login lands the user on a fully-collapsed
    // sidebar, regardless of what they had expanded in their previous
    // session (or what the previous user on this browser had expanded).
    // Layout.vue seeds collapsed=allGroups when no saved state exists, so
    // clearing the key is enough to restore the clean default.
    localStorage.removeItem('xunzhan:nav-collapsed-v3')
    return res
  }

  async function fetchUser() {
    try {
      const res = await api.getMe()
      user.value = res.data
      authed.value = true
    } catch {
      user.value = null
      authed.value = false
    }
  }

  async function logout() {
    try { await api.logout() } catch { /* server may already have cleared */ }
    user.value = null
    authed.value = false
    localStorage.removeItem('token')
    // Also drop the nav preference so the next person to log in on this
    // browser starts from the same clean collapsed state.
    localStorage.removeItem('xunzhan:nav-collapsed-v3')
  }

  const isAdmin = (): boolean => user.value?.role === 'admin'

  return { user, authed, login, fetchUser, logout, isAdmin }
})
