import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const routes = [
  { path: '/login', name: 'login', component: () => import('../views/Login.vue') },
  { path: '/', redirect: '/dashboard' },
  { path: '/dashboard', name: 'dashboard', component: () => import('../views/Dashboard.vue'), meta: { nav: 'Статистика' } },
  { path: '/providers', name: 'providers', component: () => import('../views/Providers.vue'), meta: { nav: 'Провайдеры' } },
  { path: '/upstreams', name: 'upstreams', component: () => import('../views/UpstreamKeys.vue'), meta: { nav: 'Внешние ключи' } },
  { path: '/issued', name: 'issued', component: () => import('../views/IssuedKeys.vue'), meta: { nav: 'Сгенерированные ключи' } },
  { path: '/logs', name: 'logs', component: () => import('../views/Logs.vue'), meta: { nav: 'Лента запросов' } },
  { path: '/checker', name: 'checker', component: () => import('../views/Checker.vue'), meta: { nav: 'API-чекер' } },
  { path: '/mcp', name: 'mcp', component: () => import('../views/Mcp.vue'), meta: { nav: 'MCP' } },
  { path: '/settings', name: 'settings', component: () => import('../views/Settings.vue'), meta: { nav: 'Настройки' } }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// Auth guard: every route except /login requires an authenticated session.
router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.ready) await auth.init()
  if (to.name !== 'login' && !auth.isAuthenticated) {
    return { name: 'login', query: { next: to.fullPath } }
  }
  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'dashboard' }
  }
})

export default router
export { routes }
