import { describe, it, expect, vi, afterEach } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildEventSourceUrl,
  buildApiPath: (base, ...segments) => {
    const basePath = base.endsWith('/') ? base.slice(0, -1) : base
    const encoded = segments
      .filter((segment) => segment !== undefined && segment !== null && segment !== '')
      .map((segment) => encodeURIComponent(String(segment)))
    return encoded.length ? `${basePath}/${encoded.join('/')}` : basePath
  },
}))

import {
  createTerminal,
  fetchAgentSkills,
  fetchAgents,
  fetchFlowActivities,
  fetchFlowConfig,
  fetchLogs,
  fetchMetricsSummary,
  fetchPlansList,
  fetchStatus,
  fetchTerminals,
  fetchWorkflowHistory,
  fetchWorkflows,
  saveFlowConfig,
} from '../src/lib/apiClient.js'

describe('apiClient', () => {
  afterEach(() => {
    apiFetch.mockReset()
  })

  it('fetches status payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ ok: true }) })

    const result = await fetchStatus()

    expect(result).toEqual({ ok: true })
    expect(apiFetch).toHaveBeenCalledWith('/api/status')
  })

  it('normalizes malformed status payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) })

    const result = await fetchStatus()

    expect(result).toEqual({})
  })

  it('builds log queries', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) })

    await fetchLogs({ level: 'error' })

    expect(apiFetch).toHaveBeenCalledWith('/api/logs?level=error')
  })

  it('sends terminal create payloads', async () => {
    const json = vi.fn().mockResolvedValue({ id: '1' })
    apiFetch.mockResolvedValue({ json })

    const result = await createTerminal({ agentId: 'codex', workflow: true })

    expect(result).toEqual({ id: '1', interface: 'cli', title: '' })
    expect(apiFetch).toHaveBeenCalledWith('/api/sessions', {
      method: 'POST',
      body: JSON.stringify({ agent: 'codex', workflow: true }),
    })
  })

  it('fetches plans list payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ plans: [] }) })

    const result = await fetchPlansList()

    expect(result).toEqual({ plans: [] })
    expect(apiFetch).toHaveBeenCalledWith('/api/plans')
  })

  it('normalizes malformed plans payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ plans: [null, 'bad'] }) })

    const result = await fetchPlansList()

    expect(result.plans).toEqual([])
  })

  it('normalizes malformed agent payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([null, { name: 'No id' }]) })

    const result = await fetchAgents()

    expect(result).toEqual([])
  })

  it('normalizes malformed agent skills payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([null, { name: '' }, { name: 'Skill' }]) })

    const result = await fetchAgentSkills('agent')

    expect(result).toEqual([{ name: 'Skill' }])
  })

  it('normalizes malformed terminals payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([null, { id: 12 }]) })

    const result = await fetchTerminals()

    expect(result).toEqual([{ id: '12', interface: 'cli', title: '' }])
  })

  it('normalizes malformed metrics summary payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue(null) })

    const result = await fetchMetricsSummary()

    expect(result.top_endpoints).toEqual([])
    expect(result.slowest_endpoints).toEqual([])
    expect(result.top_agents).toEqual([])
    expect(result.error_rates).toEqual([])
  })

  it('normalizes malformed workflows payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([null, { session_id: 9 }]) })

    const result = await fetchWorkflows()

    expect(result).toEqual([{ session_id: '9' }])
  })

  it('normalizes malformed workflow history payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([null, { type: 'bell' }]) })

    const result = await fetchWorkflowHistory('abc')

    expect(result).toEqual([{ type: 'bell' }])
  })

  it('fetches flow activities', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ id: 'toast_notification' }, null]) })

    const result = await fetchFlowActivities()

    expect(result).toEqual([
      { id: 'toast_notification', label: 'toast_notification', description: '', fields: [] },
    ])
    expect(apiFetch).toHaveBeenCalledWith('/api/flow/activities')
  })

  it('fetches flow config payloads', async () => {
    apiFetch.mockResolvedValue({
      json: vi.fn().mockResolvedValue({
        version: 1,
        triggers: [{ id: 't1', label: 'Trigger', event_type: 'workflow_paused' }],
        bindings_by_trigger_id: { t1: [{ activity_id: 'toast_notification' }] },
        temporal_status: { enabled: true },
      }),
    })

    const result = await fetchFlowConfig()

    expect(result.config.triggers[0].id).toBe('t1')
    expect(result.temporalStatus.enabled).toBe(true)
  })

  it('saves flow config payloads', async () => {
    apiFetch.mockResolvedValue({
      json: vi.fn().mockResolvedValue({
        version: 1,
        triggers: [],
        bindings_by_trigger_id: {},
      }),
    })

    const config = { version: 1, triggers: [], bindings_by_trigger_id: {} }
    await saveFlowConfig(config)

    expect(apiFetch).toHaveBeenCalledWith('/api/flow/config', {
      method: 'PUT',
      body: JSON.stringify(config),
    })
  })

})
