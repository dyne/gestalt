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

describe('wsStore', () => {
  it('sends subscription payloads on connect', async () => {
    const { createWsStore } = await import('../src/lib/wsStore.js')
    const { subscribe } = createWsStore({
      label: 'ws-test',
      path: '/ws/test',
      buildSubscribeMessage: (types) => ({ subscribe: types }),
    })

    const unsubscribe = subscribe('file-change', () => {})
    const socket = MockWebSocket.instances[0]

    socket.open()
    await flush()

    expect(socket.sent[0]).toBe(JSON.stringify({ subscribe: ['file-change'] }))

    unsubscribe()
  })

  it('avoids subscription payloads without a builder', async () => {
    const { createWsStore } = await import('../src/lib/wsStore.js')
    const { subscribe } = createWsStore({
      label: 'ws-test',
      path: '/ws/test',
    })

    const unsubscribe = subscribe('file-change', () => {})
    const socket = MockWebSocket.instances[0]

    socket.open()
    await flush()

    expect(socket.sent).toEqual([])

    unsubscribe()
  })
})
