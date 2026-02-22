import { render, fireEvent, cleanup } from '@testing-library/svelte'
import { tick } from 'svelte'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const apiFetch = vi.hoisted(() => vi.fn())
const buildEventSourceUrl = vi.hoisted(() => vi.fn((path) => `http://test${path}`))

vi.mock('../src/lib/api.js', () => ({
  apiFetch,
  buildEventSourceUrl,
}))

import CommandInput from '../src/components/CommandInput.svelte'

let originalSecureContext = true

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
    originalSecureContext = window.isSecureContext
    try {
      Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })
    } catch (error) {
      window.isSecureContext = true
    }
    apiFetch.mockResolvedValue({ ok: true })
  })

  afterEach(() => {
    try {
      delete window.__gestaltVoiceInputInsecureLogged
    } catch (error) {
      window.__gestaltVoiceInputInsecureLogged = undefined
    }
    try {
      Object.defineProperty(window, 'isSecureContext', {
        value: originalSecureContext,
        configurable: true,
      })
    } catch (error) {
      window.isSecureContext = originalSecureContext
    }
    vi.unstubAllGlobals()
    apiFetch.mockReset()
    cleanup()
  })

  it('submits on Enter and clears input', async () => {
    const onSubmit = vi.fn()
    const { getByPlaceholderText } = render(CommandInput, {
      props: { onSubmit, sessionId: '' },
    })

    const textarea = getByPlaceholderText(/Type command/i)
    await fireEvent.input(textarea, { target: { value: 'ls' } })
    await fireEvent.keyDown(textarea, { key: 'Enter' })

    expect(onSubmit).toHaveBeenCalledWith({ value: 'ls', source: 'text' })
    expect(textarea.value).toBe('')
  })

  it('appends voice transcripts to the input value', async () => {
    const { getByPlaceholderText, findByRole, getByText } = render(CommandInput, {
      props: { sessionId: '' },
    })

    const textarea = getByPlaceholderText(/Type command/i)
    await fireEvent.input(textarea, { target: { value: 'ls' } })

    const voiceButton = await findByRole('button', { name: /start voice input/i })
    await fireEvent.click(voiceButton)
    await tick()

    expect(getByText(/listening/i)).toBeTruthy()

    globalThis.__lastRecognition.emitResult('now')
    await tick()

    expect(textarea.value).toBe('ls now')
    expect(document.activeElement).toBe(textarea)
  })
})
