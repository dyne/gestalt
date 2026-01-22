<script>
  import WorkflowDetail from './WorkflowDetail.svelte'
  import { formatRelativeTime } from '../lib/timeUtils.js'

  export let workflow = {}
  export let expanded = false
  export let actionPending = false
  export let onToggle = () => {}
  export let onViewTerminal = () => {}
  export let onResume = () => {}
  export let temporalUiUrl = ''

  const formatTime = (value) => {
    return formatRelativeTime(value) || '-'
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

  const taskSummary = (workflow) => {
    const l1 = workflow?.current_l1 || 'No L1 set'
    const l2 = workflow?.current_l2 || 'No L2 set'
    return `${l1} / ${l2}`
  }
</script>

<article class="workflow-card" class:workflow-card--paused={workflow.status === 'paused'}>
  <div class="workflow-card__summary">
    <div class="workflow-card__identity">
      <span class="workflow-card__session">Session {workflow.session_id}</span>
      <span class="workflow-card__agent">
        {workflow.agent_name || workflow.title || 'Workflow session'}
      </span>
    </div>
    <div class="workflow-card__task">{taskSummary(workflow)}</div>
    <div class="workflow-card__status">
      <span class={`status-badge status-badge--${statusClass(workflow.status)}`}>
        {statusLabel(workflow.status)}
      </span>
      <span class="workflow-card__time" title={workflow.start_time || ''}>
        Started {formatTime(workflow.start_time)}
      </span>
      <button class="workflow-card__toggle" type="button" on:click={() => onToggle(workflow.session_id)}>
        {expanded ? 'Hide details' : 'Show details'}
      </button>
    </div>
  </div>

  {#if expanded}
    <WorkflowDetail {workflow} {onViewTerminal} {onResume} {actionPending} {temporalUiUrl} />
  {/if}
</article>

<style>
  .workflow-card {
    padding: 1.4rem;
    border-radius: 20px;
    background: rgba(var(--color-surface-rgb), 0.85);
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    box-shadow: 0 18px 40px rgba(var(--shadow-color-rgb), 0.08);
    display: flex;
    flex-direction: column;
    gap: 1.2rem;
  }

  .workflow-card--paused {
    border-color: rgba(var(--color-warning-rgb), 0.5);
    box-shadow: 0 24px 50px rgba(var(--color-warning-rgb), 0.2);
    background: rgba(var(--color-warning-rgb), 0.12);
  }

  .workflow-card__summary {
    display: grid;
    grid-template-columns: minmax(140px, 1.2fr) minmax(160px, 1.5fr) minmax(160px, 1fr);
    align-items: center;
    gap: 1.5rem;
  }

  .workflow-card__identity {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .workflow-card__session {
    text-transform: uppercase;
    letter-spacing: 0.2em;
    font-size: 0.65rem;
    color: var(--color-text-muted);
  }

  .workflow-card__agent {
    font-size: 1rem;
    font-weight: 600;
    color: var(--color-text);
  }

  .workflow-card__task {
    font-size: 0.9rem;
    color: var(--color-text-subtle);
  }

  .workflow-card__status {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.5rem;
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
    background: rgba(var(--color-success-rgb), 0.15);
    color: var(--color-success);
  }

  .status-badge--paused {
    background: rgba(var(--color-warning-rgb), 0.2);
    color: var(--color-warning);
  }

  .status-badge--stopped {
    background: rgba(var(--color-text-rgb), 0.15);
    color: var(--color-text-subtle);
  }

  .status-badge--unknown {
    background: rgba(var(--color-text-rgb), 0.2);
    color: var(--color-text-subtle);
  }

  .workflow-card__time {
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .workflow-card__toggle {
    border: none;
    background: transparent;
    color: var(--color-text);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    text-decoration: underline;
  }

  @media (max-width: 900px) {
    .workflow-card__summary {
      grid-template-columns: 1fr;
      align-items: flex-start;
    }

    .workflow-card__status {
      align-items: flex-start;
    }
  }
</style>
