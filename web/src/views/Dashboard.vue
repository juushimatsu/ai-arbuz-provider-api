<script setup>
import { ref, onMounted, computed } from 'vue'
import { api } from '../api/client'
import SvgLine from '../components/SvgLine.vue'

const summary = ref(null)
const series = ref([])
const byModel = ref([])
const byKey = ref([])
const loading = ref(true)
const error = ref(null)

const last24h = computed(() => {
  const to = new Date()
  const from = new Date(to.getTime() - 24 * 3600 * 1000)
  return { from: from.toISOString(), to: to.toISOString() }
})

async function load() {
  loading.value = true
  error.value = null
  try {
    const q = last24h.value
    const [s, sr, bm, bk] = await Promise.all([
      api.statsSummary(q),
      api.statsSeries({ ...q, bucket_seconds: 3600 }),
      api.statsBreakdown({ ...q, dimension: 'model' }),
      api.statsBreakdown({ ...q, dimension: 'key' })
    ])
    summary.value = s
    series.value = sr || []
    byModel.value = (bm || []).slice(0, 8)
    byKey.value = (bk || []).slice(0, 8)
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

const linePoints = computed(() => series.value.map(p => ({ v: p.Count })))
const errPoints = computed(() => series.value.map(p => ({ v: p.Errors })))
const ttfbPoints = computed(() => series.value.map(p => ({ v: p.TTFBMs })))

onMounted(load)
</script>

<template>
  <div class="page">
    <h2>STATS&nbsp;//&nbsp;DASHBOARD</h2>
    <div v-if="error" class="login-err">[ERR] {{ error }}</div>

    <div v-if="loading" class="empty">loading…</div>
    <div v-else-if="summary">
      <div class="kpis">
        <div class="kpi">
          <div class="label">REQUESTS_24H</div>
          <div class="mono-num big">{{ summary.total_requests }}</div>
        </div>
        <div class="kpi">
          <div class="label">SUCCESS</div>
          <div class="mono-num big ok">{{ summary.success_count }}</div>
        </div>
        <div class="kpi">
          <div class="label">ERRORS</div>
          <div class="mono-num big err">{{ summary.error_count }}</div>
        </div>
        <div class="kpi">
          <div class="label">ERROR_RATE</div>
          <div class="mono-num big">{{ (summary.error_rate * 100).toFixed(2) }}%</div>
        </div>
        <div class="kpi">
          <div class="label">TOKENS</div>
          <div class="mono-num big">{{ (summary.prompt_tokens + summary.completion_tokens) }}</div>
        </div>
        <div class="kpi">
          <div class="label">AVG_TTFB_MS</div>
          <div class="mono-num big">{{ Math.round(summary.avg_ttfb_ms || 0) }}</div>
        </div>
      </div>

      <div class="chart-grid">
        <div class="chart-card">
          <div class="label">REQUESTS / HOUR</div>
          <div class="chart"><SvgLine :points="linePoints" /></div>
        </div>
        <div class="chart-card">
          <div class="label">ERRORS / HOUR</div>
          <div class="chart"><SvgLine :points="errPoints" color="var(--danger)" /></div>
        </div>
        <div class="chart-card">
          <div class="label">LATENCY_TTFB / HOUR</div>
          <div class="chart"><SvgLine :points="ttfbPoints" color="var(--info)" /></div>
        </div>
      </div>

      <div class="breakdowns">
        <div class="bd-card">
          <div class="label">BY_MODEL</div>
          <div v-for="b in byModel" :key="b.label" class="bd-row">
            <span class="bd-label">{{ b.label || '(none)' }}</span>
            <span class="mono-num">{{ b.count }}</span>
            <span class="mono-num dim">{{ b.tokens }} tok</span>
          </div>
          <div v-if="!byModel.length" class="dim">no data</div>
        </div>
        <div class="bd-card">
          <div class="label">BY_KEY</div>
          <div v-for="b in byKey" :key="b.label" class="bd-row">
            <span class="bd-label">{{ b.label ? b.label.slice(-8) : '(none)' }}</span>
            <span class="mono-num">{{ b.count }}</span>
            <span class="mono-num dim">{{ b.tokens }} tok</span>
          </div>
          <div v-if="!byKey.length" class="dim">no data</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.kpis { display: grid; grid-template-columns: repeat(6, 1fr); gap: var(--sp-3); margin-bottom: var(--sp-5); }
.kpi { padding: var(--sp-3); background: var(--bg-panel); border: 1px solid var(--line); border-radius: var(--r-md); }
.big { font-size: var(--fs-xl); font-weight: 700; }
.ok { color: var(--accent); } .err { color: var(--danger); }
.chart-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: var(--sp-3); margin-bottom: var(--sp-5); }
.chart-card { padding: var(--sp-3); background: var(--bg-panel); border: 1px solid var(--line); border-radius: var(--r-md); }
.chart { height: 120px; margin-top: var(--sp-2); }
.breakdowns { display: grid; grid-template-columns: 1fr 1fr; gap: var(--sp-3); }
.bd-card { padding: var(--sp-3); background: var(--bg-panel); border: 1px solid var(--line); border-radius: var(--r-md); }
.bd-row { display: grid; grid-template-columns: 1fr auto auto; gap: var(--sp-3); padding: var(--sp-1) 0; border-bottom: 1px solid var(--line-soft); }
.bd-label { color: var(--fg); word-break: break-all; }
.dim { color: var(--fg-mute); }
</style>
