import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { describe, it, expect, vi, afterEach } from 'vitest'

vi.mock('../src/components/VoiceInput.svelte', async () => {
  const module = await import('./helpers/VoiceInputMock.svelte')
  return { default: module.default }
})

import DirectorComposer from '../src/components/DirectorComposer.svelte'
import DirectorComposerHarness from './helpers/DirectorComposerHarness.svelte'

afterEach(() => {
  cleanup()
})

describe('DirectorComposer', () => {
  it('auto-resizes textarea on input', async () => {
    const { findByRole } = render(DirectorComposer)
    const textarea = await findByRole('textbox')
    Object.defineProperty(textarea, 'scrollHeight', {
      value: 132,
      configurable: true,
    })

    await fireEvent.input(textarea, { target: { value: 'Hello' } })

    expect(textarea.style.height).toBe('132px')
  })

  it('submits on Enter and preserves newline on Shift+Enter', async () => {
    const events = []
    const { findByRole } = render(DirectorComposerHarness, {
      props: {
        onSubmit: (payload) => events.push(payload),
      },
    })
    const textarea = await findByRole('textbox')

    await fireEvent.input(textarea, { target: { value: 'Line one' } })
    await fireEvent.keyDown(textarea, { key: 'Enter', shiftKey: true })
    expect(events).toHaveLength(0)

    await fireEvent.keyDown(textarea, { key: 'Enter' })
    expect(events).toEqual([{ text: 'Line one', source: 'text' }])
  })

  it('appends voice transcript text and tags voice source', async () => {
    const events = []
    const { findByRole } = render(DirectorComposerHarness, {
      props: {
        onSubmit: (payload) => events.push(payload),
      },
    })
    const textarea = await findByRole('textbox')
    const voiceButton = await findByRole('button', { name: 'Voice' })
    await fireEvent.click(voiceButton)
    await fireEvent.keyDown(textarea, { key: 'Enter' })

    expect(events[0]).toEqual({ text: 'Voice sentence', source: 'voice' })
  })
})
