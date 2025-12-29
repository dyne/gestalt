import { writable } from 'svelte/store'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
// FitAddon keeps terminal sizing logic centralized and maintained.

import { apiFetch, buildWebSocketUrl } from './api.js'
import { notificationStore } from './notificationStore.js'

const terminals = new Map()
const MAX_RETRIES = 5
const BASE_DELAY_MS = 500
const MAX_DELAY_MS = 8000

export const getTerminalState = (terminalId) => {
  if (!terminalId) return null
  if (!terminals.has(terminalId)) {
    terminals.set(terminalId, createTerminalState(terminalId))
  }
  return terminals.get(terminalId)
}

export const releaseTerminalState = (terminalId) => {
  const state = terminals.get(terminalId)
  if (!state) return
  state.dispose()
  terminals.delete(terminalId)
}

const createTerminalState = (terminalId) => {
  const status = writable('disconnected')
  const bellCount = writable(0)
  const canReconnect = writable(false)

  const term = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: '"IBM Plex Mono", "JetBrains Mono", monospace',
    theme: {
      background: '#101010',
      foreground: '#f2efe9',
      cursor: '#f2efe9',
      selectionBackground: '#3a3a3a',
    },
  })

  const fitAddon = new FitAddon()
  term.loadAddon(fitAddon)

  const encoder = new TextEncoder()
  let socket
  let container
  let disposed = false
  let retryCount = 0
  let reconnectTimer
  let notifiedUnauthorized = false
  let notifiedDisconnect = false

  const sendResize = () => {
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    const payload = {
      type: 'resize',
      cols: term.cols,
      rows: term.rows,
    }
    socket.send(JSON.stringify(payload))
  }

  const scheduleFit = () => {
    if (!container || disposed) return
    requestAnimationFrame(() => {
      if (!container || disposed) return
      fitAddon.fit()
      sendResize()
    })
  }

  const attach = (element) => {
    container = element
    if (!container || disposed) return
    if (!term.element) {
      term.open(container)
    } else if (term.element.parentElement !== container) {
      container.appendChild(term.element)
    }
    scheduleFit()
  }

  const detach = () => {
    container = null
  }

  term.onData((data) => {
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    socket.send(encoder.encode(data))
  })

  term.onBell(() => {
    bellCount.update((count) => count + 1)
  })

  const clearReconnectTimer = () => {
    if (!reconnectTimer) return
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }

  const checkAuthFailure = async (event) => {
    if (event?.code === 1008 || event?.code === 4401) {
      return true
    }
    try {
      await apiFetch('/api/status')
      return false
    } catch (err) {
      return err?.status === 401
    }
  }

  const scheduleReconnect = () => {
    if (disposed) return
    if (retryCount >= MAX_RETRIES) {
      status.set('disconnected')
      canReconnect.set(true)
      if (!notifiedDisconnect) {
        notificationStore.addNotification(
          'warning',
          `Terminal ${terminalId} connection lost.`
        )
        notifiedDisconnect = true
      }
      return
    }
    const delay = Math.min(BASE_DELAY_MS * 2 ** retryCount, MAX_DELAY_MS)
    retryCount += 1
    status.set('retrying')
    canReconnect.set(false)
    clearReconnectTimer()
    reconnectTimer = setTimeout(() => {
      connect(true)
    }, delay)
  }

  const connect = (isRetry = false) => {
    if (disposed) return
    if (
      socket &&
      (socket.readyState === WebSocket.OPEN ||
        socket.readyState === WebSocket.CONNECTING)
    ) {
      return
    }

    status.set(isRetry ? 'retrying' : 'connecting')
    canReconnect.set(false)

    socket = new WebSocket(buildWebSocketUrl(`/ws/terminal/${terminalId}`))
    socket.binaryType = 'arraybuffer'

    socket.addEventListener('open', () => {
      retryCount = 0
      status.set('connected')
      canReconnect.set(false)
      notifiedUnauthorized = false
      notifiedDisconnect = false
      scheduleFit()
    })

    socket.addEventListener('message', (event) => {
      if (disposed) return
      if (typeof event.data === 'string') {
        term.write(event.data)
        return
      }
      term.write(new Uint8Array(event.data))
    })

    socket.addEventListener('close', async (event) => {
      if (disposed) return
      console.warn('terminal websocket closed', {
        terminalId,
        code: event.code,
        reason: event.reason,
      })
      if (await checkAuthFailure(event)) {
        status.set('unauthorized')
        canReconnect.set(true)
        if (!notifiedUnauthorized) {
          notificationStore.addNotification(
            'error',
            `Terminal ${terminalId} requires authentication.`
          )
          notifiedUnauthorized = true
        }
        return
      }
      scheduleReconnect()
    })

    socket.addEventListener('error', (event) => {
      console.error('terminal websocket error', event)
      if (!notifiedDisconnect) {
        notificationStore.addNotification(
          'warning',
          `Terminal ${terminalId} connection error.`
        )
        notifiedDisconnect = true
      }
    })

  }

  const reconnect = () => {
    if (disposed) return
    clearReconnectTimer()
    retryCount = 0
    if (socket && socket.readyState !== WebSocket.CLOSED) {
      socket.close()
    }
    connect(false)
  }

  const dispose = () => {
    if (disposed) return
    disposed = true
    clearReconnectTimer()
    canReconnect.set(false)
    if (socket) {
      socket.close()
    }
    term.dispose()
  }

  connect()

  return {
    term,
    status,
    bellCount,
    canReconnect,
    attach,
    detach,
    scheduleFit,
    reconnect,
    dispose,
  }
}
