import { formatTerminalLabel } from './terminalTabs.js'

const HOME_TABS = [
  { id: 'dashboard', label: 'Dashboard', isHome: true },
  { id: 'plan', label: 'Plans', isHome: true },
  { id: 'flow', label: 'Status', isHome: true },
]

const HOME_TAB_IDS = new Set(HOME_TABS.map((tab) => tab.id))

export const buildTabs = (terminalList = []) => [
  ...HOME_TABS,
  ...terminalList.map((terminal) => ({
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
