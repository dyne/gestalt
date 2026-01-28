import { render, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { notificationStore } from '../src/lib/notificationStore.js'
import { installGlobalCrashHandlers } from '../src/lib/appHealthStore.js'
import { createAppApiMocks, createLogStreamStub } from './helpers/appApiMocks.js'

const apiFetch = vi.hoisted(() => vi.fn())
const createLogStream = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

vi.mock('../src/lib/logStream.js', () => ({
  createLogStream,
}))

vi.mock('../src/views/TerminalView.svelte', async () => {
  const module = await import('./helpers/TerminalViewStub.svelte')
  return { default: module.default }
})

import App from '../src/App.svelte'

describe('App crash overlay', () => {
  let removeHandlers = null

  beforeEach(() => {
    if (!Element.prototype.animate) {
      Element.prototype.animate = () => ({
        cancel() {},
        finish() {},
        onfinish: null,
      })
    }
    vi.useFakeTimers()
    Object.defineProperty(window, 'location', {
      value: { reload: vi.fn() },
      writable: true,
    })
    apiFetch.mockImplementation(createAppApiMocks(apiFetch))
    createLogStream.mockImplementation(() => createLogStreamStub())
    removeHandlers = installGlobalCrashHandlers()
  })

  afterEach(() => {
    if (removeHandlers) {
      removeHandlers()
      removeHandlers = null
    }
    notificationStore.clear()
    sessionStorage.clear()
    apiFetch.mockReset()
    cleanup()
    vi.useRealTimers()
  })

  it('shows the crash overlay after a window error event', async () => {
    const { findByText } = render(App)

    window.dispatchEvent(new ErrorEvent('error', { message: 'boom', error: new Error('boom') }))

    expect(await findByText('UI crash detected')).toBeTruthy()
    expect(await findByText('Crash id')).toBeTruthy()
  })
})
