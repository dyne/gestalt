import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { get } from 'svelte/store'

const fetchAgents = vi.hoisted(() => vi.fn())
const fetchAgentSkills = vi.hoisted(() => vi.fn())
const fetchLogs = vi.hoisted(() => vi.fn())
const addNotification = vi.hoisted(() => vi.fn())
const subscribeAgentEvents = vi.hoisted(() => vi.fn())
const subscribeConfigEvents = vi.hoisted(() => vi.fn())
const subscribeEvents = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/apiClient.js', () => ({
  fetchAgents,
  fetchAgentSkills,
  fetchLogs,
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
  })

  afterEach(() => {
    fetchAgents.mockReset()
    fetchAgentSkills.mockReset()
    fetchLogs.mockReset()
    addNotification.mockReset()
    subscribeAgentEvents.mockReset()
    subscribeConfigEvents.mockReset()
    subscribeEvents.mockReset()
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
    fetchLogs.mockRejectedValue(new Error('logs down'))

    const store = createDashboardStore()
    await store.loadLogs()
    await store.loadLogs()

    expect(addNotification).toHaveBeenCalledTimes(1)
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
    fetchLogs.mockResolvedValue([])

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
    fetchLogs.mockResolvedValue([])

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
    fetchLogs.mockResolvedValue([])

    const store = createDashboardStore()
    await store.start()

    configHandlers.config_conflict({ path: 'config/conflict.toml' })
    configHandlers.config_validation_error({ message: 'bad config' })

    expect(addNotification).toHaveBeenCalledTimes(2)
    store.stop()
  })
})
