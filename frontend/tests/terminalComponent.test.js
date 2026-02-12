import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { writable } from 'svelte/store'
import { tick } from 'svelte'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))
const getTerminalState = vi.hoisted(() => vi.fn())
const createTerminalService = vi.hoisted(() => vi.fn())
const fetchStatus = vi.hoisted(() => vi.fn())
const fetchWorkflows = vi.hoisted(() => vi.fn())

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

vi.mock('../src/lib/terminalStore.js', () => ({
  getTerminalState,
}))

vi.mock('../src/lib/terminal/service_mcp.js', () => ({
  createTerminalService,
}))

vi.mock('../src/lib/apiClient.js', () => ({
  fetchStatus,
  fetchWorkflows,
}))

import Terminal from '../src/components/Terminal.svelte'
import TerminalView from '../src/views/TerminalView.svelte'

const buildState = () => {
  return {
    attach: vi.fn(),
    detach: vi.fn(),
    setScrollSensitivity: vi.fn(),
    setDirectInput: vi.fn(),
    reconnect: vi.fn(),
    scheduleFit: vi.fn(),
    sendCommand: vi.fn(),
    sendData: vi.fn(),
    focus: vi.fn(),
    scrollToBottom: vi.fn(),
    setAtBottom: vi.fn(),
    appendPrompt: vi.fn(),
    setVisible: vi.fn(),
    status: writable('connected'),
    historyStatus: writable('idle'),
    bellCount: writable(0),
    canReconnect: writable(false),
    atBottom: writable(true),
    segments: writable([]),
  }
}

const buildConsoleState = () => {
  return {
    status: writable('connected'),
    historyStatus: writable('idle'),
    bellCount: writable(0),
    canReconnect: writable(false),
    atBottom: writable(true),
    segments: writable([]),
    sendCommand: vi.fn(),
    sendData: vi.fn(),
    setAtBottom: vi.fn(),
    appendPrompt: vi.fn(),
    setVisible: vi.fn(),
    reconnect: vi.fn(),
    dispose: vi.fn(),
  }
}

describe('Terminal', () => {
  beforeEach(() => {
    vi.stubGlobal('requestAnimationFrame', (cb) => {
      cb()
      return 0
    })
    apiFetch.mockResolvedValue({
      json: vi.fn().mockResolvedValue([]),
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    apiFetch.mockReset()
    getTerminalState.mockReset()
    createTerminalService.mockReset()
    fetchStatus.mockReset()
    fetchWorkflows.mockReset()
    cleanup()
  })

  it('shows reconnect button when reconnectable', async () => {
    const state = buildState()
    state.canReconnect.set(true)
    getTerminalState.mockReturnValue(state)

    const { getByText } = render(Terminal, { props: { terminalId: 't1' } })
    const reconnectButton = getByText('Reconnect')
    await fireEvent.click(reconnectButton)

    expect(state.reconnect).toHaveBeenCalled()
  })

  it('focuses the command input when connected and visible', async () => {
    const state = buildState()
    getTerminalState.mockReturnValue(state)
    const focusSpy = vi
      .spyOn(HTMLTextAreaElement.prototype, 'focus')
      .mockImplementation(() => {})

    const { container } = render(Terminal, {
      props: { terminalId: 't1', visible: true },
    })
    await tick()
    await tick()

    const textarea = container.querySelector('textarea')
    expect(textarea).toBeTruthy()
    expect(focusSpy).toHaveBeenCalled()
    focusSpy.mockRestore()
  })

  it('shows the session id as the header label', () => {
    const state = buildState()
    getTerminalState.mockReturnValue(state)

    const { getByText } = render(Terminal, {
      props: { terminalId: 't1', title: 'Coder' },
    })

    expect(getByText('t1')).toBeTruthy()
  })

  it('disables the Temporal button when no workflow link exists', () => {
    const state = buildState()
    getTerminalState.mockReturnValue(state)

    const { getByText } = render(Terminal, {
      props: { terminalId: 't1', title: 'Coder' },
    })

    const temporalButton = getByText('Temporal')
    expect(temporalButton.hasAttribute('disabled')).toBe(true)
  })

  it('shows the Bottom button when scrolled up', async () => {
    const state = buildState()
    state.atBottom.set(false)
    getTerminalState.mockReturnValue(state)

    const { getByText } = render(Terminal, {
      props: { terminalId: 't1', title: 'Coder' },
    })

    expect(getByText('Bottom')).toBeTruthy()
  })

  it('renders transcript view for mcp sessions', async () => {
    const state = buildState()
    getTerminalState.mockReturnValue(state)

    const { container } = render(Terminal, {
      props: { terminalId: 't1', sessionInterface: 'mcp' },
    })

    await tick()
    expect(container.querySelector('.terminal-text__body')).toBeTruthy()
  })

  it('renders xterm canvas for cli sessions', async () => {
    const state = buildState()
    getTerminalState.mockReturnValue(state)

    const { container } = render(Terminal, {
      props: { terminalId: 't1', sessionInterface: 'cli' },
    })

    await tick()
    expect(container.querySelector('.terminal-shell__body')).toBeTruthy()
    expect(container.querySelector('.terminal-text__body')).toBeFalsy()
  })

  it('reattaches state when terminal id changes', async () => {
    const stateA = buildState()
    const stateB = buildState()
    getTerminalState.mockReturnValueOnce(stateA).mockReturnValueOnce(stateB)

    const { rerender } = render(Terminal, {
      props: { terminalId: 't1', sessionInterface: 'mcp' },
    })
    await tick()

    await rerender({ terminalId: 't2', sessionInterface: 'mcp' })
    await tick()

    expect(getTerminalState).toHaveBeenCalledWith('t2', 'mcp')
    expect(stateA.setVisible).toHaveBeenCalledWith(false)
  })

  it('renders console module without xterm when console enabled', async () => {
    createTerminalService.mockReturnValue(buildConsoleState())
    fetchStatus.mockResolvedValue({})
    fetchWorkflows.mockResolvedValue([])

    const { container } = render(TerminalView, {
      props: {
        terminalId: 't1',
        guiModules: ['console'],
        sessionInterface: 'cli',
        visible: true,
      },
    })

    await tick()
    expect(container.querySelector('.terminal-text__body')).toBeTruthy()
    expect(container.querySelector('.terminal-shell__body')).toBeFalsy()
  })

  it('renders xterm canvas when terminal module enabled', async () => {
    getTerminalState.mockReturnValue(buildState())
    fetchStatus.mockResolvedValue({})
    fetchWorkflows.mockResolvedValue([])

    const { container } = render(TerminalView, {
      props: {
        terminalId: 't1',
        guiModules: ['terminal'],
        sessionInterface: 'cli',
        visible: true,
      },
    })

    await tick()
    expect(container.querySelector('.terminal-shell__body')).toBeTruthy()
  })
})
