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

  it('renders transcript view for mcp sessions', () => {
    const state = buildState()
    getTerminalState.mockReturnValue(state)

    const { container } = render(Terminal, {
      props: { terminalId: 't1', sessionInterface: 'mcp' },
    })

    expect(container.querySelector('.terminal-text__body')).toBeTruthy()
  })

  it('renders xterm canvas for cli sessions', () => {
    const state = buildState()
    getTerminalState.mockReturnValue(state)

    const { container } = render(Terminal, {
      props: { terminalId: 't1', sessionInterface: 'cli' },
    })

    expect(container.querySelector('.terminal-shell__body')).toBeTruthy()
    expect(container.querySelector('.terminal-text__body')).toBeFalsy()
  })
})
