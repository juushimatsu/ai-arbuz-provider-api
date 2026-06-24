<script setup>
// Presentational status indicator (§5). No logic; pure prop→color.
defineProps({
  status: { type: String, default: 'off' } // ok | err | warn | off
})
</script>

<template>
  <span class="dot" :class="status" />
</template>
