import { describe, it, expect } from 'vitest'
import { createDirectorChatStore } from '../src/lib/directorChatStore.js'

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
})
