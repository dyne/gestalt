import { writable } from 'svelte/store'
import { buildEventSourceUrl } from './api.js'

const MAX_RECONNECT_DELAY_MS = 10000
const BASE_RECONNECT_DELAY_MS = 500

export const createSseStore = ({ label, path, buildQueryParams }) => {
  const statusStore = writable('disconnected')
  let currentStatus = 'disconnected'

  const subscribers = new Map()
  let source = null
  let reconnectTimer = null
  let reconnectAttempts = 0
  let nextId = 0
  let currentUrl = ''

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

  const currentTypes = () => Array.from(subscribers.keys()).sort()

  const buildUrl = () => {
    const params = buildQueryParams ? buildQueryParams(currentTypes()) : undefined
    return buildEventSourceUrl(path, params)
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

    const nextUrl = buildUrl()
    if (
      source &&
      (source.readyState === EventSource.OPEN || source.readyState === EventSource.CONNECTING)
    ) {
      if (nextUrl === currentUrl) return
      source.close()
      source = null
    }

    currentUrl = nextUrl
    setStatus('connecting')

    try {
      source = new EventSource(nextUrl)
    } catch {
      source = null
      setStatus('disconnected')
      scheduleReconnect()
      return
    }

    source.addEventListener('open', () => {
      reconnectAttempts = 0
      setStatus('connected')
    })

    source.addEventListener('message', (event) => {
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

    source.addEventListener('error', () => {
      if (!source) return
      source.close()
      source = null
      setStatus('disconnected')
      scheduleReconnect()
    })
  }

  const disconnect = () => {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    reconnectAttempts = 0
    if (source) {
      source.close()
      source = null
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
      connect()
    }
  }

  return {
    subscribe,
    connectionStatus: {
      subscribe: statusStore.subscribe,
    },
  }
}
