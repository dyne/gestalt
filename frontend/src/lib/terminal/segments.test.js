import { describe, expect, it } from 'vitest'

import {
  appendOutputSegment,
  appendPromptSegment,
  historyToSegments,
} from './segments.js'

describe('terminal segments', () => {
  it('appends output chunks to the last output segment', () => {
    let segments = []
    segments = appendOutputSegment(segments, 'hello')
    segments = appendOutputSegment(segments, ' world')
    expect(segments).toEqual([{ kind: 'output', text: 'hello world' }])
  })

  it('adds prompts with trailing newlines', () => {
    const segments = appendPromptSegment([], 'ls')
    expect(segments).toEqual([{ kind: 'prompt', text: 'ls\n' }])
  })

  it('starts a new output segment after a prompt', () => {
    let segments = appendPromptSegment([], 'whoami')
    segments = appendOutputSegment(segments, 'jrml\n')
    expect(segments).toEqual([
      { kind: 'prompt', text: 'whoami\n' },
      { kind: 'output', text: 'jrml\n' },
    ])
  })

  it('normalizes carriage returns in output', () => {
    const segments = appendOutputSegment([], 'one\rtwo\r\nthree')
    expect(segments).toEqual([{ kind: 'output', text: 'one\ntwo\nthree' }])
  })

  it('builds output segments from history', () => {
    expect(historyToSegments(['a', 'b'])).toEqual([
      { kind: 'output', text: 'a\nb' },
    ])
  })
})
