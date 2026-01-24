import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
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
