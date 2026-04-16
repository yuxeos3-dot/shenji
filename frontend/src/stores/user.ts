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

export const useUserStore = defineStore('user', () => {
  const user = ref<User | null>(null)
  const token = ref(localStorage.getItem('token') || '')

  async function login(email: string, password: string) {
    const res = await api.login({ email, password })
    token.value = res.data.token
    user.value = res.data.user
    localStorage.setItem('token', res.data.token)
    return res
  }

  async function fetchUser() {
    if (!token.value) return
    try {
      const res = await api.getMe()
      user.value = res.data
    } catch {
      logout()
    }
  }

  function logout() {
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
  }

  const isAdmin = (): boolean => user.value?.role === 'admin'

  return { user, token, login, fetchUser, logout, isAdmin }
})
