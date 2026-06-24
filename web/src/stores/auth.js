// Centralized state (Pinia). One store per concern (§3.3).
import { defineStore } from 'pinia'
import { api } from '../api/client'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null,        // { login } once authenticated
    ready: false,      // initial me() check completed
    error: null
  }),
  getters: {
    isAuthenticated: (s) => !!s.user
  },
  actions: {
    async init() {
      try {
        this.user = await api.me()
      } catch {
        this.user = null
      } finally {
        this.ready = true
      }
    },
    async login(login, password) {
      this.error = null
      try {
        await api.login(login, password)
        this.user = await api.me()
        return true
      } catch (e) {
        this.error = e.message
        return false
      }
    },
    async logout() {
      try { await api.logout() } catch { /* ignore */ }
      this.user = null
    },
    async changeCredentials(currentPassword, login, newPassword) {
      this.error = null
      try {
        await api.changeCredentials(currentPassword, login, newPassword)
        if (login) this.user = { login }
        return true
      } catch (e) {
        this.error = e.message
        return false
      }
    }
  }
})
