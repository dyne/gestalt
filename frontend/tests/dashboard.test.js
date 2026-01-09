import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

import Dashboard from '../src/views/Dashboard.svelte'

describe('Dashboard', () => {
  afterEach(() => {
    apiFetch.mockReset()
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

    expect(onCreate).toHaveBeenCalledWith('codex', false)
  })
})
