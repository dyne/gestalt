import { apiFetch } from './api.js'

const buildQuery = (params) => {
  const search = new URLSearchParams()
  Object.entries(params || {}).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return
    search.set(key, String(value))
  })
  const query = search.toString()
  return query ? `?${query}` : ''
}

export const fetchStatus = async () => {
  const response = await apiFetch('/api/status')
  return response.json()
}

export const fetchTerminals = async () => {
  const response = await apiFetch('/api/terminals')
  return response.json()
}

export const createTerminal = async ({ agentId = '', workflow } = {}) => {
  const payload = agentId ? { agent: agentId } : {}
  if (typeof workflow === 'boolean') {
    payload.workflow = workflow
  }
  const response = await apiFetch('/api/terminals', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
  return response.json()
}

export const deleteTerminal = async (terminalId) => {
  if (!terminalId) return
  await apiFetch(`/api/terminals/${terminalId}`, { method: 'DELETE' })
}

export const fetchAgents = async () => {
  const response = await apiFetch('/api/agents')
  return response.json()
}

export const fetchAgentSkills = async (agentId) => {
  if (!agentId) return []
  const response = await apiFetch(`/api/skills${buildQuery({ agent: agentId })}`)
  return response.json()
}

export const fetchLogs = async ({ level } = {}) => {
  const response = await apiFetch(`/api/logs${buildQuery({ level })}`)
  return response.json()
}

export const fetchMetricsSummary = async () => {
  const response = await apiFetch('/api/metrics/summary')
  return response.json()
}

export const fetchPlan = async ({ etag } = {}) => {
  const response = await apiFetch('/api/plan', {
    allowNotModified: true,
    headers: etag ? { 'If-None-Match': etag } : {},
  })
  const responseEtag = response.headers?.get?.('ETag') || ''
  if (response.status === 304) {
    return { content: '', etag: responseEtag || etag || '', notModified: true }
  }
  const payload = await response.json()
  return { content: payload?.content || '', etag: responseEtag || etag || '', notModified: false }
}

export const fetchWorkflows = async () => {
  const response = await apiFetch('/api/workflows')
  return response.json()
}

export const resumeWorkflow = async (sessionId, action) => {
  if (!sessionId) return
  await apiFetch(`/api/terminals/${sessionId}/workflow/resume`, {
    method: 'POST',
    body: JSON.stringify({ action }),
  })
}

export const fetchWorkflowHistory = async (terminalId) => {
  if (!terminalId) return []
  const response = await apiFetch(`/api/terminals/${terminalId}/workflow/history`)
  return response.json()
}
