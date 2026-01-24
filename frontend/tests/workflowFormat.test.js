import { describe, it, expect } from 'vitest'

import {
  buildTemporalUrl,
  formatDuration,
  truncateText,
  workflowStatusClass,
  workflowStatusLabel,
  workflowTaskSummary,
} from '../src/lib/workflowFormat.js'

describe('workflowFormat', () => {
  it('labels workflow status values', () => {
    expect(workflowStatusLabel('running')).toBe('Running')
    expect(workflowStatusLabel('paused')).toBe('Paused')
    expect(workflowStatusLabel('stopped')).toBe('Stopped')
    expect(workflowStatusLabel('unknown')).toBe('Unknown')
  })

  it('maps workflow status classes', () => {
    expect(workflowStatusClass('running')).toBe('running')
    expect(workflowStatusClass('paused')).toBe('paused')
    expect(workflowStatusClass('stopped')).toBe('stopped')
    expect(workflowStatusClass('mystery')).toBe('unknown')
  })

  it('summarizes workflow tasks', () => {
    expect(workflowTaskSummary({ current_l1: 'L1', current_l2: 'L2' })).toBe('L1 / L2')
    expect(workflowTaskSummary({})).toBe('No L1 set / No L2 set')
  })

  it('formats durations', () => {
    expect(formatDuration(0)).toBe('0s')
    expect(formatDuration(61_000)).toBe('1m 1s')
    expect(formatDuration(-1)).toBe('-')
  })

  it('truncates text and builds temporal URLs', () => {
    expect(truncateText('hello', 3)).toBe('hel...')
    expect(truncateText('short', 10)).toBe('short')
    expect(buildTemporalUrl('wf', 'run', 'https://temporal.test')).toBe(
      'https://temporal.test/namespaces/default/workflows/wf/run'
    )
    expect(buildTemporalUrl('wf', '', 'not a url')).toBe('')
  })
})
