import { render, cleanup, waitFor, fireEvent } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { notificationStore } from '../src/lib/notificationStore.js'
import { createLogStreamStub, defaultMetricsSummary } from './helpers/appApiMocks.js'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))
const createLogStream = vi.hoisted(() => vi.fn())
const eventListeners = vi.hoisted(() => new Map())

const subscribeEvents = vi.hoisted(() => (type, callback) => {
  if (!eventListeners.has(type)) {
    eventListeners.set(type, new Set())
  }
  eventListeners.get(type).add(callback)
  return () => {
    eventListeners.get(type)?.delete(callback)
  }
})

const emitEvent = (type, payload) => {
  const listeners = eventListeners.get(type)
  if (!listeners) return
  listeners.forEach((listener) => listener(payload))
}

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildEventSourceUrl,
  buildApiPath: (base, ...segments) => {
    const basePath = base.endsWith('/') ? base.slice(0, -1) : base
    const encoded = segments
      .filter((segment) => segment !== undefined && segment !== null && segment !== '')
      .map((segment) => encodeURIComponent(String(segment)))
    return encoded.length ? `${basePath}/${encoded.join('/')}` : basePath
  },
}))

vi.mock('../src/lib/eventStore.js', () => ({
  subscribe: subscribeEvents,
}))

vi.mock('../src/lib/logStream.js', () => ({
  createLogStream,
}))

vi.mock('../src/views/TerminalView.svelte', async () => {
  const module = await import('./helpers/TerminalViewStub.svelte')
  return { default: module.default }
})

import App from '../src/App.svelte'

describe('App agents tab refresh', () => {
  beforeEach(() => {
    eventListeners.clear()
    createLogStream.mockImplementation(() => createLogStreamStub())
    if (!Element.prototype.animate) {
      Element.prototype.animate = () => ({
        cancel() {},
        finish() {},
        onfinish: null,
      })
    }
  })

  afterEach(() => {
    apiFetch.mockReset()
    notificationStore.clear()
    cleanup()
  })

  it('refreshes status and shows Agents tab after terminal lifecycle events', async () => {
    let statusCalls = 0
    let sessionsCalls = 0

    apiFetch.mockImplementation((url) => {
      if (url === '/api/status') {
        statusCalls += 1
        const payload =
          statusCalls === 1
            ? { session_count: 0, agents_session_id: '', agents_tmux_session: '' }
            : { session_count: 1, agents_session_id: 'Agents 1', agents_tmux_session: '' }
        return Promise.resolve({ json: () => Promise.resolve(payload) })
      }
      if (url === '/api/sessions') {
        sessionsCalls += 1
        return Promise.resolve({ json: () => Promise.resolve([]) })
      }
      if (url === '/api/agents') {
        return Promise.resolve({ json: () => Promise.resolve([]) })
      }
      if (url === '/api/metrics/summary') {
        return Promise.resolve({ json: () => Promise.resolve(defaultMetricsSummary) })
      }
      if (url === '/api/otel/logs') {
        return Promise.resolve({ json: () => Promise.resolve({ ok: true }) })
      }
      if (url === '/api/plans') {
        return Promise.resolve({ json: () => Promise.resolve({ plans: [] }) })
      }
      return Promise.resolve({ json: () => Promise.resolve({}) })
    })

    const { queryByRole } = render(App)

    await waitFor(() => {
      expect(queryByRole('button', { name: 'Agents' })).toBeNull()
    })

    emitEvent('terminal_created', { type: 'terminal_created', data: { id: 'agents-1' } })

    await waitFor(() => {
      expect(queryByRole('button', { name: 'Agents' })).toBeTruthy()
    })

    expect(statusCalls).toBeGreaterThanOrEqual(2)
    expect(sessionsCalls).toBeGreaterThanOrEqual(2)
  })

  it('does not create an external agent when stopped agent is clicked', async () => {
    let statusCalls = 0
    let sessionsCalls = 0

    apiFetch.mockImplementation((url, options = {}) => {
      if (url === '/api/status') {
        statusCalls += 1
        return Promise.resolve({
          json: () => Promise.resolve({ session_count: 0, agents_session_id: '', agents_tmux_session: '' }),
        })
      }
      if (url === '/api/sessions') {
        sessionsCalls += 1
        return Promise.resolve({ json: () => Promise.resolve([]) })
      }
      if (url === '/api/agents') {
        return Promise.resolve({
          json: () => Promise.resolve([{ id: 'codex', name: 'Codex' }]),
        })
      }
      if (url === '/api/metrics/summary') {
        return Promise.resolve({ json: () => Promise.resolve(defaultMetricsSummary) })
      }
      if (url === '/api/otel/logs') {
        return Promise.resolve({ json: () => Promise.resolve({ ok: true }) })
      }
      if (url === '/api/plans') {
        return Promise.resolve({ json: () => Promise.resolve({ plans: [] }) })
      }
      return Promise.resolve({ json: () => Promise.resolve({}) })
    })

    const { findByText, queryByRole } = render(App)

    const button = await findByText('Codex')
    await fireEvent.click(button)

    expect(await findByText('Session not running; run gestalt-agent codex.')).toBeTruthy()

    expect(queryByRole('button', { name: 'Agents' })).toBeNull()
    expect(statusCalls).toBeGreaterThanOrEqual(1)
    expect(sessionsCalls).toBeGreaterThanOrEqual(1)
    const createCalls = apiFetch.mock.calls.filter(
      ([url, request]) => url === '/api/sessions' && request?.method === 'POST',
    )
    expect(createCalls).toHaveLength(0)
  })
})
