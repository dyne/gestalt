import { writable } from 'svelte/store'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
// FitAddon keeps terminal sizing logic centralized and maintained.

import { apiFetch, buildWebSocketUrl } from './api.js'
import { notificationStore } from './notificationStore.js'

const terminals = new Map()
const historyCache = new Map()
const MAX_RETRIES = 5
const BASE_DELAY_MS = 500
const MAX_DELAY_MS = 8000
const HISTORY_WARNING_MS = 5000
const HISTORY_LINES = 2000
const MOUSE_MODE_PARAMS = new Set([
  9,
  1000,
  1001,
  1002,
  1003,
  1005,
  1006,
  1007,
  1015,
  1016,
])

const hasModifierKey = (event) => event.ctrlKey || event.metaKey

const isCopyKey = (event) =>
  hasModifierKey(event) &&
  !event.altKey &&
  event.key.toLowerCase() === 'c'

const isPasteKey = (event) =>
  hasModifierKey(event) &&
  !event.altKey &&
  event.key.toLowerCase() === 'v'

const writeClipboardText = async (text) => {
  if (!text) return false
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch (err) {
      // Fall back to legacy clipboard handling.
    }
  }
  return writeClipboardTextFallback(text)
}

const writeClipboardTextFallback = (text) => {
  try {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.setAttribute('readonly', '')
    textarea.style.position = 'fixed'
    textarea.style.top = '-9999px'
    textarea.style.left = '-9999px'
    document.body.appendChild(textarea)
    textarea.select()
    const ok = document.execCommand?.('copy')
    document.body.removeChild(textarea)
    return Boolean(ok)
  } catch (err) {
    return false
  }
}

const readClipboardText = async () => {
  if (navigator.clipboard?.readText) {
    try {
      return await navigator.clipboard.readText()
    } catch (err) {
      // Fall back to legacy clipboard handling.
    }
  }
  return readClipboardTextFallback()
}

const readClipboardTextFallback = () => {
  try {
    const textarea = document.createElement('textarea')
    textarea.style.position = 'fixed'
    textarea.style.top = '-9999px'
    textarea.style.left = '-9999px'
    document.body.appendChild(textarea)
    textarea.focus()
    textarea.select()
    document.execCommand?.('paste')
    const text = textarea.value
    document.body.removeChild(textarea)
    return text
  } catch (err) {
    return ''
  }
}

const flattenParams = (params) => {
  const flattened = []
  for (const param of params) {
    if (Array.isArray(param)) {
      for (const value of param) {
        flattened.push(value)
      }
    } else {
      flattened.push(param)
    }
  }
  return flattened
}

const shouldSuppressMouseMode = (params) => {
  const flattened = flattenParams(params)
  if (!flattened.length) return false
  const hasMouse = flattened.some((value) => MOUSE_MODE_PARAMS.has(value))
  if (!hasMouse) return false
  return flattened.every((value) => MOUSE_MODE_PARAMS.has(value))
}

const isMouseReport = (data) => {
  if (data.startsWith('\x1b[<')) {
    return /^\x1b\[<\d+;\d+;\d+[mM]$/.test(data)
  }
  return data.startsWith('\x1b[M') && data.length === 6
}

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
  historyCache.delete(terminalId)
}

const createTerminalState = (terminalId) => {
  const status = writable('disconnected')
  const historyStatus = writable('idle')
  const bellCount = writable(0)
  const canReconnect = writable(false)
  const atBottom = writable(true)

  const term = new Terminal({
    allowProposedApi: true,
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
  let directInputEnabled = false
  let retryCount = 0
  let reconnectTimer
  let notifiedUnauthorized = false
  let notifiedDisconnect = false
  let disposeMouseHandlers
  let disposeTouchHandlers
  let touchTarget
  let historyLoaded = false
  let pendingHistory = ''
  let historyLoadPromise
  let historyWarningTimer
  let notifiedHistorySlow = false
  let notifiedHistoryError = false
  let resizeObserver

  const syncScrollState = () => {
    const buffer = term.buffer?.active
    if (!buffer) {
      atBottom.set(true)
      return
    }
    atBottom.set(buffer.viewportY >= buffer.baseY)
  }

  const getViewportElement = () => term.element?.querySelector('.xterm-viewport')

  const sendResize = () => {
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    const payload = {
      type: 'resize',
      cols: term.cols,
      rows: term.rows,
    }
    socket.send(JSON.stringify(payload))
  }

  const sendData = (data) => {
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    if (!data) return
    socket.send(encoder.encode(data))
  }

  const sendBell = async () => {
    try {
      await apiFetch(`/api/terminals/${terminalId}/bell`, {
        method: 'POST',
      })
    } catch (bellError) {
      console.warn('failed to report terminal bell', bellError)
    }
  }

  const sendCommand = (command) => {
    const payload = typeof command === 'string' ? command : ''
    if (payload) {
      sendData(payload)
    }
    sendData('\r')
  }

  const setDirectInput = (enabled) => {
    directInputEnabled = Boolean(enabled)
  }

  const scrollToBottom = () => {
    term.scrollToBottom()
    syncScrollState()
  }

  const focus = () => {
    if (disposed) return
    term.focus()
  }

  const scheduleFit = () => {
    if (!container || disposed) return
    requestAnimationFrame(() => {
      if (!container || disposed) return
      fitAddon.fit()
      sendResize()
    })
  }

  const setupTouchScroll = (element) => {
    if (!element) return () => {}
    let touchActive = false
    let lastTouchY = 0

    const getAverageTouchY = (touches) => {
      if (!touches || touches.length === 0) return null
      let total = 0
      for (const touch of touches) {
        total += touch.clientY
      }
      return total / touches.length
    }

    const handleTouchStart = (event) => {
      const averageY = getAverageTouchY(event.touches)
      if (averageY === null) return
      touchActive = true
      lastTouchY = averageY
    }

    const handleTouchMove = (event) => {
      if (!touchActive) return
      const averageY = getAverageTouchY(event.touches)
      if (averageY === null) return
      const deltaY = averageY - lastTouchY
      if (!deltaY) return
      lastTouchY = averageY
      const viewport = getViewportElement()
      if (!viewport) return
      viewport.scrollTop -= deltaY
      syncScrollState()
      event.preventDefault()
    }

    const handleTouchEnd = (event) => {
      if (event.touches && event.touches.length > 0) {
        const averageY = getAverageTouchY(event.touches)
        if (averageY !== null) {
          lastTouchY = averageY
        }
        return
      }
      touchActive = false
    }

    element.addEventListener('touchstart', handleTouchStart, {
      passive: true,
      capture: true,
    })
    element.addEventListener('touchmove', handleTouchMove, {
      passive: false,
      capture: true,
    })
    element.addEventListener('touchend', handleTouchEnd, {
      passive: true,
      capture: true,
    })
    element.addEventListener('touchcancel', handleTouchEnd, {
      passive: true,
      capture: true,
    })

    return () => {
      element.removeEventListener('touchstart', handleTouchStart, { capture: true })
      element.removeEventListener('touchmove', handleTouchMove, { capture: true })
      element.removeEventListener('touchend', handleTouchEnd, { capture: true })
      element.removeEventListener('touchcancel', handleTouchEnd, { capture: true })
    }
  }

  const attach = (element) => {
    container = element
    if (!container || disposed) return
    if (!term.element) {
      term.open(container)
    } else if (term.element.parentElement !== container) {
      container.appendChild(term.element)
    }
    const nextTouchTarget = term.element || container
    if (touchTarget !== nextTouchTarget) {
      if (disposeTouchHandlers) {
        disposeTouchHandlers()
      }
      touchTarget = nextTouchTarget
      disposeTouchHandlers = setupTouchScroll(nextTouchTarget)
    }
    flushPendingHistory()
    syncScrollState()
    scheduleFit()
    if (!resizeObserver && typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(() => {
        scheduleFit()
      })
    }
    if (resizeObserver) {
      resizeObserver.observe(container)
    }
  }

  const detach = () => {
    if (resizeObserver) {
      resizeObserver.disconnect()
    }
    container = null
  }

  term.onData((data) => {
    if (isMouseReport(data)) return
    sendData(data)
  })

  term.onBell(() => {
    bellCount.update((count) => count + 1)
    sendBell()
  })

  term.onScroll?.(() => {
    syncScrollState()
  })

  term.attachCustomKeyEventHandler((event) => {
    if (event.type !== 'keydown') return true
    if (isCopyKey(event)) {
      if (directInputEnabled && !term.hasSelection()) {
        return true
      }
      event.preventDefault()
      event.stopPropagation()
      if (!term.hasSelection()) {
        return false
      }
      const selection = term.getSelection()
      if (selection) {
        writeClipboardText(selection).catch(() => {})
      }
      return false
    }
    if (isPasteKey(event)) {
      if (directInputEnabled) {
        event.preventDefault()
        event.stopPropagation()
        readClipboardText()
          .then((text) => {
            if (!text) return
            sendData(text)
          })
          .catch(() => {})
        return false
      }
      event.preventDefault()
      event.stopPropagation()
      return false
    }
    if (directInputEnabled) {
      return true
    }
    event.preventDefault()
    event.stopPropagation()
    return false
  })

  if (term.parser?.registerCsiHandler) {
    const handler = (params) => shouldSuppressMouseMode(params)
    const handlerSet = [
      term.parser.registerCsiHandler({ prefix: '?', final: 'h' }, handler),
      term.parser.registerCsiHandler({ prefix: '?', final: 'l' }, handler),
    ]
    disposeMouseHandlers = () => {
      handlerSet.forEach((item) => item.dispose())
    }
  }

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

  const dispose = () => {
    if (disposed) return
    disposed = true
    clearReconnectTimer()
    canReconnect.set(false)
    if (socket) {
      socket.close()
    }
    if (disposeMouseHandlers) {
      disposeMouseHandlers()
    }
    if (disposeTouchHandlers) {
      disposeTouchHandlers()
    }
    term.dispose()
  }

  void connect()

  return {
    term,
    status,
    historyStatus,
    bellCount,
    canReconnect,
    atBottom,
    sendData,
    sendCommand,
    setDirectInput,
    scrollToBottom,
    focus,
    attach,
    detach,
    scheduleFit,
    reconnect,
    dispose,
  }
}
