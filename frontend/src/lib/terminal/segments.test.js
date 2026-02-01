import { describe, expect, it } from 'vitest'

import {
  appendOutputSegment,
  appendPromptSegment,
  createPromptEchoSuppressor,
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

  it('treats carriage returns as line resets', () => {
    const segments = appendOutputSegment([], 'one\rtwo\r\nthree')
    expect(segments).toEqual([{ kind: 'output', text: 'two\nthree' }])
  })

  it('resets the current line on carriage return', () => {
    let segments = appendOutputSegment([], 'alpha\nbeta')
    segments = appendOutputSegment(segments, '\rgamma')
    expect(segments).toEqual([{ kind: 'output', text: 'alpha\ngamma' }])
  })

  it('builds output segments from history', () => {
    expect(historyToSegments(['a', 'b'])).toEqual([
      { kind: 'output', text: 'a\nb' },
    ])
  })

  it('suppresses the first echoed command line', () => {
    const suppressor = createPromptEchoSuppressor()
    suppressor.markCommand('ls', 100)
    expect(suppressor.filterChunk('ls\n', 120).output).toBe('')
  })

  it('suppresses prompt-prefixed echoes', () => {
    const suppressor = createPromptEchoSuppressor()
    suppressor.markCommand('pwd', 0)
    expect(suppressor.filterChunk('> pwd\n', 1).output).toBe('')
  })

  it('passes through non-matching output', () => {
    const suppressor = createPromptEchoSuppressor()
    suppressor.markCommand('ls', 0)
    expect(suppressor.filterChunk('lsa\n', 1).output).toBe('lsa\n')
  })

  it('suppresses echoed commands split across chunks', () => {
    const suppressor = createPromptEchoSuppressor()
    suppressor.markCommand('whoami', 0)
    expect(suppressor.filterChunk('who', 1).output).toBe('')
    expect(suppressor.filterChunk('ami\n', 2).output).toBe('')
  })
})
