<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const login = ref('')
const password = ref('')
const submitting = ref(false)

async function submit() {
  submitting.value = true
  const ok = await auth.login(login.value, password.value)
  submitting.value = false
  if (ok) {
    const next = route.query.next || '/dashboard'
    router.push(next)
  }
}
</script>

<template>
  <div class="login-wrap">
    <form class="login-card" @submit.prevent="submit">
      <div class="brand">AI&nbsp;ARBUZ // PROVIDER&nbsp;API</div>
      <div class="label" style="margin-bottom: var(--sp-5)">AUTH&nbsp;//&nbsp;ACCESS&nbsp;REQUIRED</div>

      <div class="field">
        <label>LOGIN</label>
        <input v-model="login" autocomplete="username" autofocus />
      </div>
      <div class="field">
        <label>PASSWORD</label>
        <input v-model="password" type="password" autocomplete="current-password" />
      </div>

      <div v-if="auth.error" class="login-err">[ERR] {{ auth.error }}</div>

      <button class="btn" type="submit" :disabled="submitting" style="width:100%; margin-top: var(--sp-3)">
        {{ submitting ? 'authenticating…' : '> authenticate' }}
      </button>
    </form>
  </div>
</template>

<style scoped>
.login-wrap {
  height: 100vh;
  display: flex; align-items: center; justify-content: center;
  background: radial-gradient(ellipse at center, var(--bg-elev), var(--bg) 70%);
}
.login-card {
  width: 360px;
  padding: var(--sp-6);
  background: var(--bg-panel);
  border: 1px solid var(--line);
  border-radius: var(--r-md);
  box-shadow: var(--glow);
}
.brand { color: var(--accent); font-weight: 700; letter-spacing: 0.05em; font-size: var(--fs-lg); }
.login-err { color: var(--danger); font-size: var(--fs-sm); margin-top: var(--sp-2); }
</style>
