import { apiFetch, buildApiPath } from './api.js'

const buildQuery = (params) => {
  const search = new URLSearchParams()
  Object.entries(params || {}).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return
    search.set(key, String(value))
  })
  const query = search.toString()
  return query ? `?${query}` : ''
}

const normalizeArray = (value, mapItem) => {
  if (!Array.isArray(value)) return []
  const mapped = mapItem
    ? value.map((item, index) => mapItem(item, index))
    : value.slice()
  return mapped.filter((item) => item && typeof item === 'object' && !Array.isArray(item))
}

const normalizeObject = (value) => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  return value
}

const normalizeInterface = (value) => {
  if (value === undefined || value === null) return ''
  const trimmed = String(value).trim().toLowerCase()
  if (trimmed === 'cli' || trimmed === 'mcp') {
    return trimmed
  }
  return ''
}

const normalizeTerminal = (terminal) => {
  const id = terminal?.id
  if (!id) return null
  const interfaceValue = normalizeInterface(terminal?.interface) || 'cli'
  return {
    ...terminal,
    id: String(id),
    title: terminal?.title ? String(terminal.title) : '',
    interface: interfaceValue,
  }
}

const normalizeAgent = (agent) => {
  const id = agent?.id
  if (!id) return null
  const name = agent?.name ? String(agent.name) : String(id)
  return {
    ...agent,
    id: String(id),
    name,
  }
}

const normalizeSkill = (skill, index) => {
  if (!skill || typeof skill !== 'object') return null
  const name = skill?.name ? String(skill.name) : ''
  if (!name) return null
  return { ...skill, name }
}

const normalizePlan = (plan, index) => {
  if (!plan || typeof plan !== 'object') return null
  return {
    ...plan,
    filename: plan?.filename ? String(plan.filename) : '',
    title: plan?.title ? String(plan.title) : '',
    headings: Array.isArray(plan.headings) ? plan.headings.filter(Boolean) : [],
  }
}

const normalizeWorkflow = (workflow, index) => {
  if (!workflow || typeof workflow !== 'object') return null
  if (!workflow.session_id) return { ...workflow, session_id: '' }
  return { ...workflow, session_id: String(workflow.session_id) }
}

const normalizeFlowActivityField = (field) => {
  if (!field || typeof field !== 'object') return null
  const key = field?.key ? String(field.key) : ''
  if (!key) return null
  return {
    ...field,
    key,
    label: field?.label ? String(field.label) : key,
    type: field?.type ? String(field.type) : 'string',
    required: Boolean(field?.required),
  }
}

const normalizeFlowActivityDef = (def) => {
  if (!def || typeof def !== 'object') return null
  const id = def?.id ? String(def.id) : ''
  if (!id) return null
  return {
    ...def,
    id,
    label: def?.label ? String(def.label) : id,
    description: def?.description ? String(def.description) : '',
    fields: normalizeArray(def.fields, normalizeFlowActivityField),
  }
}

const normalizeFlowTrigger = (trigger) => {
  if (!trigger || typeof trigger !== 'object') return null
  const id = trigger?.id ? String(trigger.id) : ''
  if (!id) return null
  return {
    ...trigger,
    id,
    label: trigger?.label ? String(trigger.label) : id,
    event_type: trigger?.event_type ? String(trigger.event_type) : '',
    where: normalizeObject(trigger.where),
  }
}

const normalizeFlowBinding = (binding) => {
  if (!binding || typeof binding !== 'object') return null
  const activityId = binding?.activity_id ? String(binding.activity_id) : ''
  if (!activityId) return null
  return {
    ...binding,
    activity_id: activityId,
    config: normalizeObject(binding.config),
  }
}

const normalizeFlowEventTypes = (payload) => {
  const config = normalizeObject(payload)
  const eventTypes = Array.isArray(config.event_types)
    ? config.event_types.map((eventType) => String(eventType || '')).filter(Boolean)
    : []
  return {
    eventTypes,
    notifyTypes: normalizeObject(config.notify_types),
    notifyTokens: normalizeObject(config.notify_tokens),
  }
}

const normalizeFlowConfigPayload = (payload) => {
  const config = normalizeObject(payload)
  const triggers = normalizeArray(config.triggers, normalizeFlowTrigger)
  const bindings = normalizeObject(config.bindings_by_trigger_id)
  const normalizedBindings = {}
  Object.entries(bindings).forEach(([triggerId, list]) => {
    const id = String(triggerId || '')
    if (!id) return
    normalizedBindings[id] = normalizeArray(list, normalizeFlowBinding)
  })
  return {
    config: {
      version: Number.isFinite(Number(config.version)) ? Number(config.version) : 1,
      triggers,
      bindings_by_trigger_id: normalizedBindings,
    },
    temporalStatus: normalizeObject(config.temporal_status),
  }
}

export const fetchStatus = async () => {
  const response = await apiFetch('/api/status')
  const payload = await response.json()
  return normalizeObject(payload)
}

export const fetchTerminals = async () => {
  const response = await apiFetch('/api/sessions')
  const payload = await response.json()
  return normalizeArray(payload, normalizeTerminal)
}

export const createTerminal = async ({ agentId = '', workflow } = {}) => {
  const payload = agentId ? { agent: agentId } : {}
  if (typeof workflow === 'boolean') {
    payload.workflow = workflow
  }
  const response = await apiFetch('/api/sessions', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
  const result = await response.json()
  return normalizeTerminal(result) || normalizeObject(result)
}

export const deleteTerminal = async (terminalId) => {
  if (!terminalId) return
  await apiFetch(buildApiPath('/api/sessions', terminalId), { method: 'DELETE' })
}

export const fetchAgents = async () => {
  const response = await apiFetch('/api/agents')
  const payload = await response.json()
  return normalizeArray(payload, normalizeAgent)
}

export const fetchAgentSkills = async (agentId) => {
  if (!agentId) return []
  const response = await apiFetch(`/api/skills${buildQuery({ agent: agentId })}`)
  const payload = await response.json()
  return normalizeArray(payload, normalizeSkill)
}

export const fetchLogs = async ({ level } = {}) => {
  const response = await apiFetch(`/api/logs${buildQuery({ level })}`)
  const payload = await response.json()
  return normalizeArray(payload, (entry) => entry)
}

export const fetchMetricsSummary = async () => {
  const response = await apiFetch('/api/metrics/summary')
  const payload = normalizeObject(await response.json())
  return {
    ...payload,
    top_endpoints: normalizeArray(payload.top_endpoints),
    slowest_endpoints: normalizeArray(payload.slowest_endpoints),
    top_agents: normalizeArray(payload.top_agents),
    error_rates: normalizeArray(payload.error_rates),
  }
}

export const fetchPlansList = async () => {
  const response = await apiFetch('/api/plans')
  const payload = await response.json()
  const normalized = normalizeObject(payload)
  const list = Array.isArray(payload) ? payload : normalized.plans
  return {
    ...normalized,
    plans: normalizeArray(list, normalizePlan),
  }
}

export const fetchWorkflows = async () => {
  const response = await apiFetch('/api/workflows')
  const payload = await response.json()
  return normalizeArray(payload, normalizeWorkflow)
}

export const sendAgentInput = async (agentName, inputText) => {
  if (!agentName) return
  await apiFetch(`/api/agents/${encodeURIComponent(agentName)}/send-input`, {
    method: 'POST',
    body: JSON.stringify({ input: inputText }),
  })
}

export const resumeWorkflow = async (sessionId, action) => {
  if (!sessionId) return
  await apiFetch(`/api/sessions/${sessionId}/workflow/resume`, {
    method: 'POST',
    body: JSON.stringify({ action }),
  })
}

export const fetchWorkflowHistory = async (terminalId) => {
  if (!terminalId) return []
  const response = await apiFetch(`/api/sessions/${terminalId}/workflow/history`)
  const payload = await response.json()
  return normalizeArray(payload, (entry) => entry)
}

export const fetchFlowActivities = async () => {
  const response = await apiFetch('/api/flow/activities')
  const payload = await response.json()
  return normalizeArray(payload, normalizeFlowActivityDef)
}

export const fetchFlowEventTypes = async () => {
  const response = await apiFetch('/api/flow/event-types')
  const payload = await response.json()
  return normalizeFlowEventTypes(payload)
}

export const fetchFlowConfig = async () => {
  const response = await apiFetch('/api/flow/config')
  const payload = await response.json()
  return normalizeFlowConfigPayload(payload)
}

export const saveFlowConfig = async (config) => {
  const response = await apiFetch('/api/flow/config', {
    method: 'PUT',
    body: JSON.stringify(config || {}),
  })
  const payload = await response.json()
  return normalizeFlowConfigPayload(payload)
}
