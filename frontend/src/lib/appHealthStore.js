import { get, writable } from 'svelte/store'
import { logUI } from './clientLog.js'

const SESSION_ID_KEY = 'gestalt.ui.session'
const CRASH_HISTORY_KEY = 'gestalt.ui.crash_history'
const CRASH_WINDOW_MS = 30000
const MAX_CRASHES = 3
const RELOAD_DELAY_MS = 1500

const safeSessionGet = (key) => {
  if (typeof window === 'undefined') return ''
  try {
    return window.sessionStorage.getItem(key) || ''
  } catch {
    return ''
  }
}

const safeSessionSet = (key, value) => {
  if (typeof window === 'undefined') return
  try {
    window.sessionStorage.setItem(key, value)
  } catch {
    // Ignore session storage failures.
  }
}

const createSessionId = () => {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`
}

const ensureSessionId = () => {
  const existing = safeSessionGet(SESSION_ID_KEY)
  if (existing) return existing
  const next = createSessionId()
  safeSessionSet(SESSION_ID_KEY, next)
  return next
}

const readCrashHistory = () => {
  const raw = safeSessionGet(CRASH_HISTORY_KEY)
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((value) => Number.isFinite(value))
  } catch {
    return []
  }
}

const writeCrashHistory = (history) => {
  safeSessionSet(CRASH_HISTORY_KEY, JSON.stringify(history))
}

const clearCrashHistory = () => {
  safeSessionSet(CRASH_HISTORY_KEY, '[]')
}

const normalizeError = (error) => {
  if (!error) {
    return { message: 'Unknown error', stack: '' }
  }
  if (error instanceof Error) {
    return { message: error.message || 'Unknown error', stack: error.stack || '' }
  }
  if (typeof error === 'string') {
    return { message: error, stack: '' }
  }
  if (typeof error === 'object') {
    const message = typeof error.message === 'string' ? error.message : 'Unknown error'
    const stack = typeof error.stack === 'string' ? error.stack : ''
    return { message, stack }
  }
  return { message: String(error), stack: '' }
}

const state = writable({
  sessionId: ensureSessionId(),
  activeTabId: 'dashboard',
  activeView: 'dashboard',
  lastRefresh: {},
  crash: null,
  crashLoop: false,
  reloadScheduled: false,
})

let reloadTimer = null

const scheduleReload = () => {
  if (typeof window === 'undefined') return
  if (reloadTimer) return
  reloadTimer = setTimeout(() => {
    reloadTimer = null
    window.location.reload()
  }, RELOAD_DELAY_MS)
}

const updateIfChanged = (key, value) => {
  state.update((current) => {
    if (current[key] === value) return current
    return { ...current, [key]: value }
  })
}

const registerCrash = () => {
  const now = Date.now()
  const recent = readCrashHistory().filter((timestamp) => now - timestamp <= CRASH_WINDOW_MS)
  recent.push(now)
  writeCrashHistory(recent)
  return recent.length >= MAX_CRASHES
}

export const appHealthStore = {
  subscribe: state.subscribe,
}

export const setActiveTabId = (tabId) => {
  if (!tabId) return
  updateIfChanged('activeTabId', tabId)
}

export const setActiveView = (view) => {
  if (!view) return
  updateIfChanged('activeView', view)
}

export const recordRefresh = (label) => {
  if (!label) return
  const timestamp = new Date().toISOString()
  state.update((current) => {
    if (current.lastRefresh[label] === timestamp) return current
    return {
      ...current,
      lastRefresh: {
        ...current.lastRefresh,
        [label]: timestamp,
      },
    }
  })
}

export const reportCrash = (error, { source = 'unknown', view } = {}) => {
  const snapshot = get(state)
  if (snapshot.crash) return snapshot.crash
  const { message, stack } = normalizeError(error)
  const crashId = createSessionId()
  const timestamp = new Date().toISOString()
  const crashLoop = registerCrash()
  const url = typeof window !== 'undefined' ? window.location.href : ''
  const activeView = view || snapshot.activeView || 'unknown'
  const activeTab = snapshot.activeTabId || ''

  state.update((current) => ({
    ...current,
    crash: {
      id: crashId,
      message,
      stack,
      timestamp,
      url,
      view: activeView,
      tabId: activeTab,
      source,
    },
    crashLoop,
    reloadScheduled: current.reloadScheduled || !crashLoop,
  }))

  logUI({
    level: 'error',
    body: message,
    attributes: {
      'gestalt.category': 'ui-crash',
      'gestalt.crash_id': crashId,
      'gestalt.session_id': snapshot.sessionId,
      'gestalt.view': activeView,
      'gestalt.tab_id': activeTab,
      'gestalt.source': source,
      'gestalt.url': url,
      'gestalt.stack': stack,
      'gestalt.last_refresh': JSON.stringify(snapshot.lastRefresh || {}),
    },
  })

  if (!crashLoop) {
    scheduleReload()
  }

  return { id: crashId }
}

export const installGlobalCrashHandlers = () => {
  if (typeof window === 'undefined') {
    return () => {}
  }

  const onError = (event) => {
    const error = event?.error || event?.message || event
    reportCrash(error, { source: 'window.error' })
  }

  const onRejection = (event) => {
    const error = event?.reason || event
    reportCrash(error, { source: 'window.unhandledrejection' })
  }

  window.addEventListener('error', onError)
  window.addEventListener('unhandledrejection', onRejection)

  return () => {
    window.removeEventListener('error', onError)
    window.removeEventListener('unhandledrejection', onRejection)
  }
}

export const forceReload = () => {
  clearCrashHistory()
  if (typeof window !== 'undefined') {
    window.location.reload()
  }
}
