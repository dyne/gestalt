import { describe, it, expect } from 'vitest'
import { VERSION } from '../src/lib/version.js'

describe('VERSION', () => {
  it('is a non-empty string', () => {
    expect(typeof VERSION).toBe('string')
    expect(VERSION.length).toBeGreaterThan(0)
  })
})
