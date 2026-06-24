<script setup>
// Hand-rolled SVG line chart, no dependencies. ponytail: ceiling — no tooltips,
// no zoom, no axis labels beyond basics. Growth path = chart.js/recharts.
import { computed } from 'vue'

const props = defineProps({
  points: { type: Array, default: () => [] }, // [{ v: number }]
  width: { type: Number, default: 600 },
  height: { type: Number, default: 160 },
  color: { type: String, default: 'var(--accent)' }
})

const W = 600, H = 160, PAD = 8

// Stable gradient id derived from the color (CSS var → safe slug).
const gradId = computed(() => 'g-' + props.color.replace(/[^a-z]/gi, ''))

const view = computed(() => {
  const pts = props.points || []
  if (pts.length === 0) return { path: '', area: '' }
  const vals = pts.map(p => Number(p.v) || 0)
  const max = Math.max(1, ...vals)
  const stepX = pts.length > 1 ? (W - PAD * 2) / (pts.length - 1) : 0
  const xy = pts.map((p, i) => {
    const x = PAD + i * stepX
    const y = H - PAD - (Number(p.v) || 0) / max * (H - PAD * 2)
    return [x, y]
  })
  const path = xy.map(([x, y], i) => (i === 0 ? 'M' : 'L') + x.toFixed(1) + ',' + y.toFixed(1)).join(' ')
  const area = path + ` L${xy[xy.length - 1][0].toFixed(1)},${H - PAD} L${xy[0][0].toFixed(1)},${H - PAD} Z`
  return { path, area }
})

const fillUrl = computed(() => `url(#${gradId.value})`)
</script>

<template>
  <svg :viewBox="`0 0 ${W} ${H}`" :width="width" :height="height" preserveAspectRatio="none" class="svgline">
    <defs>
      <linearGradient :id="gradId" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" :stop-color="color" stop-opacity="0.25" />
        <stop offset="100%" :stop-color="color" stop-opacity="0" />
      </linearGradient>
    </defs>
    <path :d="view.area" :fill="fillUrl" />
    <path :d="view.path" fill="none" :stroke="color" stroke-width="1.5" />
  </svg>
</template>

<style scoped>
.svgline { display: block; width: 100%; height: 100%; }
</style>
