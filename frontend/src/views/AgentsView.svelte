<script>
  import Terminal from '../components/Terminal.svelte'

  export let status = null
  export let visible = true

  $: sessionId = String(status?.agents_session_id || '').trim()
  $: tmuxSessionName = String(status?.agents_tmux_session || '').trim()
</script>

<section class="agents-view">
  {#if sessionId}
    <Terminal
      sessionId={sessionId}
      title="Agents"
      promptFiles={[]}
      {visible}
      sessionInterface="cli"
      sessionRunner="server"
      tmuxSessionName={tmuxSessionName}
      guiModules={['console']}
      showInput={false}
      forceDirectInput={true}
      allowMouseReporting={true}
    />
  {:else}
    <p class="agents-empty">No agents hub session yet. Start a CLI external agent to initialize it.</p>
  {/if}
</section>

<style>
  .agents-view {
    width: 100%;
    height: calc(100vh - 64px);
    height: calc(100dvh - 64px);
    min-height: 0;
  }

  .agents-empty {
    margin: 0;
    padding: 1.25rem;
    color: var(--color-text-muted);
  }
</style>
