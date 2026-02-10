import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'

const readCssVar = (name, fallback) => {
  if (typeof window === 'undefined') return fallback
  const rootValue = window
    .getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim()
  if (rootValue) return rootValue
  const appElement = document.querySelector('.app')
  if (appElement) {
    const appValue = window
      .getComputedStyle(appElement)
      .getPropertyValue(name)
      .trim()
    if (appValue) return appValue
  }
  return fallback
}

const DEFAULT_FONT_FAMILY = '"IBM Plex Mono", "JetBrains Mono", monospace'
const DEFAULT_FONT_SIZE = 14

const readFontFamily = () => readCssVar('--terminal-font-family', DEFAULT_FONT_FAMILY)

const ensureMatchMedia = () => {
  if (typeof window === 'undefined' || window.matchMedia) return
  window.matchMedia = () => ({
    matches: false,
    media: '',
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })
}

const readRootFontSizePx = () => {
  if (typeof window === 'undefined') return DEFAULT_FONT_SIZE
  const value = window
    .getComputedStyle(document.documentElement)
    .getPropertyValue('font-size')
    .trim()
  const parsed = Number.parseFloat(value)
  if (Number.isFinite(parsed) && parsed > 0) {
    return parsed
  }
  return DEFAULT_FONT_SIZE
}

export const parseCssLengthPx = (value) => {
  if (value === undefined || value === null) return null
  const trimmed = String(value).trim()
  if (!trimmed) return null
  const parsed = Number.parseFloat(trimmed)
  if (!Number.isFinite(parsed) || parsed <= 0) return null
  if (trimmed.endsWith('rem') || trimmed.endsWith('em')) {
    return parsed * readRootFontSizePx()
  }
  return parsed
}

const readFontSize = () => {
  const value = readCssVar('--terminal-font-size', String(DEFAULT_FONT_SIZE))
  const parsed = parseCssLengthPx(value)
  if (parsed) return parsed
  return DEFAULT_FONT_SIZE
}

const buildTerminalTheme = () => ({
  background: readCssVar('--terminal-bg', '#11111b'),
  foreground: readCssVar('--terminal-text', '#cdd6f4'),
  cursor: readCssVar('--terminal-text', '#cdd6f4'),
  selectionBackground: readCssVar('--terminal-selection', 'rgba(205, 214, 244, 0.2)'),
})

const setupThemeSync = (term) => {
  const syncTheme = () => {
    term.options.theme = buildTerminalTheme()
    term.options.fontFamily = readFontFamily()
    term.options.fontSize = readFontSize()
  }
  let disposeThemeListener
  if (typeof window !== 'undefined' && window.matchMedia) {
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = () => syncTheme()
    if (media.addEventListener) {
      media.addEventListener('change', handler)
      disposeThemeListener = () => media.removeEventListener('change', handler)
    } else if (media.addListener) {
      media.addListener(handler)
      disposeThemeListener = () => media.removeListener(handler)
    }
  }
  return { syncTheme, disposeThemeListener }
}

export const createXtermTerminal = () => {
  ensureMatchMedia()
  const term = new Terminal({
    allowProposedApi: true,
    cursorBlink: true,
    fontSize: readFontSize(),
    fontFamily: readFontFamily(),
    theme: buildTerminalTheme(),
  })
  const fitAddon = new FitAddon()
  term.loadAddon(fitAddon)

  const { disposeThemeListener, syncTheme } = setupThemeSync(term)
  syncTheme()
  return { term, fitAddon, disposeThemeListener, syncTheme }
}
