import { apiFetch, buildWebSocketUrl } from '../api.js'
import { notificationStore } from '../notificationStore.js'

const MAX_RETRIES = 5
const BASE_DELAY_MS = 500
const MAX_DELAY_MS = 8000
const HISTORY_WARNING_MS = 5000
const HISTORY_LINES = 2000

export const createTerminalSocket = ({
  terminalId,
  term,
  status,
  historyStatus,
  canReconnect,
  historyCache,
  syncScrollState,
  scheduleFit,
}) => {
  let socket = null
  let retryCount = 0
  let reconnectTimer = null
  let historyWarningTimer = null
  let historyLoadPromise = null
  let historyLoaded = false
  let pendingHistory = ''
  let notifiedHistorySlow = false
  let notifiedHistoryError = false
  let notifiedUnauthorized = false
  let notifiedDisconnect = false
  let disposed = false

  const clearReconnectTimer = () => {
    if (!reconnectTimer) return
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }

  const clearHistoryWarning = () => {
    if (!historyWarningTimer) return
    clearTimeout(historyWarningTimer)
    historyWarningTimer = null
  }

  const flushPendingHistory = () => {
    if (!pendingHistory || disposed || !term.element) return
    term.write(pendingHistory)
    pendingHistory = ''
    syncScrollState()
  }

  const scheduleHistoryWarning = () => {
    if (historyWarningTimer || notifiedHistorySlow) return
    historyWarningTimer = setTimeout(() => {
      historyWarningTimer = null
      if (historyLoaded || disposed) return
      historyStatus.set('slow')
      if (!notifiedHistorySlow) {
        notificationStore.addNotification(
          'warning',
          `Terminal ${terminalId} history is taking longer to load.`
        )
        notifiedHistorySlow = true
      }
    }, HISTORY_WARNING_MS)
  }

  const loadHistory = () => {
    if (historyLoaded) {
      return Promise.resolve()
    }
    if (historyLoadPromise) {
      return historyLoadPromise
    }
    if (historyCache.has(terminalId)) {
      const cachedHistory = historyCache.get(terminalId) || ''
      if (term.element) {
        term.write(cachedHistory)
      } else {
        pendingHistory = cachedHistory
      }
      historyLoaded = true
      historyStatus.set('loaded')
      return Promise.resolve()
    }

    historyStatus.set('loading')
    scheduleHistoryWarning()
    historyLoadPromise = (async () => {
      try {
        const response = await apiFetch(
          `/api/terminals/${terminalId}/history?lines=${HISTORY_LINES}`
        )
        const payload = await response.json()
        const lines = Array.isArray(payload?.lines) ? payload.lines : []
        const historyText = lines.join('\n')
        if (historyText) {
          if (term.element) {
            term.write(historyText)
          } else {
            pendingHistory = historyText
          }
          if (term.element) {
            syncScrollState()
          }
        }
        historyCache.set(terminalId, historyText)
        historyLoaded = true
        historyStatus.set('loaded')
      } catch (err) {
        console.warn('failed to load terminal history', err)
        historyStatus.set('error')
        if (!notifiedHistoryError) {
          notificationStore.addNotification(
            'warning',
            `Terminal ${terminalId} history could not be loaded.`
          )
          notifiedHistoryError = true
        }
      } finally {
        clearHistoryWarning()
        historyLoadPromise = null
      }
    })()

    return historyLoadPromise
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

  const connect = async (isRetry = false) => {
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

    if (!isRetry) {
      await loadHistory()
    }

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
        syncScrollState()
        return
      }
      term.write(new Uint8Array(event.data))
      syncScrollState()
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

  const send = (data) => {
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    socket.send(data)
  }

  const dispose = () => {
    if (disposed) return
    disposed = true
    clearReconnectTimer()
    canReconnect.set(false)
    if (socket) {
      socket.close()
    }
  }

  return {
    connect,
    reconnect,
    send,
    dispose,
    flushPendingHistory,
  }
}
