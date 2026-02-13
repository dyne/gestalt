import { guiModuleRegistry } from './registry.js'

const defaultExternalModules = Object.freeze(['console', 'plan-progress'])

const normalizeModule = (entry) => {
  const trimmed = String(entry || '').trim().toLowerCase()
  if (!trimmed) return ''
  if (trimmed === 'terminal') return 'console'
  return trimmed
}

const normalizeModules = (modules) => {
  if (!Array.isArray(modules)) return []
  const seen = new Set()
  const normalized = []
  modules.forEach((entry) => {
    const trimmed = normalizeModule(entry)
    if (!trimmed || seen.has(trimmed) || !guiModuleRegistry[trimmed]) return
    seen.add(trimmed)
    normalized.push(trimmed)
  })
  return normalized
}

export const resolveGuiModules = (modules, runner) => {
  const resolved = normalizeModules(modules)
  if (resolved.length > 0) {
    return resolved
  }
  const runnerValue = String(runner || '').trim().toLowerCase()
  if (runnerValue === 'external') {
    return [...defaultExternalModules]
  }
  return ['console']
}
