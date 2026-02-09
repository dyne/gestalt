const DEFAULT_LINE_HEIGHT_PX = 20
const TOUCH_SCROLL_THRESHOLD_PX = 10
const INERTIA_MIN_VELOCITY_PX_PER_MS = 0.02
const INERTIA_MAX_VELOCITY_PX_PER_MS = 2
const INERTIA_DECAY = 0.98
const INERTIA_FRAME_MS = 16
const INERTIA_VELOCITY_SMOOTHING = 0.6
const INERTIA_VELOCITY_BOOST = 0.85
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

export const isCopyKey = (event) =>
  hasModifierKey(event) &&
  !event.altKey &&
  event.key.toLowerCase() === 'c'

export const isPasteKey = (event) =>
  hasModifierKey(event) &&
  !event.altKey &&
  event.key.toLowerCase() === 'v'

export const writeClipboardText = async (text) => {
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

export const readClipboardText = async () => {
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

export const shouldSuppressMouseMode = (params) => {
  const flattened = flattenParams(params)
  if (!flattened.length) return false
  const hasMouse = flattened.some((value) => MOUSE_MODE_PARAMS.has(value))
  if (!hasMouse) return false
  return flattened.every((value) => MOUSE_MODE_PARAMS.has(value))
}

export const isMouseReport = (data) => {
  if (data.startsWith('\x1b[<')) {
    return /^\x1b\[<\d+;\d+;\d+[mM]$/.test(data)
  }
  return data.startsWith('\x1b[M') && data.length === 6
}

export const setupPointerScroll = (options) => {
  const resolved =
    options && options.element
      ? options
      : {
          element: options,
        }
  const { element, term, syncScrollState, getScrollSensitivity } = resolved
  if (!element || !term || typeof window === 'undefined') return () => {}

  let activePointerId = null
  let startY = 0
  let lastY = 0
  let lastMoveTime = 0
  let isScrolling = false
  let velocityPxPerMs = 0
  let inertiaVelocityPxPerMs = 0
  let inertiaActive = false
  let inertiaFrameId = null
  let inertiaRemainderLines = 0

  const readScrollSensitivity =
    typeof getScrollSensitivity === 'function' ? getScrollSensitivity : () => 1

  const nowMs = () =>
    typeof performance !== 'undefined' && typeof performance.now === 'function'
      ? performance.now()
      : Date.now()

  const getPixelsPerLine = () => {
    if (!term.rows) return DEFAULT_LINE_HEIGHT_PX
    const rowsElement = element.querySelector('.xterm-rows')
    if (!rowsElement) return DEFAULT_LINE_HEIGHT_PX
    const height = rowsElement.getBoundingClientRect().height
    if (!height) return DEFAULT_LINE_HEIGHT_PX
    const pixelsPerLine = height / term.rows
    return pixelsPerLine || DEFAULT_LINE_HEIGHT_PX
  }

  const clampVelocity = (value) =>
    Math.max(-INERTIA_MAX_VELOCITY_PX_PER_MS, Math.min(INERTIA_MAX_VELOCITY_PX_PER_MS, value))

  const shouldAllowScrollbarDrag = (event) => {
    if (!(event.target instanceof Element)) return false
    const viewport = term.element?.querySelector('.xterm-viewport')
    if (!viewport || !viewport.contains(event.target)) return false
    const scrollbarWidth = viewport.offsetWidth - viewport.clientWidth
    if (scrollbarWidth <= 0) return false
    const rect = viewport.getBoundingClientRect()
    return event.clientX >= rect.right - scrollbarWidth
  }

  const stopInertia = () => {
    if (!inertiaActive) return
    inertiaActive = false
    if (inertiaFrameId !== null && typeof cancelAnimationFrame === 'function') {
      cancelAnimationFrame(inertiaFrameId)
    }
    inertiaFrameId = null
    inertiaVelocityPxPerMs = 0
    inertiaRemainderLines = 0
  }

  const startInertia = () => {
    if (!isScrolling) return
    const startVelocity = clampVelocity(velocityPxPerMs)
    if (Math.abs(startVelocity) < INERTIA_MIN_VELOCITY_PX_PER_MS) return
    inertiaVelocityPxPerMs = startVelocity
    inertiaActive = true
    let lastFrameTime = lastMoveTime > 0 ? lastMoveTime : nowMs()

    const step = (frameTime) => {
      if (!inertiaActive) return
      const currentTime = typeof frameTime === 'number' ? frameTime : nowMs()
      const deltaTime = Math.max(0, currentTime - lastFrameTime)
      lastFrameTime = currentTime
      if (deltaTime > 0) {
        const pixelsPerLine = getPixelsPerLine()
        const deltaLinesFloat =
          ((inertiaVelocityPxPerMs * deltaTime) / pixelsPerLine) * readScrollSensitivity() +
          inertiaRemainderLines
        const deltaLines = Math.trunc(deltaLinesFloat)
        inertiaRemainderLines = deltaLinesFloat - deltaLines
        if (deltaLines) {
          term.scrollLines(-deltaLines)
          syncScrollState?.()
        }
        const decay = Math.pow(INERTIA_DECAY, deltaTime / INERTIA_FRAME_MS)
        inertiaVelocityPxPerMs *= decay
      }
      if (Math.abs(inertiaVelocityPxPerMs) < INERTIA_MIN_VELOCITY_PX_PER_MS) {
        stopInertia()
        return
      }
      inertiaFrameId = requestAnimationFrame(step)
    }

    inertiaFrameId = requestAnimationFrame(step)
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
    if (shouldAllowScrollbarDrag(event)) return
    if (inertiaActive) {
      inertiaVelocityPxPerMs = clampVelocity(
        inertiaVelocityPxPerMs * INERTIA_VELOCITY_BOOST + velocityPxPerMs
      )
      stopInertia()
    }
    activePointerId = event.pointerId
    startY = event.clientY
    lastY = event.clientY
    lastMoveTime = typeof event.timeStamp === 'number' ? event.timeStamp : nowMs()
    isScrolling = false
    velocityPxPerMs = inertiaVelocityPxPerMs
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
    const currentTime = typeof event.timeStamp === 'number' ? event.timeStamp : nowMs()
    const totalDeltaY = currentY - startY
    if (!isScrolling && Math.abs(totalDeltaY) < TOUCH_SCROLL_THRESHOLD_PX) {
      return
    }
    let deltaTime = currentTime - lastMoveTime
    if (!isScrolling) {
      isScrolling = true
      deltaTime = INERTIA_FRAME_MS
    }
    const deltaY = currentY - lastY
    lastY = currentY
    const pixelsPerLine = getPixelsPerLine()
    const deltaLines = Math.round((deltaY / pixelsPerLine) * readScrollSensitivity())
    if (deltaLines) {
      term.scrollLines(-deltaLines)
      syncScrollState?.()
    }
    if (deltaTime > 0) {
      const nextVelocity = deltaY / deltaTime
      velocityPxPerMs =
        velocityPxPerMs * INERTIA_VELOCITY_SMOOTHING +
        nextVelocity * (1 - INERTIA_VELOCITY_SMOOTHING)
    }
    lastMoveTime = currentTime
    event.preventDefault()
    event.stopPropagation()
  }

  const handlePointerUp = (event) => {
    if (activePointerId === null || event.pointerId !== activePointerId) return
    if (event.pointerType === 'touch') {
      event.preventDefault()
      event.stopPropagation()
      startInertia()
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
    stopInertia()
    releasePointer()
    element.removeEventListener('pointerdown', handlePointerDown, { capture: true })
    element.removeEventListener('pointermove', handlePointerMove, { capture: true })
    element.removeEventListener('pointerup', handlePointerUp, { capture: true })
    element.removeEventListener('pointercancel', handlePointerUp, { capture: true })
  }
}
