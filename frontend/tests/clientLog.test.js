import { describe, it, expect, vi } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn(() => Promise.resolve()))

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

import { logUI } from '../src/lib/clientLog.js'

describe('clientLog', () => {
  it('posts OTLP log payloads with defaults', () => {
    logUI({
      level: 'warning',
      body: 'hello',
      attributes: { feature: 'toast' },
    })

    expect(apiFetch).toHaveBeenCalledWith('/api/otel/logs', {
      method: 'POST',
      body: JSON.stringify({
        severity_text: 'warning',
        body: 'hello',
        attributes: {
          feature: 'toast',
          'gestalt.source': 'frontend',
          'gestalt.category': 'ui',
        },
      }),
    })
  })
})
