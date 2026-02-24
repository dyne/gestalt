import { describe, it, expect, vi, afterEach } from 'vitest'
import { createDirectorChatStore } from '../src/lib/directorChatStore.js'

const socketFactory = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/terminal/socket.js', () => ({
  createTerminalSocket: socketFactory,
}))

afterEach(() => {
  socketFactory.mockReset()
  vi.useRealTimers()
})

describe('directorChatStore', () => {
  it('appends user messages immediately', () => {
    const store = createDirectorChatStore()

    store.appendUserMessage('Plan the release', 'text')
    const state = store.snapshot()

    expect(state.messages).toHaveLength(1)
    expect(state.messages[0].role).toBe('user')
    expect(state.messages[0].text).toBe('Plan the release')
    expect(state.messages[0].source).toBe('text')
  })

  it('appends external chat messages without prompt suppression', () => {
    const store = createDirectorChatStore()

    store.appendChatMessage({ text: 'Hello from CLI', source: 'cli' })
    const state = store.snapshot()

    expect(state.messages).toHaveLength(1)
    expect(state.messages[0].role).toBe('user')
    expect(state.messages[0].text).toBe('Hello from CLI')
    expect(state.messages[0].source).toBe('cli')
  })

  it('aggregates assistant chunks into one streaming bubble', () => {
    const store = createDirectorChatStore()

    store.appendAssistantChunk('First ')
    store.appendAssistantChunk('reply')

    const state = store.snapshot()
    expect(state.streaming).toBe(true)
    expect(state.messages).toHaveLength(1)
    expect(state.messages[0].role).toBe('assistant')
    expect(state.messages[0].text).toBe('First reply')

    store.finalizeAssistant()
    expect(store.snapshot().messages[0].status).toBe('done')
  })

  it('suppresses echoed prompts from assistant output', () => {
    const store = createDirectorChatStore()

    store.appendUserMessage('draft summary', 'text')
    store.appendAssistantChunk('draft summary\n')
    store.appendAssistantChunk('Here is the summary')

    const state = store.snapshot()
    expect(state.messages).toHaveLength(2)
    expect(state.messages[0].role).toBe('user')
    expect(state.messages[1].role).toBe('assistant')
    expect(state.messages[1].text).toBe('Here is the summary')
  })

  it('attaches terminal stream and aggregates output chunks', () => {
    let onOutput = null
    const connect = vi.fn()
    const disconnect = vi.fn()
    const dispose = vi.fn()
    socketFactory.mockImplementation((options) => {
      onOutput = options.onOutput
      return { connect, disconnect, dispose }
    })

    const store = createDirectorChatStore()
    store.attachStream('Director 1')
    store.connectStream()
    onOutput('Hello ')
    onOutput('world')

    const state = store.snapshot()
    expect(connect).toHaveBeenCalledTimes(1)
    expect(state.messages.at(-1).text).toBe('Hello world')

    store.detachStream()
    expect(disconnect).toHaveBeenCalledTimes(1)
    expect(dispose).toHaveBeenCalledTimes(1)
  })

  it('finalizes assistant output after idle debounce', () => {
    vi.useFakeTimers()
    let onOutput = null
    socketFactory.mockImplementation((options) => {
      onOutput = options.onOutput
      return { connect() {}, disconnect() {}, dispose() {} }
    })

    const store = createDirectorChatStore({ outputIdleMs: 200 })
    store.attachStream('Director 1')
    onOutput('streaming text')
    expect(store.snapshot().messages.at(-1).status).toBe('streaming')

    vi.advanceTimersByTime(220)
    expect(store.snapshot().messages.at(-1).status).toBe('done')
  })
})
