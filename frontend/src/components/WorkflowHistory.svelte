<script>
  import { onDestroy } from 'svelte'
  import { apiFetch } from '../lib/api.js'

  export let terminalId = ''

  let events = []
  let loading = false
  let refreshing = false
  let error = ''
  let lastTerminalId = ''

  const formatTime = (value) => {
    if (!value) return '-'
    const parsed = new Date(value)
    if (Number.isNaN(parsed.getTime())) return '-'
    return parsed.toLocaleString()
  }

  const eventLabel = (event) => {
    switch (event?.type) {
      case 'task_update':
        return `Task update: ${event.l1 || '-'} / ${event.l2 || '-'}`
      case 'bell':
        return 'Bell'
      case 'resume':
        return `Resume${event.action ? ` (${event.action})` : ''}`
      case 'terminate':
        return `Terminate${event.reason ? ` (${event.reason})` : ''}`
      case 'signal':
        return `Signal: ${event.signal_name || 'unknown'}`
      default:
        return 'Event'
    }
  }

  const loadHistory = async ({ silent = false } = {}) => {
    if (!terminalId) return
    if (loading || refreshing) return
    if (silent) {
      refreshing = true
    } else {
      loading = true
    }
    error = ''
    try {
      const response = await apiFetch(`/api/terminals/${terminalId}/workflow/history`)
      const payload = await response.json()
      events = Array.isArray(payload) ? payload : []
    } catch (err) {
      error = err?.message || 'Failed to load workflow history.'
    } finally {
      loading = false
      refreshing = false
    }
  }

  $: if (terminalId && terminalId !== lastTerminalId) {
    lastTerminalId = terminalId
    loadHistory()
  }

  onDestroy(() => {
    events = []
  })
</script>

<section class="history">
  <div class="history__header">
    <span class="label">Workflow history</span>
    <div class="history__actions">
      {#if refreshing}
        <span class="refreshing">Updating...</span>
      {/if}
      <button type="button" on:click={loadHistory} disabled={loading}>
        {loading ? 'Refreshing...' : 'Refresh'}
      </button>
    </div>
  </div>

  {#if loading && events.length === 0}
    <p class="muted">Loading history...</p>
  {:else if error && events.length === 0}
    <p class="error">{error}</p>
  {:else if events.length === 0}
    <p class="muted">No history events yet.</p>
  {:else}
    {#if error}
      <p class="error error--inline">{error}</p>
    {/if}
    <ul class="history__list">
      {#each events as event}
        <li class={`history__item history__item--${event.type || 'unknown'}`}>
          <span class="history__time">{formatTime(event.timestamp)}</span>
          <span class="history__label">{eventLabel(event)}</span>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .history {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .history__header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
  }

  .history__actions {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  .history__actions button {
    border: 1px solid rgba(20, 20, 20, 0.2);
    border-radius: 999px;
    padding: 0.35rem 0.9rem;
    background: #ffffff;
    font-size: 0.75rem;
    font-weight: 600;
    cursor: pointer;
  }

  .history__actions button:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  .refreshing {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    color: #6f6b62;
  }

  .label {
    font-size: 0.7rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: #6c6860;
  }

  .muted {
    color: #7d7a73;
    margin: 0;
  }

  .error {
    color: #b04a39;
    margin: 0;
  }

  .error--inline {
    margin: 0 0 0.6rem;
  }

  .history__list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .history__item {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    font-size: 0.85rem;
    color: #4c4a45;
  }

  .history__time {
    font-weight: 600;
    min-width: 140px;
  }

  .history__item--bell .history__label {
    color: #915c00;
  }

  .history__item--task_update .history__label {
    color: #1f6a48;
  }
</style>
