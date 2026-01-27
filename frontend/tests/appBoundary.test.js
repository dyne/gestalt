import { render, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { notificationStore } from '../src/lib/notificationStore.js'

const apiFetch = vi.hoisted(() => vi.fn())
const createScipStore = vi.hoisted(() =>
  vi.fn(() => ({
    status: {
      subscribe: (run) => {
        run({
          indexed: false,
          fresh: false,
          in_progress: false,
          started_at: '',
          completed_at: '',
          requested_at: '',
          duration: '',
          error: '',
          created_at: '',
          documents: 0,
          symbols: 0,
          age_hours: 0,
          languages: [],
        })
        return () => {}
      },
    },
    start: vi.fn(() => Promise.resolve()),
    stop: vi.fn(),
    reindex: vi.fn(() => Promise.resolve()),
    connectionStatus: { subscribe: () => () => {} },
  }))
)
const createLogStream = vi.hoisted(() =>
  vi.fn(() => ({
    start: vi.fn(),
    stop: vi.fn(),
    restart: vi.fn(),
    setLevel: vi.fn(),
  }))
)

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

vi.mock('../src/lib/scipStore.js', () => ({
  createScipStore,
  initialScipStatus: {},
}))

vi.mock('../src/lib/logStream.js', () => ({
  createLogStream,
}))

vi.mock('../src/views/Dashboard.svelte', async () => {
  const module = await import('./helpers/ThrowingView.svelte')
  return { default: module.default }
})

vi.mock('../src/views/TerminalView.svelte', async () => {
  const module = await import('./helpers/TerminalViewStub.svelte')
  return { default: module.default }
})

import App from '../src/App.svelte'

describe('App view boundaries', () => {
  beforeEach(() => {
    if (!Element.prototype.animate) {
      Element.prototype.animate = () => ({
        cancel() {},
        finish() {},
        onfinish: null,
      })
    }
    vi.useFakeTimers()
    sessionStorage.clear()
    Object.defineProperty(window, 'location', {
      value: { reload: vi.fn() },
      writable: true,
    })
    apiFetch.mockImplementation((url) => {
      if (url === '/api/status') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue({ terminal_count: 0 }),
        })
      }
      if (url === '/api/terminals') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([]),
        })
      }
      if (url === '/api/agents') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([]),
        })
      }
      if (url.startsWith('/api/skills')) {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([]),
        })
      }
      if (url === '/api/metrics/summary') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue({
            updated_at: '',
            top_endpoints: [],
            slowest_endpoints: [],
            top_agents: [],
            error_rates: [],
          }),
        })
      }
      if (url === '/api/otel/logs') {
        return Promise.resolve({ json: vi.fn().mockResolvedValue({ ok: true }) })
      }
      return Promise.resolve({ json: vi.fn().mockResolvedValue({}) })
    })
  })

  afterEach(() => {
    notificationStore.clear()
    sessionStorage.clear()
    apiFetch.mockReset()
    cleanup()
    vi.useRealTimers()
  })

  it('catches render errors and schedules reload', async () => {
    const { findByText, container } = render(App)

    expect(await findByText('UI crash detected')).toBeTruthy()
    expect(container.querySelector('.view-fallback')).toBeTruthy()

    vi.advanceTimersByTime(1600)
    expect(window.location.reload).toHaveBeenCalled()
  })
})
