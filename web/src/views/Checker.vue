<script setup>
import { ref, onMounted, computed } from 'vue'
import { api } from '../api/client'
import StatusDot from '../components/StatusDot.vue'

const form = ref({ base_url: '', secret: '', format: 'openai', probes: ['ping', 'models', 'chat', 'embeddings'] })
const result = ref(null)
const history = ref([])
const loading = ref(false)
const error = ref(null)

const allProbes = [
  { k: 'ping', label: 'PING' },
  { k: 'models', label: 'MODELS' },
  { k: 'chat', label: 'CHAT' },
  { k: 'embeddings', label: 'EMBED' }
]

async function run() {
  loading.value = true
  error.value = null
  result.value = null
  try {
    result.value = await api.runChecker(form.value.base_url, form.value.secret, form.value.format, form.value.probes)
    await loadHistory()
  } catch (e) { error.value = e.message }
  finally { loading.value = false }
}

async function loadHistory() {
  try { history.value = await api.listCheckerRuns() } catch { /* ignore */ }
}

function toggleProbe(k) {
  const i = form.value.probes.indexOf(k)
  if (i >= 0) form.value.probes.splice(i, 1); else form.value.probes.push(k)
}

// Sort results: passed first, then by type (§4.10).
const sortedResults = computed(() => {
  if (!result.value?.results) return []
  return [...result.value.results].sort((a, b) => {
    const sa = a.status === 'active' ? 0 : 1
    const sb = b.status === 'active' ? 0 : 1
    if (sa !== sb) return sa - sb
    return String(a.kind).localeCompare(String(b.kind))
  })
})

function exportJSON() {
  if (!result.value) return
  const blob = new Blob([JSON.stringify(result.value, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url; a.download = 'checker-' + Date.now() + '.json'; a.click()
  URL.revokeObjectURL(url)
}
onMounted(loadHistory)
</script>

<template>
  <div class="page">
    <h2>API&nbsp;CHECKER</h2>
    <div v-if="error" class="login-err">[ERR] {{ error }}</div>

    <div class="checker-form">
      <div class="field"><label>BASE_URL</label><input v-model="form.base_url" placeholder="https://api.openai.com" /></div>
      <div class="field"><label>SECRET</label><input v-model="form.secret" type="password" /></div>
      <div class="field"><label>FORMAT</label>
        <select v-model="form.format"><option value="openai">openai</option><option value="anthropic">anthropic</option></select>
      </div>
      <div class="field">
        <label>PROBES</label>
        <div class="chips">
          <span v-for="p in allProbes" :key="p.k" class="chip" :class="{ active: form.probes.includes(p.k) }" @click="toggleProbe(p.k)">{{ p.label }}</span>
        </div>
      </div>
      <button class="btn" @click="run" :disabled="loading || !form.base_url || !form.secret">
        {{ loading ? 'running…' : '> run checks' }}
      </button>
      <button class="btn" v-if="result" @click="exportJSON">export JSON</button>
    </div>

    <div v-if="sortedResults.length" class="results">
      <div class="label" style="margin: var(--sp-4) 0 var(--sp-2)">RESULTS&nbsp;//&nbsp;{{ result.base_url }}</div>
      <div v-for="r in sortedResults" :key="r.kind" class="res-row">
        <StatusDot :status="r.status === 'active' ? 'ok' : 'err'" />
        <span class="res-kind">{{ r.kind }}</span>
        <span class="tag">{{ r.http_code || '—' }}</span>
        <span class="mono-num dim">{{ r.latency_ms }}ms</span>
        <span v-if="r.error" class="err">{{ r.error }}</span>
      </div>
    </div>

    <div v-if="history.length" class="history">
      <div class="label" style="margin: var(--sp-4) 0 var(--sp-2)">HISTORY</div>
      <div v-for="h in history.slice(0, 10)" :key="h.id" class="hist-row dim">
        <span>{{ new Date(h.started_at).toLocaleString() }}</span>
        <span>{{ h.base_url }}</span>
        <span>{{ h.results?.filter(r => r.status === 'active').length }}/{{ h.results?.length }} ok</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.checker-form { max-width: 480px; }
.results, .history { border-top: 1px solid var(--line); }
.res-row { display: flex; align-items: center; gap: var(--sp-3); padding: var(--sp-2) 0; border-bottom: 1px solid var(--line-soft); }
.res-kind { width: 90px; color: var(--fg); }
.hist-row { display: flex; gap: var(--sp-4); padding: var(--sp-1) 0; }
.dim { color: var(--fg-mute); }
.err { color: var(--danger); }
</style>
