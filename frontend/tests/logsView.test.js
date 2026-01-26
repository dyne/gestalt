import { render, fireEvent, waitFor, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach } from 'vitest'

const addNotification = vi.hoisted(() => vi.fn())
const createLogStream = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/logStream.js', () => ({
  createLogStream,
}))

vi.mock('../src/lib/notificationStore.js', () => ({
  notificationStore: {
    addNotification,
  },
}))

import LogsView from '../src/views/LogsView.svelte'

describe('LogsView', () => {
  afterEach(() => {
    addNotification.mockReset()
    createLogStream.mockReset()
    cleanup()
    if ('isSecureContext' in window) {
      delete window.isSecureContext
    }
  })

  it('renders logs from the stream', async () => {
    Object.defineProperty(window, 'isSecureContext', {
      value: true,
      configurable: true,
    })

    createLogStream.mockImplementation((options) => ({
      start: vi.fn(() => {
        options?.onOpen?.()
        options?.onEntry?.({
          severity_text: 'INFO',
          body: 'hello',
          timestamp: '2025-01-01T00:00:00Z',
          attributes: { scope: 'test' },
        })
      }),
      stop: vi.fn(),
      restart: vi.fn(),
      setLevel: vi.fn(),
    }))

    const { findByText, getByLabelText } = render(LogsView)

    const entry = await findByText('hello')
    expect(entry).toBeTruthy()

    await fireEvent.click(entry)
    await findByText('scope')
    await findByText('test')

    const rawToggle = await findByText('Raw JSON')
    await fireEvent.click(rawToggle)

    const writeText = vi.fn(() => Promise.resolve())
    Object.assign(navigator, { clipboard: { writeText } })

    const copyButton = await findByText('Copy JSON')
    await fireEvent.click(copyButton)

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledTimes(1)
      expect(addNotification).toHaveBeenCalledWith('info', expect.stringContaining('Copied'))
    })

    const autoRefresh = getByLabelText('Live updates')
    await fireEvent.click(autoRefresh)
  })

  it('requests filtered logs', async () => {
    const start = vi.fn()
    const restart = vi.fn()
    const setLevel = vi.fn()
    createLogStream.mockImplementation((options) => ({
      start: vi.fn(() => {
        start()
        options?.onOpen?.()
      }),
      stop: vi.fn(),
      restart,
      setLevel,
    }))

    const { findByText, getByLabelText } = render(LogsView)

    await findByText('No logs yet.')

    const select = getByLabelText('Level')
    await fireEvent.change(select, { target: { value: 'error' } })

    await waitFor(() => {
      expect(setLevel).toHaveBeenLastCalledWith('error')
      expect(restart).toHaveBeenCalled()
      expect(start).toHaveBeenCalled()
    })
  })
})
