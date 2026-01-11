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
const DEFAULT_LINE_HEIGHT_PX = 20
const TOUCH_SCROLL_THRESHOLD_PX = 10
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
  let scrollSensitivity = 1
  let notifiedUnauthorized = false
  let notifiedDisconnect = false
  let disposeMouseHandlers
  let disposePointerHandlers
  let pointerTarget
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

  const setScrollSensitivity = (value) => {
    const next = Number(value)
    if (!Number.isFinite(next) || next <= 0) return
    scrollSensitivity = next
  }

  const setupPointerScroll = (element) => {
    if (!element) return () => {}
    let activePointerId = null
    let startY = 0
    let lastY = 0
    let isScrolling = false

    const getPixelsPerLine = () => {
      if (!term.rows) return DEFAULT_LINE_HEIGHT_PX
      const rowsElement = element.querySelector('.xterm-rows')
      if (!rowsElement) return DEFAULT_LINE_HEIGHT_PX
      const height = rowsElement.getBoundingClientRect().height
      if (!height) return DEFAULT_LINE_HEIGHT_PX
      const pixelsPerLine = height / term.rows
      return pixelsPerLine || DEFAULT_LINE_HEIGHT_PX
    }

    const releasePointer = () => {
      if (activePointerId === null) return
      if (element.releasePointerCapture) {
        try {
          element.releasePointerCapture(activePointerId)
        } catch (err) {
          // Ignore capture release errors.
        }
      }
      activePointerId = null
      isScrolling = false
    }

    const handlePointerDown = (event) => {
      if (event.pointerType !== 'touch') return
      if (activePointerId !== null) return
      activePointerId = event.pointerId
      startY = event.clientY
      lastY = event.clientY
      isScrolling = false
      event.preventDefault()
      event.stopPropagation()
      if (element.setPointerCapture) {
        try {
          element.setPointerCapture(event.pointerId)
        } catch (err) {
          // Ignore capture errors.
        }
      }
    }

    const handlePointerMove = (event) => {
      if (activePointerId === null || event.pointerId !== activePointerId) return
      if (event.pointerType !== 'touch') return
      const currentY = event.clientY
      const totalDeltaY = currentY - startY
      if (!isScrolling && Math.abs(totalDeltaY) < TOUCH_SCROLL_THRESHOLD_PX) {
        return
      }
      if (!isScrolling) {
        isScrolling = true
      }
      const deltaY = currentY - lastY
      lastY = currentY
      const pixelsPerLine = getPixelsPerLine()
      const deltaLines = Math.round((deltaY / pixelsPerLine) * scrollSensitivity)
      if (deltaLines) {
        term.scrollLines(-deltaLines)
        syncScrollState()
      }
      event.preventDefault()
      event.stopPropagation()
    }

    const handlePointerUp = (event) => {
      if (activePointerId === null || event.pointerId !== activePointerId) return
      if (event.pointerType === 'touch') {
        event.preventDefault()
        event.stopPropagation()
      }
      releasePointer()
    }

    element.addEventListener('pointerdown', handlePointerDown, {
      passive: false,
      capture: true,
    })
    element.addEventListener('pointermove', handlePointerMove, {
      passive: false,
      capture: true,
    })
    element.addEventListener('pointerup', handlePointerUp, {
      passive: false,
      capture: true,
    })
    element.addEventListener('pointercancel', handlePointerUp, {
      passive: false,
      capture: true,
    })

    return () => {
      releasePointer()
      element.removeEventListener('pointerdown', handlePointerDown, { capture: true })
      element.removeEventListener('pointermove', handlePointerMove, { capture: true })
      element.removeEventListener('pointerup', handlePointerUp, { capture: true })
      element.removeEventListener('pointercancel', handlePointerUp, { capture: true })
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
    const nextPointerTarget = term.element || container
    if (pointerTarget !== nextPointerTarget) {
      if (disposePointerHandlers) {
        disposePointerHandlers()
      }
      pointerTarget = nextPointerTarget
      disposePointerHandlers = setupPointerScroll(nextPointerTarget)
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
    if (disposePointerHandlers) {
      disposePointerHandlers()
      disposePointerHandlers = null
      pointerTarget = null
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
    if (disposePointerHandlers) {
      disposePointerHandlers()
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
    setScrollSensitivity,
    attach,
    detach,
    scheduleFit,
    reconnect,
    dispose,
  }
}
