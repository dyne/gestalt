import { writable } from 'svelte/store'
import { fetchScipStatus, triggerScipReindex } from './apiClient.js'
import { getErrorMessage } from './errorUtils.js'
import { createWsStore } from './wsStore.js'

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
    status.update((current) => applyEvent(current, event))
    if (event?.type === 'complete' || event?.type === 'error') {
      void refreshStatus()
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
    while (unsubscribes.length > 0) {
      const unsubscribe = unsubscribes.pop()
      if (typeof unsubscribe === 'function') {
        unsubscribe()
      }
    }
  }

  const reindex = async () => {
    status.update((current) => ({
      ...current,
      in_progress: true,
      started_at: new Date().toISOString(),
      completed_at: '',
      error: '',
    }))
    try {
      await triggerScipReindex()
    } catch (err) {
      const message = getErrorMessage(err, 'Failed to start SCIP indexing.')
      status.update((current) => ({ ...current, in_progress: false, error: message }))
      throw err
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
