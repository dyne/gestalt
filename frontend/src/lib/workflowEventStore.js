import { createWsStore } from './wsStore.js'

const { subscribe, connectionStatus } = createWsStore({
  label: 'workflow-events',
  path: '/api/workflows/events',
})

export { subscribe }

export const workflowEventConnectionStatus = connectionStatus
