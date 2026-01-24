import { createTerminalService } from './terminal/service.js'

const terminals = new Map()
const historyCache = new Map()

export const getTerminalState = (terminalId) => {
  if (!terminalId) return null
  if (!terminals.has(terminalId)) {
    terminals.set(terminalId, createTerminalService({ terminalId, historyCache }))
  }
  return terminals.get(terminalId)
}

export const releaseTerminalState = (terminalId) => {
  const state = terminals.get(terminalId)
  if (!state) return
  state.dispose()
  terminals.delete(terminalId)
  historyCache.delete(terminalId)
}
