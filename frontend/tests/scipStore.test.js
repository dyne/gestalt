import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { get } from 'svelte/store'

const fetchScipStatus = vi.hoisted(() => vi.fn())
const triggerScipReindex = vi.hoisted(() => vi.fn())
const wsHandlers = vi.hoisted(() => new Map())
const connectionStatusValue = vi.hoisted(() => ({ current: 'connected' }))
const connectionStatusSubscribers = vi.hoisted(() => new Set())
const connectionStatusStore = vi.hoisted(() => ({
  subscribe: (run) => {
    run(connectionStatusValue.current)
    connectionStatusSubscribers.add(run)
    return () => connectionStatusSubscribers.delete(run)
  },
}))
const subscribeMock = vi.hoisted(() =>
  vi.fn((type, callback) => {
    wsHandlers.set(type, callback)
    return () => wsHandlers.delete(type)
  })
)
const createWsStore = vi.hoisted(() =>
  vi.fn(() => ({
    subscribe: subscribeMock,
    connectionStatus: connectionStatusStore,
  }))
)

vi.mock('../src/lib/apiClient.js', () => ({
  fetchScipStatus,
  triggerScipReindex,
}))

vi.mock('../src/lib/wsStore.js', () => ({
  createWsStore,
}))

import { createScipStore, initialScipStatus } from '../src/lib/scipStore.js'

const flushPromises = () => new Promise((resolve) => setTimeout(resolve, 0))
const setConnectionStatus = (status) => {
  connectionStatusValue.current = status
  connectionStatusSubscribers.forEach((run) => run(status))
}

const emitEvent = (type, payload = {}) => {
  const handler = wsHandlers.get(type)
  if (!handler) return
  handler({ type, ...payload })
}

describe('scipStore', () => {
  beforeEach(() => {
    wsHandlers.clear()
    connectionStatusSubscribers.clear()
    setConnectionStatus('connected')
    fetchScipStatus.mockReset()
    triggerScipReindex.mockReset()
    subscribeMock.mockClear()
    createWsStore.mockClear()
  })

  afterEach(() => {
    wsHandlers.clear()
    connectionStatusSubscribers.clear()
  })

  it('loads scip status on start', async () => {
    fetchScipStatus.mockResolvedValue({ indexed: true, languages: ['Go'] })

    const store = createScipStore()
    await store.start()

    const status = get(store.status)
    expect(fetchScipStatus).toHaveBeenCalledTimes(1)
    expect(status.indexed).toBe(true)
    expect(status.languages).toEqual(['go'])
    expect(subscribeMock).toHaveBeenCalledTimes(4)
  })

  it('updates status from websocket events and refreshes on completion', async () => {
    fetchScipStatus
      .mockResolvedValueOnce({ indexed: false, languages: [] })
      .mockResolvedValueOnce({ indexed: true, languages: ['go', 'typescript'] })

    const store = createScipStore()
    await store.start()

    emitEvent('start', { timestamp: '2026-01-25T00:00:00Z', language: 'go' })
    let status = get(store.status)
    expect(status.in_progress).toBe(true)
    expect(status.started_at).toBe('2026-01-25T00:00:00Z')
    expect(status.languages).toEqual(['go'])

    emitEvent('progress', { language: 'TypeScript' })
    status = get(store.status)
    expect(status.languages).toEqual(['go', 'typescript'])

    emitEvent('complete', { timestamp: '2026-01-25T00:01:00Z' })
    await flushPromises()

    status = get(store.status)
    expect(status.in_progress).toBe(false)
    expect(status.indexed).toBe(true)
    expect(fetchScipStatus).toHaveBeenCalledTimes(2)
  })

  it('reports reindex failures', async () => {
    fetchScipStatus.mockResolvedValue(initialScipStatus)
    triggerScipReindex.mockRejectedValueOnce(new Error('indexer missing'))

    const store = createScipStore()
    await store.start()

    await expect(store.reindex()).rejects.toThrow('indexer missing')
    const status = get(store.status)
    expect(status.in_progress).toBe(false)
    expect(status.error).toContain('indexer missing')
  })
})
