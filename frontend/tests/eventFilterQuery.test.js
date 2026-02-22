import { describe, it, expect } from 'vitest'
import { parseEventFilterQuery, matchesEventTrigger } from '../src/lib/eventFilterQuery.js'

describe('eventFilterQuery', () => {
  const trigger = {
    id: 't1',
    label: 'Workflow paused',
    event_type: 'workflow_paused',
    where: { session_id: 't1', agent_name: 'Codex' },
  }

  it('matches free text terms', () => {
    const parsed = parseEventFilterQuery('workflow paused')
    expect(parsed.terms).toEqual(['workflow', 'paused'])
    expect(matchesEventTrigger(trigger, parsed)).toBe(true)
  })

  it('matches key/value filters', () => {
    const parsed = parseEventFilterQuery('event_type:workflow_paused')
    expect(matchesEventTrigger(trigger, parsed)).toBe(true)
  })

  it('combines free text and filters with AND semantics', () => {
    const parsed = parseEventFilterQuery('workflow event_type:workflow_paused session_id:t1')
    expect(matchesEventTrigger(trigger, parsed)).toBe(true)

    const mismatch = parseEventFilterQuery('workflow event_type:file-change session_id:t1')
    expect(matchesEventTrigger(trigger, mismatch)).toBe(false)
  })

  it('supports key exists filters', () => {
    const parsed = parseEventFilterQuery('session_id:')
    expect(matchesEventTrigger(trigger, parsed)).toBe(true)

    const missing = parseEventFilterQuery('missing:')
    expect(matchesEventTrigger(trigger, missing)).toBe(false)
  })
})
