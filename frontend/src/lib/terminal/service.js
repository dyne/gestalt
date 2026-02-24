import { createTerminalService as createCliService } from './service_cli.js'

export const createTerminalService = (options = {}) => {
  return createCliService(options)
}
