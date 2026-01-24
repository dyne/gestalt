import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchOtelLogs, fetchOtelMetrics, fetchOtelTraces } from './otelClient.js'
import { apiFetch } from './api.js'

vi.mock('./api.js', () => ({
  apiFetch: vi.fn(),
}))

describe('otelClient', () => {
  beforeEach(() => {
    apiFetch.mockReset()
  })

  it('builds log queries with dates', async () => {
    const since = new Date('2024-01-02T03:04:05.000Z')
    apiFetch.mockResolvedValue({ json: async () => ({ ok: true }) })

    const result = await fetchOtelLogs({ level: 'info', since, limit: 25 })
    expect(result).toEqual({ ok: true })
    expect(apiFetch).toHaveBeenCalledWith(
      `/api/otel/logs?level=info&since=${encodeURIComponent(since.toISOString())}&limit=25`,
    )
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
