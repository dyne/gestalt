import { render, fireEvent, cleanup } from '@testing-library/svelte'
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

  it('renders workflows and toggles details', async () => {
    apiFetch.mockResolvedValueOnce({
      json: vi.fn().mockResolvedValue([
        {
          session_id: '1',
          agent_name: 'Codex',
          current_l1: 'L1',
          current_l2: 'L2',
          status: 'paused',
          start_time: '2025-01-01T00:00:00Z',
          workflow_id: 'session-1',
          workflow_run_id: 'run-1',
          bell_events: [],
          task_events: [],
        },
      ]),
    })

    const onViewTerminal = vi.fn()
    const { findByText, getByText } = render(FlowView, { props: { onViewTerminal } })

    expect(await findByText('Codex')).toBeTruthy()

    await fireEvent.click(getByText('Show details'))

    const viewButton = await findByText('View Terminal')
    await fireEvent.click(viewButton)

    expect(onViewTerminal).toHaveBeenCalledWith('1')
  })
})
