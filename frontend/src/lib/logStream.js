import { buildEventSourceUrl } from './api.js'

const MAX_RECONNECT_DELAY_MS = 10000
const BASE_RECONNECT_DELAY_MS = 500

const buildLogParams = (level) => {
  const trimmed = String(level || '').trim()
  if (!trimmed) return {}
  return { level: trimmed }
}

export const createLogStream = ({
  level = '',
  onEntry,
  onStatus,
  onOpen,
  onError,
  autoReconnect = true,
} = {}) => {
  let source = null
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

  const buildUrl = () => buildEventSourceUrl('/api/logs/stream', buildLogParams(currentLevel))

  const connect = () => {
    if (!active) return
    if (
      source &&
      (source.readyState === EventSource.OPEN || source.readyState === EventSource.CONNECTING)
    ) {
      return
    }
    setStatus('connecting')
    try {
      source = new EventSource(buildUrl())
    } catch (err) {
      source = null
      setStatus('disconnected')
      if (typeof onError === 'function') {
        onError(err)
      }
      scheduleReconnect()
      return
    }

    source.addEventListener('open', () => {
      reconnectAttempts = 0
      setStatus('connected')
      if (typeof onOpen === 'function') {
        onOpen()
      }
    })

    source.addEventListener('message', (event) => {
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

    source.addEventListener('error', () => {
      if (!source) return
      source.close()
      source = null
      setStatus('disconnected')
      scheduleReconnect()
    })
  }

  const closeSource = () => {
    if (!source) return
    source.close()
    source = null
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
    closeSource()
    setStatus('disconnected')
  }

  const restart = () => {
    if (!active) return
    clearReconnect()
    closeSource()
    connect()
  }

  const setLevel = (levelValue) => {
    const nextLevel = String(levelValue || '')
    if (nextLevel === currentLevel) return
    currentLevel = nextLevel
    if (active) {
      restart()
    }
  }

  return {
    start,
    stop,
    restart,
    setLevel,
  }
}
