import { describe, it, expect } from 'vitest'
import { formatTerminalLabel } from '../src/lib/terminalTabs.js'

describe('formatTerminalLabel', () => {
  it('uses the session id for terminal labels', () => {
    expect(formatTerminalLabel({ id: '1', title: 'Codex' })).toBe('1')
    expect(formatTerminalLabel({ id: '2', title: '  ' })).toBe('2')
  })
})
