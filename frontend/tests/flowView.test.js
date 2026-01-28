import { render, cleanup } from '@testing-library/svelte'
import { describe, it, expect, afterEach, vi } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildEventSourceUrl,
}))

import FlowView from '../src/views/FlowView.svelte'

describe('FlowView', () => {
  afterEach(() => {
    apiFetch.mockReset()
    cleanup()
  })

  it('renders the flow view shell', async () => {
    apiFetch.mockResolvedValueOnce({
      json: vi.fn().mockResolvedValue([]),
    })

    const { findByText } = render(FlowView)

    expect(await findByText('Flow')).toBeTruthy()
    expect(await findByText('Refresh')).toBeTruthy()
  })
})
