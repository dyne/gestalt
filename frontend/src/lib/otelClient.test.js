import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchOtelMetrics, fetchOtelTraces } from './otelClient.js'
import { apiFetch } from './api.js'

vi.mock('./api.js', () => ({
  apiFetch: vi.fn(),
}))

describe('otelClient', () => {
  beforeEach(() => {
    apiFetch.mockReset()
  })

  it('builds trace queries with identifiers', async () => {
    apiFetch.mockResolvedValue({ json: async () => ({ traces: [] }) })

    await fetchOtelTraces({
      traceId: 'abc123',
      spanName: 'terminal.output',
      limit: 5,
    })

    expect(apiFetch).toHaveBeenCalledWith(
      '/api/otel/traces?trace_id=abc123&span_name=terminal.output&limit=5',
    )
  })

  it('builds metrics queries', async () => {
    apiFetch.mockResolvedValue({ json: async () => ({ series: [] }) })

    await fetchOtelMetrics({
      name: 'gestalt.workflow.started',
      step: 60,
      query: 'service.name=gestalt',
    })

    expect(apiFetch).toHaveBeenCalledWith(
      '/api/otel/metrics?name=gestalt.workflow.started&step=60&query=service.name%3Dgestalt',
    )
  })
})
