import { describe, expect, it } from 'vitest'

import { TextBuffer } from './textBuffer.js'

describe('TextBuffer', () => {
  it('appends chunks and preserves partial lines', () => {
    const buffer = new TextBuffer(10)
    buffer.append('hello')
    expect(buffer.text()).toBe('hello')

    buffer.append(' world\nnext')
    expect(buffer.text()).toBe('hello world\nnext')
    expect(buffer.getLines()).toEqual(['hello world', 'next'])
  })

  it('treats carriage returns as line resets', () => {
    const buffer = new TextBuffer(10)
    buffer.append('one\rtwo\r\nthree')
    expect(buffer.getLines()).toEqual(['two', 'three'])
  })

  it('loads history and clears carry', () => {
    const buffer = new TextBuffer(10)
    buffer.append('partial')
    buffer.loadHistory(['alpha', 'bravo'])
    expect(buffer.text()).toBe('alpha\nbravo')
  })

  it('trims to the max line count', () => {
    const buffer = new TextBuffer(2)
    buffer.append('one\ntwo\nthree\nfour\n')
    expect(buffer.getLines()).toEqual(['three', 'four'])
  })
})
