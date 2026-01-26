import { describe, it, expect, vi, afterEach } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

import {
  createTerminal,
  fetchLogs,
  fetchPlansList,
  fetchScipStatus,
  fetchStatus,
  triggerScipReindex,
} from '../src/lib/apiClient.js'

describe('apiClient', () => {
  afterEach(() => {
    apiFetch.mockReset()
  })

  it('fetches status payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ ok: true }) })

    const result = await fetchStatus()

    expect(result).toEqual({ ok: true })
    expect(apiFetch).toHaveBeenCalledWith('/api/status')
  })

  it('fetches scip status payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ indexed: true }) })

    const result = await fetchScipStatus()

    expect(result).toEqual({ indexed: true })
    expect(apiFetch).toHaveBeenCalledWith('/api/scip/status')
  })

  it('builds log queries', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) })

    await fetchLogs({ level: 'error' })

    expect(apiFetch).toHaveBeenCalledWith('/api/logs?level=error')
  })

  it('sends terminal create payloads', async () => {
    const json = vi.fn().mockResolvedValue({ id: '1' })
    apiFetch.mockResolvedValue({ json })

    const result = await createTerminal({ agentId: 'codex', workflow: true })

    expect(result).toEqual({ id: '1' })
    expect(apiFetch).toHaveBeenCalledWith('/api/terminals', {
      method: 'POST',
      body: JSON.stringify({ agent: 'codex', workflow: true }),
    })
  })

  it('fetches plans list payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ plans: [] }) })

    const result = await fetchPlansList()

    expect(result).toEqual({ plans: [] })
    expect(apiFetch).toHaveBeenCalledWith('/api/plans')
  })

  it('triggers scip reindex', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ status: 'indexing started' }) })

    const result = await triggerScipReindex()

    expect(result).toEqual({ status: 'indexing started' })
    expect(apiFetch).toHaveBeenCalledWith('/api/scip/reindex', { method: 'POST' })
  })
})
