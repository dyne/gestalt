<script>
  import Terminal from '../components/Terminal.svelte'

  export let terminalId = ''
  export let visible = true
</script>

<section class="terminal-view">
  {#if terminalId}
    <Terminal {terminalId} {visible} />
  {:else}
    <div class="empty">
      <h1>No terminal selected</h1>
      <p>Create a terminal from the dashboard to begin.</p>
    </div>
  {/if}
</section>

<style>
  .terminal-view {
    padding: 2rem clamp(1.5rem, 4vw, 3.5rem) 3.5rem;
  }

  .empty {
    padding: 2.5rem;
    border-radius: 24px;
    background: #ffffff;
    border: 1px solid rgba(20, 20, 20, 0.1);
    max-width: 420px;
  }

  h1 {
    margin: 0 0 0.5rem;
    font-size: 1.8rem;
  }

  p {
    margin: 0;
    color: #6d6a61;
  }
</style>
