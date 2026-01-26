import { describe, it, expect, vi } from 'vitest'

const logUI = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/clientLog.js', () => ({
  logUI,
}))

import { notificationPreferences, notificationStore } from '../src/lib/notificationStore.js'

const resetPreferences = () => {
  notificationPreferences.set({
    enabled: true,
    durationMs: 0,
    levelFilter: 'all',
  })
}

const subscribeToStore = () => {
  let current = []
  const unsubscribe = notificationStore.subscribe((value) => {
    current = value
  })
  return { get: () => current, unsubscribe }
}

describe('notificationStore', () => {
  it('adds and dismisses entries', async () => {
    resetPreferences()
    notificationStore.clear()
    const { get, unsubscribe } = subscribeToStore()

    const id = notificationStore.addNotification('info', 'hello', { duration: 10 })
    expect(get().length).toBe(1)
    expect(get()[0].id).toBe(id)
    expect(get()[0].level).toBe('info')

    await new Promise((resolve) => setTimeout(resolve, 30))
    expect(get().length).toBe(0)
    unsubscribe()
  })

  it('does not auto-dismiss errors by default', async () => {
    resetPreferences()
    notificationStore.clear()
    const { get, unsubscribe } = subscribeToStore()

    const id = notificationStore.addNotification('error', 'boom')
    expect(get().length).toBe(1)

    await new Promise((resolve) => setTimeout(resolve, 30))
    expect(get().length).toBe(1)

    notificationStore.dismiss(id)
    expect(get().length).toBe(0)
    unsubscribe()
  })

  it('clear removes active timers', async () => {
    resetPreferences()
    notificationStore.clear()
    const { get, unsubscribe } = subscribeToStore()

    notificationStore.addNotification('warning', 'heads up', { duration: 100 })
    expect(get().length).toBe(1)

    notificationStore.clear()
    expect(get().length).toBe(0)

    await new Promise((resolve) => setTimeout(resolve, 120))
    expect(get().length).toBe(0)
    unsubscribe()
  })

  it('honors level filter', async () => {
    resetPreferences()
    notificationStore.clear()
    notificationPreferences.set({
      enabled: true,
      durationMs: 0,
      levelFilter: 'warning',
    })

    const { get, unsubscribe } = subscribeToStore()

    const ignored = notificationStore.addNotification('info', 'skip me')
    expect(ignored).toBeNull()
    expect(get().length).toBe(0)

    const id = notificationStore.addNotification('warning', 'show me', { duration: 10 })
    expect(get().length).toBe(1)
    expect(get()[0].id).toBe(id)

    await new Promise((resolve) => setTimeout(resolve, 30))
    expect(get().length).toBe(0)
    unsubscribe()
  })

  it('applies duration override', async () => {
    resetPreferences()
    notificationStore.clear()
    notificationPreferences.set({
      enabled: true,
      durationMs: 5,
      levelFilter: 'all',
    })

    const { get, unsubscribe } = subscribeToStore()

    notificationStore.addNotification('info', 'short')
    expect(get().length).toBe(1)

    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(get().length).toBe(0)
    unsubscribe()
  })
})
