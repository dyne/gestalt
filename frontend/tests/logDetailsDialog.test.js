import { render, fireEvent, waitFor, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach } from 'vitest'

const addNotification = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/notificationStore.js', () => ({
  notificationStore: {
    addNotification,
  },
}))

import LogDetailsDialog from '../src/components/LogDetailsDialog.svelte'

describe('LogDetailsDialog', () => {
  afterEach(() => {
    addNotification.mockReset()
    cleanup()
  })

  it('renders context and copies JSON', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.assign(navigator, { clipboard: { writeText } })

    const entry = {
      id: 'log-1',
      level: 'info',
      timestamp: '2025-01-01T00:00:00Z',
      message: 'Hello world',
      context: { source: 'tests', toast: 'true' },
      raw: { scope: 'unit' },
    }

    const { findByText, getByRole } = render(LogDetailsDialog, {
      entry,
      open: true,
    })

    await findByText('source')
    await findByText('tests')

    const copyButton = getByRole('button', { name: /copy json/i })
    await fireEvent.click(copyButton)

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledTimes(1)
      expect(addNotification).toHaveBeenCalledWith('info', expect.stringContaining('Copied'))
    })
  })
})
