import { writable } from 'svelte/store'
import { buildWebSocketUrl } from './api.js'

const statusStore = writable('disconnected')
let currentStatus = 'disconnected'

const subscribers = new Map()
let socket = null
let reconnectTimer = null
let reconnectAttempts = 0
let nextId = 0

const logStatus = (status) => {
  if (typeof console === 'undefined') return
  console.info(`[workflow-events] ${status}`)
}

const setStatus = (status) => {
  if (currentStatus === status) return
  currentStatus = status
  statusStore.set(status)
  logStatus(status)
}

const hasSubscribers = () => subscribers.size > 0

const scheduleReconnect = () => {
  if (reconnectTimer || !hasSubscribers()) return
  const delay = Math.min(10000, 500 * 2 ** reconnectAttempts)
  reconnectAttempts += 1
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    connect()
  }, delay)
}

const connect = () => {
  if (typeof window === 'undefined') return
  if (!hasSubscribers()) {
    setStatus('disconnected')
    return
  }
  if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
    return
  }

  setStatus('connecting')
  try {
    socket = new WebSocket(buildWebSocketUrl('/api/workflows/events'))
  } catch {
    socket = null
    setStatus('disconnected')
    scheduleReconnect()
    return
  }

  socket.addEventListener('open', () => {
    reconnectAttempts = 0
    setStatus('connected')
  })

  socket.addEventListener('message', (event) => {
    let payload = null
    try {
      payload = JSON.parse(event.data)
    } catch {
      return
    }
    if (!payload?.type) return
    const listeners = subscribers.get(payload.type)
    if (!listeners) return
    listeners.forEach((listener) => {
      try {
        listener(payload)
      } catch {
        // Ignore listener errors to keep the stream alive.
      }
    })
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

const disconnect = () => {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  reconnectAttempts = 0
  if (socket) {
    socket.close()
    socket = null
  }
  setStatus('disconnected')
}

export const subscribe = (eventType, callback) => {
  if (!eventType || typeof callback !== 'function') {
    return () => {}
  }

  nextId += 1
  const id = `${Date.now()}-${nextId}`
  if (!subscribers.has(eventType)) {
    subscribers.set(eventType, new Map())
  }
  subscribers.get(eventType).set(id, callback)

  connect()

  return () => {
    const listeners = subscribers.get(eventType)
    if (!listeners) return
    listeners.delete(id)
    if (listeners.size === 0) {
      subscribers.delete(eventType)
    }
    if (!hasSubscribers()) {
      disconnect()
    }
  }
}

export const workflowEventConnectionStatus = {
  subscribe: statusStore.subscribe,
}
