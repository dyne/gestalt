import { cleanup, fireEvent, render } from '@testing-library/svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import VoiceInput from '../src/components/VoiceInput.svelte'

class MockSpeechRecognition {
  constructor() {
    globalThis.__lastRecognition = this
    this.continuous = false
    this.interimResults = false
    this.lang = 'en-US'
  }

  start() {
    this.onstart?.()
  }

  stop() {
    this.onend?.()
  }

  emitResult(transcript) {
    const results = [
      {
        0: { transcript },
        isFinal: true,
        length: 1,
      },
    ]
    this.onresult?.({
      resultIndex: 0,
      results,
    })
  }

  emitError(error) {
    this.onerror?.({ error })
  }
}

describe('VoiceInput', () => {
  beforeEach(() => {
    vi.stubGlobal('SpeechRecognition', MockSpeechRecognition)
    vi.stubGlobal('__lastRecognition', null)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    cleanup()
  })

  it('starts recognition on click and forwards transcripts', async () => {
    const onTranscript = vi.fn()
    const { getByRole } = render(VoiceInput, { props: { onTranscript } })

    const button = getByRole('button', { name: /start voice input/i })
    await fireEvent.click(button)

    globalThis.__lastRecognition.emitResult('hello world')

    expect(onTranscript).toHaveBeenCalledWith('hello world')
  })

  it('disables the button when recognition is not supported', () => {
    vi.unstubAllGlobals()
    const { getByRole } = render(VoiceInput)
    const button = getByRole('button')
    expect(button.disabled).toBe(true)
  })

  it('shows a friendly error message on recognition errors', async () => {
    const { getByRole, findByText } = render(VoiceInput)
    const button = getByRole('button', { name: /start voice input/i })
    await fireEvent.click(button)

    globalThis.__lastRecognition.emitError('not-allowed')

    expect(await findByText(/permission denied/i)).toBeTruthy()
  })
})
