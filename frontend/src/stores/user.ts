import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '../api'

export interface User {
  id: number
  name: string
}

export const useUserStore = defineStore('user', () => {
  const user = ref<User | null>(null)
  const authed = ref(false)

  async function login(password: string) {
    const res = await api.login({ password })
    user.value = res.data.user
    authed.value = true
    localStorage.removeItem('token')
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
  }

  return { user, authed, login, fetchUser, logout }
})
