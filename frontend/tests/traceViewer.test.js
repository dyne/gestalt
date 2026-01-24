import { render, fireEvent } from '@testing-library/svelte'
import { describe, expect, it } from 'vitest'
import TraceViewer from '../src/components/TraceViewer.svelte'

describe('TraceViewer', () => {
  it('renders traces and toggles details', async () => {
    const trace = {
      trace_id: 'abc123',
      name: 'websocket.connect',
      duration_ms: 42,
      start_time: '2025-01-01T00:00:00Z',
      status: 'error',
      service: 'gestalt',
    }

    const { getByText, queryByText } = render(TraceViewer, { traces: [trace] })

    const title = getByText('websocket.connect')
    expect(title).toBeTruthy()
    expect(getByText('abc123')).toBeTruthy()

    expect(queryByText(/trace_id/)).toBeNull()
    await fireEvent.click(title)
    expect(getByText(/trace_id/)).toBeTruthy()
  })
})
