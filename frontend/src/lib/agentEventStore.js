import { createWsStore } from './wsStore.js'

const { subscribe, connectionStatus } = createWsStore({
  label: 'agent-events',
  path: '/api/agents/events',
})

export { subscribe }

export const agentEventConnectionStatus = connectionStatus
