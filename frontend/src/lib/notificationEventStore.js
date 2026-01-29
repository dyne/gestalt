import { createSseStore } from './sseStore.js'

const { subscribe, connectionStatus } = createSseStore({
  label: 'notifications',
  path: '/api/notifications/stream',
})

export { subscribe }

export const notificationEventConnectionStatus = connectionStatus
