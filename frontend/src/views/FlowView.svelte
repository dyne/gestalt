<script>
  import { onDestroy, onMount } from 'svelte'
  import { apiFetch } from '../lib/api.js'

  export let onViewTerminal = () => {}

  let workflows = []
  let loading = false
  let refreshing = false
  let error = ''
  let refreshTimer = null
  let expandedIds = new Set()

  const refreshIntervalMs = 5000

  const formatTime = (value) => {
    if (!value) return '-'
    const parsed = new Date(value)
    if (Number.isNaN(parsed.getTime())) return '-'
    return parsed.toLocaleString()
  }

  const statusLabel = (status = '') => {
    switch (status) {
      case 'running':
        return 'Running'
      case 'paused':
        return 'Paused'
      case 'stopped':
        return 'Stopped'
      default:
        return 'Unknown'
    }
  }

  const statusClass = (status = '') => {
    switch (status) {
      case 'running':
        return 'running'
      case 'paused':
        return 'paused'
      case 'stopped':
        return 'stopped'
      default:
        return 'unknown'
    }
  }

  const bellEvents = (workflow) =>
    Array.isArray(workflow?.bell_events) ? workflow.bell_events : []

  const latestBellContext = (workflow) => {
    const events = bellEvents(workflow)
    if (events.length === 0) return ''
    const lastEvent = events[events.length - 1]
    return lastEvent?.context || ''
  }

  const syncExpanded = (items) => {
    const ids = new Set(items.map((item) => item.session_id))
    const next = new Set()
    for (const id of expandedIds) {
      if (ids.has(id)) {
        next.add(id)
      }
    }
    expandedIds = next
  }

  const loadWorkflows = async ({ silent = false } = {}) => {
    if (loading || refreshing) return
    if (silent) {
      refreshing = true
    } else {
      loading = true
    }
    error = ''
    try {
      const response = await apiFetch('/api/workflows')
      const payload = await response.json()
      workflows = Array.isArray(payload) ? payload : []
      syncExpanded(workflows)
    } catch (err) {
      error = err?.message || 'Failed to load workflows.'
    } finally {
      loading = false
      refreshing = false
    }
  }

  const toggleExpanded = (id) => {
    const next = new Set(expandedIds)
    if (next.has(id)) {
      next.delete(id)
    } else {
      next.add(id)
    }
    expandedIds = next
  }

  onMount(() => {
    loadWorkflows()
    refreshTimer = setInterval(() => loadWorkflows({ silent: true }), refreshIntervalMs)
  })

  onDestroy(() => {
    if (refreshTimer) {
      clearInterval(refreshTimer)
      refreshTimer = null
    }
  })
</script>

<section class="flow-view">
  <header class="flow-view__header">
    <div>
      <p class="eyebrow">Workflow flow</p>
      <h1>Flow</h1>
    </div>
    <div class="refresh-actions">
      {#if refreshing}
        <span class="refreshing">Updating...</span>
      {/if}
      <button class="refresh" type="button" on:click={loadWorkflows} disabled={loading}>
        {loading ? 'Refreshing...' : 'Refresh'}
      </button>
    </div>
  </header>

  {#if loading && workflows.length === 0}
    <p class="muted">Loading workflows...</p>
  {:else if error && workflows.length === 0}
    <p class="error">{error}</p>
  {:else if workflows.length === 0}
    <p class="muted">No active workflows.</p>
  {:else}
    {#if error}
      <p class="error error--inline">{error}</p>
    {/if}
    <div class="workflow-list">
      {#each workflows as workflow (workflow.session_id)}
        <article
          class="workflow-card"
          class:workflow-card--paused={workflow.status === 'paused'}
        >
          <div class="workflow-card__summary">
            <div>
              <p class="workflow-card__session">Session {workflow.session_id}</p>
              <h2>{workflow.agent_name || workflow.title || 'Workflow session'}</h2>
              <p class="workflow-card__task">
                {workflow.current_l1 || 'No L1 set'}
                <span class="divider">/</span>
                {workflow.current_l2 || 'No L2 set'}
              </p>
            </div>
            <div class="workflow-card__meta">
              <span class={`status-badge status-badge--${statusClass(workflow.status)}`}>
                {statusLabel(workflow.status)}
              </span>
              <span class="workflow-card__time">Started {formatTime(workflow.start_time)}</span>
              <button
                class="workflow-card__toggle"
                type="button"
                on:click={() => toggleExpanded(workflow.session_id)}
              >
                {expandedIds.has(workflow.session_id) ? 'Hide details' : 'Show details'}
              </button>
            </div>
          </div>

          {#if expandedIds.has(workflow.session_id)}
            <div class="workflow-card__details">
              <div class="workflow-detail">
                <span class="label">Workflow ID</span>
                <span class="value">{workflow.workflow_id}</span>
              </div>
              <div class="workflow-detail">
                <span class="label">Run ID</span>
                <span class="value">{workflow.workflow_run_id || '-'}</span>
              </div>
              <div class="workflow-detail workflow-detail--wide">
                <span class="label">Event timeline</span>
                {#if bellEvents(workflow).length === 0}
                  <p class="muted">No bell events yet.</p>
                {:else}
                  <ul class="timeline">
                    {#each bellEvents(workflow) as event}
                      <li>
                        <span class="timeline__time">{formatTime(event.timestamp)}</span>
                        <span class="timeline__label">Bell</span>
                      </li>
                    {/each}
                  </ul>
                {/if}
              </div>
              <div class="workflow-detail workflow-detail--wide">
                <span class="label">Output context</span>
                {#if latestBellContext(workflow)}
                  <pre class="context">{latestBellContext(workflow)}</pre>
                {:else}
                  <p class="muted">No bell context yet.</p>
                {/if}
              </div>
              <div class="workflow-actions">
                {#if workflow.status === 'paused'}
                  <button type="button" disabled title="Resume actions are not available yet">
                    Resume
                  </button>
                  <button type="button" disabled title="Abort actions are not available yet">
                    Abort
                  </button>
                {/if}
                <button type="button" on:click={() => onViewTerminal(workflow.session_id)}>
                  View Terminal
                </button>
              </div>
            </div>
          {/if}
        </article>
      {/each}
    </div>
  {/if}
</section>

<style>
  .flow-view {
    padding: 2.5rem clamp(1.5rem, 4vw, 3.5rem) 3.5rem;
    display: flex;
    flex-direction: column;
    gap: 2rem;
  }

  .flow-view__header {
    display: flex;
    justify-content: space-between;
    align-items: center;
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

  .refresh {
    border: 1px solid rgba(20, 20, 20, 0.2);
    border-radius: 999px;
    padding: 0.6rem 1.2rem;
    background: #ffffff;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }

  .refresh-actions {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .refreshing {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    color: #6f6b62;
  }

  .muted {
    color: #7d7a73;
    margin: 0.5rem 0 0;
  }

  .error {
    color: #b04a39;
    margin: 0.5rem 0 0;
  }

  .error--inline {
    margin: 0 0 1rem;
  }

  .workflow-list {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  .workflow-card {
    padding: 1.6rem;
    border-radius: 20px;
    background: #ffffffd9;
    border: 1px solid rgba(20, 20, 20, 0.08);
    box-shadow: 0 20px 40px rgba(20, 20, 20, 0.08);
    display: flex;
    flex-direction: column;
    gap: 1.2rem;
  }

  .workflow-card--paused {
    border-color: rgba(196, 135, 0, 0.5);
    box-shadow: 0 24px 50px rgba(196, 135, 0, 0.2);
    background: #fff7e3;
  }

  .workflow-card__summary {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 2rem;
  }

  .workflow-card__session {
    text-transform: uppercase;
    letter-spacing: 0.2em;
    font-size: 0.65rem;
    color: #6c6860;
    margin: 0 0 0.5rem;
  }

  h2 {
    margin: 0 0 0.4rem;
    font-size: 1.4rem;
    color: #161616;
  }

  .workflow-card__task {
    margin: 0;
    font-size: 0.9rem;
    color: #4c4a45;
  }

  .divider {
    margin: 0 0.35rem;
    color: #b6b1a8;
  }

  .workflow-card__meta {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.6rem;
  }

  .status-badge {
    text-transform: uppercase;
    letter-spacing: 0.18em;
    font-size: 0.65rem;
    font-weight: 700;
    padding: 0.35rem 0.7rem;
    border-radius: 999px;
  }

  .status-badge--running {
    background: rgba(35, 125, 84, 0.15);
    color: #1f6a48;
  }

  .status-badge--paused {
    background: rgba(196, 135, 0, 0.2);
    color: #915c00;
  }

  .status-badge--stopped {
    background: rgba(90, 90, 90, 0.15);
    color: #4a4a4a;
  }

  .status-badge--unknown {
    background: rgba(160, 160, 160, 0.2);
    color: #5f5f5f;
  }

  .workflow-card__time {
    font-size: 0.8rem;
    color: #6c6860;
  }

  .workflow-card__toggle {
    border: none;
    background: transparent;
    color: #151515;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    text-decoration: underline;
  }

  .workflow-card__details {
    border-top: 1px solid rgba(20, 20, 20, 0.08);
    padding-top: 1.2rem;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1.2rem;
  }

  .workflow-detail {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .workflow-detail--wide {
    grid-column: span 2;
  }

  .label {
    font-size: 0.7rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: #6c6860;
  }

  .value {
    font-size: 0.85rem;
    color: #161616;
    word-break: break-all;
  }

  .timeline {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .timeline li {
    display: flex;
    gap: 0.75rem;
    font-size: 0.85rem;
    color: #4c4a45;
  }

  .timeline__time {
    font-weight: 600;
    min-width: 140px;
  }

  .context {
    margin: 0;
    padding: 0.75rem;
    border-radius: 12px;
    background: #1b1b1b;
    color: #f6f3ed;
    font-size: 0.8rem;
    max-height: 200px;
    overflow: auto;
    white-space: pre-wrap;
  }

  .workflow-actions {
    grid-column: span 2;
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
  }

  .workflow-actions button {
    border: 1px solid rgba(20, 20, 20, 0.2);
    border-radius: 999px;
    padding: 0.5rem 1.2rem;
    background: #ffffff;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }

  .workflow-actions button:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  @media (max-width: 900px) {
    .workflow-card__summary {
      flex-direction: column;
      align-items: flex-start;
    }

    .workflow-card__meta {
      align-items: flex-start;
    }

    .workflow-card__details {
      grid-template-columns: 1fr;
    }

    .workflow-detail--wide,
    .workflow-actions {
      grid-column: span 1;
    }
  }

  @media (max-width: 720px) {
    .flow-view__header {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
