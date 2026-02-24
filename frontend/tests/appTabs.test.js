import { render, fireEvent, cleanup, waitFor } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { notificationStore } from '../src/lib/notificationStore.js'
import { createAppApiMocks, createLogStreamStub } from './helpers/appApiMocks.js'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))
const createLogStream = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildEventSourceUrl,
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
    apiFetch.mockImplementation(
      createAppApiMocks(apiFetch, {
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
        agents: [
          {
            id: 'coder',
            name: 'Coder',
            hidden: false,
            running: true,
            session_id: 't1',
          },
        ],
      }),
    )
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
})
