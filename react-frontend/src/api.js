// 所有请求走 /api，由 Vite 代理转发到 http://localhost:8080。
const BASE = '/api'

async function request(path, options = {}) {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })

  if (res.status === 204) return null

  const data = await res.json().catch(() => null)
  if (!res.ok) {
    throw new Error(data?.error || `请求失败（${res.status}）`)
  }
  return data
}

export const api = {
  health: () => request('/healthz'),
  list: () => request('/users'),
  get: (id) => request(`/users/${id}`),
  create: (payload) =>
    request('/users', { method: 'POST', body: JSON.stringify(payload) }),
  replace: (id, payload) =>
    request(`/users/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  patch: (id, payload) =>
    request(`/users/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  remove: (id) => request(`/users/${id}`, { method: 'DELETE' }),
}
