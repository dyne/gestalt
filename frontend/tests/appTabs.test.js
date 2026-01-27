import { render, fireEvent, cleanup, waitFor } from '@testing-library/svelte'
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

vi.mock('../src/views/TerminalView.svelte', async () => {
  const module = await import('./helpers/TerminalViewStub.svelte')
  return { default: module.default }
})

import App from '../src/App.svelte'

describe('App tab switching', () => {
  beforeEach(() => {
    if (!Element.prototype.animate) {
      Element.prototype.animate = () => ({
        cancel() {},
        finish() {},
        onfinish: null,
      })
    }
    apiFetch.mockImplementation((url) => {
      if (url === '/api/status') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue({ terminal_count: 1 }),
        })
      }
      if (url === '/api/terminals') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([
            {
              id: 't1',
              title: 'Shell',
              role: 'shell',
              created_at: new Date().toISOString(),
            },
          ]),
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
    apiFetch.mockReset()
    notificationStore.clear()
    cleanup()
  })

  it('switches between home and terminal tabs', async () => {
    const { container, findByRole } = render(App)

    const planTab = await findByRole('button', { name: 'Plan' })
    const statusTab = await findByRole('button', { name: 'Status' })
    const terminalTab = await findByRole('button', { name: 'Shell' })

    await fireEvent.click(planTab)
    await waitFor(() => {
      const active = container.querySelector('section.view[data-active="true"]')
      expect(active).toBeTruthy()
      expect(active?.textContent).toContain('Project Plans')
    })

    await fireEvent.click(statusTab)
    await waitFor(() => {
      const active = container.querySelector('section.view[data-active="true"]')
      expect(active).toBeTruthy()
      expect(active?.textContent).toContain('Flow')
    })

    await fireEvent.click(terminalTab)
    await waitFor(() => {
      const terminalSection = container.querySelector('section.view--terminals[data-active="true"]')
      expect(terminalSection).toBeTruthy()
    })
  })
})
