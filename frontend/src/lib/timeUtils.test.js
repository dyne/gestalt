import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatRelativeTime, resetServerTimeOffset, setServerTimeOffset } from './timeUtils.js'

describe('formatRelativeTime', () => {
  const baseTime = new Date('2026-01-08T12:00:00Z')

  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(baseTime)
  })

  afterEach(() => {
    resetServerTimeOffset()
    vi.useRealTimers()
  })

  it('formats recent times with relative phrases', () => {
    const cases = [
      { seconds: 0, expected: 'just now' },
      { seconds: 1, expected: 'just now' },
      { seconds: 59, expected: 'just now' },
      { minutes: 1, expected: '1 minute ago' },
      { minutes: 59, expected: '59 minutes ago' },
      { hours: 1, expected: '1 hour ago' },
      { hours: 23, expected: '23 hours ago' },
      { days: 1, expected: '1 day ago' },
      { days: 6, expected: '6 days ago' },
    ]

    for (const entry of cases) {
      const seconds = entry.seconds ?? 0
      const minutes = entry.minutes ?? 0
      const hours = entry.hours ?? 0
      const days = entry.days ?? 0
      const offsetMs = (((days * 24 + hours) * 60 + minutes) * 60 + seconds) * 1000
      const timestamp = new Date(baseTime.getTime() - offsetMs)
      expect(formatRelativeTime(timestamp)).toBe(entry.expected)
    }
  })

  it('handles ISO 8601 strings', () => {
    const timestamp = new Date(baseTime.getTime() - 2 * 60 * 60 * 1000).toISOString()
    expect(formatRelativeTime(timestamp)).toBe('2 hours ago')
  })

  it('uses server time offsets when provided', () => {
    const serverNow = new Date(baseTime.getTime() - 24 * 60 * 1000)
    setServerTimeOffset(serverNow)
    expect(formatRelativeTime(serverNow)).toBe('just now')
  })

  it('falls back to a short date after a week', () => {
    const timestamp = new Date(baseTime.getTime() - 7 * 24 * 60 * 60 * 1000)
    const expected = timestamp.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    })
    expect(formatRelativeTime(timestamp)).toBe(expected)
  })
})
