import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { writable } from 'svelte/store'

const createDashboardStore = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/dashboardStore.js', () => ({
  createDashboardStore,
}))

import Dashboard from '../src/views/Dashboard.svelte'

const buildDashboardStore = (stateOverrides = {}) => {
  const store = writable({
    agents: [],
    agentsLoading: false,
    agentsError: '',
    agentSkills: {},
    agentSkillsLoading: false,
    agentSkillsError: '',
    logs: [],
    logsLoading: false,
    logsError: '',
    logLevelFilter: 'info',
    logsAutoRefresh: true,
    metricsSummary: null,
    metricsLoading: false,
    metricsError: '',
    metricsAutoRefresh: true,
    configExtractionCount: 0,
    configExtractionLast: '',
    gitContext: 'not a git repo',
    scipStatus: {
      indexed: false,
      fresh: false,
      in_progress: false,
      started_at: '',
      completed_at: '',
      duration: '',
      error: '',
      created_at: '',
      documents: 0,
      symbols: 0,
      age_hours: 0,
      languages: [],
    },
    ...stateOverrides,
  })

  return {
    subscribe: store.subscribe,
    set: store.set,
    update: store.update,
    start: vi.fn(() => Promise.resolve()),
    stop: vi.fn(),
    loadAgents: vi.fn(() => Promise.resolve()),
    loadLogs: vi.fn(() => Promise.resolve()),
    setLogLevelFilter: vi.fn(),
    setLogsAutoRefresh: vi.fn(),
    setMetricsAutoRefresh: vi.fn(),
    loadMetricsSummary: vi.fn(() => Promise.resolve()),
    setTerminals: vi.fn(),
    setStatus: vi.fn(),
    reindexScip: vi.fn(() => Promise.resolve()),
  }
}

describe('Dashboard', () => {
  beforeEach(() => {
    createDashboardStore.mockReset()
  })

  afterEach(() => {
    cleanup()
  })

  it('renders agent buttons and calls onCreate', async () => {
    const dashboardStore = buildDashboardStore({
      agents: [{ id: 'codex', name: 'Codex' }],
    })
    createDashboardStore.mockReturnValue(dashboardStore)

    const onCreate = vi.fn().mockResolvedValue()

    const { findByText } = render(Dashboard, {
      props: {
        terminals: [],
        status: { terminal_count: 0 },
        onCreate,
      },
    })

    const button = await findByText('Codex')
    await fireEvent.click(button)

    expect(onCreate).toHaveBeenCalledWith('codex')
  })

  it('opens log details from recent logs', async () => {
    const dashboardStore = buildDashboardStore({
      logs: [
        {
          id: 'log-1',
          level: 'info',
          timestamp: '2026-01-25T12:00:00Z',
          message: 'Log entry',
          context: {
            source: 'system',
            toast: 'true',
            toast_id: 'toast-1',
          },
          raw: { scope: 'unit' },
        },
      ],
    })
    createDashboardStore.mockReturnValue(dashboardStore)

    const { findByText, getByRole } = render(Dashboard, {
      props: {
        terminals: [],
        status: { terminal_count: 0 },
      },
    })

    const logButton = await findByText('Log entry')
    await fireEvent.click(logButton)

    const dialog = getByRole('dialog')
    expect(dialog).toBeTruthy()
    await findByText('source')
    await findByText('toast')
    await findByText('toast_id')
  })
})
