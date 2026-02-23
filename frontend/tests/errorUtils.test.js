import { describe, expect, it } from 'vitest'

import { getErrorMessage } from '../src/lib/errorUtils.js'

describe('errorUtils', () => {
  it('returns the error message when present', () => {
    const message = getErrorMessage({
      message: 'request failed',
    })
    expect(message).toBe('request failed')
  })
})
