import { createWsStore } from './wsStore.js'

const { subscribe, connectionStatus } = createWsStore({
  label: 'config-events',
  path: '/api/config/events',
})

export { subscribe }

export const configEventConnectionStatus = connectionStatus
