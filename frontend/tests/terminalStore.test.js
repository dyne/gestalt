import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const buildWebSocketUrl = vi.hoisted(() => vi.fn((path) => `ws://test${path}`))
const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))
const addNotification = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildWebSocketUrl,
  buildEventSourceUrl,
  buildApiPath: (base, ...segments) => {
    const basePath = base.endsWith('/') ? base.slice(0, -1) : base
    const encoded = segments
      .filter((segment) => segment !== undefined && segment !== null && segment !== '')
      .map((segment) => encodeURIComponent(String(segment)))
    return encoded.length ? `${basePath}/${encoded.join('/')}` : basePath
  },
}))

vi.mock('../src/lib/notificationStore.js', () => ({
  notificationStore: {
    addNotification,
  },
}))

import { createTerminalService } from '../src/lib/terminal/service.js'
import { getTerminalState, releaseTerminalState } from '../src/lib/terminalStore.js'

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

  close(code = 1000, reason = '') {
    this.readyState = MockWebSocket.CLOSED
    this.dispatch('close', { code, reason })
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

const flush = () => Promise.resolve()
const waitForSocket = async () => {
  for (let i = 0; i < 5; i += 1) {
    if (MockWebSocket.instances.length > 0) {
      return
    }
    await flush()
  }
}

describe('terminalStore', () => {
  let originalWebSocket
  let originalAnimationFrame

  beforeEach(() => {
    originalWebSocket = globalThis.WebSocket
    originalAnimationFrame = globalThis.requestAnimationFrame
    globalThis.WebSocket = MockWebSocket
    globalThis.requestAnimationFrame = (cb) => setTimeout(cb, 0)
    MockWebSocket.instances = []
    buildWebSocketUrl.mockClear()
    apiFetch.mockReset()
    apiFetch.mockImplementation((path) => {
      if (path.includes('/history')) {
        return Promise.resolve({
          ok: true,
          json: async () => ({ lines: [] }),
        })
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      })
    })
    addNotification.mockReset()
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
    globalThis.requestAnimationFrame = originalAnimationFrame
  })

  it('creates terminal service instances', async () => {
    const service = createTerminalService({ terminalId: 'svc', historyCache: new Map() })
    const seen = []
    const unsubscribe = service.status.subscribe((value) => seen.push(value))

    service.setVisible(true)
    await waitForSocket()
    expect(seen.length).toBeGreaterThan(0)

    unsubscribe()
    service.dispose()
  })

  it('hydrates text from cached history on connect', async () => {
    const historyCache = new Map()
    historyCache.set('cached', {
      text: 'hello\nworld',
      lines: ['hello', 'world'],
      cursor: 7,
    })
    const service = createTerminalService({ terminalId: 'cached', historyCache })
    let currentSegments = []
    const unsubscribe = service.segments.subscribe((value) => {
      currentSegments = value
    })

    service.setVisible(true)
    await waitForSocket()

    expect(currentSegments).toEqual([{ kind: 'output', text: 'hello\nworld' }])

    unsubscribe()
    service.dispose()
  })

  it('connects and updates status on open', async () => {
    const state = getTerminalState('abc')
    const seen = []
    const unsubscribe = state.status.subscribe((value) => seen.push(value))

    state.setVisible(true)
    await waitForSocket()
    const socket = MockWebSocket.instances[0]
    expect(socket.url).toBe('ws://test/ws/session/abc')
    socket.open()

    await flush()
    expect(seen).toContain('connected')

    unsubscribe()
    releaseTerminalState('abc')
  })

  it('marks unauthorized when auth fails on close', async () => {
    apiFetch.mockImplementation((path) => {
      if (path.includes('/history')) {
        return Promise.resolve({
          ok: true,
          json: async () => ({ lines: [] }),
        })
      }
      if (path.includes('/api/status')) {
        return Promise.reject({ status: 401 })
      }
      return Promise.resolve({ ok: true, json: async () => ({}) })
    })
    const state = getTerminalState('auth')
    let status = ''
    const unsubscribe = state.status.subscribe((value) => {
      status = value
    })

    state.setVisible(true)
    await waitForSocket()
    const socket = MockWebSocket.instances[0]
    socket.open()
    socket.close(1008)

    await flush()
    await flush()

    expect(status).toBe('unauthorized')
    expect(addNotification).toHaveBeenCalled()

    unsubscribe()
    releaseTerminalState('auth')
  })

  it('retries after non-auth close', async () => {
    vi.useFakeTimers()
    const state = getTerminalState('retry')
    let status = ''
    const unsubscribe = state.status.subscribe((value) => {
      status = value
    })

    state.setVisible(true)
    await waitForSocket()
    const socket = MockWebSocket.instances[0]
    socket.open()
    socket.close(1006)

    await flush()
    await flush()
    expect(status).toBe('retrying')

    vi.advanceTimersByTime(500)
    await flush()

    expect(MockWebSocket.instances.length).toBeGreaterThan(1)

    vi.useRealTimers()
    unsubscribe()
    releaseTerminalState('retry')
  })

  it('recreates services when interface changes', () => {
    const mcpState = getTerminalState('swap', 'mcp')
    const disposeSpy = vi.spyOn(mcpState, 'dispose')

    const cliState = getTerminalState('swap', 'cli')

    expect(cliState).not.toBe(mcpState)
    expect(disposeSpy).toHaveBeenCalled()

    releaseTerminalState('swap')
  })

  it('warns when history load is slow', async () => {
    vi.useFakeTimers()
    let resolveFetch
    apiFetch.mockImplementation((path) => {
      if (path.includes('/history')) {
        return new Promise((resolve) => {
          resolveFetch = resolve
        })
      }
      return Promise.resolve({ ok: true, json: async () => ({}) })
    })

    const state = getTerminalState('slow')
    let historyStatus = ''
    const unsubscribe = state.historyStatus.subscribe((value) => {
      historyStatus = value
    })

    state.setVisible(true)
    await flush()
    vi.advanceTimersByTime(5000)
    await flush()

    expect(historyStatus).toBe('slow')
    expect(addNotification).toHaveBeenCalled()

    resolveFetch({ ok: true, json: async () => ({ lines: [] }) })
    await flush()
    await flush()

    expect(historyStatus).toBe('loaded')

    vi.useRealTimers()
    unsubscribe()
    releaseTerminalState('slow')
  })

  it('does not open websocket for external runner sessions', async () => {
    const state = getTerminalState('ext', 'cli', 'external')
    let status = ''
    const unsubscribe = state.status.subscribe((value) => {
      status = value
    })

    state.setVisible(true)
    await flush()

    expect(MockWebSocket.instances.length).toBe(0)
    expect(status).toBe('disconnected')

    unsubscribe()
    releaseTerminalState('ext')
  })
})
