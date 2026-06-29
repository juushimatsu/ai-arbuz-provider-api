// Isolated API layer (§3.3). Components never call fetch directly; they use
// these resource helpers which centralize auth, JSON handling, and errors.

// Base path is relative so the same code works in dev (vite proxy) and prod
// (backend serves both /api and the SPA).
const BASE = ''

class ApiError extends Error {
  constructor(status, message) {
    super(message)
    this.status = status
  }
}

async function request(method, path, body) {
  const opts = { method, credentials: 'include', headers: {} }
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = typeof body === 'string' ? body : JSON.stringify(body)
  }
  const res = await fetch(BASE + path, opts)
  const ct = res.headers.get('content-type') || ''
  if (!res.ok) {
    let msg = res.statusText
    if (ct.includes('application/json')) {
      try { const e = await res.json(); msg = e.error || msg } catch { /* keep */ }
    }
    throw new ApiError(res.status, msg)
  }
  if (ct.includes('application/json')) return res.json()
  return res.text()
}

export const api = {
  // auth
  login: (login, password) => request('POST', '/api/auth/login', { login, password }),
  logout: () => request('POST', '/api/auth/logout'),
  me: () => request('GET', '/api/auth/me'),
  changeCredentials: (current_password, login, new_password) =>
    request('POST', '/api/auth/credentials', { current_password, login, new_password }),

  // providers
  listProviders: () => request('GET', '/api/providers'),
  createProvider: (p) => request('POST', '/api/providers', p),
  getProvider: (id) => request('GET', '/api/providers/' + id),
  updateProvider: (id, p) => request('PUT', '/api/providers/' + id, p),
  deleteProvider: (id) => request('DELETE', '/api/providers/' + id),

  // upstreams
  listUpstreams: (providerId) =>
    request('GET', '/api/upstreams' + (providerId ? '?provider_id=' + providerId : '')),
  createUpstream: (u) => request('POST', '/api/upstreams', u),
  getUpstream: (id) => request('GET', '/api/upstreams/' + id),
  updateUpstream: (id, u) => request('PUT', '/api/upstreams/' + id, u),
  deleteUpstream: (id) => request('DELETE', '/api/upstreams/' + id),

  // issued keys
  listIssued: (providerId) =>
    request('GET', '/api/issued' + (providerId ? '?provider_id=' + providerId : '')),
  createIssued: (k) => request('POST', '/api/issued', k),
  getIssued: (id) => request('GET', '/api/issued/' + id),
  updateIssued: (id, k) => request('PUT', '/api/issued/' + id, k),
  deleteIssued: (id) => request('DELETE', '/api/issued/' + id),
  revokeIssued: (id) => request('POST', '/api/issued/' + id + '/revoke'),
  pauseIssued: (id) => request('POST', '/api/issued/' + id + '/pause'),
  resumeIssued: (id) => request('POST', '/api/issued/' + id + '/resume'),

  // logs
  listLogs: (params) => request('GET', '/api/logs' + qs(params)),
  getLog: (id) => request('GET', '/api/logs/' + id),

  // stats
  statsSummary: (params) => request('GET', '/api/stats/summary' + qs(params)),
  statsSeries: (params) => request('GET', '/api/stats/series' + qs(params)),
  statsBreakdown: (params) => request('GET', '/api/stats/breakdown' + qs(params)),

  // model search + checker
  searchModels: (base_url, secret, format) =>
    request('POST', '/api/models/search', { base_url, secret, format }),
  runChecker: (base_url, secret, format, probes) =>
    request('POST', '/api/checker/run', { base_url, secret, format, probes }),
  listCheckerRuns: () => request('GET', '/api/checker/runs'),
  checkAllUpstream: (probes, providerId) =>
    request('POST', '/api/upstream/check-all', { probes, provider_id: providerId }),
  searchProviderModels: (id) => request('POST', '/api/providers/' + id + '/search-models'),
  // model availability + "model -> preferred key" mapping
  providerModels: (id) => request('GET', '/api/providers/' + id + '/models'),
  listModelPrefs: (id) => request('GET', '/api/providers/' + id + '/model-prefs'),
  setModelPref: (id, model, upstream_key_id) =>
    request('PUT', '/api/providers/' + id + '/model-prefs', { model, upstream_key_id }),
  deleteModelPref: (id, model) =>
    request('DELETE', '/api/providers/' + id + '/model-prefs/' + encodeURIComponent(model)),
  getCheckerRun: (id) => request('GET', '/api/checker/runs/' + id),

  // mcp
  listMCP: () => request('GET', '/api/mcp'),
  createMCP: (m) => request('POST', '/api/mcp', m),
  updateMCP: (id, m) => request('PUT', '/api/mcp/' + id, m),
  deleteMCP: (id) => request('DELETE', '/api/mcp/' + id),
  discoverMCPTools: (id) => request('POST', '/api/mcp/' + id + '/tools'),
  callMCPTool: (id, name, args) =>
    request('POST', '/api/mcp/' + id + '/call', { name, arguments: args }),
}

export { ApiError }

// qs builds a query string from a params object, skipping empty values.
function qs(params) {
  if (!params) return ''
  const parts = []
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    parts.push(encodeURIComponent(k) + '=' + encodeURIComponent(v))
  }
  return parts.length ? '?' + parts.join('&') : ''
}