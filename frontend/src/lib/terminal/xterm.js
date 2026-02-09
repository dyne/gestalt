import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'

const readCssVar = (name, fallback) => {
  if (typeof window === 'undefined') return fallback
  const value = window
    .getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim()
  return value || fallback
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
  const term = new Terminal({
    allowProposedApi: true,
    cursorBlink: true,
    fontSize: 14,
    fontFamily: '"IBM Plex Mono", "JetBrains Mono", monospace',
    theme: buildTerminalTheme(),
  })
  const fitAddon = new FitAddon()
  term.loadAddon(fitAddon)

  const { disposeThemeListener } = setupThemeSync(term)
  return { term, fitAddon, disposeThemeListener }
}
