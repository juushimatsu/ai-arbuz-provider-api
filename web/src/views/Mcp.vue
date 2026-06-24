<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/client'
import StatusDot from '../components/StatusDot.vue'

const list = ref([])
const loading = ref(true)
const error = ref(null)
const editing = ref(null)
const discovering = ref(false)
const callBox = ref(null) // { id, name, arguments, result }

async function load() {
  loading.value = true
  try { list.value = await api.listMCP() } catch (e) { error.value = e.message }
  finally { loading.value = false }
}

function newServer() {
  editing.value = { name: '', kind: 'client', transport: 'http', address: '', status: 'active' }
}
function edit(m) { editing.value = { ...m } }

async function save() {
  const e = editing.value
  try {
    if (e.id) await api.updateMCP(e.id, e)
    else await api.createMCP(e)
    editing.value = null
    await load()
  } catch (err) { error.value = err.message }
}

async function remove(id) {
  if (!confirm('Delete MCP server?')) return
  try { await api.deleteMCP(id); await load() } catch (e) { error.value = e.message }
}

async function discover(id) {
  discovering.value = true
  try {
    const res = await api.discoverMCPTools(id)
    await load()
  } catch (e) { error.value = e.message }
  finally { discovering.value = false }
}

function openCall(m) { callBox.value = { id: m.id, name: '', arguments: '{}', result: null, error: null } }
async function doCall() {
  const c = callBox.value
  c.error = null; c.result = null
  try {
    let args = c.arguments
    try { args = JSON.parse(c.arguments) } catch { /* send raw */ }
    c.result = await api.callMCPTool(c.id, c.name, args)
  } catch (e) { c.error = e.message }
}
onMounted(load)
</script>

<template>
  <div class="page">
    <h2>MCP&nbsp;SERVERS</h2>
    <div v-if="error" class="login-err">[ERR] {{ error }}</div>
    <div class="label dim" style="margin-bottom: var(--sp-3)">tools-only bridge (JSON-RPC over HTTP)</div>

    <button class="btn" @click="newServer">+ add MCP server</button>

    <table class="feed" style="margin-top: var(--sp-4)">
      <thead><tr><th>NAME</th><th>KIND</th><th>TRANSPORT</th><th>ADDRESS</th><th>TOOLS</th><th>STATUS</th><th></th></tr></thead>
      <tbody>
        <tr v-for="m in list" :key="m.id">
          <td>{{ m.name }}</td>
          <td><span class="tag">{{ m.kind }}</span></td>
          <td>{{ m.transport }}</td>
          <td class="dim">{{ m.address }}</td>
          <td>{{ (m.tools || []).length }}</td>
          <td><StatusDot :status="m.status === 'active' ? 'ok' : 'off'" /> {{ m.status }}</td>
          <td>
            <button class="btn" @click="discover(m.id)" :disabled="discovering">discover</button>
            <button class="btn" @click="openCall(m)">call</button>
            <button class="btn" @click="edit(m)">edit</button>
            <button class="btn danger" @click="remove(m.id)">del</button>
          </td>
        </tr>
        <tr v-if="!list.length"><td colspan="7" class="empty">no MCP servers</td></tr>
      </tbody>
    </table>

    <div v-if="editing" class="modal-bg" @click.self="editing = null">
      <div class="modal">
        <h3>{{ editing.id ? 'EDIT' : 'NEW' }} MCP</h3>
        <div class="field"><label>NAME</label><input v-model="editing.name" /></div>
        <div class="field"><label>KIND</label>
          <select v-model="editing.kind"><option value="client">client</option><option value="server">server</option><option value="wrapper">wrapper</option></select>
        </div>
        <div class="field"><label>TRANSPORT</label>
          <select v-model="editing.transport"><option value="http">http</option></select>
        </div>
        <div class="field"><label>ADDRESS</label><input v-model="editing.address" placeholder="https://mcp.example.com/mcp" /></div>
        <div class="field"><label>STATUS</label>
          <select v-model="editing.status"><option value="active">active</option><option value="disabled">disabled</option></select>
        </div>
        <div class="actions">
          <button class="btn" @click="save">save</button>
          <button class="btn danger" @click="editing = null">cancel</button>
        </div>
      </div>
    </div>

    <div v-if="callBox" class="modal-bg" @click.self="callBox = null">
      <div class="modal">
        <h3>CALL&nbsp;TOOL</h3>
        <div class="field"><label>TOOL NAME</label><input v-model="callBox.name" /></div>
        <div class="field"><label>ARGUMENTS (JSON)</label><textarea v-model="callBox.arguments" rows="4"></textarea></div>
        <button class="btn" @click="doCall">> call</button>
        <div v-if="callBox.error" class="login-err">[ERR] {{ callBox.error }}</div>
        <pre v-if="callBox.result" class="payload">{{ JSON.stringify(callBox.result, null, 2) }}</pre>
        <div class="actions"><button class="btn danger" @click="callBox = null">close</button></div>
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
.payload { background: var(--bg-elev); padding: var(--sp-2); overflow: auto; max-height: 200px; white-space: pre-wrap; word-break: break-all; font-size: var(--fs-xs); color: var(--fg-dim); }
</style>
