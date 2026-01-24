import { writable } from 'svelte/store'
import { buildWebSocketUrl } from './api.js'

const MAX_RECONNECT_DELAY_MS = 10000
const BASE_RECONNECT_DELAY_MS = 500

const normalizePayload = (payload) => {
  if (!payload) return null
  if (typeof payload === 'string') return payload
  if (payload instanceof ArrayBuffer) return payload
  return JSON.stringify(payload)
}

export const createWsStore = ({ label, path, buildSubscribeMessage }) => {
  const statusStore = writable('disconnected')
  let currentStatus = 'disconnected'

  const subscribers = new Map()
  let socket = null
  let reconnectTimer = null
  let reconnectAttempts = 0
  let nextId = 0

  const logStatus = (status) => {
    if (typeof console === 'undefined') return
    console.info(`[${label}] ${status}`)
  }

  const setStatus = (status) => {
    if (currentStatus === status) return
    currentStatus = status
    statusStore.set(status)
    logStatus(status)
  }

  const hasSubscribers = () => subscribers.size > 0

  const sendSubscriptions = () => {
    if (!buildSubscribeMessage) return
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    const types = Array.from(subscribers.keys())
    const payload = normalizePayload(buildSubscribeMessage(types))
    if (!payload) return
    socket.send(payload)
  }

  const scheduleReconnect = () => {
    if (reconnectTimer || !hasSubscribers()) return
    const delay = Math.min(
      MAX_RECONNECT_DELAY_MS,
      BASE_RECONNECT_DELAY_MS * 2 ** reconnectAttempts
    )
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
    if (
      socket &&
      (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)
    ) {
      return
    }

    setStatus('connecting')
    try {
      socket = new WebSocket(buildWebSocketUrl(path))
    } catch {
      socket = null
      setStatus('disconnected')
      scheduleReconnect()
      return
    }

    socket.addEventListener('open', () => {
      reconnectAttempts = 0
      setStatus('connected')
      sendSubscriptions()
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

  const subscribe = (eventType, callback) => {
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
    sendSubscriptions()

    return () => {
      const listeners = subscribers.get(eventType)
      if (!listeners) return
      listeners.delete(id)
      if (listeners.size === 0) {
        subscribers.delete(eventType)
      }
      if (!hasSubscribers()) {
        disconnect()
        return
      }
      sendSubscriptions()
    }
  }

  return {
    subscribe,
    connectionStatus: {
      subscribe: statusStore.subscribe,
    },
  }
}
