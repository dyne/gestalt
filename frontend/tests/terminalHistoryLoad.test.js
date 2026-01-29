import { render, fireEvent, cleanup, waitFor } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

import { notificationStore } from '../src/lib/notificationStore.js'
import { createAppApiMocks, createLogStreamStub } from './helpers/appApiMocks.js'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))
const buildWebSocketUrl = vi.hoisted(() => vi.fn((path) => `ws://test${path}`))
const createLogStream = vi.hoisted(() => vi.fn())

const MockTerminal = vi.hoisted(
  () =>
    class {
      constructor() {
        this.cols = 80
        this.rows = 24
        this.element = null
        this.options = {}
        this.parser = {
          registerCsiHandler: () => ({ dispose() {} }),
        }
      }
      loadAddon() {}
      open(container) {
        this.element = document.createElement('div')
        container.appendChild(this.element)
      }
      write() {}
      onData() {}
      onBell() {}
      onScroll() {}
      attachCustomKeyEventHandler() {
        return true
      }
      hasSelection() {
        return false
      }
      getSelection() {
        return ''
      }
      scrollToBottom() {}
      focus() {}
      dispose() {}
    }
)

const MockFitAddon = vi.hoisted(
  () =>
    class {
      fit() {}
    }
)

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3

  constructor() {
    this.readyState = MockWebSocket.CONNECTING
    this.listeners = new Map()
  }

  addEventListener(type, handler) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set())
    }
    this.listeners.get(type).add(handler)
  }

  close() {
    this.readyState = MockWebSocket.CLOSED
    this.dispatch('close', { code: 1000, reason: '' })
  }

  send() {}

  dispatch(type, payload) {
    const handlers = this.listeners.get(type)
    if (!handlers) return
    handlers.forEach((handler) => handler(payload))
  }
}

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildEventSourceUrl,
  buildWebSocketUrl,
  buildApiPath: (base, ...segments) => {
    const basePath = base.endsWith('/') ? base.slice(0, -1) : base
    const encoded = segments
      .filter((segment) => segment !== undefined && segment !== null && segment !== '')
      .map((segment) => encodeURIComponent(String(segment)))
    return encoded.length ? `${basePath}/${encoded.join('/')}` : basePath
  },
}))

vi.mock('../src/lib/logStream.js', () => ({
  createLogStream,
}))

vi.mock('@xterm/xterm', () => ({
  Terminal: MockTerminal,
}))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: MockFitAddon,
}))

import App from '../src/App.svelte'

describe('terminal history loads', () => {
  let originalWebSocket

  beforeEach(() => {
    originalWebSocket = globalThis.WebSocket
    globalThis.WebSocket = MockWebSocket

    const terminals = [
      {
        id: 't1',
        title: 'Shell',
        role: 'shell',
        created_at: new Date().toISOString(),
      },
      {
        id: 't2',
        title: 'Ops',
        role: 'shell',
        created_at: new Date().toISOString(),
      },
    ]

    const appMocks = createAppApiMocks(apiFetch, {
      status: { session_count: terminals.length },
      terminals,
      agents: [],
    })

    apiFetch.mockImplementation((url) => {
      if (typeof url === 'string' && url.includes('/history')) {
        return Promise.resolve({ json: async () => ({ lines: [] }) })
      }
      return appMocks(url)
    })

    createLogStream.mockImplementation(() => createLogStreamStub())
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
    apiFetch.mockReset()
    notificationStore.clear()
    cleanup()
  })

  it('loads history only for the active terminal', async () => {
    const { findByRole } = render(App)
    const terminalTab = await findByRole('button', { name: 't1' })

    const initialHistoryCalls = apiFetch.mock.calls.filter(([url]) =>
      String(url).includes('/history')
    )
    expect(initialHistoryCalls).toHaveLength(0)

    await fireEvent.click(terminalTab)

    await waitFor(() => {
      const historyCalls = apiFetch.mock.calls.filter(([url]) =>
        String(url).includes('/history')
      )
      expect(historyCalls).toHaveLength(1)
    })
  })
})
