<script>
  export let terminals = []
  export let status = null
  export let loading = false
  export let error = ''
  export let onCreate = () => {}

  let creating = false
  let localError = ''

  const createTerminal = async () => {
    creating = true
    localError = ''
    try {
      await onCreate()
    } catch (err) {
      localError = err?.message || 'Failed to create terminal.'
    } finally {
      creating = false
    }
  }

  const formatTime = (value) => {
    if (!value) return '—'
    const parsed = new Date(value)
    if (Number.isNaN(parsed.getTime())) return '—'
    return parsed.toLocaleString()
  }
</script>

<section class="dashboard">
  <header class="dashboard__header">
    <div>
      <p class="eyebrow">Gestalt IDE</p>
      <h1>Session dashboard</h1>
    </div>
    <button class="cta" on:click={createTerminal} disabled={creating || loading}>
      {creating ? 'Creating…' : 'New terminal'}
    </button>
  </header>

  <section class="dashboard__status">
    <div class="status-card">
      <span class="label">Active terminals</span>
      <strong class="value">{status?.terminal_count ?? '—'}</strong>
    </div>
    <div class="status-card">
      <span class="label">Server time</span>
      <strong class="value">{formatTime(status?.server_time)}</strong>
    </div>
  </section>

  <section class="dashboard__list">
    <div class="list-header">
      <h2>Live terminals</h2>
      <p class="subtle">{terminals.length} session(s)</p>
    </div>

    {#if loading}
      <p class="muted">Loading sessions…</p>
    {:else if error || localError}
      <p class="error">{error || localError}</p>
    {:else if terminals.length === 0}
      <p class="muted">No terminals running. Create one to get started.</p>
    {:else}
      <ul class="terminal-list">
        {#each terminals as terminal}
          <li class="terminal-row">
            <div>
              <p class="terminal-title">
                {terminal.title || 'Untitled terminal'}
              </p>
              <p class="terminal-meta">
                ID {terminal.id} · {terminal.role || 'general'} · {formatTime(terminal.created_at)}
              </p>
            </div>
            <span class="chip">{terminal.status}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</section>

<style>
  :global(body) {
    background: radial-gradient(circle at top left, #f6f0e4, #f1f4f6 60%, #f6f6f1);
  }

  .dashboard {
    padding: 2.5rem clamp(1.5rem, 4vw, 3.5rem) 3.5rem;
    display: flex;
    flex-direction: column;
    gap: 2.5rem;
  }

  .dashboard__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1.5rem;
  }

  .eyebrow {
    text-transform: uppercase;
    letter-spacing: 0.24em;
    font-size: 0.7rem;
    color: #6d6a61;
    margin: 0 0 0.6rem;
  }

  h1 {
    margin: 0;
    font-size: clamp(2rem, 3.5vw, 3rem);
    font-weight: 600;
    color: #161616;
  }

  .cta {
    border: none;
    border-radius: 999px;
    padding: 0.85rem 1.6rem;
    font-size: 0.95rem;
    font-weight: 600;
    background: #151515;
    color: #f6f3ed;
    cursor: pointer;
    transition: transform 160ms ease, box-shadow 160ms ease, opacity 160ms ease;
    box-shadow: 0 10px 30px rgba(10, 10, 10, 0.2);
  }

  .cta:disabled {
    cursor: not-allowed;
    opacity: 0.6;
    transform: none;
    box-shadow: none;
  }

  .cta:not(:disabled):hover {
    transform: translateY(-2px);
  }

  .dashboard__status {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 1rem;
  }

  .status-card {
    padding: 1.5rem;
    border-radius: 20px;
    background: #ffffffd9;
    border: 1px solid rgba(20, 20, 20, 0.08);
    box-shadow: 0 20px 50px rgba(20, 20, 20, 0.08);
  }

  .label {
    display: block;
    font-size: 0.8rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: #6c6860;
  }

  .value {
    display: block;
    margin-top: 0.35rem;
    font-size: 1.6rem;
    color: #141414;
  }

  .dashboard__list {
    padding: 1.5rem;
    border-radius: 24px;
    background: #ffffff;
    border: 1px solid rgba(20, 20, 20, 0.08);
  }

  .list-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 1rem;
  }

  .subtle {
    margin: 0;
    font-size: 0.9rem;
    color: #7a776f;
  }

  .terminal-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .terminal-row {
    padding: 1rem 1.2rem;
    border-radius: 16px;
    border: 1px solid rgba(20, 20, 20, 0.06);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    background: #f9f7f2;
  }

  .terminal-title {
    margin: 0 0 0.3rem;
    font-size: 1rem;
    font-weight: 600;
  }

  .terminal-meta {
    margin: 0;
    font-size: 0.85rem;
    color: #6f6b62;
  }

  .chip {
    padding: 0.3rem 0.75rem;
    border-radius: 999px;
    background: #151515;
    color: #f4f1eb;
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.1em;
  }

  .muted {
    color: #7d7a73;
    margin: 0.5rem 0 0;
  }

  .error {
    color: #b04a39;
    margin: 0.5rem 0 0;
  }

  @media (max-width: 720px) {
    .dashboard__header {
      flex-direction: column;
      align-items: flex-start;
    }

    .list-header {
      flex-direction: column;
      align-items: flex-start;
      gap: 0.4rem;
    }

    .terminal-row {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
