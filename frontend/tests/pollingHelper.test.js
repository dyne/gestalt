import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

import { createPollingHelper } from '../src/lib/pollingHelper.js'

describe('pollingHelper', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('runs on an interval when started', () => {
    const onPoll = vi.fn()
    const helper = createPollingHelper({ intervalMs: 1000, onPoll })

    helper.start()
    vi.advanceTimersByTime(1000)
    vi.advanceTimersByTime(1000)

    expect(onPoll).toHaveBeenCalledTimes(2)
  })

  it('does not schedule multiple timers', () => {
    const onPoll = vi.fn()
    const helper = createPollingHelper({ intervalMs: 500, onPoll })

    helper.start()
    helper.start()
    vi.advanceTimersByTime(500)

    expect(onPoll).toHaveBeenCalledTimes(1)
  })

  it('stops polling when requested', () => {
    const onPoll = vi.fn()
    const helper = createPollingHelper({ intervalMs: 400, onPoll })

    helper.start()
    vi.advanceTimersByTime(400)
    helper.stop()
    vi.advanceTimersByTime(800)

    expect(onPoll).toHaveBeenCalledTimes(1)
  })
})
