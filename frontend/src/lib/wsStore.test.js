import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createWsStore } from './wsStore.js'

vi.mock('./api.js', () => ({
  buildWebSocketUrl: (path) => `ws://test${path}`,
}))

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  static instances = []

  constructor(url) {
    this.url = url
    this.readyState = MockWebSocket.CONNECTING
    this.sent = []
    this.listeners = new Map()
    MockWebSocket.instances.push(this)
  }

  addEventListener(type, listener) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set())
    }
    this.listeners.get(type).add(listener)
  }

  send(data) {
    this.sent.push(data)
  }

  close() {
    if (this.readyState === MockWebSocket.CLOSED) return
    this.readyState = MockWebSocket.CLOSED
    this.dispatch('close', {})
  }

  dispatch(type, event) {
    const listeners = this.listeners.get(type)
    if (!listeners) return
    listeners.forEach((listener) => listener(event))
  }

  open() {
    this.readyState = MockWebSocket.OPEN
    this.dispatch('open', {})
  }

  message(data) {
    this.dispatch('message', { data })
  }

  error() {
    this.dispatch('error', {})
  }
}

describe('createWsStore', () => {
  beforeEach(() => {
    MockWebSocket.instances = []
    vi.useFakeTimers()
    vi.stubGlobal('WebSocket', MockWebSocket)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('connects, subscribes, and dispatches typed messages', () => {
    const { subscribe, connectionStatus } = createWsStore({
      label: 'events',
      path: '/ws/events',
      buildSubscribeMessage: (types) => ({ subscribe: types }),
    })

    const statusUpdates = []
    const stopStatus = connectionStatus.subscribe((value) => statusUpdates.push(value))
    const received = []

    const stop = subscribe('alpha', (payload) => received.push(payload))
    const socket = MockWebSocket.instances[0]
    expect(socket).toBeDefined()
    expect(statusUpdates.at(-1)).toBe('connecting')

    socket.open()
    expect(statusUpdates.at(-1)).toBe('connected')
    expect(socket.sent).toHaveLength(1)
    expect(JSON.parse(socket.sent[0])).toEqual({ subscribe: ['alpha'] })

    socket.message(JSON.stringify({ type: 'alpha', value: 1 }))
    socket.message(JSON.stringify({ type: 'beta', value: 2 }))
    socket.message('not-json')
    expect(received).toHaveLength(1)
    expect(received[0]).toEqual({ type: 'alpha', value: 1 })

    stop()
    stopStatus()
  })

  it('resubscribes and reconnects after close when listeners remain', () => {
    const { subscribe, connectionStatus } = createWsStore({
      label: 'events',
      path: '/ws/events',
    })

    const statusUpdates = []
    const stopStatus = connectionStatus.subscribe((value) => statusUpdates.push(value))
    const stop = subscribe('alpha', () => {})

    const socket = MockWebSocket.instances[0]
    socket.open()
    socket.close()

    expect(statusUpdates.at(-1)).toBe('disconnected')
    expect(MockWebSocket.instances).toHaveLength(1)

    vi.advanceTimersByTime(500)
    expect(MockWebSocket.instances).toHaveLength(2)
    MockWebSocket.instances[1].open()
    expect(statusUpdates.at(-1)).toBe('connected')

    stop()
    stopStatus()
  })

  it('disconnects and cancels reconnects when last listener unsubscribes', () => {
    const { subscribe, connectionStatus } = createWsStore({
      label: 'events',
      path: '/ws/events',
    })

    const statusUpdates = []
    const stopStatus = connectionStatus.subscribe((value) => statusUpdates.push(value))
    const stop = subscribe('alpha', () => {})
    const socket = MockWebSocket.instances[0]
    socket.open()

    stop()
    vi.runAllTimers()

    expect(statusUpdates.at(-1)).toBe('disconnected')
    expect(MockWebSocket.instances).toHaveLength(1)
    stopStatus()
  })

  it('handles subscribe message builders that return empty payloads', () => {
    const { subscribe } = createWsStore({
      label: 'events',
      path: '/ws/events',
      buildSubscribeMessage: () => null,
    })

    const stop = subscribe('alpha', () => {})
    const socket = MockWebSocket.instances[0]
    socket.open()

    expect(socket.sent).toHaveLength(0)
    stop()
  })

  it('swallows listener errors to keep streams alive', () => {
    const { subscribe } = createWsStore({
      label: 'events',
      path: '/ws/events',
    })

    const stop = subscribe('alpha', () => {
      throw new Error('boom')
    })
    const socket = MockWebSocket.instances[0]
    socket.open()

    expect(() => socket.message(JSON.stringify({ type: 'alpha' }))).not.toThrow()
    socket.message(JSON.stringify({ type: 'alpha' }))

    stop()
  })
})
