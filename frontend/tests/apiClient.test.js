import { describe, it, expect, vi, afterEach } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildEventSourceUrl,
  buildApiPath: (base, ...segments) => {
    const basePath = base.endsWith('/') ? base.slice(0, -1) : base
    const encoded = segments
      .filter((segment) => segment !== undefined && segment !== null && segment !== '')
      .map((segment) => encodeURIComponent(String(segment)))
    return encoded.length ? `${basePath}/${encoded.join('/')}` : basePath
  },
}))

import {
  createTerminal,
  fetchAgentSkills,
  fetchAgents,
  fetchFlowActivities,
  fetchFlowConfig,
  fetchGitLog,
  exportFlowConfig,
  importFlowConfig,
  fetchLogs,
  fetchMetricsSummary,
  fetchPlansList,
  fetchStatus,
  fetchTerminals,
  sendInputToAgentSession,
  sendSessionInput,
  saveFlowConfig,
  sendDirectorPrompt,
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

  it('normalizes malformed status payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) })

    const result = await fetchStatus()

    expect(result).toEqual({})
  })

  it('builds log queries', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) })

    await fetchLogs({ level: 'error' })

    expect(apiFetch).toHaveBeenCalledWith('/api/logs?level=error')
  })

  it('sends terminal create payloads', async () => {
    const json = vi.fn().mockResolvedValue({ id: '1' })
    apiFetch.mockResolvedValue({ json })

    const result = await createTerminal({ agentId: 'codex' })

    expect(result).toEqual({
      id: '1',
      interface: 'cli',
      title: '',
      runner: '',
      gui_modules: [],
      model: '',
    })
    expect(apiFetch).toHaveBeenCalledWith('/api/sessions', {
      method: 'POST',
      body: JSON.stringify({ agent: 'codex' }),
    })
  })

  it('fetches plans list payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ plans: [] }) })

    const result = await fetchPlansList()

    expect(result).toEqual({ plans: [] })
    expect(apiFetch).toHaveBeenCalledWith('/api/plans')
  })

  it('normalizes malformed plans payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ plans: [null, 'bad'] }) })

    const result = await fetchPlansList()

    expect(result.plans).toEqual([])
  })

  it('normalizes malformed agent payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([null, { name: 'No id' }]) })

    const result = await fetchAgents()

    expect(result).toEqual([])
  })

  it('preserves hidden flags on agents', async () => {
    apiFetch.mockResolvedValue({
      json: vi.fn().mockResolvedValue([{ id: 'hidden', name: 'Hidden', hidden: true }]),
    })

    const result = await fetchAgents()

    expect(result).toEqual([{ id: 'hidden', name: 'Hidden', hidden: true, model: '' }])
  })

  it('normalizes malformed agent skills payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([null, { name: '' }, { name: 'Skill' }]) })

    const result = await fetchAgentSkills('agent')

    expect(result).toEqual([{ name: 'Skill' }])
  })

  it('normalizes malformed terminals payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([null, { id: 12 }]) })

    const result = await fetchTerminals()

    expect(result).toEqual([
      {
        id: '12',
        interface: 'cli',
        title: '',
        runner: '',
        gui_modules: [],
        model: '',
      },
    ])
  })

  it('normalizes malformed metrics summary payloads', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue(null) })

    const result = await fetchMetricsSummary()

    expect(result.top_endpoints).toEqual([])
    expect(result.slowest_endpoints).toEqual([])
    expect(result.top_agents).toEqual([])
    expect(result.error_rates).toEqual([])
  })

  it('fetches and normalizes git log payloads', async () => {
    apiFetch.mockResolvedValue({
      json: vi.fn().mockResolvedValue({
        branch: 'main',
        commits: [
          {
            sha: '1234567890abcdef1234567890abcdef12345678',
            short_sha: '1234567890ab',
            committed_at: '2026-02-18T00:00:00Z',
            subject: 'feat(ui): add panel',
            files_truncated: false,
            stats: {
              files_changed: 1,
              lines_added: 10,
              lines_deleted: 2,
              has_binary: false,
            },
            files: [{ path: 'frontend/src/views/Dashboard.svelte', added: 10, deleted: 2, binary: false }],
          },
        ],
      }),
    })

    const result = await fetchGitLog({ limit: 20 })

    expect(apiFetch).toHaveBeenCalledWith('/api/git/log?limit=20')
    expect(result.branch).toBe('main')
    expect(result.commits).toHaveLength(1)
    expect(result.commits[0].stats.files_changed).toBe(1)
  })

  it('fetches flow activities', async () => {
    apiFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ id: 'toast_notification' }, null]) })

    const result = await fetchFlowActivities()

    expect(result).toEqual([
      { id: 'toast_notification', label: 'toast_notification', description: '', fields: [] },
    ])
    expect(apiFetch).toHaveBeenCalledWith('/api/flow/activities')
  })

  it('fetches flow config payloads', async () => {
    apiFetch.mockResolvedValue({
      json: vi.fn().mockResolvedValue({
        version: 1,
        triggers: [{ id: 't1', label: 'Trigger', event_type: 'file-change' }],
        bindings_by_trigger_id: { t1: [{ activity_id: 'toast_notification' }] },
        storage_path: '.gestalt/flow/automations.json',
      }),
    })

    const result = await fetchFlowConfig()

    expect(result.config.triggers[0].id).toBe('t1')
    expect(result.storagePath).toBe('.gestalt/flow/automations.json')
  })

  it('saves flow config payloads', async () => {
    apiFetch.mockResolvedValue({
      json: vi.fn().mockResolvedValue({
        version: 1,
        triggers: [],
        bindings_by_trigger_id: {},
      }),
    })

    const config = { version: 1, triggers: [], bindings_by_trigger_id: {} }
    await saveFlowConfig(config)

    expect(apiFetch).toHaveBeenCalledWith('/api/flow/config', {
      method: 'PUT',
      body: JSON.stringify(config),
    })
  })

  it('exports flow config payloads', async () => {
    const response = { ok: true }
    apiFetch.mockResolvedValue(response)

    const result = await exportFlowConfig()

    expect(result).toBe(response)
    expect(apiFetch).toHaveBeenCalledWith('/api/flow/config/export')
  })

  it('imports flow config payloads', async () => {
    apiFetch.mockResolvedValue({
      json: vi.fn().mockResolvedValue({
        version: 1,
        triggers: [],
        bindings_by_trigger_id: {},
      }),
    })

    const yamlText = 'version: 1\nflows: []\n'
    await importFlowConfig(yamlText)

    expect(apiFetch).toHaveBeenCalledWith('/api/flow/config/import', {
      method: 'POST',
      headers: {
        'Content-Type': 'text/yaml; charset=utf-8',
      },
      body: yamlText,
    })
  })

  it('sends direct session input payloads', async () => {
    apiFetch.mockResolvedValue({ ok: true })

    await sendSessionInput('Coder 1', 'hello')

    expect(apiFetch).toHaveBeenCalledWith('/api/sessions/Coder%201/input', {
      method: 'POST',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: 'hello',
    })
  })

  it('sends to existing agent session ids', async () => {
    apiFetch
      .mockResolvedValueOnce({
        json: vi.fn().mockResolvedValue([{ id: 'coder', name: 'Coder', session_id: 'Coder 1' }]),
      })
      .mockResolvedValueOnce({ ok: true })

    await sendInputToAgentSession('coder', 'Coder', 'run')

    expect(apiFetch).toHaveBeenNthCalledWith(1, '/api/agents')
    expect(apiFetch).toHaveBeenNthCalledWith(2, '/api/sessions/Coder%201/input', {
      method: 'POST',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: 'run',
    })
  })

  it('fails with guidance when agent session is not running', async () => {
    apiFetch
      .mockResolvedValueOnce({
        json: vi.fn().mockResolvedValue([{ id: 'coder', name: 'Coder', session_id: '' }]),
      })

    await expect(sendInputToAgentSession('coder', 'Coder', 'run')).rejects.toThrow(
      'session not running; run gestalt-agent coder',
    )

    expect(apiFetch).toHaveBeenNthCalledWith(1, '/api/agents')
    expect(apiFetch).toHaveBeenCalledTimes(1)
  })

  it('creates director session and sends text prompt with notify event', async () => {
    apiFetch
      .mockResolvedValueOnce({ json: vi.fn().mockResolvedValue({ id: 'Director 1' }) })
      .mockResolvedValueOnce({
        json: vi.fn().mockResolvedValue({ lines: ['codex ready'] }),
      })
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce({ ok: true })

    const result = await sendDirectorPrompt('summarize repo', 'text')

    expect(result.sessionId).toBe('Director 1')
    expect(result.notifyError).toBe('')
    expect(apiFetch).toHaveBeenNthCalledWith(1, '/api/sessions', {
      method: 'POST',
      body: JSON.stringify({ agent: 'director' }),
    })
    expect(apiFetch).toHaveBeenNthCalledWith(2, '/api/sessions/Director%201/output')
    expect(apiFetch).toHaveBeenNthCalledWith(3, '/api/sessions/Director%201/input', {
      method: 'POST',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: 'summarize repo',
    })
    expect(apiFetch).toHaveBeenNthCalledWith(4, '/api/sessions/Director%201/notify', {
      method: 'POST',
      body: JSON.stringify({
        session_id: 'Director 1',
        payload: {
          type: 'prompt-text',
          message: 'summarize repo',
        },
      }),
    })
  })

  it('reuses director session id from 409 create conflict', async () => {
    const conflict = new Error('Conflict')
    conflict.status = 409
    conflict.data = { session_id: 'Director 9' }
    apiFetch
      .mockRejectedValueOnce(conflict)
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce({ ok: true })

    const result = await sendDirectorPrompt('resume', 'voice')

    expect(result.sessionId).toBe('Director 9')
    expect(apiFetch).toHaveBeenNthCalledWith(2, '/api/sessions/Director%209/input', {
      method: 'POST',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: 'resume',
    })
    expect(apiFetch).toHaveBeenNthCalledWith(3, '/api/sessions/Director%209/notify', {
      method: 'POST',
      body: JSON.stringify({
        session_id: 'Director 9',
        payload: {
          type: 'prompt-voice',
          message: 'resume',
        },
      }),
    })
  })

  it('reports notify failure without failing successful director input', async () => {
    apiFetch
      .mockResolvedValueOnce({ json: vi.fn().mockResolvedValue({ id: 'Director 1' }) })
      .mockResolvedValueOnce({
        json: vi.fn().mockResolvedValue({ lines: ['codex ready'] }),
      })
      .mockResolvedValueOnce({ ok: true })
      .mockRejectedValueOnce(new Error('notify unavailable'))

    const result = await sendDirectorPrompt('continue')

    expect(result.sessionId).toBe('Director 1')
    expect(result.notifyError).toContain('notify unavailable')
  })

})
