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
    "messages": [{"role": "user", "content": "РџСЂРёРІРµС‚!"}]
  }'`)

const curlModels = computed(() => `curl ${base.value}/models \\
  -H "Authorization: Bearer $ARBUZ_KEY"`)

const pyCode = computed(() => `from openai import OpenAI

client = OpenAI(
    base_url="${base.value}",
    api_key="Р’РђРЁ_РЎР“Р•РќР•Р РР РћР’РђРќРќР«Р™_РљР›Р®Р§",  # РєР»СЋС‡ РёР· СЂР°Р·РґРµР»Р° В«РЎРіРµРЅРµСЂРёСЂРѕРІР°РЅРЅС‹Рµ РєР»СЋС‡РёВ»
)

resp = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "РџСЂРёРІРµС‚!"}],
)
print(resp.choices[0].message.content)`)

const jsCode = computed(() => `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${base.value}",
  apiKey: process.env.ARBUZ_KEY,
});

const resp = await client.chat.completions.create({
  model: "gpt-4o",
  messages: [{ role: "user", content: "РџСЂРёРІРµС‚!" }],
});
console.log(resp.choices[0].message.content);`)

const curlAnthropic = computed(() => `curl ${base.value}/messages \\
  -H "Authorization: Bearer $ARBUZ_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "РџСЂРёРІРµС‚!"}]
  }'`)
</script>

<template>
  <div class="page">
    <h2>INTEGRATION</h2>

    <div class="card">
      <div class="label">Р§РўРћ РќРЈР–РќРћ</div>
      <p class="muted">
        1. РЎРѕР·РґР°Р№ РїСЂРѕРІР°Р№РґРµСЂР° Рё РїСЂРёРІСЏР¶Рё Рє РЅРµРјСѓ РІРЅРµС€РЅРёР№ РєР»СЋС‡ (СЂР°Р·РґРµР»С‹ В«РџСЂРѕРІР°Р№РґРµСЂС‹В» Рё В«Р’РЅРµС€РЅРёРµ РєР»СЋС‡РёВ»).<br />
        2. РЎРіРµРЅРµСЂРёСЂСѓР№ РєР»РёРµРЅС‚СЃРєРёР№ РєР»СЋС‡ РІ СЂР°Р·РґРµР»Рµ В«РЎРіРµРЅРµСЂРёСЂРѕРІР°РЅРЅС‹Рµ РєР»СЋС‡РёВ» вЂ” РѕРЅ РЅР°С‡РёРЅР°РµС‚СЃСЏ СЃ
        <code>sk-arbuzвЂ¦</code> Рё РїРѕРєР°Р·С‹РІР°РµС‚СЃСЏ РѕРґРёРЅ СЂР°Р·.<br />
        3. РСЃРїРѕР»СЊР·СѓР№ РµРіРѕ РєР°Рє РѕР±С‹С‡РЅС‹Р№ OpenAI/Anthropic API-РєР»СЋС‡, СѓРєР°Р·Р°РІ <b>BASE_URL</b> РЅРёР¶Рµ.
      </p>
      <div class="kv">
        <span class="k">BASE_URL</span>
        <span class="v"><code>{{ base }}</code>
          <button class="mini" @click="copy(base, 'b')">{{ copied==='b' ? 'ok' : 'copy' }}</button>
        </span>
        <span class="k">AUTH</span>
        <span class="v"><code>Authorization: Bearer &lt;РІР°С€ РєР»СЋС‡&gt;</code></span>
        <span class="k">FORMATS</span>
        <span class="v">OpenAI (<code>/chat/completions</code>, <code>/models</code>, <code>/embeddings</code>) В· Anthropic (<code>/messages</code>)</span>
      </div>
    </div>

    <div class="card">
      <div class="label">cURL вЂ” chat completions <button class="mini" @click="copy(curlChat, 'c')">{{ copied==='c' ? 'ok' : 'copy' }}</button></div>
      <pre>{{ curlChat }}</pre>
    </div>

    <div class="card">
      <div class="label">cURL вЂ” СЃРїРёСЃРѕРє РјРѕРґРµР»РµР№ <button class="mini" @click="copy(curlModels, 'm')">{{ copied==='m' ? 'ok' : 'copy' }}</button></div>
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
      <div class="label">cURL вЂ” Anthropic-С„РѕСЂРјР°С‚ <button class="mini" @click="copy(curlAnthropic, 'a')">{{ copied==='a' ? 'ok' : 'copy' }}</button></div>
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
