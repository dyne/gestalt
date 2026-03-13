<script>
  import { createEventDispatcher, onDestroy, onMount } from 'svelte'

  export let onTranscript = () => {}
  export let continuous = false
  export let lang = ''
  export let disabled = false

  const dispatch = createEventDispatcher()

  let recognition = null
  let recognitionConstructor = null
  let needsReset = false
  let isListening = false
  let isSupported = true
  let errorMessage = ''

  const getRecognitionConstructor = () => {
    if (typeof window === 'undefined') return null
    return window.SpeechRecognition || window.webkitSpeechRecognition || null
  }

  const describeError = (errorCode) => {
    if (errorCode === 'not-allowed') return 'Microphone permission denied.'
    if (errorCode === 'service-not-allowed') return 'Voice input is blocked by the browser.'
    if (errorCode === 'no-speech') return 'No speech detected.'
    if (errorCode === 'network') return 'Network error during recognition.'
    if (errorCode === 'language-not-supported') return 'Language not supported.'
    return 'Voice recognition error.'
  }

  const vibrate = (pattern) => {
    if (typeof navigator === 'undefined') return
    if (typeof navigator.vibrate !== 'function') return
    navigator.vibrate(pattern)
  }

  const handleResult = (event) => {
    const transcripts = []
    for (let index = event.resultIndex; index < event.results.length; index += 1) {
      const result = event.results[index]
      const alternative = result?.[0]?.transcript
      if (alternative) {
        transcripts.push(alternative)
      }
    }
    const text = transcripts.join(' ').trim()
    if (!text) return
    errorMessage = ''
    onTranscript(text)
    dispatch('transcript', { text })
  }

  const detachRecognition = () => {
    if (!recognition) return
    recognition.onstart = null
    recognition.onend = null
    recognition.onerror = null
    recognition.onresult = null
    if (typeof recognition.abort === 'function') {
      try {
        recognition.abort()
      } catch (error) {
        // Ignore abort errors from already stopped instances.
      }
    }
    recognition = null
  }

  const configureRecognition = (instance) => {
    instance.continuous = continuous
    instance.interimResults = false
    instance.lang = lang || navigator.language || 'en-US'

    instance.onstart = () => {
      isListening = true
      errorMessage = ''
      vibrate(10)
      dispatch('start')
    }

    instance.onend = () => {
      isListening = false
      vibrate(10)
      dispatch('stop')
    }

    instance.onerror = (event) => {
      isListening = false
      errorMessage = describeError(event.error)
      if (event.error === 'network' || event.error === 'aborted') {
        needsReset = true
      }
      dispatch('error', { error: event.error, message: errorMessage })
    }

    instance.onresult = handleResult
  }

  const ensureRecognition = () => {
    if (!recognitionConstructor) return null
    if (recognition && !needsReset) return recognition
    detachRecognition()
    recognition = new recognitionConstructor()
    configureRecognition(recognition)
    needsReset = false
    return recognition
  }

  const startListening = () => {
    if (disabled) return
    if (!ensureRecognition()) return
    errorMessage = ''
    try {
      recognition.start()
    } catch (error) {
      errorMessage = 'Unable to start voice recognition.'
      dispatch('error', { error, message: errorMessage })
    }
  }

  const stopListening = () => {
    if (!recognition) return
    try {
      recognition.stop()
    } catch (error) {
      errorMessage = 'Unable to stop voice recognition.'
      dispatch('error', { error, message: errorMessage })
    }
  }

  const toggleListening = () => {
    if (isListening) {
      stopListening()
      return
    }
    startListening()
  }

  const statusTitle = () => {
    if (!isSupported) return 'Voice input not supported in this browser'
    if (disabled) return 'Voice input unavailable while disabled'
    if (isListening) return 'Stop voice input'
    return 'Start voice input'
  }

  onMount(() => {
    recognitionConstructor = getRecognitionConstructor()
    if (!recognitionConstructor) {
      isSupported = false
      return
    }
    if (!window.isSecureContext) {
      isSupported = false
      dispatch('error', {
        error: 'insecure-context',
        message: 'Voice input requires HTTPS (or localhost).',
      })
      return
    }

    // Recognition is created lazily on user interaction.
  })

  onDestroy(() => {
    detachRecognition()
  })
</script>

<button
  class:listening={isListening}
  class="voice-input"
  type="button"
  on:click={toggleListening}
  disabled={!isSupported || disabled}
  aria-pressed={isListening}
  aria-label={statusTitle()}
  title={statusTitle()}
>
  <span class="voice-input__icon" aria-hidden="true">
    <svg viewBox="0 0 24 24" role="img" focusable="false">
      <path
        d="M12 15.2c2.1 0 3.8-1.7 3.8-3.8V6.8C15.8 4.7 14.1 3 12 3S8.2 4.7 8.2 6.8v4.6c0 2.1 1.7 3.8 3.8 3.8zm6-3.8c0-.6-.4-1-1-1s-1 .4-1 1c0 3-2.3 5.3-5 5.6-2.7-.3-5-2.6-5-5.6 0-.6-.4-1-1-1s-1 .4-1 1c0 3.7 2.7 6.7 6.2 7.3V21H9.8c-.6 0-1 .4-1 1s.4 1 1 1h4.4c.6 0 1-.4 1-1s-.4-1-1-1h-1.2v-2.3c3.5-.6 6.2-3.6 6.2-7.3z"
      />
    </svg>
  </span>
  <span class="voice-input__pulse" aria-hidden="true"></span>
</button>

{#if errorMessage}
  <div class="voice-input__error" role="status">{errorMessage}</div>
{/if}

<style>
  .voice-input {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 2.75rem;
    height: 2.75rem;
    min-width: 44px;
    min-height: 44px;
    border-radius: 12px;
    border: 1px solid rgba(var(--terminal-border-rgb), 0.2);
    background: var(--terminal-bg);
    color: var(--terminal-text);
    cursor: pointer;
    transition: border-color 120ms ease, transform 120ms ease, background-color 160ms ease;
  }

  .voice-input:hover:not(:disabled) {
    border-color: rgba(var(--color-info-rgb), 0.35);
  }

  .voice-input:active:not(:disabled) {
    transform: translateY(1px);
  }

  .voice-input:focus-visible {
    outline: none;
    border-color: rgba(var(--color-info-rgb), 0.35);
    box-shadow: 0 0 0 2px rgba(var(--color-info-rgb), 0.2);
  }

  .voice-input:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  .voice-input__icon {
    width: 1.15rem;
    height: 1.15rem;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .voice-input__icon svg {
    width: 100%;
    height: 100%;
    fill: currentColor;
  }

  .voice-input.listening {
    background: rgba(var(--color-danger-rgb), 0.12);
    border-color: rgba(var(--color-danger-rgb), 0.45);
    color: rgb(var(--color-danger-rgb));
  }

  .voice-input__pulse {
    position: absolute;
    inset: -6px;
    border-radius: 16px;
    border: 2px solid rgba(var(--color-danger-rgb), 0.35);
    opacity: 0;
    transform: scale(0.92);
    pointer-events: none;
  }

  .voice-input.listening .voice-input__pulse {
    animation: voice-pulse 1.2s ease-out infinite;
    opacity: 1;
  }

  .voice-input__error {
    margin-top: 0.4rem;
    font-size: 0.75rem;
    color: rgb(var(--color-danger-rgb));
  }

  @keyframes voice-pulse {
    0% {
      opacity: 0.75;
      transform: scale(0.92);
    }
    70% {
      opacity: 0;
      transform: scale(1.1);
    }
    100% {
      opacity: 0;
      transform: scale(1.1);
    }
  }

  @media (max-width: 640px) {
    .voice-input {
      width: 3rem;
      height: 3rem;
      border-radius: 14px;
    }

    .voice-input__icon {
      width: 1.25rem;
      height: 1.25rem;
    }
  }
</style>
