<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/client'
import StatusDot from '../components/StatusDot.vue'

const list = ref([])
const providers = ref([])
const loading = ref(true)
const error = ref(null)
const editing = ref(null)
const searchResults = ref(null)
const searching = ref(false)
const checkResults = ref(null)
const checking = ref(false)
async function checkAll(probes) {
  checking.value = true; error.value = null; checkResults.value = null
  try {
    const res = await api.checkAllUpstream(probes)
    checkResults.value = res.keys || []
  } catch (err) { error.value = err.message }
  finally { checking.value = false }
}

async function load() {
  loading.value = true
  try {
    const [ups, provs] = await Promise.all([api.listUpstreams(), api.listProviders()])
    list.value = ups
    providers.value = provs
  } catch (e) { error.value = e.message }
  finally { loading.value = false }
}

function newKey() {
  editing.value = { provider_id: '', name: '', base_url: '', format: 'openai', secret: '', models: [], use_global_models: true, priority: 0, status: 'active', _modelsText: '', upstream_limits: { tokens: {}, requests: {} } }
}
function edit(k) {
  editing.value = { ...k, secret: '', _modelsText: (k.models || []).join('\n') }
}

async function save() {
  const e = editing.value
  const payload = {
    provider_id: e.provider_id, name: e.name, base_url: e.base_url,
    format: e.format, secret: e.secret, models: splitLines(e._modelsText),
    use_global_models: e.use_global_models, priority: Number(e.priority) || 0,
    status: e.status, upstream_limits: e.upstream_limits || { tokens: {}, requests: {} }
  }
  try {
    if (e.id) await api.updateUpstream(e.id, payload)
    else await api.createUpstream(payload)
    editing.value = null
    await load()
  } catch (err) { error.value = err.message }
}

async function remove(id) {
  if (!confirm('Delete upstream key?')) return
  try { await api.deleteUpstream(id); await load() } catch (e) { error.value = e.message }
}

// Auto-search models from the upstream's /v1/models (§4.9).
async function autoSearch() {
  const e = editing.value
  if (!e.base_url || !e.secret) { error.value = 'base_url and secret required for search'; return }
  searching.value = true
  try {
    const res = await api.searchModels(e.base_url, e.secret, e.format)
    searchResults.value = res.models || []
  } catch (err) { error.value = err.message }
  finally { searching.value = false }
}
function togglePick(m) {
  const list = splitLines(editing.value._modelsText)
  const i = list.indexOf(m)
  if (i >= 0) list.splice(i, 1); else list.push(m)
  editing.value._modelsText = list.join('\n')
}
function picked(m) { return splitLines(editing.value._modelsText).includes(m) }

function splitLines(t) { return (t || '').split('\n').map(s => s.trim()).filter(Boolean) }
function provName(id) { return providers.value.find(p => p.id === id)?.name || id?.slice(-6) || '—' }
onMounted(load)
</script>

<template>
  <div class="page">
    <h2>UPSTREAM&nbsp;KEYS</h2>
    <div v-if="error" class="login-err">[ERR] {{ error }}</div>

    <div class="bar">
      <button class="btn" @click="newKey">+ add upstream key</button>
      <span class="dim">check all:</span>
      <button class="btn" :disabled="checking || !list.length" @click="checkAll(['models'])">models</button>
      <button class="btn" :disabled="checking || !list.length" @click="checkAll(['chat'])">chat</button>
      <button class="btn" :disabled="checking || !list.length" @click="checkAll(['models','chat'])">both</button>
      <span v-if="checking" class="dim">checking…</span>
    </div>

    <table class="feed" style="margin-top: var(--sp-4)">
      <thead><tr><th>NAME</th><th>PROVIDER</th><th>BASE_URL</th><th>FMT</th><th>MODELS</th><th>STATUS</th><th></th></tr></thead>
      <tbody>
        <tr v-for="k in list" :key="k.id">
          <td>{{ k.name }} <span class="dim">…{{ k.secret_tail }}</span></td>
          <td>{{ provName(k.provider_id) }}</td>
          <td class="dim">{{ k.base_url }}</td>
          <td><span class="tag">{{ k.format }}</span></td>
          <td>{{ k.use_global_models ? 'global' : (k.models || []).length }}</td>
          <td><StatusDot :status="k.status === 'active' ? 'ok' : 'off'" /> {{ k.status }}</td>
          <td>
            <button class="btn" @click="edit(k)">edit</button>
            <button class="btn danger" @click="remove(k.id)">del</button>
          </td>
        </tr>
        <tr v-if="!list.length"><td colspan="7" class="empty">no upstream keys</td></tr>
      </tbody>
    </table>

    <div v-if="checkResults" class="check-results">
      <h3>CHECK&nbsp;RESULTS</h3>
      <div v-for="kr in checkResults" :key="kr.id" class="cr-row">
        <span class="cr-name">{{ kr.name }}</span>
        <span v-for="r in kr.results" :key="r.kind" class="cr-probe">
          <StatusDot :status="r.status === 'active' ? 'ok' : 'err'" />
          {{ r.kind }}<template v-if="r.http_code"> · {{ r.http_code }}</template><template v-if="r.latency_ms"> · {{ r.latency_ms }}ms</template><template v-if="r.error"> · {{ r.error }}</template>
        </span>
      </div>
      <div v-if="!checkResults.length" class="dim">no keys</div>
    </div>

    <div v-if="editing" class="modal-bg" @click.self="editing = null">
      <div class="modal">
        <h3>{{ editing.id ? 'EDIT UPSTREAM' : 'NEW UPSTREAM' }}</h3>
        <div class="field"><label>NAME</label><input v-model="editing.name" /></div>
        <div class="field"><label>PROVIDER</label>
          <select v-model="editing.provider_id">
            <option v-for="p in providers" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
        </div>
        <div class="field"><label>BASE_URL</label><input v-model="editing.base_url" placeholder="https://api.openai.com" /></div>
        <div class="field"><label>FORMAT</label>
          <select v-model="editing.format"><option value="openai">openai</option><option value="anthropic">anthropic</option></select>
        </div>
        <div class="field"><label>SECRET{{ editing.id ? ' (leave blank to keep)' : '' }}</label><input v-model="editing.secret" type="password" /></div>
        <div class="field">
          <label><input type="checkbox" v-model="editing.use_global_models" style="width:auto;margin-right:var(--sp-2)" /> USE_GLOBAL_MODELS</label>
        </div>
        <div class="field">
          <label>MODELS (one per line)</label>
          <textarea v-model="editing._modelsText" rows="4" :disabled="editing.use_global_models"></textarea>
          <button class="btn" @click="autoSearch" :disabled="searching" style="margin-top:var(--sp-2)">
            {{ searching ? 'searching…' : '> auto-search models' }}
          </button>
          <div v-if="searchResults" class="picker">
            <div v-for="m in searchResults" :key="m" class="pick" :class="{ on: picked(m) }" @click="togglePick(m)">{{ m }}</div>
            <div v-if="!searchResults.length" class="dim">no models found</div>
          </div>
        </div>
        <div class="field"><label>PRIORITY (failover order)</label><input v-model.number="editing.priority" type="number" /></div>
        <div class="field"><label>STATUS</label>
          <select v-model="editing.status"><option value="active">active</option><option value="disabled">disabled</option></select>
        </div>
        <div class="actions">
          <button class="btn" @click="save">save</button>
          <button class="btn danger" @click="editing = null">cancel</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-bg { position: fixed; inset: 0; background: rgba(0,0,0,0.6); display: flex; align-items: center; justify-content: center; z-index: 10; }
.modal { width: 520px; max-height: 85vh; overflow: auto; padding: var(--sp-5); background: var(--bg-panel); border: 1px solid var(--accent-2); border-radius: var(--r-md); box-shadow: var(--glow); }
.modal h3 { margin: 0 0 var(--sp-4); color: var(--accent); }
.actions { display: flex; gap: var(--sp-3); margin-top: var(--sp-4); }
.dim { color: var(--fg-mute); }
.picker { margin-top: var(--sp-2); max-height: 140px; overflow: auto; border: 1px solid var(--line); padding: var(--sp-2); }
.pick { padding: var(--sp-1) var(--sp-2); cursor: pointer; color: var(--fg-dim); }
.pick:hover { background: var(--bg-hover); color: var(--fg); }
.pick.on { color: var(--accent); }
.pick.on::before { content: '[x] '; }
.pick:not(.on)::before { content: '[ ] '; }
.bar { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; }
.check-results { margin-top: var(--sp-4); border: 1px solid var(--line); padding: var(--sp-3); }
.check-results h3 { margin: 0 0 var(--sp-2); color: var(--accent); }
.cr-row { display: flex; gap: var(--sp-3); align-items: center; padding: var(--sp-1) 0; flex-wrap: wrap; }
.cr-name { min-width: 140px; font-family: var(--font-mono); }
.cr-probe { display: inline-flex; align-items: center; gap: var(--sp-1); color: var(--fg-dim); font-size: 0.9em; }
</style>
