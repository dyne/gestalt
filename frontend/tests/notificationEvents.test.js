import { render, cleanup, waitFor } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { get } from 'svelte/store'
import { notificationPreferences, notificationStore } from '../src/lib/notificationStore.js'
import { createAppApiMocks, createLogStreamStub } from './helpers/appApiMocks.js'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))
const createLogStream = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildEventSourceUrl,
}))

vi.mock('../src/lib/logStream.js', () => ({
  createLogStream,
}))

vi.mock('../src/views/TerminalView.svelte', async () => {
  const module = await import('./helpers/TerminalViewStub.svelte')
  return { default: module.default }
})

import App from '../src/App.svelte'

class MockEventSource {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 2
  static instances = []

  constructor(url) {
    this.url = url
    this.readyState = MockEventSource.CONNECTING
    this.listeners = new Map()
    MockEventSource.instances.push(this)
  }

  addEventListener(type, listener) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set())
    }
    this.listeners.get(type).add(listener)
  }

  close() {
    if (this.readyState === MockEventSource.CLOSED) return
    this.readyState = MockEventSource.CLOSED
  }

  dispatch(type, event) {
    const listeners = this.listeners.get(type)
    if (!listeners) return
    listeners.forEach((listener) => listener(event))
  }

  open() {
    this.readyState = MockEventSource.OPEN
    this.dispatch('open', {})
  }

  message(data) {
    this.dispatch('message', { data })
  }

  error() {
    this.dispatch('error', {})
  }
}

describe('notification stream', () => {
  beforeEach(() => {
    if (!Element.prototype.animate) {
      Element.prototype.animate = () => ({
        cancel() {},
        finish() {},
        onfinish: null,
      })
    }
    MockEventSource.instances = []
    apiFetch.mockImplementation(createAppApiMocks(apiFetch))
    createLogStream.mockImplementation(() => createLogStreamStub())
    notificationPreferences.set({
      enabled: true,
      durationMs: 0,
      levelFilter: 'all',
    })
    notificationStore.clear()
    vi.stubGlobal('EventSource', MockEventSource)
  })

  afterEach(() => {
    notificationStore.clear()
    cleanup()
    vi.unstubAllGlobals()
  })

  it('adds toasts from notification events', async () => {
    render(App)

    await waitFor(() => {
      expect(
        MockEventSource.instances.some((instance) =>
          instance.url.includes('/api/notifications/stream')
        )
      ).toBe(true)
    })

    const source = MockEventSource.instances.find((instance) =>
      instance.url.includes('/api/notifications/stream')
    )
    expect(source).toBeTruthy()
    source.open()
    source.message(JSON.stringify({ type: 'toast', level: 'info', message: 'Hello' }))

    await waitFor(() => {
      const items = get(notificationStore)
      expect(items).toHaveLength(1)
      expect(items[0].message).toBe('Hello')
      expect(items[0].level).toBe('info')
    })
  })
})
