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
  list: (status) => request(`/approvals${status ? `?status=${status}` : ''}`),
  get: (id) => request(`/approvals/${id}`),
  create: (payload) =>
    request('/approvals', { method: 'POST', body: JSON.stringify(payload) }),
  patch: (id, payload) =>
    request(`/approvals/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  approve: (id, remark) =>
    request(`/approvals/${id}/approve`, {
      method: 'POST',
      body: JSON.stringify({ remark }),
    }),
  reject: (id, remark) =>
    request(`/approvals/${id}/reject`, {
      method: 'POST',
      body: JSON.stringify({ remark }),
    }),
  resubmit: (id) => request(`/approvals/${id}/resubmit`, { method: 'POST' }),
  remove: (id) => request(`/approvals/${id}`, { method: 'DELETE' }),
}
