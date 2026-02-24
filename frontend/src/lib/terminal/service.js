import { createTerminalService as createCliService } from './service_cli.js'
import { createTerminalService as createExternalService } from './service_external.js'

const resolveRunner = (value) => {
  if (typeof value !== 'string') return ''
  return value.trim().toLowerCase()
}

export const createTerminalService = (options = {}) => {
  const runnerValue = resolveRunner(options.sessionRunner)
  if (runnerValue === 'external') {
    return createExternalService(options)
  }
  return createCliService(options)
}
