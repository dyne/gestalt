<script>
  import { createEventDispatcher } from 'svelte'
  import VoiceInput from './VoiceInput.svelte'

  export let disabled = false
  export let placeholder = 'Ask Director what to do next…'
  export let submitLabel = 'Send'

  const dispatch = createEventDispatcher()

  let text = ''
  let textarea = null
  let lastSource = 'text'

  const resize = () => {
    if (!textarea) return
    textarea.style.height = 'auto'
    textarea.style.height = `${textarea.scrollHeight}px`
  }

  const submit = () => {
    const value = text.trim()
    if (!value || disabled) return
    dispatch('submit', {
      text: value,
      source: lastSource === 'voice' ? 'voice' : 'text',
    })
    text = ''
    lastSource = 'text'
    resize()
  }

  const handleInput = () => {
    lastSource = 'text'
    resize()
  }

  const handleVoiceTranscript = (event) => {
    const transcript = String(event?.detail?.text || '').trim()
    if (!transcript) return
    text = text ? `${text} ${transcript}` : transcript
    lastSource = 'voice'
    resize()
  }

  const handleKeyDown = (event) => {
    if (event.key !== 'Enter' || event.shiftKey) return
    event.preventDefault()
    submit()
  }
</script>

<div class="director-composer">
  <textarea
    bind:this={textarea}
    class="director-composer__input"
    bind:value={text}
    placeholder={placeholder}
    disabled={disabled}
    on:input={handleInput}
    on:keydown={handleKeyDown}
    rows="3"
  ></textarea>
  <div class="director-composer__actions">
    <VoiceInput disabled={disabled} on:transcript={handleVoiceTranscript} />
    <button
      class="director-composer__send"
      type="button"
      on:click={submit}
      disabled={disabled || !text.trim()}
    >
      {submitLabel}
    </button>
  </div>
</div>

<style>
  .director-composer {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.75rem;
    align-items: stretch;
  }

  .director-composer__input {
    width: 100%;
    min-height: 5.5rem;
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 16px;
    background: rgba(var(--color-surface-rgb), 0.7);
    color: var(--color-text);
    font: inherit;
    resize: none;
    padding: 0.75rem 0.85rem;
    line-height: 1.4;
  }

  .director-composer__input:focus-visible {
    outline: 2px solid rgba(var(--color-info-rgb), 0.35);
    outline-offset: 2px;
  }

  .director-composer__actions {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    justify-content: space-between;
  }

  .director-composer__send {
    border: none;
    border-radius: 999px;
    padding: 0.85rem 1.1rem;
    font-size: 0.9rem;
    font-weight: 600;
    background: var(--color-contrast-bg);
    color: var(--color-contrast-text);
    cursor: pointer;
    transition: transform 160ms ease, box-shadow 160ms ease, opacity 160ms ease;
    box-shadow: 0 10px 30px rgba(var(--shadow-color-rgb), 0.2);
  }

  .director-composer__send:disabled {
    cursor: not-allowed;
    opacity: 0.6;
    transform: none;
    box-shadow: none;
  }

  .director-composer__send:not(:disabled):hover {
    transform: translateY(-2px);
  }

  @media (max-width: 640px) {
    .director-composer {
      grid-template-columns: 1fr;
    }

    .director-composer__actions {
      flex-direction: row;
      align-items: center;
      justify-content: space-between;
    }
  }
</style>
