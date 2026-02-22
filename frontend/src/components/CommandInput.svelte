<script>
  import { onDestroy, onMount } from 'svelte'

  import { logUI } from '../lib/clientLog.js'
  import { createCommandHistory } from '../lib/commandHistory.js'
  import VoiceInput from './VoiceInput.svelte'

  export let sessionId = ''
  export let agentName = ''
  export let onSubmit = () => {}
  export let disabled = false
  export let directInput = false
  export let showDirectInputToggle = false
  export let onDirectInputChange = () => {}

  let value = ''
  let textarea
  let isResizing = false
  let resizeStartY = 0
  let resizeStartHeight = 0
  let manualHeight = 0
  let commandStyle = ''
  let hasOverflow = false
  let lastInputSource = 'text'
  const maxHeight = 240
  const historyLimit = 1000

  const history = createCommandHistory({ historyLimit })
  let isVoiceListening = false
  let voiceInputAvailable = false

  const hasSpeechRecognition = () => {
    if (typeof window === 'undefined') return false
    return Boolean(window.SpeechRecognition || window.webkitSpeechRecognition)
  }

  const logInsecureVoiceInput = () => {
    if (typeof window === 'undefined') return
    if (window.__gestaltVoiceInputInsecureLogged) return
    window.__gestaltVoiceInputInsecureLogged = true
    logUI({
      level: 'warning',
      body: 'voice input unavailable: insecure context (requires HTTPS or localhost)',
      attributes: {
        origin: window.location?.origin || '',
      },
    })
  }

  const updateVoiceInputAvailability = () => {
    const speechSupported = hasSpeechRecognition()
    const secureContext = typeof window !== 'undefined' && window.isSecureContext
    voiceInputAvailable = speechSupported && secureContext
    if (speechSupported && !secureContext) {
      logInsecureVoiceInput()
    }
  }

  const resizeTextarea = () => {
    if (!textarea) return
    const limit = getMaxHeightLimit()
    textarea.style.height = 'auto'
    textarea.style.height = `${Math.min(textarea.scrollHeight, limit)}px`
    hasOverflow = textarea.scrollHeight - textarea.clientHeight > 1
  }

  const getMaxHeightLimit = () => {
    if (!textarea) return maxHeight
    const computed = getComputedStyle(textarea)
    const maxValue = Number.parseFloat(computed.maxHeight)
    if (Number.isFinite(maxValue) && maxValue > 0) {
      return maxValue
    }
    return maxHeight
  }

  const getMinHeightLimit = () => {
    if (!textarea) return 64
    const computed = getComputedStyle(textarea)
    const lineHeight = Number.parseFloat(computed.lineHeight) || 20
    const paddingTop = Number.parseFloat(computed.paddingTop) || 0
    const paddingBottom = Number.parseFloat(computed.paddingBottom) || 0
    return Math.ceil(lineHeight * 2 + paddingTop + paddingBottom)
  }

  const getMaxDragHeight = () => {
    if (typeof window === 'undefined') return maxHeight
    return Math.max(getMinHeightLimit(), window.innerHeight * 0.5)
  }

  const clamp = (value, min, max) => Math.min(Math.max(value, min), max)

  const stopResize = () => {
    if (!isResizing) return
    isResizing = false
    if (typeof window === 'undefined') return
    window.removeEventListener('pointermove', handleResizeMove)
    window.removeEventListener('pointerup', stopResize)
    window.removeEventListener('pointercancel', stopResize)
  }

  const handleResizeMove = (event) => {
    if (!isResizing) return
    const delta = resizeStartY - event.clientY
    const minHeight = getMinHeightLimit()
    const maxHeightLimit = getMaxDragHeight()
    manualHeight = clamp(resizeStartHeight + delta, minHeight, maxHeightLimit)
    resizeTextarea()
  }

  const startResize = (event) => {
    if (disabled) return
    if (event.button !== 0 && event.pointerType !== 'touch') return
    event.preventDefault()
    const minHeight = getMinHeightLimit()
    const maxHeightLimit = getMaxDragHeight()
    const currentHeight = textarea?.getBoundingClientRect().height || minHeight
    resizeStartY = event.clientY
    resizeStartHeight = clamp(currentHeight, minHeight, maxHeightLimit)
    manualHeight = resizeStartHeight
    isResizing = true
    if (typeof window === 'undefined') return
    window.addEventListener('pointermove', handleResizeMove)
    window.addEventListener('pointerup', stopResize)
    window.addEventListener('pointercancel', stopResize)
  }

  const submit = () => {
    if (disabled) return
    const next = value
    const source = lastInputSource === 'voice' ? 'voice' : 'text'
    lastInputSource = 'text'
    value = ''
    resizeTextarea()
    onSubmit({ value: next, source })
    const trimmed = next.trim()
    if (trimmed) {
      history.record(trimmed)
      const attributes = {}
    if (sessionId) {
      attributes['terminal.id'] = sessionId
    }
      if (agentName) {
        attributes['agent.name'] = agentName
      }
      logUI({
        level: 'info',
        body: next,
        attributes,
      })
    }
    history.resetNavigation()
    requestAnimationFrame(() => textarea?.focus())
  }

  const moveHistory = (direction) => {
    const nextValue = history.move(direction, value)
    if (nextValue === null) return
    value = nextValue
    resizeTextarea()
    requestAnimationFrame(() => {
      if (!textarea) return
      textarea.selectionStart = textarea.value.length
      textarea.selectionEnd = textarea.value.length
    })
  }

  const handleKeydown = (event) => {
    if (event.ctrlKey && !event.altKey) {
      if (event.key === 'ArrowUp') {
        event.preventDefault()
        moveHistory(-1)
        return
      }
      if (event.key === 'ArrowDown') {
        event.preventDefault()
        moveHistory(1)
        return
      }
    }

    if (event.key !== 'Enter') return
    if (event.ctrlKey || event.shiftKey) {
      event.preventDefault()
      insertNewline()
      return
    }
    event.preventDefault()
    submit()
  }

  const insertNewline = () => {
    if (!textarea) {
      value = `${value}\n`
      resizeTextarea()
      return
    }
    const start = textarea.selectionStart ?? value.length
    const end = textarea.selectionEnd ?? value.length
    value = `${value.slice(0, start)}\n${value.slice(end)}`
    resizeTextarea()
    requestAnimationFrame(() => {
      if (!textarea) return
      const next = start + 1
      textarea.selectionStart = next
      textarea.selectionEnd = next
    })
  }

  const handleTranscript = (text) => {
    const transcript = text.trim()
    if (!transcript) return
    lastInputSource = 'voice'
    const hasContent = value.trim().length > 0
    value = hasContent ? `${value.trimEnd()} ${transcript}` : transcript
    resizeTextarea()
    requestAnimationFrame(() => textarea?.focus())
  }

  const handleVoiceStart = () => {
    isVoiceListening = true
  }

  const handleVoiceStop = () => {
    isVoiceListening = false
  }

  const handleVoiceError = () => {
    isVoiceListening = false
  }

  const handleDirectToggle = (event) => {
    onDirectInputChange(event.target.checked)
  }

  export function focusInput() {
    textarea?.focus()
  }

  onMount(() => {
    updateVoiceInputAvailability()
    resizeTextarea()
    textarea?.focus()
  })

  onDestroy(() => {
    stopResize()
  })

  $: if (sessionId) {
    history.load(sessionId)
  }

  $: commandStyle = manualHeight
    ? `--command-input-height: ${manualHeight}px;`
    : ''
</script>

<div
  class="command-input"
  class:command-input--resizing={isResizing}
  style={commandStyle}
>
  <div
    class="command-input__resize"
    role="separator"
    aria-label="Resize command input"
    aria-orientation="horizontal"
    on:pointerdown={startResize}
  ></div>
  <label class="sr-only" for={`command-${sessionId}`}>Command input</label>
  <div class="command-input__row">
    <textarea
      id={`command-${sessionId}`}
      bind:this={textarea}
      bind:value
      class:textarea--no-scroll={!hasOverflow}
      rows="3"
      placeholder="Type command... (One Enter sends, double Enter to run, Shift/Ctrl+Enter newline, Ctrl+Up/Down history)"
      on:input={() => {
        lastInputSource = 'text'
        resizeTextarea()
      }}
      on:keydown={handleKeydown}
      disabled={disabled}
    ></textarea>
    <div class="command-input__actions">
      {#if voiceInputAvailable}
        <VoiceInput
          onTranscript={handleTranscript}
          on:start={handleVoiceStart}
          on:stop={handleVoiceStop}
          on:error={handleVoiceError}
          {disabled}
        />
      {/if}
      {#if showDirectInputToggle}
        <label class="direct-toggle" title="Direct input switch">
          <input
            type="checkbox"
            checked={directInput}
            on:change={handleDirectToggle}
            aria-label="Direct input switch"
            disabled={disabled}
          />
          <span class="direct-toggle__switch"></span>
        </label>
      {/if}
    </div>
  </div>
  {#if isVoiceListening}
    <div class="command-input__listening" role="status">Listening...</div>
  {/if}
</div>

<style>
  .command-input {
    position: sticky;
    bottom: 0;
    z-index: 10;
    padding: 0.85rem 1rem 1rem;
    background: var(--terminal-panel);
    border-top: 1px solid rgba(var(--terminal-border-rgb), 0.12);
    box-shadow: 0 -12px 24px rgba(var(--shadow-color-rgb), 0.35);
  }

  .command-input--resizing {
    user-select: none;
  }

  .command-input__resize {
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 6px;
    cursor: row-resize;
    touch-action: none;
  }

  .command-input__resize::after {
    content: '';
    position: absolute;
    inset: 2px 30%;
    border-radius: 999px;
    background: rgba(var(--terminal-border-rgb), 0.5);
  }

  .command-input__row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.85rem;
    align-items: end;
  }

  .command-input__actions {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.5rem;
  }

  .command-input__listening {
    margin-top: 0.5rem;
    font-size: 0.8rem;
    color: rgba(var(--terminal-text-rgb), 0.8);
    letter-spacing: 0.02em;
  }

  textarea {
    width: 100%;
    min-width: 0;
    min-height: 4.8rem;
    max-height: var(--command-input-height, 15rem);
    padding: 0.75rem 0.85rem;
    border-radius: 12px;
    border: 1px solid rgba(var(--terminal-border-rgb), 0.2);
    background: var(--terminal-bg);
    color: var(--terminal-text);
    font-family: var(--terminal-input-font-family, "IBM Plex Mono", "JetBrains Mono", monospace);
    font-size: var(--terminal-input-font-size, 0.95rem);
    line-height: 1.45;
    resize: none;
    outline: none;
    overflow-y: auto;
  }

  textarea.textarea--no-scroll {
    scrollbar-width: none;
  }

  textarea.textarea--no-scroll::-webkit-scrollbar {
    display: none;
  }

  textarea:focus {
    border-color: rgba(var(--color-info-rgb), 0.35);
    box-shadow: 0 0 0 2px rgba(var(--color-info-rgb), 0.2);
  }

  textarea:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .direct-toggle {
    position: relative;
    width: 20px;
    height: 54px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    user-select: none;
  }

  .direct-toggle input {
    position: absolute;
    inset: 0;
    opacity: 0;
    cursor: pointer;
  }

  .direct-toggle__switch {
    position: relative;
    display: block;
    width: 18px;
    height: 48px;
    border-radius: 999px;
    background: rgba(var(--color-text-rgb), 0.15);
    transition: background 0.2s ease;
    box-shadow: inset 0 0 0 1px rgba(var(--color-text-rgb), 0.1);
  }

  .direct-toggle__switch::after {
    content: '';
    position: absolute;
    left: 2px;
    bottom: 2px;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: var(--terminal-text);
    transition: transform 0.2s ease;
  }

  .direct-toggle input:checked + .direct-toggle__switch {
    background: rgba(var(--color-success-rgb), 0.6);
  }

  .direct-toggle input:checked + .direct-toggle__switch::after {
    transform: translateY(-28px);
  }

  .direct-toggle input:disabled + .direct-toggle__switch {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    border: 0;
  }
</style>
