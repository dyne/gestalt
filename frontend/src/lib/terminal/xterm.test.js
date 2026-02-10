import { describe, it, expect } from 'vitest'

import { parseCssLengthPx } from './xterm.js'

describe('parseCssLengthPx', () => {
  it('converts rem to px using root font size', () => {
    document.documentElement.style.fontSize = '20px'
    expect(parseCssLengthPx('0.5rem')).toBeCloseTo(10)
  })

  it('returns numeric px values as-is', () => {
    expect(parseCssLengthPx('12px')).toBeCloseTo(12)
  })
})
