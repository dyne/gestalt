import { describe, expect, it } from 'vitest'

import { resolveGuiModules } from '../src/lib/guiModules/resolve.js'

describe('guiModules', () => {
  it('normalizes explicit module lists', () => {
    const result = resolveGuiModules([' Terminal ', 'console', 'unknown', 'terminal', null])

    expect(result).toEqual(['terminal', 'console'])
  })

  it('defaults to terminal for server sessions', () => {
    const result = resolveGuiModules([], 'server')

    expect(result).toEqual(['terminal'])
  })

  it('defaults to console + plan-progress for external sessions', () => {
    const result = resolveGuiModules([], 'external')

    expect(result).toEqual(['console', 'plan-progress'])
  })

  it('treats missing runner as server', () => {
    const result = resolveGuiModules([], '')

    expect(result).toEqual(['terminal'])
  })
})
