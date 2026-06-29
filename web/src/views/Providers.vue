<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/client'
import StatusDot from '../components/StatusDot.vue'

const list = ref([])
const loading = ref(true)
const error = ref(null)
const editing = ref(null) // null | {id?, name, ...}

async function load() {
  loading.value = true
  try { list.value = await api.listProviders() } catch (e) { error.value = e.message }
  finally { loading.value = false }
}

function newProvider() {
  modelRouting.value = []; upstreamKeys.value = []
  editing.value = { name: '', strategy: 'failover', global_models: [], fallback_models: [], status: 'active', _modelsText: '', _fallbackText: '' }
}
function edit(p) {
  editing.value = { ...p, _modelsText: (p.global_models || []).join('\n'), _fallbackText: (p.fallback_models || []).join('\n') }
  loadRouting(p.id)
}

async function save() {
  const e = editing.value
  const payload = {
    name: e.name,
    strategy: e.strategy,
    global_models: splitLines(e._modelsText),
    fallback_models: splitLines(e._fallbackText),
    status: e.status
  }
  try {
    if (e.id) await api.updateProvider(e.id, payload)
    else await api.createProvider(payload)
    editing.value = null
    await load()
  } catch (err) { error.value = err.message }
}

async function remove(id) {
  if (!confirm('Delete provider?')) return
  try { await api.deleteProvider(id); await load() } catch (e) { error.value = e.message }
}

const searching = ref(false)
async function autoSearchModels() {
  const e = editing.value
  if (!e || !e.id) { error.value = 'save the provider and add upstream keys first, then auto-search'; return }
  searching.value = true
  error.value = null
  try {
    const res = await api.searchProviderModels(e.id)
    if (res.models && res.models.length) {
      const merged = Array.from(new Set([...splitLines(e._modelsText), ...res.models]))
      e._modelsText = merged.join('\n')
    } else {
      error.value = res.error || 'no models reported by this provider\'s keys'
    }
  } catch (err) { error.value = err.message }
  finally { searching.value = false }
}

// --- model routing: real availability + "model -> preferred key" mapping ---
const modelRouting = ref([])
const upstreamKeys = ref([])
const routingLoading = ref(false)

async function loadRouting(id) {
  if (!id) { modelRouting.value = []; upstreamKeys.value = []; return }
  routingLoading.value = true
  try {
    const [models, keys] = await Promise.all([api.providerModels(id), api.listUpstreams(id)])
    modelRouting.value = models || []
    upstreamKeys.value = keys || []
  } catch (e) { error.value = e.message }
  finally { routingLoading.value = false }
}

function keyLabel(id) {
  const k = upstreamKeys.value.find(x => x.id === id)
  if (!k) return id ? id.slice(0, 6) : ''
  return (k.name && k.name.trim()) ? k.name : id.slice(0, 6)
}

async function choosePref(model, keyId) {
  const e = editing.value
  try {
    if (keyId) await api.setModelPref(e.id, model, keyId)
    else await api.deleteModelPref(e.id, model)
    await loadRouting(e.id)
  } catch (err) { error.value = err.message }
}

function splitLines(t) { return (t || '').split('\n').map(s => s.trim()).filter(Boolean) }
onMounted(load)
</script>

<template>
  <div class="page">
    <h2>PROVIDERS</h2>
    <div v-if="error" class="login-err">[ERR] {{ error }}</div>

    <button class="btn" @click="newProvider">+ new provider</button>

    <table class="feed" style="margin-top: var(--sp-4)">
      <thead><tr><th>NAME</th><th>STRATEGY</th><th>MODELS</th><th>FALLBACK</th><th>STATUS</th><th></th></tr></thead>
      <tbody>
        <tr v-for="p in list" :key="p.id">
          <td>{{ p.name }}</td>
          <td><span class="tag">{{ p.strategy }}</span></td>
          <td>{{ (p.global_models || []).length }}</td>
          <td>{{ (p.fallback_models || []).length }}</td>
          <td><StatusDot :status="p.status === 'active' ? 'ok' : 'off'" /> {{ p.status }}</td>
          <td>
            <button class="btn" @click="edit(p)">edit</button>
            <button class="btn danger" @click="remove(p.id)">del</button>
          </td>
        </tr>
        <tr v-if="!list.length"><td colspan="6" class="empty">no providers</td></tr>
      </tbody>
    </table>

    <!-- inline editor (ponytail: modal-free; just a panel) -->
    <div v-if="editing" class="modal-bg" @click.self="editing = null">
      <div class="modal">
        <h3>{{ editing.id ? 'EDIT PROVIDER' : 'NEW PROVIDER' }}</h3>
        <div class="field"><label>NAME</label><input v-model="editing.name" /></div>
        <div class="field"><label>STRATEGY</label>
          <select v-model="editing.strategy"><option value="failover">failover</option><option value="round_robin">round_robin</option></select>
        </div>
        <div class="field">
          <label>GLOBAL_MODELS (one per line)
            <button type="button" class="btn mini-btn" :disabled="searching || !editing.id" @click="autoSearchModels">{{ searching ? 'searching…' : 'auto-search models' }}</button>
          </label>
          <textarea v-model="editing._modelsText" rows="4"></textarea>
          <small v-if="!editing.id" class="dim">save provider &amp; add keys to enable auto-search</small>
        </div>
        <div class="field"><label>FALLBACK_MODELS (one per line)</label><textarea v-model="editing._fallbackText" rows="3"></textarea></div>
        <div v-if="editing.id" class="field">
          <label>MODEL ROUTING (model &rarr; preferred key)</label>
          <small class="dim" v-if="routingLoading">loading…</small>
          <small class="dim" v-else-if="!modelRouting.length">no models served by any live key</small>
          <table v-else class="routing">
            <thead><tr><th>model</th><th>preferred key</th></tr></thead>
            <tbody>
              <tr v-for="m in modelRouting" :key="m.model">
                <td>{{ m.model }}<span v-if="!m.keys || !m.keys.length" class="warn"> (no live key)</span></td>
                <td>
                  <select :value="m.preferred_key_id || ''" @change="choosePref(m.model, $event.target.value)">
                    <option value="">auto (priority)</option>
                    <option v-for="kid in (m.keys || [])" :key="kid" :value="kid">{{ keyLabel(kid) }}</option>
                  </select>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
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
.modal { width: 480px; max-height: 80vh; overflow: auto; padding: var(--sp-5); background: var(--bg-panel); border: 1px solid var(--accent-2); border-radius: var(--r-md); box-shadow: var(--glow); }
.modal h3 { margin: 0 0 var(--sp-4); color: var(--accent); }
.actions { display: flex; gap: var(--sp-3); margin-top: var(--sp-4); }
.routing { width: 100%; border-collapse: collapse; margin-top: var(--sp-2); font-size: 0.85em; }
.routing th, .routing td { text-align: left; padding: var(--sp-1) var(--sp-2); border-bottom: 1px solid var(--border, #333); }
.routing select { width: 100%; }
.warn { color: var(--warn, #d99); }
</style>