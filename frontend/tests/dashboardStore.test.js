import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { get } from 'svelte/store'
import { createLogStreamStub } from './helpers/appApiMocks.js'

const fetchAgents = vi.hoisted(() => vi.fn())
const fetchAgentSkills = vi.hoisted(() => vi.fn())
const fetchMetricsSummary = vi.hoisted(() => vi.fn())
const addNotification = vi.hoisted(() => vi.fn())
const subscribeAgentEvents = vi.hoisted(() => vi.fn())
const subscribeConfigEvents = vi.hoisted(() => vi.fn())
const subscribeEvents = vi.hoisted(() => vi.fn())
const createLogStream = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/apiClient.js', () => ({
  fetchAgents,
  fetchAgentSkills,
  fetchMetricsSummary,
}))

vi.mock('../src/lib/notificationStore.js', () => ({
  notificationStore: {
    addNotification,
  },
}))

vi.mock('../src/lib/agentEventStore.js', () => ({
  subscribe: subscribeAgentEvents,
}))

vi.mock('../src/lib/configEventStore.js', () => ({
  subscribe: subscribeConfigEvents,
}))

vi.mock('../src/lib/eventStore.js', () => ({
  subscribe: subscribeEvents,
}))

vi.mock('../src/lib/logStream.js', () => ({
  createLogStream,
}))

import { createDashboardStore } from '../src/lib/dashboardStore.js'

describe('dashboardStore', () => {
  let agentHandlers
  let configHandlers
  let eventHandlers

  beforeEach(() => {
    agentHandlers = {}
    configHandlers = {}
    eventHandlers = {}
    subscribeAgentEvents.mockImplementation((type, callback) => {
      agentHandlers[type] = callback
      return () => {
        delete agentHandlers[type]
      }
    })
    subscribeConfigEvents.mockImplementation((type, callback) => {
      configHandlers[type] = callback
      return () => {
        delete configHandlers[type]
      }
    })
    subscribeEvents.mockImplementation((type, callback) => {
      eventHandlers[type] = callback
      return () => {
        delete eventHandlers[type]
      }
    })
    createLogStream.mockImplementation((options) =>
      createLogStreamStub({
        start: () => options?.onOpen?.(),
        restart: () => options?.onOpen?.(),
      }),
    )
  })

  afterEach(() => {
    fetchAgents.mockReset()
    fetchAgentSkills.mockReset()
    fetchMetricsSummary.mockReset()
    addNotification.mockReset()
    subscribeAgentEvents.mockReset()
    subscribeConfigEvents.mockReset()
    subscribeEvents.mockReset()
    createLogStream.mockReset()
  })

  it('loads agents and skills', async () => {
    fetchAgents.mockResolvedValue([{ id: 'a1', name: 'Agent 1' }])
    fetchAgentSkills.mockResolvedValue([{ name: 'skill-a' }])

    const store = createDashboardStore()
    await store.loadAgents()

    const value = get(store)
    expect(value.agents).toHaveLength(1)
    expect(value.agentSkills).toEqual({ a1: ['skill-a'] })
    expect(value.agentsLoading).toBe(false)
  })

  it('captures agent load errors', async () => {
    fetchAgents.mockRejectedValue(new Error('boom'))

    const store = createDashboardStore()
    await store.loadAgents()

    const value = get(store)
    expect(value.agentsError).toBeTruthy()
    expect(value.agentsLoading).toBe(false)
  })

  it('notifies once for repeated log errors', async () => {
    createLogStream.mockImplementation((options) => ({
      start: vi.fn(() => options?.onError?.(new Error('logs down'))),
      stop: vi.fn(),
      restart: vi.fn(() => options?.onError?.(new Error('logs down'))),
      setLevel: vi.fn(),
    }))
    fetchAgents.mockResolvedValue([])
    fetchMetricsSummary.mockResolvedValue({})

    const store = createDashboardStore()
    await store.start()
    await store.loadLogs()
    await store.loadLogs()

    expect(addNotification).toHaveBeenCalledTimes(1)
    store.stop()
  })

  it('loads metrics summary', async () => {
    fetchMetricsSummary.mockResolvedValue({
      updated_at: '2026-01-24T00:00:00Z',
      top_endpoints: [],
      slowest_endpoints: [],
      top_agents: [],
      error_rates: [],
    })

    const store = createDashboardStore()
    await store.loadMetricsSummary()

    const value = get(store)
    expect(value.metricsSummary).toEqual({
      updated_at: '2026-01-24T00:00:00Z',
      top_endpoints: [],
      slowest_endpoints: [],
      top_agents: [],
      error_rates: [],
    })
    expect(value.metricsLoading).toBe(false)
  })

  it('batches log bursts into scheduled updates', async () => {
    vi.useFakeTimers()
    let onEntry = null
    createLogStream.mockImplementation((options) => {
      onEntry = options?.onEntry
      return {
        start: vi.fn(() => options?.onOpen?.()),
        stop: vi.fn(),
        restart: vi.fn(() => options?.onOpen?.()),
        setLevel: vi.fn(),
      }
    })
    fetchAgents.mockResolvedValue([])
    fetchMetricsSummary.mockResolvedValue({})

    const store = createDashboardStore()
    await store.start()

    for (let i = 0; i < 500; i += 1) {
      onEntry?.({ body: `log ${i}` })
    }

    expect(get(store).logs.length).toBe(0)

    vi.advanceTimersByTime(100)

    expect(get(store).logs.length).toBe(500)
    store.stop()
    vi.useRealTimers()
  })

  it('syncs agent running state when terminals change', async () => {
    fetchAgents.mockResolvedValue([{ id: 'a1', name: 'Agent 1', terminal_id: 't1' }])
    fetchAgentSkills.mockResolvedValue([])

    const store = createDashboardStore()
    store.setTerminals([{ id: 't1' }])
    await store.loadAgents()

    let value = get(store)
    expect(value.agents[0].running).toBe(true)
    expect(value.agents[0].terminal_id).toBe('t1')

    store.setTerminals([])
    value = get(store)
    expect(value.agents[0].running).toBe(false)
    expect(value.agents[0].terminal_id).toBe('')
  })

  it('tracks config extraction events and resets', async () => {
    vi.useFakeTimers()
    fetchAgents.mockResolvedValue([])
    fetchMetricsSummary.mockResolvedValue({})

    const store = createDashboardStore()
    await store.start()

    configHandlers.config_extracted({ path: 'config/agents/codex.toml' })
    let value = get(store)
    expect(value.configExtractionCount).toBe(1)
    expect(value.configExtractionLast).toBe('config/agents/codex.toml')

    vi.advanceTimersByTime(5000)
    value = get(store)
    expect(value.configExtractionCount).toBe(0)
    expect(value.configExtractionLast).toBe('')

    store.stop()
    vi.useRealTimers()
  })

  it('updates git context from status and events', async () => {
    fetchAgents.mockResolvedValue([])
    fetchMetricsSummary.mockResolvedValue({})

    const store = createDashboardStore()
    store.setStatus({ git_origin: 'origin', git_branch: 'main' })
    let value = get(store)
    expect(value.gitContext).toBe('origin/main')

    await store.start()
    eventHandlers.git_branch_changed({ path: 'feature-x' })
    value = get(store)
    expect(value.gitContext).toBe('origin/feature-x')
    store.stop()
  })

  it('notifies on config conflicts and validation errors', async () => {
    fetchAgents.mockResolvedValue([])
    fetchMetricsSummary.mockResolvedValue({})

    const store = createDashboardStore()
    await store.start()

    configHandlers.config_conflict({ path: 'config/conflict.toml' })
    configHandlers.config_validation_error({ message: 'bad config' })

    expect(addNotification).toHaveBeenCalledTimes(2)
    store.stop()
  })
})
