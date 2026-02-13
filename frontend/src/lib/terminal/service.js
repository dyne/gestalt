import { createTerminalService as createCliService } from './service_cli.js'
import { createTerminalService as createExternalService } from './service_external.js'
import { createTerminalService as createMcpService } from './service_mcp.js'

const resolveInterface = (value) => {
  if (typeof value !== 'string') return ''
  return value.trim().toLowerCase()
}

export const createTerminalService = (options = {}) => {
  const runnerValue = resolveInterface(options.sessionRunner)
  if (runnerValue === 'external') {
    return createExternalService(options)
  }
  const interfaceValue = resolveInterface(options.sessionInterface)
  if (interfaceValue === 'cli') {
    return createCliService(options)
  }
  return createMcpService(options)
}
