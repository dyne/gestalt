import { describe, it, expect } from 'vitest'

import { normalizeLogEntry } from '../src/lib/logEntry.js'

describe('normalizeLogEntry', () => {
  it('maps OTLP log records to the view model', () => {
    const timeUnixNano = '1700000000000000'
    const record = {
      timeUnixNano,
      observedTimeUnixNano: '1700000001000000',
      severityNumber: 13,
      severityText: 'WARN',
      body: { stringValue: 'hello' },
      attributes: [
        { key: 'gestalt.category', value: { stringValue: 'ui' } },
        { key: 'http.route', value: { stringValue: '/api/status' } },
        { key: 'count', value: { intValue: 3 } },
      ],
      resource: {
        attributes: [{ key: 'service.name', value: { stringValue: 'gestalt' } }],
      },
      scope: { name: 'gestalt/ui' },
    }

    const normalized = normalizeLogEntry(record)

    expect(normalized).toBeTruthy()
    expect(normalized.level).toBe('warning')
    expect(normalized.message).toBe('hello')
    expect(normalized.timestampISO).toBe(new Date(Math.floor(Number(timeUnixNano) / 1e6)).toISOString())
    expect(normalized.attributes['gestalt.category']).toBe('ui')
    expect(normalized.attributes['http.route']).toBe('/api/status')
    expect(normalized.attributes.count).toBe('3')
    expect(normalized.resourceAttributes['service.name']).toBe('gestalt')
    expect(normalized.scopeName).toBe('gestalt/ui')
  })
})
