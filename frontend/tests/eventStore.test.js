import { describe, it, expect, vi, beforeEach } from 'vitest'

const buildWebSocketUrl = vi.hoisted(() => vi.fn((path) => `ws://test${path}`))

vi.mock('../src/lib/api.js', () => ({
  buildWebSocketUrl,
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
    this.listeners = new Map()
    this.sent = []
    MockWebSocket.instances.push(this)
  }

  addEventListener(type, handler) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set())
    }
    this.listeners.get(type).add(handler)
  }

  send(data) {
    this.sent.push(data)
  }

  close() {
    this.readyState = MockWebSocket.CLOSED
    this.dispatch('close', {})
  }

  open() {
    this.readyState = MockWebSocket.OPEN
    this.dispatch('open', {})
  }

  dispatch(type, payload) {
    const handlers = this.listeners.get(type)
    if (!handlers) return
    handlers.forEach((handler) => handler(payload))
  }
}

beforeEach(() => {
  MockWebSocket.instances = []
  vi.resetModules()
  global.WebSocket = MockWebSocket
})

const flush = () => Promise.resolve()

describe('eventStore', () => {
  it('subscribes and dispatches events', async () => {
    const { subscribe, eventConnectionStatus } = await import('../src/lib/eventStore.js')
    const received = []
    const statuses = []

    const unsubscribeStatus = eventConnectionStatus.subscribe((value) => {
      statuses.push(value)
    })
    const unsubscribe = subscribe('file_changed', (payload) => {
      received.push(payload.path)
    })

    const socket = MockWebSocket.instances[0]
    socket.open()
    await flush()

    expect(socket.sent[0]).toBe(JSON.stringify({ subscribe: ['file_changed'] }))

    socket.dispatch('message', {
      data: JSON.stringify({ type: 'file_changed', path: '.gestalt/PLAN.org' }),
    })

    expect(received).toEqual(['.gestalt/PLAN.org'])
    expect(statuses).toContain('connected')

    unsubscribe()
    unsubscribeStatus()
  })

  it('unsubscribes listeners', async () => {
    const { subscribe } = await import('../src/lib/eventStore.js')
    const received = []
    const unsubscribe = subscribe('git_branch_changed', (payload) => {
      received.push(payload.path)
    })

    const socket = MockWebSocket.instances[0]
    socket.open()
    await flush()

    unsubscribe()

    socket.dispatch('message', {
      data: JSON.stringify({ type: 'git_branch_changed', path: 'main' }),
    })

    expect(received).toEqual([])
  })
})
