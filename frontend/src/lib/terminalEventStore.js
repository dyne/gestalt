import { createWsStore } from './wsStore.js'

const { subscribe, connectionStatus } = createWsStore({
  label: 'terminal-events',
  path: '/api/terminals/events',
})

export { subscribe }

export const terminalEventConnectionStatus = connectionStatus
