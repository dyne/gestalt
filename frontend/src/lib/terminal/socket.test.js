import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { createTerminalSocket } from './socket.js'

vi.mock('../api.js', () => ({
  apiFetch: vi.fn(),
  buildWebSocketUrl: (path) => path,
}))

vi.mock('../notificationStore.js', () => ({
  notificationStore: {
    addNotification: vi.fn(),
  },
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
    MockWebSocket.instances.push(this)
  }

  addEventListener(type, listener) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set())
    }
    this.listeners.get(type).add(listener)
  }

  close() {
    this.readyState = MockWebSocket.CLOSED
    this.dispatch('close', { code: 1006, reason: '' })
  }

  dispatch(type, event) {
    const listeners = this.listeners.get(type)
    if (!listeners) return
    listeners.forEach((listener) => listener(event))
  }
}

const createStoreStub = () => ({
  set: vi.fn(),
})

const createTerminalStub = () => ({
  element: {},
  write: vi.fn(),
})

describe('createTerminalSocket', () => {
  beforeEach(() => {
    MockWebSocket.instances = []
    vi.useFakeTimers()
    vi.stubGlobal('WebSocket', MockWebSocket)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.clearAllMocks()
  })

  it('includes cursor from history response in websocket url', async () => {
    const { apiFetch } = await import('../api.js')
    apiFetch.mockResolvedValue({
      json: async () => ({ lines: ['hello'], cursor: 123 }),
    })

    const socketManager = createTerminalSocket({
      terminalId: 'alpha',
      term: createTerminalStub(),
      status: createStoreStub(),
      historyStatus: createStoreStub(),
      canReconnect: createStoreStub(),
      historyCache: new Map(),
      syncScrollState: () => {},
      scheduleFit: () => {},
    })

    await socketManager.connect()

    expect(MockWebSocket.instances).toHaveLength(1)
    expect(MockWebSocket.instances[0].url).toContain('/ws/terminal/alpha?cursor=123')
  })

  it('reuses history cursor on reconnect without reloading history', async () => {
    const { apiFetch } = await import('../api.js')
    apiFetch.mockImplementation(async (path) => {
      if (path.startsWith('/api/terminals/')) {
        return { json: async () => ({ lines: ['hello'], cursor: 77 }) }
      }
      return {}
    })

    const socketManager = createTerminalSocket({
      terminalId: 'bravo',
      term: createTerminalStub(),
      status: createStoreStub(),
      historyStatus: createStoreStub(),
      canReconnect: createStoreStub(),
      historyCache: new Map(),
      syncScrollState: () => {},
      scheduleFit: () => {},
    })

    await socketManager.connect()

    expect(MockWebSocket.instances).toHaveLength(1)
    MockWebSocket.instances[0].readyState = MockWebSocket.CLOSED

    await socketManager.connect(true)

    expect(MockWebSocket.instances).toHaveLength(2)
    expect(MockWebSocket.instances[1].url).toContain('/ws/terminal/bravo?cursor=77')
    expect(apiFetch).toHaveBeenCalledTimes(1)
  })
})
