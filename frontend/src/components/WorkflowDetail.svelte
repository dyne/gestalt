<script>
  import { onDestroy } from 'svelte'
  import WorkflowHistory from './WorkflowHistory.svelte'
  import {
    buildTemporalUrl,
    formatDuration,
    formatWorkflowTime,
    timestampValue,
    truncateText,
  } from '../lib/workflowFormat.js'

  export let workflow = {}
  export let onViewTerminal = () => {}
  export let onResume = () => {}
  export let actionPending = false
  export let temporalUiUrl = ''

  let copyStatus = ''
  let copyTimer = null

  const writeClipboardText = async (text) => {
    if (!text) return false
    if (navigator.clipboard?.writeText) {
      try {
        await navigator.clipboard.writeText(text)
        return true
      } catch (err) {
        // Fall back to legacy clipboard handling.
      }
    }
    return writeClipboardTextFallback(text)
  }

  const writeClipboardTextFallback = (text) => {
    try {
      const textarea = document.createElement('textarea')
      textarea.value = text
      textarea.setAttribute('readonly', '')
      textarea.style.position = 'fixed'
      textarea.style.top = '-9999px'
      textarea.style.left = '-9999px'
      document.body.appendChild(textarea)
      textarea.select()
      const ok = document.execCommand?.('copy')
      document.body.removeChild(textarea)
      return Boolean(ok)
    } catch (err) {
      return false
    }
  }

  const copyWorkflowId = async () => {
    if (!workflow?.workflow_id) return
    const copied = await writeClipboardText(workflow.workflow_id)
    copyStatus = copied ? 'Copied' : 'Copy failed'
    if (copyTimer) {
      clearTimeout(copyTimer)
    }
    copyTimer = setTimeout(() => {
      copyStatus = ''
    }, 2000)
  }

  $: bellEvents = Array.isArray(workflow?.bell_events)
    ? [...workflow.bell_events].sort((a, b) => timestampValue(a.timestamp) - timestampValue(b.timestamp))
    : []

  $: taskEvents = Array.isArray(workflow?.task_events)
    ? [...workflow.task_events].sort((a, b) => timestampValue(a.timestamp) - timestampValue(b.timestamp))
    : []

  $: latestBell = bellEvents.length > 0 ? bellEvents[bellEvents.length - 1] : null
  $: latestBellContext = latestBell?.context || ''
  $: pauseDuration =
    workflow?.status === 'paused' && latestBell
      ? formatDuration(Date.now() - timestampValue(latestBell.timestamp))
      : '-'
  $: waitingSince = latestBell ? formatWorkflowTime(latestBell.timestamp) : '-'
  $: temporalUrl = buildTemporalUrl(workflow?.workflow_id, workflow?.workflow_run_id, temporalUiUrl)

  onDestroy(() => {
    if (copyTimer) {
      clearTimeout(copyTimer)
      copyTimer = null
    }
  })
</script>

<section class="workflow-detail">
  <div class="detail-grid">
    <div class="detail-item detail-item--copy">
      <span class="label">Workflow ID</span>
      <div class="detail-copy">
        <span class="value">{workflow.workflow_id || '-'}</span>
        <button type="button" on:click={copyWorkflowId} disabled={!workflow.workflow_id}>
          Copy
        </button>
        {#if copyStatus}
          <span class="copy-status">{copyStatus}</span>
        {/if}
      </div>
    </div>
    <div class="detail-item">
      <span class="label">Run ID</span>
      <span class="value">{workflow.workflow_run_id || '-'}</span>
    </div>
    <div class="detail-item">
      <span class="label">Paused for</span>
      <span class="value">{pauseDuration}</span>
    </div>
    <div class="detail-item">
      <span class="label">Waiting since</span>
      <span class="value" title={latestBell?.timestamp || ''}>{waitingSince}</span>
    </div>
  </div>

  <WorkflowHistory terminalId={workflow.session_id} />

  <div class="detail-section">
    <span class="label">Task history</span>
    {#if taskEvents.length === 0}
      <p class="muted">No task updates yet.</p>
    {:else}
      <ul class="task-list">
        {#each taskEvents as event}
          <li>
            <span class="task-time" title={event.timestamp || ''}>
              {formatWorkflowTime(event.timestamp)}
            </span>
            <span class="task-label">{event.l1 || '-'} / {event.l2 || '-'}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <div class="detail-section">
    <span class="label">Bell events</span>
    {#if bellEvents.length === 0}
      <p class="muted">No bell events yet.</p>
    {:else}
      <ul class="bell-list">
        {#each bellEvents as event}
          <li>
            <span class="bell-time" title={event.timestamp || ''}>
              {formatWorkflowTime(event.timestamp)}
            </span>
            <span class="bell-label">Bell</span>
            <span class="bell-context">{truncateText(event.context)}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <div class="detail-section">
    <span class="label">Output snippet</span>
    {#if latestBellContext}
      <pre class="context">{latestBellContext}</pre>
    {:else}
      <p class="muted">No bell context yet.</p>
    {/if}
  </div>

  <div class="detail-actions">
    {#if workflow.status === 'paused'}
      <button
        type="button"
        disabled={actionPending}
        on:click={() => onResume(workflow.session_id, 'continue')}
      >
        Resume
      </button>
      <button
        type="button"
        disabled={actionPending}
        on:click={() => onResume(workflow.session_id, 'abort')}
      >
        Abort
      </button>
    {/if}
    <button type="button" on:click={() => onViewTerminal(workflow.session_id)}>
      View Terminal
    </button>
    {#if temporalUrl}
      <a href={temporalUrl} target="_blank" rel="noopener noreferrer">
        View in Temporal
      </a>
    {/if}
  </div>
</section>

<style>
  .workflow-detail {
    border-top: 1px solid rgba(var(--color-text-rgb), 0.08);
    padding-top: 1.2rem;
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  .detail-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1.2rem;
  }

  .detail-item {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .detail-item--copy {
    grid-column: span 2;
  }

  .detail-copy {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
    align-items: center;
  }

  .detail-copy button {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.35rem 0.9rem;
    background: var(--color-surface);
    font-size: 0.75rem;
    font-weight: 600;
    cursor: pointer;
  }

  .detail-copy button:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  .copy-status {
    font-size: 0.75rem;
    color: var(--color-success);
  }

  .label {
    font-size: 0.7rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .value {
    font-size: 0.85rem;
    color: var(--color-text);
    word-break: break-all;
  }

  .detail-section {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .muted {
    color: var(--color-text-subtle);
    margin: 0;
  }

  .task-list,
  .bell-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .task-list li,
  .bell-list li {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    font-size: 0.85rem;
    color: var(--color-text-subtle);
  }

  .task-time,
  .bell-time {
    font-weight: 600;
    min-width: 140px;
  }

  .bell-context {
    color: var(--color-text-muted);
  }

  .context {
    margin: 0;
    padding: 0.75rem;
    border-radius: 12px;
    background: var(--color-contrast-bg);
    color: var(--color-contrast-text);
    font-size: 0.8rem;
    max-height: 200px;
    overflow: auto;
    white-space: pre-wrap;
  }

  .detail-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
  }

  .detail-actions button,
  .detail-actions a {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.5rem 1.2rem;
    background: var(--color-surface);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    text-decoration: none;
    color: var(--color-text);
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .detail-actions button:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  @media (max-width: 900px) {
    .detail-grid {
      grid-template-columns: 1fr;
    }

    .detail-item--copy {
      grid-column: span 1;
    }
  }
</style>
