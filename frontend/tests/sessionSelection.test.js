import { describe, it, expect } from 'vitest'
import { isCliSession } from '../src/lib/sessionSelection.js'

describe('session selection helper', () => {
  it('returns true only for cli sessions', () => {
    expect(isCliSession({ runner: 'external', interface: 'cli' })).toBe(true)
    expect(isCliSession({ runner: 'external', interface: 'legacy' })).toBe(false)
    expect(isCliSession({ runner: 'server', interface: 'cli' })).toBe(true)
    expect(isCliSession(null)).toBe(false)
  })
})
