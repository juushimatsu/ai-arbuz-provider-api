import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const routes = [
  { path: '/login', name: 'login', component: () => import('../views/Login.vue') },
  { path: '/', redirect: '/dashboard' },
  { path: '/dashboard', name: 'dashboard', component: () => import('../views/Dashboard.vue'), meta: { nav: 'РЎС‚Р°С‚РёСЃС‚РёРєР°' } },
  { path: '/providers', name: 'providers', component: () => import('../views/Providers.vue'), meta: { nav: 'РџСЂРѕРІР°Р№РґРµСЂС‹' } },
  { path: '/upstreams', name: 'upstreams', component: () => import('../views/UpstreamKeys.vue'), meta: { nav: 'Р’РЅРµС€РЅРёРµ РєР»СЋС‡Рё' } },
  { path: '/issued', name: 'issued', component: () => import('../views/IssuedKeys.vue'), meta: { nav: 'РЎРіРµРЅРµСЂРёСЂРѕРІР°РЅРЅС‹Рµ РєР»СЋС‡Рё' } },
  { path: '/logs', name: 'logs', component: () => import('../views/Logs.vue'), meta: { nav: 'Р›РµРЅС‚Р° Р·Р°РїСЂРѕСЃРѕРІ' } },
  { path: '/checker', name: 'checker', component: () => import('../views/Checker.vue'), meta: { nav: 'API-С‡РµРєРµСЂ' } },
  { path: '/mcp', name: 'mcp', component: () => import('../views/Mcp.vue'), meta: { nav: 'MCP' } },
  { path: '/integration', name: 'integration', component: () => import('../views/Integration.vue'), meta: { nav: 'Интеграция' } },
  { path: '/settings', name: 'settings', component: () => import('../views/Settings.vue'), meta: { nav: 'РќР°СЃС‚СЂРѕР№РєРё' } }
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
