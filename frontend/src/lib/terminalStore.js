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

const normalizeRunner = (value) => {
  if (value === undefined || value === null) return ''
  const trimmed = String(value).trim().toLowerCase()
  if (trimmed === 'external' || trimmed === 'server') {
    return trimmed
  }
  return ''
}

const normalizeOptions = (options) => ({
  allowMouseReporting: Boolean(options?.allowMouseReporting),
})

export const getTerminalState = (sessionId, sessionInterface, sessionRunner, options = {}) => {
  if (!sessionId) return null
  const interfaceValue = normalizeInterface(sessionInterface)
  const runnerValue = normalizeRunner(sessionRunner)
  const optionValue = normalizeOptions(options)
  const existing = terminals.get(sessionId)
  if (
    existing &&
    (existing.interface !== interfaceValue ||
      existing.runner !== runnerValue ||
      existing.allowMouseReporting !== optionValue.allowMouseReporting)
  ) {
    existing.state?.dispose?.()
    terminals.delete(sessionId)
  }
  if (!terminals.has(sessionId)) {
    const state = createTerminalService({
      terminalId: sessionId,
      historyCache,
      sessionInterface: interfaceValue,
      sessionRunner: runnerValue,
      ...optionValue,
    })
    terminals.set(sessionId, {
      state,
      interface: interfaceValue,
      runner: runnerValue,
      allowMouseReporting: optionValue.allowMouseReporting,
    })
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
