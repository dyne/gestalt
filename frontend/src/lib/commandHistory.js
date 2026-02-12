import { apiFetch, buildApiPath } from './api.js'

const DEFAULT_HISTORY_LIMIT = 1000
const DEFAULT_LOAD_LIMIT = 100

const normalizeHistory = (payload) =>
  Array.isArray(payload)
    ? payload
        .map((entry) => entry?.command)
        .filter((command) => typeof command === 'string' && command !== '')
    : []

export const createCommandHistory = ({
  historyLimit = DEFAULT_HISTORY_LIMIT,
  loadLimit = DEFAULT_LOAD_LIMIT,
} = {}) => {
  let history = []
  let historyIndex = -1
  let draft = ''
  let loadedFor = ''

  const resetNavigation = () => {
    historyIndex = -1
    draft = ''
  }

  const record = (command) => {
    if (!command) return
    history = [...history, command]
    if (history.length > historyLimit) {
      history = history.slice(history.length - historyLimit)
    }
  }

  const move = (direction, currentValue) => {
    if (!history.length) return null
    if (direction < 0) {
      if (historyIndex === -1) {
        draft = currentValue
        historyIndex = history.length - 1
      } else if (historyIndex > 0) {
        historyIndex -= 1
      }
    } else if (direction > 0) {
      if (historyIndex === -1) return null
      if (historyIndex < history.length - 1) {
        historyIndex += 1
      } else {
        historyIndex = -1
      }
    } else {
      return null
    }
    return historyIndex === -1 ? draft : history[historyIndex] || ''
  }

  const load = async (sessionId) => {
    if (!sessionId || sessionId === loadedFor) return
    loadedFor = sessionId
    try {
      const response = await apiFetch(
        `${buildApiPath('/api/sessions', sessionId, 'input-history')}?limit=${loadLimit}`
      )
      const payload = await response.json()
      history = normalizeHistory(payload)
      resetNavigation()
    } catch (err) {
      console.warn('failed to load input history', err)
    }
  }

  return {
    record,
    move,
    resetNavigation,
    load,
  }
}
