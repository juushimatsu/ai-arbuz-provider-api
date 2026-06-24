<script setup>
import { ref } from 'vue'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const currentPassword = ref('')
const login = ref('')
const newPassword = ref('')
const confirm = ref('')
const saved = ref(false)
const localError = ref(null)
const guardMode = ref('…')

// Response guard mode is read-only here (configured via ARBUZ_GUARD_MODE).
onMounted(async () => {
  try {
    const res = await fetch('/api/health')
    const j = await res.json()
    guardMode.value = j.guard || 'off'
  } catch {
    guardMode.value = 'unknown'
  }
})

async function save() {
  localError.value = null
  saved.value = false
  if (!currentPassword.value) {
    localError.value = 'current password required'
    return
  }
  if (newPassword.value && newPassword.value !== confirm.value) {
    localError.value = 'passwords do not match'
    return
  }
  const ok = await auth.changeCredentials(
    currentPassword.value,
    login.value || undefined,
    newPassword.value || undefined
  )
  if (ok) {
    saved.value = true
    currentPassword.value = ''
    newPassword.value = ''
    confirm.value = ''
  } else {
    localError.value = auth.error
  }
}
</script>

<template>
  <div class="page">
    <h2>SETTINGS</h2>

    <div class="card">
      <div class="label" style="margin-bottom: var(--sp-4)">ACCOUNT&nbsp;//&nbsp;CREDENTIALS</div>
      <div class="field"><label>CURRENT_PASSWORD</label><input v-model="currentPassword" type="password" autocomplete="current-password" /></div>
      <div class="field"><label>LOGIN</label><input v-model="login" :placeholder="auth.user?.login" /></div>
      <div class="field"><label>NEW_PASSWORD (min 8)</label><input v-model="newPassword" type="password" /></div>
      <div class="field"><label>CONFIRM</label><input v-model="confirm" type="password" /></div>
      <div v-if="localError" class="login-err">[ERR] {{ localError }}</div>
      <div v-if="saved" class="ok-msg">[OK] credentials updated</div>
      <button class="btn" @click="save" style="margin-top: var(--sp-3)">> update</button>
    </div>

    <div class="card">
      <div class="label" style="margin-bottom: var(--sp-3)">SYSTEM&nbsp;//&nbsp;INFO</div>
      <div class="kv">
        <span class="k">VERSION</span><span class="v">1.0.0</span>
        <span class="k">USER</span><span class="v">{{ auth.user?.login }}</span>
        <span class="k">RESPONSE_GUARD</span><span class="v" :class="{ guardOff: guardMode === 'off' }">{{ guardMode }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.card { max-width: 480px; padding: var(--sp-4); background: var(--bg-panel); border: 1px solid var(--line); border-radius: var(--r-md); margin-bottom: var(--sp-4); }
.ok-msg { color: var(--accent); margin-top: var(--sp-2); }
.kv { display: grid; grid-template-columns: 100px 1fr; gap: var(--sp-1) var(--sp-3); }
.kv .k { color: var(--fg-mute); font-size: var(--fs-xs); text-transform: uppercase; }
.kv .guardOff { color: var(--danger, #e06c75); }
</style>