<script setup>
import { ref, computed } from 'vue'

// The base URL clients should use = this panel's own origin + /v1.
const base = computed(() => window.location.origin + '/v1')
const copied = ref('')
function copy(text, id) {
  try { navigator.clipboard.writeText(text) } catch (e) {}
  copied.value = id
  setTimeout(() => { if (copied.value === id) copied.value = '' }, 1500)
}

const curlChat = computed(() => `curl ${base.value}/chat/completions \\
  -H "Authorization: Bearer $ARBUZ_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Привет!"}]
  }'`)

const curlModels = computed(() => `curl ${base.value}/models \\
  -H "Authorization: Bearer $ARBUZ_KEY"`)

const pyCode = computed(() => `from openai import OpenAI

client = OpenAI(
    base_url="${base.value}",
    api_key="ВАШ_СГЕНЕРИРОВАННЫЙ_КЛЮЧ",  # ключ из раздела «Сгенерированные ключи»
)

resp = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Привет!"}],
)
print(resp.choices[0].message.content)`)

const jsCode = computed(() => `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${base.value}",
  apiKey: process.env.ARBUZ_KEY,
});

const resp = await client.chat.completions.create({
  model: "gpt-4o",
  messages: [{ role: "user", content: "Привет!" }],
});
console.log(resp.choices[0].message.content);`)

const curlAnthropic = computed(() => `curl ${base.value}/messages \\
  -H "Authorization: Bearer $ARBUZ_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "Привет!"}]
  }'`)
</script>

<template>
  <div class="page">
    <h2>INTEGRATION</h2>

    <div class="card">
      <div class="label">ЧТО НУЖНО</div>
      <p class="muted">
        1. Создай провайдера и привяжи к нему внешний ключ (разделы «Провайдеры» и «Внешние ключи»).<br />
        2. Сгенерируй клиентский ключ в разделе «Сгенерированные ключи» — он начинается с
        <code>sk-arbuz…</code> и показывается один раз.<br />
        3. Используй его как обычный OpenAI/Anthropic API-ключ, указав <b>BASE_URL</b> ниже.
      </p>
      <div class="kv">
        <span class="k">BASE_URL</span>
        <span class="v"><code>{{ base }}</code>
          <button class="mini" @click="copy(base, 'b')">{{ copied==='b' ? 'ok' : 'copy' }}</button>
        </span>
        <span class="k">AUTH</span>
        <span class="v"><code>Authorization: Bearer &lt;ваш ключ&gt;</code></span>
        <span class="k">FORMATS</span>
        <span class="v">OpenAI (<code>/chat/completions</code>, <code>/models</code>, <code>/embeddings</code>) · Anthropic (<code>/messages</code>)</span>
      </div>
    </div>

    <div class="card">
      <div class="label">cURL — chat completions <button class="mini" @click="copy(curlChat, 'c')">{{ copied==='c' ? 'ok' : 'copy' }}</button></div>
      <pre>{{ curlChat }}</pre>
    </div>

    <div class="card">
      <div class="label">cURL — список моделей <button class="mini" @click="copy(curlModels, 'm')">{{ copied==='m' ? 'ok' : 'copy' }}</button></div>
      <pre>{{ curlModels }}</pre>
    </div>

    <div class="card">
      <div class="label">Python (openai SDK) <button class="mini" @click="copy(pyCode, 'py')">{{ copied==='py' ? 'ok' : 'copy' }}</button></div>
      <pre>{{ pyCode }}</pre>
    </div>

    <div class="card">
      <div class="label">Node.js (openai SDK) <button class="mini" @click="copy(jsCode, 'js')">{{ copied==='js' ? 'ok' : 'copy' }}</button></div>
      <pre>{{ jsCode }}</pre>
    </div>

    <div class="card">
      <div class="label">cURL — Anthropic-формат <button class="mini" @click="copy(curlAnthropic, 'a')">{{ copied==='a' ? 'ok' : 'copy' }}</button></div>
      <pre>{{ curlAnthropic }}</pre>
    </div>
  </div>
</template>

<style scoped>
.card { padding: var(--sp-4); background: var(--bg-panel); border: 1px solid var(--line); border-radius: var(--r-md); margin-bottom: var(--sp-4); }
.label { color: var(--accent); margin-bottom: var(--sp-3); text-transform: uppercase; font-size: var(--fs-xs); letter-spacing: 1px; }
.muted { color: var(--fg-mute); line-height: 1.6; }
pre { background: var(--bg); border: 1px solid var(--line); border-radius: var(--r-sm); padding: var(--sp-3); overflow-x: auto; white-space: pre; color: var(--fg); }
code { color: var(--accent); }
.kv { display: grid; grid-template-columns: 110px 1fr; gap: var(--sp-2) var(--sp-3); margin-top: var(--sp-3); }
.kv .k { color: var(--fg-mute); font-size: var(--fs-xs); text-transform: uppercase; }
.mini { margin-left: var(--sp-2); font-size: var(--fs-xs); padding: 1px 8px; background: transparent; border: 1px solid var(--line); color: var(--fg-mute); border-radius: var(--r-sm); cursor: pointer; }
.mini:hover { color: var(--accent); border-color: var(--accent); }
</style>
