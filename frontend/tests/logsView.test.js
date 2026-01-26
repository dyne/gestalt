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
    const writeText = vi.fn(() => Promise.resolve())
    Object.assign(navigator, { clipboard: { writeText } })

    createLogStream.mockImplementation((options) => ({
      start: vi.fn(() => {
        options?.onOpen?.()
        options?.onEntry?.({
          timeUnixNano: '1700000000000000',
          severityText: 'INFO',
          body: { stringValue: 'hello' },
          attributes: [{ key: 'scope', value: { stringValue: 'test' } }],
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

    const copyButton = await findByText('Copy JSON')
    await fireEvent.click(copyButton)

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledTimes(1)
      expect(addNotification).toHaveBeenCalledWith('info', expect.stringContaining('Copied'))
    })

    const autoRefresh = getByLabelText('Live updates')
    await fireEvent.click(autoRefresh)
  })

  it('hides copy controls when clipboard is unavailable', async () => {
    Object.defineProperty(window, 'isSecureContext', {
      value: false,
      configurable: true,
    })

    createLogStream.mockImplementation((options) => ({
      start: vi.fn(() => {
        options?.onOpen?.()
        options?.onEntry?.({
          timeUnixNano: '1700000000000000',
          severityText: 'INFO',
          body: { stringValue: 'hello' },
          attributes: [{ key: 'scope', value: { stringValue: 'test' } }],
        })
      }),
      stop: vi.fn(),
      restart: vi.fn(),
      setLevel: vi.fn(),
    }))

    const { findByText, queryByRole } = render(LogsView)

    const entry = await findByText('hello')
    await fireEvent.click(entry)

    const rawToggle = await findByText('Raw JSON')
    await fireEvent.click(rawToggle)

    const copyButton = queryByRole('button', { name: 'Copy JSON' })
    expect(copyButton).toBeNull()
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
