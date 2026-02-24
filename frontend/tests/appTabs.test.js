import { render, fireEvent, cleanup, waitFor } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { get } from 'svelte/store'
import { notificationStore } from '../src/lib/notificationStore.js'
import { createAppApiMocks, createLogStreamStub } from './helpers/appApiMocks.js'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))
const createLogStream = vi.hoisted(() => vi.fn())

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
    const appMocks = createAppApiMocks(apiFetch, {
      status: { session_count: 1 },
      terminals: [
        {
          id: 't1',
          title: 'Shell',
          role: 'shell',
          created_at: new Date().toISOString(),
          interface: 'cli',
        },
      ],
      agents: [],
    })
    apiFetch.mockImplementation((url, options = {}) => {
      if (url === '/api/sessions' && options.method === 'POST') {
        return Promise.resolve({
          json: () => Promise.resolve({ id: 'Director 1' }),
        })
      }
      if (url === '/api/sessions/Director%201/input' && options.method === 'POST') {
        return Promise.resolve({ ok: true })
      }
      if (url === '/api/sessions/Director%201/notify' && options.method === 'POST') {
        return Promise.resolve({ ok: true })
      }
      return appMocks(url)
    })
    createLogStream.mockImplementation(() => createLogStreamStub())
  })

  afterEach(() => {
    apiFetch.mockReset()
    notificationStore.clear()
    cleanup()
  })

  it('switches between home and terminal tabs', async () => {
    const { container, findByRole, queryByRole } = render(App)

    const planTab = await findByRole('button', { name: 'Plans' })
    const flowTab = await findByRole('button', { name: 'Flow' })
    const directorInput = await findByRole('textbox')

    await fireEvent.click(planTab)
    await waitFor(() => {
      const active = container.querySelector('section.view[data-active="true"]')
      expect(active).toBeTruthy()
      expect(active?.textContent).toContain('Plans')
    })

    await fireEvent.click(flowTab)
    await waitFor(() => {
      const active = container.querySelector('section.view[data-active="true"]')
      expect(active).toBeTruthy()
      expect(active?.textContent).toContain('Flow')
    })

    expect(queryByRole('button', { name: 'Chat' })).toBeNull()

    await fireEvent.input(directorInput, { target: { value: 'Plan today' } })
    await fireEvent.keyDown(directorInput, { key: 'Enter' })

    await waitFor(async () => {
      const chatTab = await findByRole('button', { name: 'Chat' })
      expect(chatTab).toBeTruthy()
    })

    await waitFor(() => {
      const active = container.querySelector('section.view[data-active="true"]')
      expect(active).toBeTruthy()
      expect(active?.textContent).toContain('Chat')
    })

    const dashboardTab = await findByRole('button', { name: 'Open dashboard' })
    await fireEvent.click(dashboardTab)
    await waitFor(() => {
      const active = container.querySelector('section.view[data-active="true"]')
      expect(active).toBeTruthy()
      expect(active?.textContent).toContain('Director')
    })
  })

  it('keeps chat transition on notify failure and emits warning', async () => {
    const appMocks = createAppApiMocks(apiFetch, {
      status: { session_count: 0 },
      terminals: [],
      agents: [],
    })
    apiFetch.mockImplementation((url, options = {}) => {
      if (url === '/api/sessions' && options.method === 'POST') {
        return Promise.resolve({ json: () => Promise.resolve({ id: 'Director 1' }) })
      }
      if (url === '/api/sessions/Director%201/input' && options.method === 'POST') {
        return Promise.resolve({ ok: true })
      }
      if (url === '/api/sessions/Director%201/notify' && options.method === 'POST') {
        return Promise.reject(new Error('notify unavailable'))
      }
      return appMocks(url)
    })

    const { findByRole } = render(App)
    const input = await findByRole('textbox')
    await fireEvent.input(input, { target: { value: 'Ship update' } })
    await fireEvent.keyDown(input, { key: 'Enter' })

    expect(await findByRole('button', { name: 'Chat' })).toBeTruthy()
    await waitFor(() => {
      const notifications = get(notificationStore)
      const last = notifications.at(-1)
      expect(last?.level).toBe('warning')
    })
  })

  it('surfaces input failures and does not switch to chat', async () => {
    const appMocks = createAppApiMocks(apiFetch, {
      status: { session_count: 0 },
      terminals: [],
      agents: [],
    })
    apiFetch.mockImplementation((url, options = {}) => {
      if (url === '/api/sessions' && options.method === 'POST') {
        return Promise.resolve({ json: () => Promise.resolve({ id: 'Director 1' }) })
      }
      if (url === '/api/sessions/Director%201/input' && options.method === 'POST') {
        const failure = new Error('bridge down')
        failure.status = 503
        return Promise.reject(failure)
      }
      return appMocks(url)
    })

    const { findByRole, queryByRole, findByText } = render(App)
    const input = await findByRole('textbox')
    await fireEvent.input(input, { target: { value: 'Ship update' } })
    await fireEvent.keyDown(input, { key: 'Enter' })

    expect(await findByText('Director session bridge is unavailable.')).toBeTruthy()
    expect(queryByRole('button', { name: 'Chat' })).toBeNull()
  })
})
