import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

import CommandInput from '../src/components/CommandInput.svelte'

describe('CommandInput', () => {
  beforeEach(() => {
    vi.stubGlobal('requestAnimationFrame', (cb) => {
      cb()
      return 0
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    apiFetch.mockReset()
    cleanup()
  })

  it('submits on Enter and clears input', async () => {
    const onSubmit = vi.fn()
    const { getByPlaceholderText } = render(CommandInput, {
      props: { onSubmit, terminalId: '' },
    })

    const textarea = getByPlaceholderText(/Type command/i)
    await fireEvent.input(textarea, { target: { value: 'ls' } })
    await fireEvent.keyDown(textarea, { key: 'Enter' })

    expect(onSubmit).toHaveBeenCalledWith('ls')
    expect(textarea.value).toBe('')
  })
})
