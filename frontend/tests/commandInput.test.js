import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { tick } from 'svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
}))

import CommandInput from '../src/components/CommandInput.svelte'

class MockSpeechRecognition {
  constructor() {
    globalThis.__lastRecognition = this
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
}

describe('CommandInput', () => {
  beforeEach(() => {
    vi.stubGlobal('requestAnimationFrame', (cb) => {
      cb()
      return 0
    })
    vi.stubGlobal('SpeechRecognition', MockSpeechRecognition)
    vi.stubGlobal('__lastRecognition', null)
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

  it('appends voice transcripts to the input value', async () => {
    const { getByPlaceholderText, getByRole, getByText } = render(CommandInput, {
      props: { terminalId: '' },
    })

    const textarea = getByPlaceholderText(/Type command/i)
    await fireEvent.input(textarea, { target: { value: 'ls' } })

    const voiceButton = getByRole('button', { name: /start voice input/i })
    await fireEvent.click(voiceButton)
    await tick()

    expect(getByText(/listening/i)).toBeTruthy()

    globalThis.__lastRecognition.emitResult('now')
    await tick()

    expect(textarea.value).toBe('ls now')
    expect(document.activeElement).toBe(textarea)
  })
})
