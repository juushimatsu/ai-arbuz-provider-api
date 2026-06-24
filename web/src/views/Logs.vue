<script setup>
import { ref, onMounted, computed, watch } from 'vue'
import { api } from '../api/client'
import StatusDot from '../components/StatusDot.vue'

const logs = ref([])
const loading = ref(true)
const error = ref(null)
const selected = ref(null)
const selectedDetail = ref(null)

const filters = ref({ success: '', model: '', q: '', limit: 200 })
const chips = [
  { k: 'success', v: '', label: 'ALL' },
  { k: 'success', v: 'true', label: 'SUCCESS' },
  { k: 'success', v: 'false', label: 'ERROR' }
]

async function load() {
  loading.value = true
  error.value = null
  try {
    const params = { limit: filters.value.limit }
    if (filters.value.success) params.success = filters.value.success
    if (filters.value.model) params.model = filters.value.model
    logs.value = await api.listLogs(params)
  } catch (e) { error.value = e.message }
  finally { loading.value = false }
}

const filtered = computed(() => {
  if (!filters.value.q) return logs.value
  const q = filters.value.q.toLowerCase()
  return logs.value.filter(l =>
    (l.model || '').toLowerCase().includes(q) ||
    (l.id || '').toLowerCase().includes(q) ||
    (l.issued_key_id || '').toLowerCase().includes(q) ||
    (l.error_code || '').toLowerCase().includes(q)
  )
})

async function select(l) {
  selected.value = l
  selectedDetail.value = null
  try { selectedDetail.value = await api.getLog(l.id) } catch { selectedDetail.value = l }
}

function fmtTime(s) { return s ? new Date(s).toLocaleTimeString() : '—' }
function fmtMs(n) { return n != null ? n + 'ms' : '—' }
function chipActive(c) { return filters.value.success === c.v }

watch(() => [filters.value.success, filters.value.model], load)
onMounted(load)
</script>

<template>
  <div class="page">
    <div class="toolbar">
      <div class="chips">
        <span v-for="c in chips" :key="c.label" class="chip" :class="{ active: chipActive(c) }" @click="filters.success = c.v">{{ c.label }}</span>
      </div>
      <div class="searchline"><input v-model="filters.q" placeholder="filter feed…" /></div>
    </div>

    <div class="feed-wrap">
      <table class="feed">
        <thead><tr><th>T</th><th>STATUS</th><th>MODEL</th><th>KEY</th><th>TOK</th><th>TTFB</th><th>TIME</th></tr></thead>
        <tbody>
          <tr v-for="l in filtered" :key="l.id" :class="{ selected: selected?.id === l.id }" @click="select(l)">
            <td><span class="tag">{{ l.in_format }}</span></td>
            <td><StatusDot :status="l.success ? 'ok' : 'err'" /></td>
            <td>{{ l.model || '—' }}</td>
            <td class="dim">{{ (l.issued_key_id || '').slice(-6) }}</td>
            <td class="mono-num">{{ l.total_tokens || 0 }}</td>
            <td class="mono-num">{{ fmtMs(l.latency_ttfb_ms) }}</td>
            <td class="dim">{{ fmtTime(l.timestamp) }}</td>
          </tr>
          <tr v-if="!filtered.length"><td colspan="7" class="empty">{{ loading ? 'loading…' : 'no logs' }}</td></tr>
        </tbody>
      </table>
    </div>

    <div v-if="error" class="login-err">[ERR] {{ error }}</div>
  </div>

  <!-- Inspector (right column) — rendered via teleport into the inspector slot.
       ponytail: simpler than named-router-view plumbing for a per-page detail. -->
  <Teleport to=".col-inspector">
    <div class="inspector">
      <h3>INSPECTOR</h3>
      <div v-if="!selected" class="dim">select a row</div>
      <template v-else>
        <div class="kv">
          <span class="k">ID</span><span class="v">{{ selected.id }}</span>
          <span class="k">STATUS</span><span class="v"><StatusDot :status="selected.success ? 'ok' : 'err'" /> {{ selected.success ? 'success' : 'fail' }}</span>
          <span class="k">MODEL</span><span class="v">{{ selected.model || '—' }}</span>
          <span class="k">FORMAT</span><span class="v">{{ selected.in_format }} → {{ selected.out_format }}</span>
          <span class="k">ISSUED_KEY</span><span class="v">{{ selected.issued_key_id }}</span>
          <span class="k">UPSTREAM_KEY</span><span class="v">{{ selected.upstream_key_id || '—' }}</span>
          <span class="k">TOKENS</span><span class="v mono-num">prompt {{ selected.prompt_tokens || 0 }} / completion {{ selected.completion_tokens || 0 }} / total {{ selected.total_tokens || 0 }}</span>
          <span class="k">TTFB</span><span class="v mono-num">{{ fmtMs(selected.latency_ttfb_ms) }}</span>
          <span class="k">TOTAL</span><span class="v mono-num">{{ fmtMs(selected.total_ms) }}</span>
          <span class="k">STREAMED</span><span class="v">{{ selected.streamed ? 'yes' : 'no' }}</span>
          <span class="k">TIMESTAMP</span><span class="v">{{ selected.timestamp }}</span>
          <span v-if="selected.error_code" class="k">ERROR</span>
          <span v-if="selected.error_code" class="v err">{{ selected.error_code }}</span>
        </div>
        <div v-if="selectedDetail?.payload">
          <div class="label" style="margin-bottom: var(--sp-2)">PAYLOAD</div>
          <details><summary class="dim">request body</summary><pre class="payload">{{ selectedDetail.payload.request_body }}</pre></details>
          <details><summary class="dim">response body</summary><pre class="payload">{{ selectedDetail.payload.response_body }}</pre></details>
        </div>
      </template>
    </div>
  </Teleport>
</template>

<style scoped>
.toolbar { display: flex; gap: var(--sp-3); margin-bottom: var(--sp-3); align-items: center; flex-wrap: wrap; }
.toolbar .chips { flex: 0 0 auto; }
.toolbar .searchline { flex: 1; min-width: 200px; }
.feed-wrap { max-height: calc(100vh - 220px); overflow: auto; border: 1px solid var(--line); }
.dim { color: var(--fg-mute); }
.err { color: var(--danger); }
.payload { background: var(--bg-elev); padding: var(--sp-2); overflow: auto; max-height: 200px; white-space: pre-wrap; word-break: break-all; font-size: var(--fs-xs); color: var(--fg-dim); }
</style>
