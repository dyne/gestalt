import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { get } from 'svelte/store'

const triggerScipReindex = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/apiClient.js', () => ({
  triggerScipReindex,
}))

import { createScipStore, initialScipStatus } from '../src/lib/scipStore.js'

describe('scipStore', () => {
  beforeEach(() => {
    triggerScipReindex.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('starts with default status', async () => {
    const store = createScipStore()
    await store.start()

    const status = get(store.status)
    expect(status).toEqual(initialScipStatus)
  })

  it('tracks reindex requests', async () => {
    triggerScipReindex.mockResolvedValueOnce({})

    const store = createScipStore()
    await store.start()

    await store.reindex()

    const status = get(store.status)
    expect(triggerScipReindex).toHaveBeenCalledTimes(1)
    expect(status.in_progress).toBe(false)
    expect(status.requested_at).not.toBe('')
    expect(status.error).toBe('')
  })

  it('reports reindex failures', async () => {
    triggerScipReindex.mockRejectedValueOnce(new Error('indexer missing'))

    const store = createScipStore()
    await store.start()

    await expect(store.reindex()).rejects.toThrow('indexer missing')

    const status = get(store.status)
    expect(status.in_progress).toBe(false)
    expect(status.error).toContain('indexer missing')
  })

  it('debounces simultaneous reindex attempts', async () => {
    let resolveReindex = null
    triggerScipReindex.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveReindex = resolve
        })
    )

    const store = createScipStore()
    await store.start()

    const first = store.reindex()
    const second = store.reindex()
    await second

    expect(triggerScipReindex).toHaveBeenCalledTimes(1)

    resolveReindex()
    await first
  })

  it('allows reindex after stop clears in-flight state', async () => {
    let resolveReindex = null
    triggerScipReindex.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveReindex = resolve
        })
    )

    const store = createScipStore()
    await store.start()

    const first = store.reindex()
    store.stop()

    resolveReindex()
    await first

    await store.start()
    triggerScipReindex.mockResolvedValueOnce({})
    await store.reindex()

    expect(triggerScipReindex).toHaveBeenCalledTimes(2)
  })
})
