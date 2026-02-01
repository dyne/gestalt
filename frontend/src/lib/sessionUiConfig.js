import { get, writable } from 'svelte/store'

const DEFAULT_SCROLLBACK_LINES = 2000

const normalizeScrollbackLines = (value) => {
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return DEFAULT_SCROLLBACK_LINES
  }
  return Math.floor(parsed)
}

const normalizeText = (value) => {
  if (value === undefined || value === null) return ''
  const text = String(value).trim()
  return text
}

const configStore = writable({
  scrollbackLines: DEFAULT_SCROLLBACK_LINES,
  fontFamily: '',
  fontSize: '',
  inputFontFamily: '',
  inputFontSize: '',
})

export const sessionUiConfig = {
  subscribe: configStore.subscribe,
}

export const setSessionUiConfigFromStatus = (status) => {
  if (!status || typeof status !== 'object') return
  const next = {
    scrollbackLines: normalizeScrollbackLines(status.session_scrollback_lines),
    fontFamily: normalizeText(status.session_font_family),
    fontSize: normalizeText(status.session_font_size),
    inputFontFamily: normalizeText(status.session_input_font_family),
    inputFontSize: normalizeText(status.session_input_font_size),
  }
  configStore.set(next)
}

export const getSessionUiConfig = () => get(configStore)

export const buildTerminalStyle = (config) => {
  if (!config) return ''
  const parts = []
  if (config.fontFamily) {
    parts.push(`--terminal-font-family: ${config.fontFamily}`)
  }
  if (config.fontSize) {
    parts.push(`--terminal-font-size: ${config.fontSize}`)
  }
  if (config.inputFontFamily) {
    parts.push(`--terminal-input-font-family: ${config.inputFontFamily}`)
  }
  if (config.inputFontSize) {
    parts.push(`--terminal-input-font-size: ${config.inputFontSize}`)
  }
  return parts.join('; ')
}
