import { formatTerminalLabel } from './terminalTabs.js'

const HOME_TABS = [
  { id: 'dashboard', label: 'Dashboard', isHome: true },
  { id: 'agents', label: 'Agents', isHome: true },
  { id: 'plan', label: 'Plans', isHome: true },
  { id: 'flow', label: 'Flow', isHome: true },
]

const HOME_TAB_IDS = new Set(HOME_TABS.map((tab) => tab.id))

export const buildTabs = (terminalList = []) => [
  ...HOME_TABS,
  ...terminalList
    .filter((terminal) => {
      const runner = String(terminal?.runner || '').trim().toLowerCase()
      const sessionInterface = String(terminal?.interface || '').trim().toLowerCase()
      return !(runner === 'external' && sessionInterface === 'cli')
    })
    .map((terminal) => ({
      id: terminal.id,
      label: formatTerminalLabel(terminal),
      isHome: false,
    })),
]

export const resolveActiveView = (activeId) => {
  if (HOME_TAB_IDS.has(activeId)) {
    return activeId
  }
  return 'terminal'
}

export const ensureActiveTab = (activeId, tabs, fallbackId = 'dashboard') => {
  if (tabs.find((tab) => tab.id === activeId)) {
    return activeId
  }
  return fallbackId
}
