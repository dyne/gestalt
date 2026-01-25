import { buildWebSocketUrl } from './api.js'

const MAX_RECONNECT_DELAY_MS = 10000
const BASE_RECONNECT_DELAY_MS = 500

const buildLogPath = (level) => {
  const trimmed = String(level || '').trim()
  if (!trimmed) return '/ws/logs'
  return `/ws/logs?level=${encodeURIComponent(trimmed)}`
}

export const createLogStream = ({
  level = '',
  onEntry,
  onStatus,
  onOpen,
  onError,
  autoReconnect = true,
} = {}) => {
  let socket = null
  let reconnectTimer = null
  let reconnectAttempts = 0
  let active = false
  let currentLevel = String(level || '')

  const setStatus = (status) => {
    if (typeof onStatus === 'function') {
      onStatus(status)
    }
  }

  const clearReconnect = () => {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  const scheduleReconnect = () => {
    if (!active || !autoReconnect) return
    if (reconnectTimer) return
    const delay = Math.min(MAX_RECONNECT_DELAY_MS, BASE_RECONNECT_DELAY_MS * 2 ** reconnectAttempts)
    reconnectAttempts += 1
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      connect()
    }, delay)
  }

  const sendFilter = () => {
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    const payload = { level: currentLevel }
    socket.send(JSON.stringify(payload))
  }

  const connect = () => {
    if (!active) return
    if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
      return
    }
    setStatus('connecting')
    try {
      socket = new WebSocket(buildWebSocketUrl(buildLogPath(currentLevel)))
    } catch (err) {
      socket = null
      setStatus('disconnected')
      if (typeof onError === 'function') {
        onError(err)
      }
      scheduleReconnect()
      return
    }

    socket.addEventListener('open', () => {
      reconnectAttempts = 0
      setStatus('connected')
      if (typeof onOpen === 'function') {
        onOpen()
      }
      if (currentLevel) {
        sendFilter()
      }
    })

    socket.addEventListener('message', (event) => {
      let payload = null
      try {
        payload = JSON.parse(event.data)
      } catch {
        return
      }
      if (payload?.type === 'error') {
        if (typeof onError === 'function') {
          onError(new Error(payload.message || 'Log stream error'))
        }
        return
      }
      if (typeof onEntry === 'function') {
        onEntry(payload)
      }
    })

    socket.addEventListener('close', () => {
      socket = null
      setStatus('disconnected')
      scheduleReconnect()
    })

    socket.addEventListener('error', () => {
      socket?.close()
    })
  }

  const closeSocket = () => {
    if (!socket) return
    socket.close()
    socket = null
  }

  const start = () => {
    active = true
    clearReconnect()
    connect()
  }

  const stop = () => {
    active = false
    clearReconnect()
    reconnectAttempts = 0
    closeSocket()
    setStatus('disconnected')
  }

  const restart = () => {
    if (!active) return
    clearReconnect()
    closeSocket()
    connect()
  }

  const setLevel = (levelValue) => {
    currentLevel = String(levelValue || '')
    if (socket && socket.readyState === WebSocket.OPEN) {
      sendFilter()
    }
  }

  return {
    start,
    stop,
    restart,
    setLevel,
  }
}
