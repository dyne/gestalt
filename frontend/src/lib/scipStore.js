import { get, writable } from 'svelte/store'
import { fetchScipStatus, triggerScipReindex } from './apiClient.js'
import { getErrorMessage } from './errorUtils.js'
import { createWsStore } from './wsStore.js'

const REINDEX_TIMEOUT_MS = 5 * 60 * 1000

export const initialScipStatus = {
  indexed: false,
  fresh: false,
  in_progress: false,
  started_at: '',
  completed_at: '',
  duration: '',
  error: '',
  created_at: '',
  documents: 0,
  symbols: 0,
  age_hours: 0,
  languages: [],
}

const normalizeStatus = (payload) => {
  const rawLanguages = Array.isArray(payload?.languages) ? payload.languages : []
  const languages = Array.from(
    new Set(rawLanguages.filter(Boolean).map((language) => String(language).toLowerCase()))
  ).sort()
  return {
    ...initialScipStatus,
    ...(payload || {}),
    languages,
  }
}

const mergeLanguages = (currentLanguages, language) => {
  const next = new Set(currentLanguages || [])
  if (language) {
    next.add(String(language).toLowerCase())
  }
  return Array.from(next).sort()
}

const applyEvent = (current, event) => {
  const timestamp = event?.timestamp || new Date().toISOString()
  const language = event?.language || ''
  const languages = mergeLanguages(current.languages, language)

  if (event?.type === 'start') {
    return {
      ...current,
      in_progress: true,
      started_at: timestamp,
      completed_at: '',
      duration: '',
      error: '',
      languages,
    }
  }
  if (event?.type === 'progress') {
    return {
      ...current,
      in_progress: true,
      started_at: current.started_at || timestamp,
      error: '',
      languages,
    }
  }
  if (event?.type === 'error') {
    return {
      ...current,
      in_progress: false,
      completed_at: timestamp,
      error: event?.message || 'SCIP indexing failed.',
      languages,
    }
  }
  if (event?.type === 'complete') {
    return {
      ...current,
      in_progress: false,
      completed_at: timestamp,
      duration: '',
      error: '',
      languages,
    }
  }
  return current
}

export const createScipStore = () => {
  const status = writable(initialScipStatus)
  const events = createWsStore({ label: 'scip-events', path: '/api/scip/events' })
  const unsubscribes = []
  let started = false
  let reindexTimeoutId = null
  let reindexInFlight = false

  const refreshStatus = async () => {
    try {
      const payload = await fetchScipStatus()
      status.set(normalizeStatus(payload))
      return payload
    } catch (err) {
      const message = getErrorMessage(err, 'Failed to load SCIP status.')
      status.update((current) => (current.error ? current : { ...current, error: message }))
      if (typeof console !== 'undefined') {
        console.error('[scip-store] status refresh failed', err)
      }
      return null
    }
  }

  const handleEvent = (event) => {
    if (reindexTimeoutId) {
      clearTimeout(reindexTimeoutId)
      reindexTimeoutId = null
    }

    try {
      status.update((current) => applyEvent(current, event))
    } catch (err) {
      if (typeof console !== 'undefined') {
        console.error('[scip-store] event handling error', err, event)
      }
    }

    if (event?.type === 'complete' || event?.type === 'error') {
      try {
        void refreshStatus()
      } catch (err) {
        if (typeof console !== 'undefined') {
          console.error('[scip-store] status refresh error', err)
        }
      }
    }
  }

  const start = async () => {
    if (started) return
    started = true
    unsubscribes.push(events.subscribe('start', handleEvent))
    unsubscribes.push(events.subscribe('progress', handleEvent))
    unsubscribes.push(events.subscribe('complete', handleEvent))
    unsubscribes.push(events.subscribe('error', handleEvent))
    await refreshStatus()
  }

  const stop = () => {
    started = false
    if (reindexTimeoutId) {
      clearTimeout(reindexTimeoutId)
      reindexTimeoutId = null
    }
    reindexInFlight = false
    while (unsubscribes.length > 0) {
      const unsubscribe = unsubscribes.pop()
      if (typeof unsubscribe === 'function') {
        unsubscribe()
      }
    }
  }

  /**
   * Trigger SCIP reindexing via API and await completion via WebSocket events.
   *
   * Flow:
   * 1. Check WebSocket connection to /api/scip/events is active (wait 2s if not)
   * 2. Call POST /api/scip/reindex to start background indexing
   * 3. Set local state to in_progress=true
   * 4. Wait for 'complete' or 'error' events via WebSocket
   * 5. Fallback: After 5min timeout, poll /api/scip/status for final state
   *
   * @throws {Error} If WebSocket disconnected or API call fails
   */
  const reindex = async () => {
    if (reindexInFlight) {
      if (typeof console !== 'undefined') {
        console.warn('[scip-store] reindex already in progress')
      }
      return
    }

    reindexInFlight = true
    try {
      const currentStatus = get(events.connectionStatus)
      if (currentStatus !== 'connected') {
        if (typeof console !== 'undefined') {
          console.warn('[scip-store] WS not connected, waiting...')
        }
        await new Promise((resolve) => setTimeout(resolve, 2000))
        const retryStatus = get(events.connectionStatus)
        if (retryStatus !== 'connected') {
          const err = new Error('SCIP events connection unavailable. Please refresh the page.')
          status.update((current) => ({
            ...current,
            error: 'Event stream disconnected. Refresh and retry.',
            in_progress: false,
          }))
          throw err
        }
      }

      if (reindexTimeoutId) {
        clearTimeout(reindexTimeoutId)
        reindexTimeoutId = null
      }

      status.update((current) => ({
        ...current,
        in_progress: true,
        started_at: new Date().toISOString(),
        completed_at: '',
        error: '',
      }))

      await triggerScipReindex()

      reindexTimeoutId = setTimeout(() => {
        reindexTimeoutId = null
        if (typeof console !== 'undefined') {
          console.warn('[scip-store] reindex timeout, forcing status refresh')
        }
        void refreshStatus()
      }, REINDEX_TIMEOUT_MS)
    } catch (err) {
      if (reindexTimeoutId) {
        clearTimeout(reindexTimeoutId)
        reindexTimeoutId = null
      }
      const message = getErrorMessage(err, 'Failed to start SCIP indexing.')
      status.update((current) => ({ ...current, in_progress: false, error: message }))
      throw err
    } finally {
      reindexInFlight = false
    }
  }

  return {
    status: {
      subscribe: status.subscribe,
    },
    refreshStatus,
    start,
    stop,
    reindex,
    connectionStatus: events.connectionStatus,
  }
}
