import { writable } from 'svelte/store'

export const createTerminalService = () => {
  const status = writable('disconnected')
  const historyStatus = writable('loaded')
  const bellCount = writable(0)
  const canReconnect = writable(false)
  const atBottom = writable(true)
  const segments = writable([])

  const noop = () => {}

  return {
    status,
    historyStatus,
    bellCount,
    canReconnect,
    atBottom,
    segments,
    sendData: noop,
    sendCommand: noop,
    setAtBottom: noop,
    appendPrompt: noop,
    setDirectInput: noop,
    scrollToBottom: noop,
    focus: noop,
    setScrollSensitivity: noop,
    attach: noop,
    detach: noop,
    scheduleFit: noop,
    setVisible: noop,
    reconnect: noop,
    dispose: noop,
  }
}
