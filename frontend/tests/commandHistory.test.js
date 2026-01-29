import { describe, it, expect, vi } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildEventSourceUrl,
}))

import { createCommandHistory } from '../src/lib/commandHistory.js'

describe('commandHistory', () => {
  it('moves through history and restores the draft', () => {
    const history = createCommandHistory()
    history.record('first')
    history.record('second')

    let value = 'draft'
    value = history.move(-1, value)
    expect(value).toBe('second')

    value = history.move(-1, value)
    expect(value).toBe('first')

    value = history.move(1, value)
    expect(value).toBe('second')

    value = history.move(1, value)
    expect(value).toBe('draft')
  })

  it('ignores move requests with no active history', () => {
    const history = createCommandHistory()
    history.record('first')

    expect(history.move(1, 'draft')).toBeNull()
    expect(history.move(-1, 'draft')).toBe('first')
  })
})
