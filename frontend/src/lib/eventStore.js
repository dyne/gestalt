import { createWsStore } from './wsStore.js'

const { subscribe, connectionStatus } = createWsStore({
  label: 'events',
  path: '/ws/events',
  buildSubscribeMessage: (types) => ({ subscribe: types }),
})

export { subscribe }

export const eventConnectionStatus = connectionStatus
