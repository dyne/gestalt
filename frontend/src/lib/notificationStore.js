import { writable } from 'svelte/store'
import { apiFetch } from './api.js'

const defaultConfig = {
  info: { autoClose: true, duration: 5000 },
  warning: { autoClose: true, duration: 7000 },
  error: { autoClose: false, duration: 0 },
}

const defaultPreferences = {
  enabled: true,
  durationMs: 0,
  levelFilter: 'all',
}

const loadPreferences = () => {
  if (typeof window === 'undefined') {
    return { ...defaultPreferences }
  }
  try {
    const raw = window.localStorage.getItem('gestalt_notifications')
    if (!raw) return { ...defaultPreferences }
    const parsed = JSON.parse(raw)
    return { ...defaultPreferences, ...parsed }
  } catch {
    return { ...defaultPreferences }
  }
}

const savePreferences = (prefs) => {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem('gestalt_notifications', JSON.stringify(prefs))
  } catch {
    // Ignore storage failures.
  }
}

const preferencesStore = writable(loadPreferences())
let currentPreferences = { ...defaultPreferences }
preferencesStore.subscribe((value) => {
  currentPreferences = { ...defaultPreferences, ...value }
  savePreferences(currentPreferences)
})

const { subscribe, update, set } = writable([])
const timers = new Map()
let counter = 0

const nextId = () => {
  counter += 1
  return `${Date.now()}-${counter}`
}

const normalizeLevel = (level) => {
  const raw = String(level || '').toLowerCase()
  return defaultConfig[raw] ? raw : 'info'
}

const levelRank = (level) => {
  switch (level) {
    case 'info':
      return 1
    case 'warning':
      return 2
    case 'error':
      return 3
    default:
      return 1
  }
}

const shouldDisplay = (level) => {
  if (!currentPreferences.enabled) return false
  const filter = currentPreferences.levelFilter
  if (!filter || filter === 'all') return true
  return levelRank(level) >= levelRank(filter)
}

const resolveOptions = (level, options = {}) => {
  const defaults = defaultConfig[level] || defaultConfig.info
  const durationOverride = currentPreferences.durationMs
  const duration =
    options.duration ?? (durationOverride > 0 ? durationOverride : defaults.duration)
  return {
    autoClose: options.autoClose ?? defaults.autoClose,
    duration,
  }
}

const normalizeContext = (context) => {
  if (!context || typeof context !== 'object') {
    return {}
  }
  const normalized = {}
  for (const [key, value] of Object.entries(context)) {
    if (!key) continue
    if (value === undefined || value === null) continue
    normalized[key] = String(value)
  }
  return normalized
}

const logToast = (level, message, context) => {
  if (typeof window === 'undefined') {
    return
  }
  const payload = {
    level,
    message,
    context,
  }
  void apiFetch('/api/logs', {
    method: 'POST',
    body: JSON.stringify(payload),
  }).catch(() => {
    // Ignore log transport errors to avoid loops.
  })
}

const addNotification = (level, message, options = {}) => {
  const normalized = normalizeLevel(level)
  const text = String(message ?? '')
  const id = nextId()
  const context = {
    toast_id: id,
    ...normalizeContext(options.context),
  }
  if (text.trim()) {
    logToast(normalized, text, context)
  }
  if (!shouldDisplay(normalized)) {
    return null
  }
  const { autoClose, duration } = resolveOptions(normalized, options)
  const notification = {
    id,
    level: normalized,
    message: text,
    timestamp: new Date().toISOString(),
    autoClose,
    duration,
  }

  update((items) => [...items, notification])

  if (autoClose && duration > 0) {
    const timeout = setTimeout(() => {
      dismiss(id)
    }, duration)
    timers.set(id, timeout)
  }

  return id
}

const dismiss = (id) => {
  const timer = timers.get(id)
  if (timer) {
    clearTimeout(timer)
    timers.delete(id)
  }
  update((items) => items.filter((item) => item.id !== id))
}

const clear = () => {
  for (const timer of timers.values()) {
    clearTimeout(timer)
  }
  timers.clear()
  set([])
}

export const notificationStore = {
  subscribe,
  addNotification,
  dismiss,
  clear,
}

export const notificationPreferences = {
  subscribe: preferencesStore.subscribe,
  set: preferencesStore.set,
  update: preferencesStore.update,
}
