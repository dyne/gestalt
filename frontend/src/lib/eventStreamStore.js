import { createSseStore } from './sseStore.js'

const { subscribe, connectionStatus } = createSseStore({
  label: 'events',
  path: '/api/events/stream',
  buildQueryParams: (types) => (types.length ? { types: types.join(',') } : {}),
})

export { subscribe }

export const eventStreamConnectionStatus = connectionStatus
