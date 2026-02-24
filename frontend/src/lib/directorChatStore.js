import { get, writable } from 'svelte/store'
import { createPromptEchoSuppressor } from './terminal/segments.js'

const buildInitialState = () => ({
  sessionId: '',
  messages: [],
  streaming: false,
  error: '',
})

export const createDirectorChatStore = () => {
  const state = writable(buildInitialState())
  const suppressor = createPromptEchoSuppressor()
  let nextMessageId = 1

  const setSession = (sessionId) => {
    state.update((current) => ({
      ...current,
      sessionId: sessionId ? String(sessionId) : '',
    }))
  }

  const appendUserMessage = (text, source = 'text') => {
    const value = String(text || '').trim()
    if (!value) return null
    const message = {
      id: `msg-${nextMessageId++}`,
      role: 'user',
      text: value,
      source: source === 'voice' ? 'voice' : 'text',
      createdAt: new Date().toISOString(),
      status: 'sent',
    }
    suppressor.markCommand(value)
    state.update((current) => ({
      ...current,
      messages: [...current.messages, message],
      error: '',
    }))
    return message
  }

  const appendAssistantChunk = (chunk) => {
    const filtered = suppressor.filterChunk(chunk)
    const text = String(filtered?.output || '')
    if (!text) return
    state.update((current) => {
      const messages = current.messages.slice()
      const lastMessage = messages[messages.length - 1]
      if (lastMessage?.role === 'assistant' && lastMessage?.status === 'streaming') {
        messages[messages.length - 1] = {
          ...lastMessage,
          text: `${lastMessage.text}${text}`,
        }
      } else {
        messages.push({
          id: `msg-${nextMessageId++}`,
          role: 'assistant',
          text,
          createdAt: new Date().toISOString(),
          status: 'streaming',
        })
      }
      return {
        ...current,
        messages,
        streaming: true,
      }
    })
  }

  const finalizeAssistant = () => {
    state.update((current) => {
      const messages = current.messages.slice()
      const lastIndex = messages.length - 1
      if (lastIndex >= 0 && messages[lastIndex].role === 'assistant') {
        messages[lastIndex] = {
          ...messages[lastIndex],
          status: 'done',
        }
      }
      return {
        ...current,
        messages,
        streaming: false,
      }
    })
  }

  const setError = (message) => {
    state.update((current) => ({
      ...current,
      error: String(message || ''),
    }))
  }

  const clear = () => {
    state.set(buildInitialState())
  }

  return {
    subscribe: state.subscribe,
    setSession,
    appendUserMessage,
    appendAssistantChunk,
    finalizeAssistant,
    setError,
    clear,
    snapshot: () => get(state),
  }
}
