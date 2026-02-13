<script>
  export let status = null

  $: sessionId = String(status?.agents_session_id || '').trim()
</script>

<section class="agents-view">
  {#if sessionId}
    <p class="agents-ready">Agents hub ready: <code>{sessionId}</code></p>
  {:else}
    <p class="agents-empty">No agents hub session yet. Start a CLI external agent to initialize it.</p>
  {/if}
</section>

<style>
  .agents-view {
    padding: 1.25rem;
    color: var(--color-text);
  }

  .agents-ready {
    margin: 0;
  }

  .agents-empty {
    margin: 0;
    color: var(--color-text-muted);
  }

  code {
    font-family: var(--font-mono);
  }
</style>
