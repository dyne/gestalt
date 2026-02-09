import { createTerminalService as createCliService } from './service_cli.js'
import { createTerminalService as createMcpService } from './service_mcp.js'

const resolveInterface = (value) => {
  if (typeof value !== 'string') return ''
  return value.trim().toLowerCase()
}

export const createTerminalService = (options = {}) => {
  const interfaceValue = resolveInterface(options.sessionInterface)
  if (interfaceValue === 'cli') {
    return createCliService(options)
  }
  return createMcpService(options)
}
