import { render, fireEvent, waitFor, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())
const addNotification = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

vi.mock('../src/lib/notificationStore.js', () => ({
  notificationStore: {
    addNotification,
  },
}))

import LogsView from '../src/views/LogsView.svelte'

describe('LogsView', () => {
  afterEach(() => {
    apiFetch.mockReset()
    addNotification.mockReset()
    cleanup()
  })

  it('renders logs from the API', async () => {
    apiFetch.mockResolvedValueOnce({
      json: vi.fn().mockResolvedValue([
        {
          severity_text: 'INFO',
          body: 'hello',
          timestamp: '2025-01-01T00:00:00Z',
          attributes: { scope: 'test' },
        },
      ]),
    })

    const { findByText, getByLabelText } = render(LogsView)

    const entry = await findByText('hello')
    expect(entry).toBeTruthy()

    const autoRefresh = getByLabelText('Auto refresh')
    await fireEvent.click(autoRefresh)
  })

  it('requests filtered logs', async () => {
    apiFetch.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
    apiFetch.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })

    const { findByText, getByLabelText } = render(LogsView)

    await findByText('No logs yet.')

    const autoRefresh = getByLabelText('Auto refresh')
    await fireEvent.click(autoRefresh)

    const select = getByLabelText('Level')
    await fireEvent.change(select, { target: { value: 'error' } })

    await waitFor(() => {
      expect(apiFetch).toHaveBeenLastCalledWith('/api/otel/logs?level=error')
    })
  })
})
