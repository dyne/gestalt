import { createTerminalService } from './terminal/service.js'

const terminals = new Map()
const historyCache = new Map()

const normalizeInterface = (value) => {
  if (value === undefined || value === null) return ''
  const trimmed = String(value).trim().toLowerCase()
  if (trimmed === 'cli' || trimmed === 'mcp') {
    return trimmed
  }
  return ''
}

export const getTerminalState = (sessionId, sessionInterface) => {
  if (!sessionId) return null
  const interfaceValue = normalizeInterface(sessionInterface)
  const existing = terminals.get(sessionId)
  if (existing && existing.interface !== interfaceValue) {
    existing.state?.dispose?.()
    terminals.delete(sessionId)
  }
  if (!terminals.has(sessionId)) {
    const state = createTerminalService({
      terminalId: sessionId,
      historyCache,
      sessionInterface: interfaceValue,
    })
    terminals.set(sessionId, { state, interface: interfaceValue })
  }
  return terminals.get(sessionId)?.state || null
}

export const releaseTerminalState = (sessionId) => {
  const entry = terminals.get(sessionId)
  if (!entry) return
  entry.state?.dispose?.()
  terminals.delete(sessionId)
  historyCache.delete(sessionId)
}
