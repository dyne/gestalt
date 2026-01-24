import { describe, it, expect } from 'vitest'
import { get } from 'svelte/store'

import { createViewStateMachine } from '../src/lib/viewStateMachine.js'

describe('viewStateMachine', () => {
  it('starts in idle state', () => {
    const machine = createViewStateMachine()
    expect(get(machine)).toEqual({ loading: false, refreshing: false, error: '' })
  })

  it('tracks loading and refreshing transitions', () => {
    const machine = createViewStateMachine()
    machine.start()
    expect(get(machine)).toEqual({ loading: true, refreshing: false, error: '' })

    machine.finish()
    machine.start({ silent: true })
    expect(get(machine)).toEqual({ loading: false, refreshing: true, error: '' })
  })

  it('keeps error until cleared', () => {
    const machine = createViewStateMachine()
    machine.setError('boom')
    machine.finish()
    expect(get(machine).error).toBe('boom')
    machine.clearError()
    expect(get(machine).error).toBe('')
  })
})
