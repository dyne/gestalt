import { writable } from 'svelte/store'
import { triggerScipReindex } from './apiClient.js'
import { getErrorMessage } from './errorUtils.js'

export const initialScipStatus = {
  indexed: false,
  fresh: false,
  in_progress: false,
  started_at: '',
  completed_at: '',
  requested_at: '',
  duration: '',
  error: '',
  created_at: '',
  documents: 0,
  symbols: 0,
  age_hours: 0,
  languages: [],
}

export const createScipStore = () => {
  const status = writable(initialScipStatus)
  let started = false
  let reindexInFlight = false

  const start = async () => {
    if (started) return
    started = true
  }

  const stop = () => {
    started = false
    reindexInFlight = false
  }

  const reindex = async () => {
    if (reindexInFlight) {
      if (typeof console !== 'undefined') {
        console.warn('[scip-store] reindex already in progress')
      }
      return
    }

    reindexInFlight = true
    const requestedAt = new Date().toISOString()

    status.update((current) => ({
      ...current,
      in_progress: true,
      started_at: requestedAt,
      error: '',
    }))

    try {
      await triggerScipReindex()
      status.update((current) => ({
        ...current,
        in_progress: false,
        requested_at: requestedAt,
        error: '',
      }))
    } catch (err) {
      const message = getErrorMessage(err, 'Failed to start SCIP indexing.')
      status.update((current) => ({
        ...current,
        in_progress: false,
        error: message,
      }))
      throw err
    } finally {
      reindexInFlight = false
    }
  }

  return {
    status: {
      subscribe: status.subscribe,
    },
    start,
    stop,
    reindex,
  }
}
