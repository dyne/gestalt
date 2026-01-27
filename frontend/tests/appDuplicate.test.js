import { render, fireEvent, cleanup, within } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { notificationStore } from '../src/lib/notificationStore.js'

const apiFetch = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

vi.mock('../src/views/TerminalView.svelte', async () => {
  const module = await import('./helpers/TerminalViewStub.svelte')
  return { default: module.default }
})

import App from '../src/App.svelte'

describe('App duplicate agent handling', () => {
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

  it('switches to existing tab and shows an info toast on 409', async () => {
    apiFetch.mockImplementation((url, options = {}) => {
      if (url === '/api/status') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue({ terminal_count: 1 }),
        })
      }
      if (url === '/api/terminals' && (!options.method || options.method === 'GET')) {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([
            {
              id: '1',
              title: 'Codex',
              role: 'shell',
              created_at: new Date().toISOString(),
            },
          ]),
        })
      }
      if (url === '/api/agents') {
        return Promise.resolve({
          json: vi.fn().mockResolvedValue([{ id: 'codex', name: 'Codex' }]),
        })
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
      if (url === '/api/terminals' && options.method === 'POST') {
        const error = new Error('agent "Codex" is already running')
        error.status = 409
        error.data = { terminal_id: '1' }
        return Promise.reject(error)
      }
      return Promise.reject(new Error(`Unexpected API call: ${url}`))
    })

    const { container, findByText } = render(App)
    const agentsSection = container.querySelector('.dashboard__agents')
    const agentLabel = await within(agentsSection).findByText('Codex')
    const agentButton = agentLabel.closest('button')

    await fireEvent.click(agentButton)

    expect(await findByText('agent "Codex" is already running')).toBeTruthy()

    const tabBar = container.querySelector('nav[aria-label="App tabs"]')
    const tabButton = within(tabBar).getByRole('button', { name: 'Codex' })
    const tabItem = tabButton.closest('.tabbar__item')
    expect(tabItem?.dataset.active).toBe('true')
  })
})
