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
  editing.value = { name: '', strategy: 'failover', global_models: [], fallback_models: [], status: 'active', _modelsText: '', _fallbackText: '' }
}
function edit(p) {
  editing.value = { ...p, _modelsText: (p.global_models || []).join('\n'), _fallbackText: (p.fallback_models || []).join('\n') }
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
          <select v-model="editing.strategy"><option value="failover">failover</option></select>
        </div>
        <div class="field"><label>GLOBAL_MODELS (one per line)</label><textarea v-model="editing._modelsText" rows="4"></textarea></div>
        <div class="field"><label>FALLBACK_MODELS (one per line)</label><textarea v-model="editing._fallbackText" rows="3"></textarea></div>
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
</style>
