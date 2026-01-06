import { describe, it, expect } from 'vitest'
import { formatTerminalLabel } from '../src/lib/terminalTabs.js'

describe('formatTerminalLabel', () => {
  it('uses terminal title when available, otherwise falls back to Terminal {id}', () => {
    expect(formatTerminalLabel({ id: '1', title: 'Codex' })).toBe('Codex')
    expect(formatTerminalLabel({ id: '2', title: '  ' })).toBe('Terminal 2')
  })
})
