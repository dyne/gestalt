<script>
  import { onDestroy } from 'svelte'
  import { fetchWorkflowHistory } from '../lib/apiClient.js'
  import { getErrorMessage } from '../lib/errorUtils.js'
  import { formatRelativeTime } from '../lib/timeUtils.js'

  export let terminalId = ''

  let events = []
  let loading = false
  let refreshing = false
  let error = ''
  let lastTerminalId = ''

  const formatTime = (value) => {
    return formatRelativeTime(value) || '-'
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
      const payload = await fetchWorkflowHistory(terminalId)
      events = Array.isArray(payload) ? payload : []
    } catch (err) {
      error = getErrorMessage(err, 'Failed to load workflow history.')
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
          <span class="history__time" title={event.timestamp || ''}>
            {formatTime(event.timestamp)}
          </span>
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
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.35rem 0.9rem;
    background: var(--color-surface);
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
    color: var(--color-text-muted);
  }

  .label {
    font-size: 0.7rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .muted {
    color: var(--color-text-subtle);
    margin: 0;
  }

  .error {
    color: var(--color-danger);
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
    color: var(--color-text-subtle);
  }

  .history__time {
    font-weight: 600;
    min-width: 140px;
  }

  .history__item--bell .history__label {
    color: var(--color-warning);
  }

  .history__item--task_update .history__label {
    color: var(--color-success);
  }
</style>
