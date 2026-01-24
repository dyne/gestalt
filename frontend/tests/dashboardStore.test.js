import { describe, it, expect, vi, afterEach } from 'vitest'
import { get } from 'svelte/store'

const fetchAgents = vi.hoisted(() => vi.fn())
const fetchAgentSkills = vi.hoisted(() => vi.fn())
const fetchLogs = vi.hoisted(() => vi.fn())
const addNotification = vi.hoisted(() => vi.fn())

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

import { createDashboardStore } from '../src/lib/dashboardStore.js'

describe('dashboardStore', () => {
  afterEach(() => {
    fetchAgents.mockReset()
    fetchAgentSkills.mockReset()
    fetchLogs.mockReset()
    addNotification.mockReset()
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
})
