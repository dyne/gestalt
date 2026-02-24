import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { writable } from 'svelte/store'
import { tick } from 'svelte'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))
const getTerminalState = vi.hoisted(() => vi.fn())

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
    cleanup()
  })

  it('shows reconnect button when reconnectable', async () => {
    const state = buildState()
    state.canReconnect.set(true)
    getTerminalState.mockReturnValue(state)

    const { getByText } = render(Terminal, { props: { sessionId: 't1' } })
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

    const { getByPlaceholderText } = render(Terminal, {
      props: { sessionId: 't1', visible: true, showInput: true, sessionInterface: 'cli' },
    })
    await tick()
    await tick()

    const textarea = getByPlaceholderText(/Type command/)
    expect(textarea).toBeTruthy()
    expect(focusSpy).toHaveBeenCalled()
    focusSpy.mockRestore()
  })

  it('shows the session id as the header label', () => {
    const state = buildState()
    getTerminalState.mockReturnValue(state)

    const { getByText } = render(Terminal, {
      props: { sessionId: 't1', title: 'Coder' },
    })

    expect(getByText('t1')).toBeTruthy()
  })

  it('shows the Bottom button when scrolled up', async () => {
    const state = buildState()
    state.atBottom.set(false)
    getTerminalState.mockReturnValue(state)

    const { getByText } = render(Terminal, {
      props: { sessionId: 't1', title: 'Coder' },
    })

    expect(getByText('Bottom')).toBeTruthy()
  })

  it('renders xterm canvas for cli sessions', async () => {
    const state = buildState()
    getTerminalState.mockReturnValue(state)

    const { container } = render(Terminal, {
      props: { sessionId: 't1', sessionInterface: 'cli' },
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
      props: { sessionId: 't1', sessionInterface: 'cli' },
    })
    await tick()

    await rerender({ sessionId: 't2', sessionInterface: 'cli' })
    await tick()

    expect(getTerminalState).toHaveBeenCalledWith('t2', 'cli', '', { allowMouseReporting: false })
    expect(stateA.setVisible).toHaveBeenCalledWith(false)
  })

  it('renders xterm canvas in TerminalView', async () => {
    getTerminalState.mockReturnValue(buildState())

    const { container } = render(TerminalView, {
      props: {
        sessionId: 't1',
        sessionInterface: 'cli',
        visible: true,
      },
    })

    await tick()
    expect(container.querySelector('.terminal-shell__body')).toBeTruthy()
  })

  it('renders canvas for external sessions', async () => {
    getTerminalState.mockReturnValue(buildState())

    const { container, queryByText } = render(Terminal, {
      props: {
        sessionId: 't1',
        sessionInterface: 'cli',
        sessionRunner: 'external',
      },
    })

    await tick()
    expect(container.querySelector('.terminal-shell__body')).toBeTruthy()
    expect(queryByText('This session is managed in tmux.')).toBeFalsy()
  })

  it('hides command input when showInput is false', async () => {
    getTerminalState.mockReturnValue(buildState())

    const { queryByPlaceholderText } = render(Terminal, {
      props: {
        sessionId: 't1',
        showInput: false,
      },
    })

    await tick()
    expect(queryByPlaceholderText('Type command')).toBeFalsy()
  })

  it('forces direct input mode when requested', async () => {
    const state = buildState()
    getTerminalState.mockReturnValue(state)

    render(Terminal, {
      props: {
        sessionId: 't1',
        forceDirectInput: true,
      },
    })

    await tick()
    expect(state.setDirectInput).toHaveBeenCalledWith(true)
  })

  it('notifies when connection fails after retries', async () => {
    const state = buildState()
    const onConnectionFailed = vi.fn()
    state.status.set('disconnected')
    state.canReconnect.set(true)
    getTerminalState.mockReturnValue(state)

    render(Terminal, {
      props: {
        sessionId: 'hub',
        sessionInterface: 'cli',
        sessionRunner: 'server',
        onConnectionFailed,
      },
    })

    await tick()
    expect(onConnectionFailed).toHaveBeenCalledWith('hub')
  })
})
