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

export const getTerminalState = (terminalId, sessionInterface) => {
  if (!terminalId) return null
  const interfaceValue = normalizeInterface(sessionInterface)
  const existing = terminals.get(terminalId)
  if (existing && existing.interface !== interfaceValue) {
    existing.state?.dispose?.()
    terminals.delete(terminalId)
  }
  if (!terminals.has(terminalId)) {
    const state = createTerminalService({
      terminalId,
      historyCache,
      sessionInterface: interfaceValue,
    })
    terminals.set(terminalId, { state, interface: interfaceValue })
  }
  return terminals.get(terminalId)?.state || null
}

export const releaseTerminalState = (terminalId) => {
  const entry = terminals.get(terminalId)
  if (!entry) return
  entry.state?.dispose?.()
  terminals.delete(terminalId)
  historyCache.delete(terminalId)
}
