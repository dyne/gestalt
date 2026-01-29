import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { tick } from 'svelte'
import { writable } from 'svelte/store'

const createDashboardStore = vi.hoisted(() => vi.fn())
const addNotification = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/dashboardStore.js', () => ({
  createDashboardStore,
}))

vi.mock('../src/lib/notificationStore.js', () => ({
  notificationStore: {
    addNotification,
  },
}))

import Dashboard from '../src/views/Dashboard.svelte'

const buildDashboardStore = (stateOverrides = {}) => {
  const store = writable({
    agents: [],
    agentsLoading: false,
    agentsError: '',
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
  }
}

describe('Dashboard', () => {
  beforeEach(() => {
    createDashboardStore.mockReset()
    addNotification.mockReset()
  })

  afterEach(() => {
    cleanup()
    if ('isSecureContext' in window) {
      delete window.isSecureContext
    }
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
        status: { session_count: 0 },
        onCreate,
      },
    })

    const button = await findByText('Codex')
    await fireEvent.click(button)

    expect(onCreate).toHaveBeenCalledWith('codex')
  })

  it('shows the temporal menu only when running and URL is available', async () => {
    const dashboardStore = buildDashboardStore({
      agents: [{ id: 'codex', name: 'Codex', running: true, session_id: 'Codex 1' }],
    })
    createDashboardStore.mockReturnValue(dashboardStore)

    const { findByText, queryByText, rerender } = render(Dashboard, {
      props: {
        terminals: [],
        status: { session_count: 0, temporal_ui_url: 'https://temporal.test' },
      },
    })

    expect(await findByText('Open in Temporal')).toBeTruthy()
    expect(queryByText('No skills assigned')).toBeNull()

    await rerender({
      terminals: [],
      status: { session_count: 0 },
    })

    expect(queryByText('Open in Temporal')).toBeNull()
  })

  it('hides the temporal menu for stopped agents', async () => {
    const dashboardStore = buildDashboardStore({
      agents: [{ id: 'codex', name: 'Codex', running: false, session_id: '' }],
    })
    createDashboardStore.mockReturnValue(dashboardStore)

    const { queryByText } = render(Dashboard, {
      props: {
        terminals: [],
        status: { session_count: 0, temporal_ui_url: 'https://temporal.test' },
      },
    })

    expect(queryByText('Open in Temporal')).toBeNull()
  })

  it('expands log details from recent logs', async () => {
    Object.defineProperty(window, 'isSecureContext', {
      value: true,
      configurable: true,
    })
    const writeText = vi.fn(() => Promise.resolve())
    Object.assign(navigator, { clipboard: { writeText } })

    const dashboardStore = buildDashboardStore({
      logs: [
        {
          id: 'log-1',
          level: 'info',
          timestampISO: '2026-01-25T12:00:00Z',
          message: 'Log entry',
          attributes: {
            source: 'system',
            toast: 'true',
            toast_id: 'toast-1',
          },
          resourceAttributes: {},
          scopeName: '',
          raw: { scope: 'unit' },
        },
      ],
    })
    createDashboardStore.mockReturnValue(dashboardStore)

    const { findByText } = render(Dashboard, {
      props: {
        terminals: [],
        status: { session_count: 0 },
      },
    })

    const logButton = await findByText('Log entry')
    await fireEvent.click(logButton)

    await findByText('source')
    await findByText('toast')
    await findByText('toast_id')

    const rawToggle = await findByText('Raw JSON')
    await fireEvent.click(rawToggle)

    const copyButton = await findByText('Copy JSON')
    await fireEvent.click(copyButton)

    expect(writeText).toHaveBeenCalledTimes(1)
    expect(addNotification).toHaveBeenCalledWith('info', expect.stringContaining('Copied'))
  })

  it('copies status pills to clipboard', async () => {
    const dashboardStore = buildDashboardStore()
    createDashboardStore.mockReturnValue(dashboardStore)

    Object.defineProperty(window, 'isSecureContext', {
      value: true,
      configurable: true,
    })
    const writeText = vi.fn(() => Promise.resolve())
    Object.assign(navigator, { clipboard: { writeText } })

    const { findByText } = render(Dashboard, {
      props: {
        terminals: [],
        status: {
          session_count: 0,
          working_dir: '/repo/path',
          git_origin: 'origin',
          git_branch: 'origin/main',
        },
      },
    })

    const workdir = await findByText('/repo/path')
    await fireEvent.click(workdir)

    const remote = await findByText('origin')
    await fireEvent.click(remote)

    const branch = await findByText('main')
    await fireEvent.click(branch)

    expect(writeText).toHaveBeenCalledWith('/repo/path')
    expect(writeText).toHaveBeenCalledWith('origin')
    expect(writeText).toHaveBeenCalledWith('main')
    expect(addNotification).toHaveBeenCalledWith('info', expect.stringContaining('Copied'))
  })

  it('hides copy actions when clipboard is unavailable', async () => {
    Object.defineProperty(window, 'isSecureContext', {
      value: false,
      configurable: true,
    })

    const dashboardStore = buildDashboardStore({
      logs: [
        {
          id: 'log-1',
          level: 'info',
          timestampISO: '2026-01-25T12:00:00Z',
          message: 'Log entry',
          attributes: { source: 'system' },
          resourceAttributes: {},
          scopeName: '',
          raw: { scope: 'unit' },
        },
      ],
    })
    createDashboardStore.mockReturnValue(dashboardStore)

    const { findByText, queryByRole } = render(Dashboard, {
      props: {
        terminals: [],
        status: {
          session_count: 0,
          working_dir: '/repo/path',
          git_origin: 'origin',
          git_branch: 'origin/main',
        },
      },
    })

    const workdir = await findByText('/repo/path')
    expect(workdir.closest('button')).toBeNull()

    const logEntry = await findByText('Log entry')
    await fireEvent.click(logEntry)

    const rawToggle = await findByText('Raw JSON')
    await fireEvent.click(rawToggle)

    const copyButton = queryByRole('button', { name: 'Copy JSON' })
    expect(copyButton).toBeNull()
  })

})
