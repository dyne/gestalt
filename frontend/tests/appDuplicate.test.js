import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { notificationStore } from '../src/lib/notificationStore.js'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildEventSourceUrl,
}))

vi.mock('../src/views/TerminalView.svelte', async () => {
  const module = await import('./helpers/TerminalViewStub.svelte')
  return { default: module.default }
})

import App from '../src/App.svelte'

describe('App dashboard director submit', () => {
  beforeEach(() => {
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

  it('does not create a terminal session directly from dashboard submit', async () => {
    apiFetch.mockImplementation((url, options = {}) => {
      if (url === '/api/status') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue({ session_count: 0, agents_session_id: '' }),
        })
      }
      if (url === '/api/sessions' && (!options.method || options.method === 'GET')) {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([]),
        })
      }
      if (url === '/api/agents') {
        return Promise.resolve({ json: vi.fn().mockResolvedValue([]) })
      }
      if (url === '/api/skills') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([]),
        })
      }
      if (url.startsWith('/api/skills?agent=')) {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([]),
        })
      }
      return Promise.reject(new Error(`Unexpected API call: ${url}`))
    })

    const { findByRole, queryByRole } = render(App)
    const input = await findByRole('textbox')
    await fireEvent.input(input, { target: { value: 'hello director' } })
    await fireEvent.keyDown(input, { key: 'Enter' })

    expect(await findByRole('button', { name: 'Chat' })).toBeTruthy()
    expect(queryByRole('button', { name: 'Agents' })).toBeNull()
    const createCalls = apiFetch.mock.calls.filter(
      ([url, request]) => url === '/api/sessions' && request?.method === 'POST',
    )
    expect(createCalls).toHaveLength(0)
  })
})
