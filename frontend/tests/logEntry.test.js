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
    }

    const normalized = normalizeLogEntry(record)

    expect(normalized).toBeTruthy()
    expect(normalized.level).toBe('warning')
    expect(normalized.message).toBe('hello')
    expect(normalized.timestamp).toBe(new Date(Math.floor(Number(timeUnixNano) / 1e6)).toISOString())
    expect(normalized.context['gestalt.category']).toBe('ui')
    expect(normalized.context['http.route']).toBe('/api/status')
    expect(normalized.context.count).toBe('3')
  })
})
