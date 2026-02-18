import { describe, it, expect } from 'vitest'
import { parseConventionalCommit } from '../src/lib/conventionalCommit.js'

describe('parseConventionalCommit', () => {
  it('parses feat commits with scope', () => {
    const parsed = parseConventionalCommit('feat(parser): add support')
    expect(parsed.type).toBe('feat')
    expect(parsed.scope).toBe('parser')
    expect(parsed.displayTitle).toBe('add support')
    expect(parsed.badgeClass).toBe('conventional-badge--feat')
  })

  it('returns plain values for non-conventional subjects', () => {
    const parsed = parseConventionalCommit('merge branch main')
    expect(parsed.type).toBe('')
    expect(parsed.displayTitle).toBe('merge branch main')
    expect(parsed.badgeClass).toBe('conventional-badge--default')
  })
})
