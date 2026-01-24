import { writable } from 'svelte/store'

export const createViewStateMachine = () => {
  const state = writable({
    loading: false,
    refreshing: false,
    error: '',
  })

  const start = ({ silent = false } = {}) => {
    state.update((current) => ({
      ...current,
      loading: !silent,
      refreshing: silent,
      error: '',
    }))
  }

  const finish = () => {
    state.update((current) => ({
      ...current,
      loading: false,
      refreshing: false,
    }))
  }

  const setError = (message) => {
    state.update((current) => ({
      ...current,
      error: message,
    }))
  }

  const clearError = () => {
    state.update((current) => ({
      ...current,
      error: '',
    }))
  }

  return {
    subscribe: state.subscribe,
    start,
    finish,
    setError,
    clearError,
  }
}
