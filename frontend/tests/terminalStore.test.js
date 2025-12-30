import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const buildWebSocketUrl = vi.hoisted(() => vi.fn((path) => `ws://test${path}`))
const apiFetch = vi.hoisted(() => vi.fn())
const addNotification = vi.hoisted(() => vi.fn())

const MockTerminal = vi.hoisted(
  () =>
    class {
      constructor() {
        this.cols = 80
        this.rows = 24
        this.element = null
        this.writes = []
        this.parser = {
          registerCsiHandler: () => ({ dispose() {} }),
        }
      }
      loadAddon() {}
      open(container) {
        this.element = document.createElement('div')
        container.appendChild(this.element)
      }
      write(data) {
        this.writes.push(data)
      }
      onData(handler) {
        this._onData = handler
      }
      onBell(handler) {
        this._onBell = handler
      }
      attachCustomKeyEventHandler(handler) {
        this._keyHandler = handler
        return true
      }
      hasSelection() {
        return false
      }
      getSelection() {
        return ''
      }
      dispose() {}
    }
)

const MockFitAddon = vi.hoisted(
  () =>
    class {
      fit() {}
    }
)

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildWebSocketUrl,
}))

vi.mock('../src/lib/notificationStore.js', () => ({
  notificationStore: {
    addNotification,
  },
}))

vi.mock('@xterm/xterm', () => ({
  Terminal: MockTerminal,
}))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: MockFitAddon,
}))

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
      if (path.includes('/output')) {
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

  it('connects and updates status on open', async () => {
    const state = getTerminalState('abc')
    const seen = []
    const unsubscribe = state.status.subscribe((value) => seen.push(value))

    await waitForSocket()
    const socket = MockWebSocket.instances[0]
    expect(socket.url).toBe('ws://test/ws/terminal/abc')
    socket.open()

    await flush()
    expect(seen).toContain('connected')

    unsubscribe()
    releaseTerminalState('abc')
  })

  it('marks unauthorized when auth fails on close', async () => {
    apiFetch.mockImplementation((path) => {
      if (path.includes('/output')) {
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

  it('loads history before attaching terminal output', async () => {
    apiFetch.mockImplementation((path) => {
      if (path.includes('/output')) {
        return Promise.resolve({
          ok: true,
          json: async () => ({ lines: ['first', 'second'] }),
        })
      }
      return Promise.resolve({ ok: true, json: async () => ({}) })
    })

    const state = getTerminalState('history')
    let historyStatus = ''
    const unsubscribe = state.historyStatus.subscribe((value) => {
      historyStatus = value
    })

    await flush()
    await flush()
    expect(historyStatus).toBe('loaded')
    expect(state.term.writes.length).toBe(0)

    const container = document.createElement('div')
    state.attach(container)

    expect(state.term.writes).toContain('first\nsecond')

    unsubscribe()
    releaseTerminalState('history')
  })

  it('warns when history load is slow', async () => {
    vi.useFakeTimers()
    let resolveFetch
    apiFetch.mockImplementation((path) => {
      if (path.includes('/output')) {
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
})
