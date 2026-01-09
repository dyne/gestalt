<script>
  import { onDestroy } from 'svelte'

  export let workflow = {}
  export let onViewTerminal = () => {}

  let copyStatus = ''
  let copyTimer = null

  const formatTime = (value) => {
    if (!value) return '-'
    const parsed = new Date(value)
    if (Number.isNaN(parsed.getTime())) return '-'
    return parsed.toLocaleString()
  }

  const timestampValue = (value) => {
    const parsed = new Date(value)
    const time = parsed.getTime()
    return Number.isNaN(time) ? 0 : time
  }

  const formatDuration = (milliseconds) => {
    if (!Number.isFinite(milliseconds) || milliseconds < 0) return '-'
    const totalSeconds = Math.floor(milliseconds / 1000)
    const seconds = totalSeconds % 60
    const totalMinutes = Math.floor(totalSeconds / 60)
    const minutes = totalMinutes % 60
    const hours = Math.floor(totalMinutes / 60)
    const parts = []
    if (hours > 0) parts.push(`${hours}h`)
    if (minutes > 0 || hours > 0) parts.push(`${minutes}m`)
    parts.push(`${seconds}s`)
    return parts.join(' ')
  }

  const truncateText = (text, maxLength = 160) => {
    if (!text) return ''
    if (text.length <= maxLength) return text
    return `${text.slice(0, maxLength)}...`
  }

  const buildTemporalUrl = (workflowId, runId) => {
    if (!workflowId) return ''
    const namespace = 'default'
    const encodedId = encodeURIComponent(workflowId)
    const encodedRun = runId ? `/${encodeURIComponent(runId)}` : ''
    return `http://localhost:8233/namespaces/${namespace}/workflows/${encodedId}${encodedRun}`
  }

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

  $: timeline = [
    ...taskEvents.map((event) => ({
      type: 'task',
      timestamp: event.timestamp,
      label: `Task: ${event.l1 || '-'} / ${event.l2 || '-'}`,
    })),
    ...bellEvents.map((event) => ({
      type: 'bell',
      timestamp: event.timestamp,
      label: 'Bell',
    })),
  ].sort((a, b) => timestampValue(a.timestamp) - timestampValue(b.timestamp))

  $: latestBell = bellEvents.length > 0 ? bellEvents[bellEvents.length - 1] : null
  $: latestBellContext = latestBell?.context || ''
  $: pauseDuration =
    workflow?.status === 'paused' && latestBell
      ? formatDuration(Date.now() - timestampValue(latestBell.timestamp))
      : '-'
  $: waitingSince = latestBell ? formatTime(latestBell.timestamp) : '-'
  $: temporalUrl = buildTemporalUrl(workflow?.workflow_id, workflow?.workflow_run_id)

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
      <span class="value">{waitingSince}</span>
    </div>
  </div>

  <div class="detail-section">
    <span class="label">Timeline</span>
    {#if timeline.length === 0}
      <p class="muted">No events yet.</p>
    {:else}
      <ul class="timeline">
        {#each timeline as event}
          <li class={`timeline__item timeline__item--${event.type}`}>
            <span class="timeline__time">{formatTime(event.timestamp)}</span>
            <span class="timeline__label">{event.label}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <div class="detail-section">
    <span class="label">Task history</span>
    {#if taskEvents.length === 0}
      <p class="muted">No task updates yet.</p>
    {:else}
      <ul class="task-list">
        {#each taskEvents as event}
          <li>
            <span class="task-time">{formatTime(event.timestamp)}</span>
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
            <span class="bell-time">{formatTime(event.timestamp)}</span>
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
    {#if temporalUrl}
      <a href={temporalUrl} target="_blank" rel="noopener noreferrer">
        View in Temporal
      </a>
    {/if}
  </div>
</section>

<style>
  .workflow-detail {
    border-top: 1px solid rgba(20, 20, 20, 0.08);
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
    border: 1px solid rgba(20, 20, 20, 0.2);
    border-radius: 999px;
    padding: 0.35rem 0.9rem;
    background: #ffffff;
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
    color: #2e6b46;
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

  .detail-section {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .muted {
    color: #7d7a73;
    margin: 0;
  }

  .timeline,
  .task-list,
  .bell-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .timeline__item,
  .task-list li,
  .bell-list li {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    font-size: 0.85rem;
    color: #4c4a45;
  }

  .timeline__time,
  .task-time,
  .bell-time {
    font-weight: 600;
    min-width: 140px;
  }

  .timeline__item--task .timeline__label {
    color: #1f6a48;
  }

  .timeline__item--bell .timeline__label {
    color: #915c00;
  }

  .bell-context {
    color: #6c6860;
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

  .detail-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
  }

  .detail-actions button,
  .detail-actions a {
    border: 1px solid rgba(20, 20, 20, 0.2);
    border-radius: 999px;
    padding: 0.5rem 1.2rem;
    background: #ffffff;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    text-decoration: none;
    color: #151515;
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
