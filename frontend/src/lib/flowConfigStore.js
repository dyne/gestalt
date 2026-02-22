import { get, writable } from 'svelte/store'
import {
  fetchFlowActivities,
  fetchFlowConfig,
  fetchFlowEventTypes,
  saveFlowConfig,
} from './apiClient.js'
import { getErrorMessage } from './errorUtils.js'

const defaultConfig = {
  version: 1,
  triggers: [],
  bindings_by_trigger_id: {},
}

const buildState = () => ({
  activities: [],
  eventTypes: [],
  config: defaultConfig,
  storagePath: '',
  loading: false,
  error: '',
  saving: false,
  saveError: '',
  dirty: false,
  lastSavedAt: '',
})

const serializeConfig = (config) =>
  JSON.stringify({
    version: config?.version || 1,
    triggers: Array.isArray(config?.triggers) ? config.triggers : [],
    bindings_by_trigger_id:
      config?.bindings_by_trigger_id && typeof config.bindings_by_trigger_id === 'object'
        ? config.bindings_by_trigger_id
        : {},
  })

const normalizeBindings = (bindings) => {
  if (!bindings || typeof bindings !== 'object' || Array.isArray(bindings)) return {}
  const normalized = {}
  Object.entries(bindings).forEach(([triggerId, list]) => {
    const items = Array.isArray(list) ? list : []
    normalized[triggerId] = items
      .map((binding) => ({
        activity_id: binding?.activity_id ? String(binding.activity_id) : '',
        config:
          binding?.config && typeof binding.config === 'object' && !Array.isArray(binding.config)
            ? binding.config
            : {},
      }))
      .filter((binding) => binding.activity_id)
  })
  return normalized
}

const normalizeConfig = (config) => ({
  version: Number.isFinite(Number(config?.version)) ? Number(config.version) : 1,
  triggers: Array.isArray(config?.triggers) ? config.triggers : [],
  bindings_by_trigger_id: normalizeBindings(config?.bindings_by_trigger_id),
})

export const createFlowConfigStore = () => {
  const state = writable(buildState())
  let baseline = serializeConfig(defaultConfig)
  let loadPromise = null

  const setConfig = (config, { markClean = false } = {}) => {
    const nextConfig = normalizeConfig(config)
    if (markClean) {
      baseline = serializeConfig(nextConfig)
    }
    state.update((current) => ({
      ...current,
      config: nextConfig,
      dirty: serializeConfig(nextConfig) !== baseline,
    }))
  }

  const updateConfig = (updater) => {
    if (!updater) return
    state.update((current) => {
      const nextConfig = normalizeConfig(
        typeof updater === 'function' ? updater(current.config) : updater,
      )
      return {
        ...current,
        config: nextConfig,
        dirty: serializeConfig(nextConfig) !== baseline,
      }
    })
  }

  const load = async () => {
    if (loadPromise) return loadPromise
    state.update((current) => ({
      ...current,
      loading: true,
      error: '',
      saveError: '',
    }))
    loadPromise = Promise.all([fetchFlowActivities(), fetchFlowConfig(), fetchFlowEventTypes()])
      .then(([activities, payload, eventTypesPayload]) => {
        const nextConfig = normalizeConfig(payload?.config || {})
        baseline = serializeConfig(nextConfig)
        state.update((current) => ({
          ...current,
          activities: Array.isArray(activities) ? activities : [],
          eventTypes: Array.isArray(eventTypesPayload?.eventTypes)
            ? eventTypesPayload.eventTypes
            : [],
          config: nextConfig,
          storagePath: payload?.storagePath || '',
          loading: false,
          dirty: false,
        }))
      })
      .catch((err) => {
        state.update((current) => ({
          ...current,
          loading: false,
          error: getErrorMessage(err, 'Failed to load Flow configuration.'),
        }))
      })
      .finally(() => {
        loadPromise = null
      })
    return loadPromise
  }

  const save = async () => {
    const snapshot = get(state)
    if (snapshot.saving) return
    state.update((current) => ({
      ...current,
      saving: true,
      saveError: '',
    }))
    try {
      const payload = await saveFlowConfig(snapshot.config)
      const nextConfig = normalizeConfig(payload?.config || {})
      baseline = serializeConfig(nextConfig)
      state.update((current) => ({
        ...current,
        config: nextConfig,
        storagePath: payload?.storagePath || current.storagePath,
        saving: false,
        dirty: false,
        lastSavedAt: new Date().toISOString(),
      }))
    } catch (err) {
      state.update((current) => ({
        ...current,
        saving: false,
        saveError: getErrorMessage(err, 'Failed to save Flow configuration.'),
      }))
    }
  }

  return {
    subscribe: state.subscribe,
    load,
    save,
    setConfig,
    updateConfig,
  }
}

export const flowConfigStore = createFlowConfigStore()
