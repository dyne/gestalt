import { describe, it, expect } from 'vitest'
import { isExternalCliSession } from '../src/lib/sessionSelection.js'

describe('session selection helper', () => {
  it('returns true only for external cli sessions', () => {
    expect(isExternalCliSession({ runner: 'external', interface: 'cli' })).toBe(true)
    expect(isExternalCliSession({ runner: 'external', interface: 'legacy' })).toBe(false)
    expect(isExternalCliSession({ runner: 'server', interface: 'cli' })).toBe(false)
    expect(isExternalCliSession(null)).toBe(false)
  })
})
