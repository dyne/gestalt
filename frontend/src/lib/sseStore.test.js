import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createSseStore } from './sseStore.js'

vi.mock('./api.js', () => ({
  buildEventSourceUrl: (path, params = {}) => {
    const search = new URLSearchParams(params).toString()
    return `http://test${path}${search ? `?${search}` : ''}`
  },
}))

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

describe('createSseStore', () => {
  beforeEach(() => {
    MockEventSource.instances = []
    vi.useFakeTimers()
    vi.stubGlobal('EventSource', MockEventSource)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('connects, subscribes, and dispatches typed messages', () => {
    const { subscribe, connectionStatus } = createSseStore({
      label: 'events',
      path: '/api/events/stream',
    })

    const statusUpdates = []
    const stopStatus = connectionStatus.subscribe((value) => statusUpdates.push(value))
    const received = []

    const stop = subscribe('alpha', (payload) => received.push(payload))
    const source = MockEventSource.instances[0]
    expect(source).toBeDefined()
    expect(statusUpdates.at(-1)).toBe('connecting')

    source.open()
    expect(statusUpdates.at(-1)).toBe('connected')

    source.message(JSON.stringify({ type: 'alpha', value: 1 }))
    source.message(JSON.stringify({ type: 'beta', value: 2 }))
    source.message('not-json')
    expect(received).toHaveLength(1)
    expect(received[0]).toEqual({ type: 'alpha', value: 1 })

    stop()
    stopStatus()
  })

  it('reconnects after errors when listeners remain', () => {
    const { subscribe, connectionStatus } = createSseStore({
      label: 'events',
      path: '/api/events/stream',
    })

    const statusUpdates = []
    const stopStatus = connectionStatus.subscribe((value) => statusUpdates.push(value))
    const stop = subscribe('alpha', () => {})

    const source = MockEventSource.instances[0]
    source.open()
    source.error()

    expect(statusUpdates.at(-1)).toBe('disconnected')
    expect(MockEventSource.instances).toHaveLength(1)

    vi.advanceTimersByTime(500)
    expect(MockEventSource.instances).toHaveLength(2)
    MockEventSource.instances[1].open()
    expect(statusUpdates.at(-1)).toBe('connected')

    stop()
    stopStatus()
  })

  it('disconnects when last listener unsubscribes', () => {
    const { subscribe, connectionStatus } = createSseStore({
      label: 'events',
      path: '/api/events/stream',
    })

    const statusUpdates = []
    const stopStatus = connectionStatus.subscribe((value) => statusUpdates.push(value))
    const stop = subscribe('alpha', () => {})
    const source = MockEventSource.instances[0]
    source.open()

    stop()
    vi.runAllTimers()

    expect(statusUpdates.at(-1)).toBe('disconnected')
    expect(MockEventSource.instances).toHaveLength(1)
    stopStatus()
  })

  it('ignores malformed payloads without crashing', () => {
    const { subscribe } = createSseStore({
      label: 'events',
      path: '/api/events/stream',
    })

    const received = []
    const stop = subscribe('alpha', (payload) => received.push(payload))
    const source = MockEventSource.instances[0]
    source.open()

    expect(() => {
      source.message(JSON.stringify({ path: '/tmp/file' }))
      source.message(JSON.stringify(['alpha']))
      source.message('null')
    }).not.toThrow()

    expect(received).toHaveLength(0)
    stop()
  })

  it('reconnects when query params change with subscriptions', () => {
    const { subscribe } = createSseStore({
      label: 'events',
      path: '/api/events/stream',
      buildQueryParams: (types) => ({ types: types.join(',') }),
    })

    const stopAlpha = subscribe('alpha', () => {})
    let source = MockEventSource.instances[0]
    expect(new URL(source.url).searchParams.get('types')).toBe('alpha')

    const stopBeta = subscribe('beta', () => {})
    expect(MockEventSource.instances).toHaveLength(2)
    source = MockEventSource.instances[1]
    expect(new URL(source.url).searchParams.get('types')).toBe('alpha,beta')

    stopBeta()
    stopAlpha()
  })
})
