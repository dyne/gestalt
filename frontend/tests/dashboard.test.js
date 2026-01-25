import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())
const initialScipStatus = vi.hoisted(() => ({
  indexed: false,
  fresh: false,
  in_progress: false,
  started_at: '',
  completed_at: '',
  duration: '',
  error: '',
  created_at: '',
  documents: 0,
  symbols: 0,
  age_hours: 0,
  languages: [],
}))
const createScipStore = vi.hoisted(() =>
  vi.fn(() => ({
    status: {
      subscribe: (run) => {
        run({ ...initialScipStatus, languages: [] })
        return () => {}
      },
    },
    start: vi.fn(() => Promise.resolve()),
    stop: vi.fn(),
    reindex: vi.fn(() => Promise.resolve()),
    connectionStatus: { subscribe: () => () => {} },
  }))
)

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

vi.mock('../src/lib/scipStore.js', () => ({
  createScipStore,
  initialScipStatus,
}))

import Dashboard from '../src/views/Dashboard.svelte'

describe('Dashboard', () => {
  beforeEach(() => {
    apiFetch.mockReset()
  })

  afterEach(() => {
    cleanup()
  })

  it('renders agent buttons and calls onCreate', async () => {
    // Mock agents API call
    apiFetch.mockImplementation((url) => {
      if (url === '/api/agents') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([{ id: 'codex', name: 'Codex' }]),
        })
      }
      if (url.startsWith('/api/skills?agent=')) {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([]),
        })
      }
      if (url.startsWith('/api/logs')) {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([]),
        })
      }
      if (url === '/api/metrics/summary') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue({
            updated_at: '2026-01-24T00:00:00Z',
            top_endpoints: [],
            slowest_endpoints: [],
            top_agents: [],
            error_rates: [],
          }),
        })
      }
      return Promise.reject(new Error('Unexpected API call'))
    })
    
    const onCreate = vi.fn().mockResolvedValue()

    const { findByText } = render(Dashboard, {
      props: {
        terminals: [],
        status: { terminal_count: 0 },
        onCreate,
      },
    })

    const button = await findByText('Codex')
    await fireEvent.click(button)

    expect(onCreate).toHaveBeenCalledWith('codex')
  })
})
