<script>
  import { onMount } from 'svelte'

  import { apiFetch } from '../lib/api.js'

  export let terminalId = ''
  export let onSubmit = () => {}
  export let disabled = false
  export let directInput = false
  export let onDirectInputChange = () => {}

  let value = ''
  let textarea
  let history = []
  let historyIndex = -1
  let draft = ''
  let historyLoadedFor = ''

  const maxHeight = 240
  const historyLimit = 1000

  const resizeTextarea = () => {
    if (!textarea) return
    textarea.style.height = 'auto'
    textarea.style.height = `${Math.min(textarea.scrollHeight, maxHeight)}px`
  }

  const submit = () => {
    if (disabled) return
    const next = value
    value = ''
    resizeTextarea()
    onSubmit(next)
    const trimmed = next.trim()
    if (trimmed) {
      history = [...history, trimmed]
      if (history.length > historyLimit) {
        history = history.slice(history.length - historyLimit)
      }
    }
    historyIndex = -1
    draft = ''
    requestAnimationFrame(() => textarea?.focus())
  }

  const applyHistory = () => {
    if (historyIndex === -1) {
      value = draft
    } else {
      value = history[historyIndex] || ''
    }
    resizeTextarea()
    requestAnimationFrame(() => {
      if (!textarea) return
      textarea.selectionStart = textarea.value.length
      textarea.selectionEnd = textarea.value.length
    })
  }

  const moveHistory = (direction) => {
    if (!history.length) return
    if (direction < 0) {
      if (historyIndex === -1) {
        draft = value
        historyIndex = history.length - 1
      } else if (historyIndex > 0) {
        historyIndex -= 1
      }
    } else if (direction > 0) {
      if (historyIndex === -1) return
      if (historyIndex < history.length - 1) {
        historyIndex += 1
      } else {
        historyIndex = -1
      }
    }
    applyHistory()
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

  const handleDirectToggle = (event) => {
    onDirectInputChange(event.target.checked)
  }

  export function focusInput() {
    textarea?.focus()
  }

  const loadHistory = async () => {
    if (!terminalId) return
    try {
      const response = await apiFetch(
        `/api/terminals/${terminalId}/input-history?limit=100`
      )
      const payload = await response.json()
      history = Array.isArray(payload)
        ? payload
            .map((entry) => entry?.command)
            .filter((command) => typeof command === 'string' && command !== '')
        : []
      historyIndex = -1
      draft = ''
    } catch (err) {
      console.warn('failed to load input history', err)
    }
  }

  onMount(() => {
    resizeTextarea()
    textarea?.focus()
  })

  $: if (terminalId && terminalId !== historyLoadedFor) {
    historyLoadedFor = terminalId
    loadHistory()
  }
</script>

<div class="command-input">
  <label class="sr-only" for={`command-${terminalId}`}>Command input</label>
  <div class="command-input__row">
    <textarea
      id={`command-${terminalId}`}
      bind:this={textarea}
      bind:value
      rows="3"
      placeholder="Type command... (One Enter sends, double Enter to run, Shift/Ctrl+Enter newline, Ctrl+Up/Down history)"
      on:input={resizeTextarea}
      on:keydown={handleKeydown}
      disabled={disabled}
    ></textarea>
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
  </div>
</div>

<style>
  .command-input {
    padding: 0.85rem 1rem 1rem;
    background: #171717;
    border-top: 1px solid rgba(255, 255, 255, 0.06);
  }

  .command-input__row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.85rem;
    align-items: end;
  }

  textarea {
    width: 100%;
    min-height: 4.8rem;
    max-height: 15rem;
    padding: 0.75rem 0.85rem;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: #0f0f0f;
    color: #f2efe9;
    font-family: '"IBM Plex Mono", "JetBrains Mono", monospace';
    font-size: 0.95rem;
    line-height: 1.45;
    resize: vertical;
    outline: none;
  }

  textarea:focus {
    border-color: rgba(255, 255, 255, 0.3);
    box-shadow: 0 0 0 2px rgba(242, 239, 233, 0.15);
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
    background: rgba(255, 255, 255, 0.15);
    transition: background 0.2s ease;
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.1);
  }

  .direct-toggle__switch::after {
    content: '';
    position: absolute;
    left: 2px;
    bottom: 2px;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: #f2efe9;
    transition: transform 0.2s ease;
  }

  .direct-toggle input:checked + .direct-toggle__switch {
    background: rgba(111, 196, 129, 0.6);
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
