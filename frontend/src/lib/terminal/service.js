import { writable } from 'svelte/store'

import { createTerminalSocket } from './socket.js'
import {
  appendOutputSegment,
  appendPromptSegment,
  createPromptEchoSuppressor,
  historyToSegments,
} from './segments.js'

export const createTerminalService = ({ terminalId, historyCache }) => {
  const status = writable('disconnected')
  const historyStatus = writable('idle')
  const bellCount = writable(0)
  const canReconnect = writable(false)
  const atBottom = writable(true)
  const segments = writable([])

  const encoder = new TextEncoder()
  const cache = historyCache || new Map()
  const echoSuppressor = createPromptEchoSuppressor()
  let socketManager
  let disposed = false
  let isVisible = false

  const sendData = (data) => {
    if (!socketManager) return
    if (!data) return
    socketManager.send(encoder.encode(data))
  }

  const sendCommand = (command) => {
    const payload = typeof command === 'string' ? command : ''
    echoSuppressor.markCommand(payload)
    if (payload) {
      sendData(payload)
    }
    sendData('\r')
  }

  const setAtBottom = (value) => {
    atBottom.set(Boolean(value))
  }

  const handleOutput = (chunk) => {
    const { output } = echoSuppressor.filterChunk(chunk)
    if (!output) return
    segments.update((current) => appendOutputSegment(current, output))
  }

  const handleHistory = (lines) => {
    segments.set(historyToSegments(lines))
  }

  const appendPrompt = (prompt) => {
    segments.update((current) => appendPromptSegment(current, prompt))
  }

  socketManager = createTerminalSocket({
    terminalId,
    status,
    historyStatus,
    canReconnect,
    historyCache: cache,
    onOutput: handleOutput,
    onHistory: handleHistory,
  })

  const { connect, disconnect, reconnect, dispose: disposeSocket } = socketManager

  const setVisible = (value) => {
    const next = Boolean(value)
    if (next === isVisible) return
    isVisible = next
    if (isVisible) {
      connect()
      return
    }
    disconnect?.()
  }

  const dispose = () => {
    if (disposed) return
    disposed = true
    if (disposeSocket) {
      disposeSocket()
    }
    canReconnect.set(false)
  }

  return {
    status,
    historyStatus,
    bellCount,
    canReconnect,
    atBottom,
    segments,
    sendData,
    sendCommand,
    setAtBottom,
    appendPrompt,
    setVisible,
    reconnect,
    dispose,
  }
}
