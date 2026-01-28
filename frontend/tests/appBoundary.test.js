import { render, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { notificationStore } from '../src/lib/notificationStore.js'
import { createAppApiMocks, createLogStreamStub } from './helpers/appApiMocks.js'

const apiFetch = vi.hoisted(() => vi.fn())
const createLogStream = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
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
    apiFetch.mockImplementation(createAppApiMocks(apiFetch))
    createLogStream.mockImplementation(() => createLogStreamStub())
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
