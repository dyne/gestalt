<script>
  import { onDestroy, onMount } from 'svelte'
  import { fetchWorkflows, resumeWorkflow as resumeWorkflowSession } from '../lib/apiClient.js'
  import { subscribe as subscribeWorkflowEvents } from '../lib/workflowEventStore.js'
  import { getErrorMessage } from '../lib/errorUtils.js'
  import WorkflowCard from '../components/WorkflowCard.svelte'

  export let onViewTerminal = () => {}
  export let temporalUiUrl = ''

  let workflows = []
  let loading = false
  let refreshing = false
  let error = ''
  let refreshTimer = null
  let eventRefreshTimer = null
  let expandedIds = new Set()
  let pendingActions = new Set()

  const refreshIntervalMs = 5000

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
      const payload = await fetchWorkflows()
      workflows = Array.isArray(payload) ? payload : []
      syncExpanded(workflows)
    } catch (err) {
      error = getErrorMessage(err, 'Failed to load workflows.')
    } finally {
      loading = false
      refreshing = false
    }
  }

  const scheduleEventRefresh = () => {
    if (eventRefreshTimer) return
    eventRefreshTimer = setTimeout(() => {
      eventRefreshTimer = null
      loadWorkflows({ silent: true })
    }, 250)
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

  const resumeWorkflow = async (sessionId, action) => {
    if (!sessionId || pendingActions.has(sessionId)) return
    const next = new Set(pendingActions)
    next.add(sessionId)
    pendingActions = next
    error = ''
    try {
      await resumeWorkflowSession(sessionId, action)
      await loadWorkflows({ silent: true })
    } catch (err) {
      error = getErrorMessage(err, 'Failed to resume workflow.')
    } finally {
      const cleared = new Set(pendingActions)
      cleared.delete(sessionId)
      pendingActions = cleared
    }
  }

  onMount(() => {
    loadWorkflows()
    refreshTimer = setInterval(() => loadWorkflows({ silent: true }), refreshIntervalMs)
    const eventTypes = [
      'workflow_started',
      'workflow_paused',
      'workflow_resumed',
      'workflow_completed',
      'workflow_error',
    ]
    const unsubscribers = eventTypes.map((eventType) => subscribeWorkflowEvents(eventType, scheduleEventRefresh))
    return () => {
      unsubscribers.forEach((unsubscribe) => unsubscribe())
    }
  })

  onDestroy(() => {
    if (refreshTimer) {
      clearInterval(refreshTimer)
      refreshTimer = null
    }
    if (eventRefreshTimer) {
      clearTimeout(eventRefreshTimer)
      eventRefreshTimer = null
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
        <WorkflowCard
          {workflow}
          {temporalUiUrl}
          expanded={expandedIds.has(workflow.session_id)}
          actionPending={pendingActions.has(workflow.session_id)}
          onToggle={toggleExpanded}
          {onViewTerminal}
          onResume={resumeWorkflow}
        />
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
    color: var(--color-text-muted);
    margin: 0 0 0.6rem;
  }

  h1 {
    margin: 0;
    font-size: clamp(2rem, 3.5vw, 3rem);
    font-weight: 600;
    color: var(--color-text);
  }

  .refresh {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.6rem 1.2rem;
    background: var(--color-surface);
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
    color: var(--color-text-muted);
  }

  .muted {
    color: var(--color-text-subtle);
    margin: 0.5rem 0 0;
  }

  .error {
    color: var(--color-danger);
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

  @media (max-width: 900px) {
    .workflow-list {
      gap: 1rem;
    }
  }

  @media (max-width: 720px) {
    .flow-view__header {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
