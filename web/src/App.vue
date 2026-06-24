<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from './stores/auth'
import { routes } from './router'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const isLogin = computed(() => route.name === 'login')
const navItems = computed(() =>
  routes.filter(r => r.meta?.nav).map(r => ({ to: r.path, label: r.meta.nav }))
)

const sectionLabel = computed(() => route.meta?.nav || route.name || '')
</script>

<template>
  <div v-if="isLogin">
    <router-view />
  </div>
  <div v-else class="app-shell">
    <header class="topbar">
      <span class="brand">AI&nbsp;ARBUZ // PROVIDER&nbsp;API</span>
      <span class="label">{{ sectionLabel.toUpperCase() }}</span>
      <span class="live">LIVE_FEED_ACTIVE</span>
    </header>

    <div class="app-body">
      <aside class="col col-sidebar">
        <nav class="nav">
          <div class="nav-section">SYS_NAV</div>
          <router-link
            v-for="item in navItems"
            :key="item.to"
            :to="item.to"
            class="nav-item"
          >{{ item.label }}</router-link>
          <div class="nav-section" style="margin-top: var(--sp-4)">SESSION</div>
          <a class="nav-item" @click="auth.logout().then(() => router.push('/login'))">[ logout ]</a>
        </nav>
      </aside>

      <main class="col col-main">
        <router-view />
      </main>

      <aside class="col col-inspector">
        <router-view name="inspector" />
      </aside>
    </div>

    <footer class="footerbar">
      <span>SYS://ok</span>
      <span>v1.0.0</span>
      <span style="margin-left:auto">user: {{ auth.user?.login || '—' }}</span>
    </footer>
  </div>
</template>
