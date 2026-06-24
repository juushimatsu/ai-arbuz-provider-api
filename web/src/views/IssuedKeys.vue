<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/client'
import StatusDot from '../components/StatusDot.vue'

const list = ref([])
const providers = ref([])
const loading = ref(true)
const error = ref(null)
const editing = ref(null)
const justCreated = ref(null) // shows the one-time token

const WINDOWS = ['5h', '24h', '30d']

async function load() {
  loading.value = true
  try {
    const [iss, provs] = await Promise.all([api.listIssued(), api.listProviders()])
    list.value = iss
    providers.value = provs
  } catch (e) { error.value = e.message }
  finally { loading.value = false }
}

function newKey() {
  editing.value = {
    provider_id: '', name: '', valid_days: 30, status: 'active',
    limits: { tokens: { '5h': 0, '24h': 0, '30d': 0 }, requests: { '5h': 0, '24h': 0, '30d': 0 } }
  }
}

async function save() {
  const e = editing.value
  const payload = {
    provider_id: e.provider_id, name: e.name, valid_days: Number(e.valid_days) || 0,
    status: e.status, limits: stripZeros(e.limits)
  }
  try {
    const res = await api.createIssued(payload)
    editing.value = null
    justCreated.value = res // contains the one-time token
    await load()
  } catch (err) { error.value = err.message }
}

async function revoke(id) {
  if (!confirm('Revoke key? It will stop working immediately.')) return
  try { await api.revokeIssued(id); await load() } catch (e) { error.value = e.message }
}
async function remove(id) {
  if (!confirm('Permanently delete key?')) return
  try { await api.deleteIssued(id); await load() } catch (e) { error.value = e.message }
}

function stripZeros(limits) {
  const out = { tokens: {}, requests: {} }
  for (const w of WINDOWS) {
    const t = Number(limits.tokens?.[w]) || 0
    const r = Number(limits.requests?.[w]) || 0
    if (t > 0) out.tokens[w] = t
    if (r > 0) out.requests[w] = r
  }
  return out
}
function provName(id) { return providers.value.find(p => p.id === id)?.name || id?.slice(-6) || '—' }
function fmtDate(s) { return s ? new Date(s).toLocaleString() : '—' }
onMounted(load)
</script>

<template>
  <div class="page">
    <h2>ISSUED&nbsp;KEYS</h2>
    <div v-if="error" class="login-err">[ERR] {{ error }}</div>

    <button class="btn" @click="newKey">+ generate key</button>

    <table class="feed" style="margin-top: var(--sp-4)">
      <thead><tr><th>NAME</th><th>PROVIDER</th><th>LIMITS</th><th>EXPIRES</th><th>STATUS</th><th></th></tr></thead>
      <tbody>
        <tr v-for="k in list" :key="k.id">
          <td>{{ k.name }}</td>
          <td>{{ provName(k.provider_id) }}</td>
          <td class="dim">
            <span v-if="Object.keys(k.limits?.tokens||{}).length || Object.keys(k.limits?.requests||{}).length">
              {{ Object.keys(k.limits?.tokens||{}).length + Object.keys(k.limits?.requests||{}).length }} caps
            </span>
            <span v-else>unlimited</span>
          </td>
          <td class="dim">{{ fmtDate(k.expires_at) }}</td>
          <td><StatusDot :status="k.status === 'active' ? 'ok' : 'err'" /> {{ k.status }}</td>
          <td>
            <button class="btn danger" v-if="k.status==='active'" @click="revoke(k.id)">revoke</button>
            <button class="btn danger" @click="remove(k.id)">del</button>
          </td>
        </tr>
        <tr v-if="!list.length"><td colspan="6" class="empty">no issued keys</td></tr>
      </tbody>
    </table>

    <!-- one-time token reveal -->
    <div v-if="justCreated" class="modal-bg" @click.self="justCreated = null">
      <div class="modal">
        <h3>KEY&nbsp;CREATED</h3>
        <div class="warn-line">⚠ shown once — copy now</div>
        <code class="token-box">{{ justCreated.token }}</code>
        <div class="kv" style="margin-top: var(--sp-3)">
          <span class="k">ID</span><span class="v">{{ justCreated.id }}</span>
          <span class="k">EXPIRES</span><span class="v">{{ fmtDate(justCreated.expires_at) }}</span>
        </div>
        <div class="actions"><button class="btn" @click="justCreated = null">done</button></div>
      </div>
    </div>

    <div v-if="editing" class="modal-bg" @click.self="editing = null">
      <div class="modal">
        <h3>NEW&nbsp;ISSUED&nbsp;KEY</h3>
        <div class="field"><label>NAME</label><input v-model="editing.name" /></div>
        <div class="field"><label>PROVIDER</label>
          <select v-model="editing.provider_id">
            <option v-for="p in providers" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
        </div>
        <div class="field"><label>VALID_DAYS (0 = never)</label><input v-model.number="editing.valid_days" type="number" /></div>

        <div class="label" style="margin: var(--sp-3) 0 var(--sp-2)">TOKEN LIMITS (0 = no cap)</div>
        <table class="mini">
          <thead><tr><th>WINDOW</th><th>TOKENS</th><th>REQUESTS</th></tr></thead>
          <tbody>
            <tr v-for="w in WINDOWS" :key="w">
              <td>{{ w }}</td>
              <td><input v-model.number="editing.limits.tokens[w]" type="number" /></td>
              <td><input v-model.number="editing.limits.requests[w]" type="number" /></td>
            </tr>
          </tbody>
        </table>

        <div class="actions">
          <button class="btn" @click="save">generate</button>
          <button class="btn danger" @click="editing = null">cancel</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-bg { position: fixed; inset: 0; background: rgba(0,0,0,0.6); display: flex; align-items: center; justify-content: center; z-index: 10; }
.modal { width: 520px; max-height: 85vh; overflow: auto; padding: var(--sp-5); background: var(--bg-panel); border: 1px solid var(--accent-2); border-radius: var(--r-md); box-shadow: var(--glow); }
.modal h3 { margin: 0 0 var(--sp-3); color: var(--accent); }
.actions { display: flex; gap: var(--sp-3); margin-top: var(--sp-4); }
.dim { color: var(--fg-mute); }
.warn-line { color: var(--warn); margin-bottom: var(--sp-2); }
.token-box { display: block; padding: var(--sp-3); background: var(--bg-elev); border: 1px solid var(--accent-2); color: var(--accent); word-break: break-all; }
.mini { width: 100%; border-collapse: collapse; }
.mini th { text-align: left; color: var(--fg-mute); font-size: var(--fs-xs); padding: var(--sp-1); border-bottom: 1px solid var(--line); }
.mini td { padding: var(--sp-1); }
.mini input { width: 100%; padding: var(--sp-1) var(--sp-2); background: var(--bg-elev); border: 1px solid var(--line); color: var(--fg); font-family: var(--font-mono); }
</style>
