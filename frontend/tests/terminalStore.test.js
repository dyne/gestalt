import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const buildWebSocketUrl = vi.hoisted(() => vi.fn((path) => `ws://test${path}`))
const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))
const addNotification = vi.hoisted(() => vi.fn())

const MockTerminal = vi.hoisted(
  () =>
    class {
      constructor() {
        this.cols = 80
        this.rows = 24
        this.element = null
        this.writes = []
        this.scrollLinesCalls = []
        this.parser = {
          registerCsiHandler: () => ({ dispose() {} }),
        }
      }
      loadAddon() {}
      open(container) {
        this.element = document.createElement('div')
        this.viewport = document.createElement('div')
        this.viewport.className = 'xterm-viewport'
        this.element.appendChild(this.viewport)
        this.element.setPointerCapture = () => {}
        this.element.releasePointerCapture = () => {}
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
      scrollLines(lines) {
        this.scrollLinesCalls.push(lines)
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
  buildEventSourceUrl,
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

const createPointerEvent = (
  type,
  { pointerId = 1, pointerType = 'touch', clientX = 0, clientY = 0, timeStamp = 0 } = {}
) => {
  const event = new Event(type, { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'pointerId', { value: pointerId })
  Object.defineProperty(event, 'pointerType', { value: pointerType })
  Object.defineProperty(event, 'clientX', { value: clientX })
  Object.defineProperty(event, 'clientY', { value: clientY })
  Object.defineProperty(event, 'timeStamp', { value: timeStamp })
  return event
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
    expect(service.term).toBeTruthy()
    const seen = []
    const unsubscribe = service.status.subscribe((value) => seen.push(value))

    await waitForSocket()
    expect(seen.length).toBeGreaterThan(0)

    unsubscribe()
    service.dispose()
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
      if (path.includes('/history')) {
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

  it('scrolls on touch pointer move', async () => {
    const state = getTerminalState('touch-scroll')
    const container = document.createElement('div')
    state.attach(container)

    const element = state.term.element
    element.dispatchEvent(
      createPointerEvent('pointerdown', { pointerType: 'touch', clientY: 100, pointerId: 1 })
    )
    element.dispatchEvent(
      createPointerEvent('pointermove', { pointerType: 'touch', clientY: 80, pointerId: 1 })
    )
    element.dispatchEvent(
      createPointerEvent('pointerup', { pointerType: 'touch', clientY: 80, pointerId: 1 })
    )
    element.dispatchEvent(
      createPointerEvent('pointermove', { pointerType: 'touch', clientY: 60, pointerId: 1 })
    )

    expect(state.term.scrollLinesCalls).toEqual([1])

    releaseTerminalState('touch-scroll')
  })

  it('applies scroll sensitivity multiplier', async () => {
    const state = getTerminalState('touch-sensitivity')
    const container = document.createElement('div')
    state.attach(container)
    state.setScrollSensitivity(2)

    const element = state.term.element
    element.dispatchEvent(
      createPointerEvent('pointerdown', { pointerType: 'touch', clientY: 100, pointerId: 1 })
    )
    element.dispatchEvent(
      createPointerEvent('pointermove', { pointerType: 'touch', clientY: 80, pointerId: 1 })
    )
    element.dispatchEvent(
      createPointerEvent('pointerup', { pointerType: 'touch', clientY: 80, pointerId: 1 })
    )

    expect(state.term.scrollLinesCalls).toEqual([2])

    releaseTerminalState('touch-sensitivity')
  })

  it('waits for the scroll threshold before scrolling', async () => {
    const state = getTerminalState('touch-threshold')
    const container = document.createElement('div')
    state.attach(container)

    const element = state.term.element
    element.dispatchEvent(
      createPointerEvent('pointerdown', { pointerType: 'touch', clientY: 100, pointerId: 1 })
    )
    element.dispatchEvent(
      createPointerEvent('pointermove', { pointerType: 'touch', clientY: 95, pointerId: 1 })
    )

    expect(state.term.scrollLinesCalls).toEqual([])

    element.dispatchEvent(
      createPointerEvent('pointermove', { pointerType: 'touch', clientY: 85, pointerId: 1 })
    )
    element.dispatchEvent(
      createPointerEvent('pointerup', { pointerType: 'touch', clientY: 85, pointerId: 1 })
    )

    expect(state.term.scrollLinesCalls).toEqual([1])

    releaseTerminalState('touch-threshold')
  })

  it('continues with inertia after touch release', async () => {
    const originalRaf = globalThis.requestAnimationFrame
    const originalCancel = globalThis.cancelAnimationFrame
    let rafTime = 32
    let rafCallbacks = []

    globalThis.requestAnimationFrame = (callback) => {
      rafCallbacks.push(callback)
      return rafCallbacks.length
    }
    globalThis.cancelAnimationFrame = () => {}

    const runRaf = () => {
      const callbacks = rafCallbacks
      rafCallbacks = []
      rafTime += 16
      callbacks.forEach((callback) => callback(rafTime))
    }

    const state = getTerminalState('touch-inertia')
    const container = document.createElement('div')
    state.attach(container)

    const element = state.term.element
    element.dispatchEvent(
      createPointerEvent('pointerdown', {
        pointerType: 'touch',
        clientY: 100,
        pointerId: 1,
        timeStamp: 0,
      })
    )
    element.dispatchEvent(
      createPointerEvent('pointermove', {
        pointerType: 'touch',
        clientY: 0,
        pointerId: 1,
        timeStamp: 16,
      })
    )
    element.dispatchEvent(
      createPointerEvent('pointerup', {
        pointerType: 'touch',
        clientY: 0,
        pointerId: 1,
        timeStamp: 32,
      })
    )

    runRaf()

    expect(state.term.scrollLinesCalls.length).toBeGreaterThan(1)

    releaseTerminalState('touch-inertia')
    globalThis.requestAnimationFrame = originalRaf
    globalThis.cancelAnimationFrame = originalCancel
  })

  it('accumulates momentum with successive swipes', async () => {
    const originalRaf = globalThis.requestAnimationFrame
    const originalCancel = globalThis.cancelAnimationFrame
    let rafTime = 32
    let rafCallbacks = []

    globalThis.requestAnimationFrame = (callback) => {
      rafCallbacks.push(callback)
      return rafCallbacks.length
    }
    globalThis.cancelAnimationFrame = () => {}

    const runRaf = () => {
      const callbacks = rafCallbacks
      rafCallbacks = []
      rafTime += 16
      callbacks.forEach((callback) => callback(rafTime))
    }

    const state = getTerminalState('touch-inertia-boost')
    const container = document.createElement('div')
    state.attach(container)

    const element = state.term.element
    element.dispatchEvent(
      createPointerEvent('pointerdown', {
        pointerType: 'touch',
        clientY: 120,
        pointerId: 1,
        timeStamp: 0,
      })
    )
    element.dispatchEvent(
      createPointerEvent('pointermove', {
        pointerType: 'touch',
        clientY: 0,
        pointerId: 1,
        timeStamp: 16,
      })
    )
    element.dispatchEvent(
      createPointerEvent('pointerup', {
        pointerType: 'touch',
        clientY: 0,
        pointerId: 1,
        timeStamp: 32,
      })
    )

    runRaf()

    element.dispatchEvent(
      createPointerEvent('pointerdown', {
        pointerType: 'touch',
        clientY: 120,
        pointerId: 2,
        timeStamp: 48,
      })
    )
    element.dispatchEvent(
      createPointerEvent('pointermove', {
        pointerType: 'touch',
        clientY: 0,
        pointerId: 2,
        timeStamp: 64,
      })
    )
    element.dispatchEvent(
      createPointerEvent('pointerup', {
        pointerType: 'touch',
        clientY: 0,
        pointerId: 2,
        timeStamp: 80,
      })
    )

    runRaf()

    expect(state.term.scrollLinesCalls.length).toBeGreaterThan(2)

    releaseTerminalState('touch-inertia-boost')
    globalThis.requestAnimationFrame = originalRaf
    globalThis.cancelAnimationFrame = originalCancel
  })

  it('ignores mouse pointer events for scrolling', async () => {
    const state = getTerminalState('mouse-scroll')
    const container = document.createElement('div')
    state.attach(container)

    const element = state.term.element
    element.dispatchEvent(
      createPointerEvent('pointerdown', { pointerType: 'mouse', clientY: 100, pointerId: 1 })
    )
    element.dispatchEvent(
      createPointerEvent('pointermove', { pointerType: 'mouse', clientY: 80, pointerId: 1 })
    )
    element.dispatchEvent(
      createPointerEvent('pointerup', { pointerType: 'mouse', clientY: 80, pointerId: 1 })
    )

    expect(state.term.scrollLinesCalls).toEqual([])

    releaseTerminalState('mouse-scroll')
  })

  it('allows scrollbar touch drag without custom scroll handling', async () => {
    const state = getTerminalState('scrollbar-touch')
    const container = document.createElement('div')
    state.attach(container)

    const viewport = state.term.viewport
    Object.defineProperty(viewport, 'offsetWidth', { value: 100 })
    Object.defineProperty(viewport, 'clientWidth', { value: 80 })
    viewport.getBoundingClientRect = () => ({
      left: 0,
      right: 100,
      top: 0,
      bottom: 100,
      width: 100,
      height: 100,
    })
    viewport.dispatchEvent(
      createPointerEvent('pointerdown', {
        pointerType: 'touch',
        clientX: 95,
        clientY: 100,
        pointerId: 1,
      })
    )
    viewport.dispatchEvent(
      createPointerEvent('pointermove', {
        pointerType: 'touch',
        clientX: 95,
        clientY: 0,
        pointerId: 1,
      })
    )
    viewport.dispatchEvent(
      createPointerEvent('pointerup', {
        pointerType: 'touch',
        clientX: 95,
        clientY: 0,
        pointerId: 1,
      })
    )

    expect(state.term.scrollLinesCalls).toEqual([])

    releaseTerminalState('scrollbar-touch')
  })
})
