import { writable } from 'svelte/store'

import { apiFetch, buildApiPath } from '../api.js'
import { createXtermTerminal } from './xterm.js'
import {
  isCopyKey,
  isPasteKey,
  isMouseReport,
  readClipboardText,
  setupPointerScroll,
  shouldSuppressMouseMode,
  writeClipboardText,
} from './input.js'
import { createTerminalSocket } from './socket.js'
import {
  appendOutputSegment,
  appendPromptSegment,
  historyToSegments,
} from './segments.js'

export const createTerminalService = ({ terminalId, historyCache }) => {
  const status = writable('disconnected')
  const historyStatus = writable('idle')
  const bellCount = writable(0)
  const canReconnect = writable(false)
  const atBottom = writable(true)
  const segments = writable([])

  const { term, fitAddon, disposeThemeListener } = createXtermTerminal()

  const encoder = new TextEncoder()
  const cache = historyCache || new Map()
  let socketManager
  let container
  let disposed = false
  let directInputEnabled = false
  let scrollSensitivity = 1
  let disposeMouseHandlers
  let disposePointerHandlers
  let pointerTarget
  let resizeObserver
  let isVisible = false

  const syncScrollState = () => {
    const buffer = term.buffer?.active
    if (!buffer) {
      atBottom.set(true)
      return
    }
    atBottom.set(buffer.viewportY >= buffer.baseY)
  }

  const sendResize = () => {
    if (!socketManager) return
    const payload = {
      type: 'resize',
      cols: term.cols,
      rows: term.rows,
    }
    socketManager.send(JSON.stringify(payload))
  }

  const sendData = (data) => {
    if (!socketManager) return
    if (!data) return
    socketManager.send(encoder.encode(data))
  }

  const sendBell = async () => {
    try {
      await apiFetch(buildApiPath('/api/sessions', terminalId, 'bell'), {
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

  const setAtBottom = (value) => {
    atBottom.set(Boolean(value))
  }

  const handleOutput = (chunk) => {
    segments.update((current) => appendOutputSegment(current, chunk))
  }

  const handleHistory = (lines) => {
    segments.set(historyToSegments(lines))
  }

  const appendPrompt = (prompt) => {
    segments.update((current) => appendPromptSegment(current, prompt))
  }

  socketManager = createTerminalSocket({
    terminalId,
    term,
    status,
    historyStatus,
    canReconnect,
    historyCache: cache,
    onOutput: handleOutput,
    onHistory: handleHistory,
    scheduleFit,
  })

  const { connect, disconnect, reconnect, dispose: disposeSocket, flushPendingHistory } =
    socketManager

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
      disposePointerHandlers = setupPointerScroll({
        element: nextPointerTarget,
        term,
        syncScrollState,
        getScrollSensitivity: () => scrollSensitivity,
      })
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

  const dispose = () => {
    if (disposed) return
    disposed = true
    if (disposeSocket) {
      disposeSocket()
    }
    canReconnect.set(false)
    if (disposeMouseHandlers) {
      disposeMouseHandlers()
    }
    if (disposePointerHandlers) {
      disposePointerHandlers()
    }
    if (disposeThemeListener) {
      disposeThemeListener()
    }
    term.dispose()
  }

  return {
    term,
    status,
    historyStatus,
    bellCount,
    canReconnect,
    atBottom,
    segments,
    sendData,
    sendCommand,
    setDirectInput,
    setAtBottom,
    appendPrompt,
    scrollToBottom,
    focus,
    setScrollSensitivity,
    attach,
    detach,
    scheduleFit,
    setVisible,
    reconnect,
    dispose,
  }
}
