import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '../api'

function getCookie(name: string): string {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'))
  return match ? decodeURIComponent(match[2]) : ''
}

export interface User {
  id: number
  name: string
  email: string
  role: string
  telegram_id?: string
}

export const useUserStore = defineStore('user', () => {
  const user = ref<User | null>(null)

  function isLoggedIn(): boolean {
    return !!getCookie('csrf_token')
  }

  async function login(email: string, password: string) {
    const res = await api.login({ email, password })
    user.value = res.data.user
    return res
  }

  async function fetchUser() {
    if (!isLoggedIn()) return
    try {
      const res = await api.getMe()
      user.value = res.data
    } catch {
      logout()
    }
  }

  function logout() {
    user.value = null
    document.cookie = 'csrf_token=; path=/; max-age=0'
  }

  const isAdmin = (): boolean => user.value?.role === 'admin'

  return { user, login, fetchUser, logout, isAdmin, isLoggedIn }
})
