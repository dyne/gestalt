import { describe, it, expect, vi } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn(() => Promise.resolve()))
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildEventSourceUrl,
}))

import { logUI } from '../src/lib/clientLog.js'
import { loadUiLogFixture } from './helpers/otelLogFixture.js'

describe('clientLog', () => {
  it('posts OTLP log payloads with defaults', () => {
    const fixture = loadUiLogFixture()
    logUI({
      level: 'warning',
      body: 'hello',
      attributes: { feature: 'toast' },
    })

    expect(apiFetch).toHaveBeenCalledWith('/api/otel/logs', {
      method: 'POST',
      body: JSON.stringify(fixture),
    })
  })
})
