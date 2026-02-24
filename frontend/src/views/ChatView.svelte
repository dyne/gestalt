<script>
  import DirectorComposer from '../components/DirectorComposer.svelte'

  export let messages = []
  export let streaming = false
  export let loading = false
  export let error = ''
  export let onDirectorSubmit = async () => {}
</script>

<section class="chat-view">
  <div class="chat-view__history" aria-live="polite">
    {#if messages.length === 0}
      <p class="chat-view__empty">Start by sending a message to Director.</p>
    {:else}
      {#each messages as message (message.id)}
        <article class={`chat-bubble chat-bubble--${message.role}`}>
          <p>{message.text}</p>
        </article>
      {/each}
    {/if}
  </div>

  {#if error}
    <p class="chat-view__error">{error}</p>
  {/if}

  <DirectorComposer disabled={loading} on:submit={(event) => onDirectorSubmit(event.detail)} />

  {#if streaming}
    <p class="chat-view__streaming">Director is responding…</p>
  {/if}
</section>

<style>
  .chat-view {
    padding: 2rem clamp(1rem, 4vw, 3rem) 2.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    min-height: calc(100vh - 64px);
  }

  .chat-view__history {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    min-height: 16rem;
  }

  .chat-view__empty {
    margin: 0;
    color: var(--color-text-subtle);
  }

  .chat-bubble {
    max-width: min(75ch, 80%);
    border-radius: 16px;
    padding: 0.7rem 0.85rem;
    border: 1px solid rgba(var(--color-text-rgb), 0.12);
    background: rgba(var(--color-surface-rgb), 0.75);
    color: var(--color-text);
  }

  .chat-bubble p {
    margin: 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .chat-bubble--assistant {
    align-self: flex-start;
  }

  .chat-bubble--user {
    align-self: flex-end;
    background: rgba(var(--color-info-rgb), 0.12);
    border-color: rgba(var(--color-info-rgb), 0.35);
  }

  .chat-view__streaming {
    margin: 0;
    color: var(--color-text-subtle);
    font-size: 0.9rem;
  }

  .chat-view__error {
    margin: 0;
    color: var(--color-danger);
  }
</style>
