<script>
  import { formatRelativeTime } from '../lib/timeUtils.js'

  export let traces = []
  export let loading = false
  export let error = ''
  export let emptyLabel = 'No traces yet.'

  let expanded = new Set()

  const entryKey = (entry, index) =>
    `${entry.trace_id || entry.traceId || entry.id || 'trace'}-${index}`

  const toggleExpanded = (key) => {
    const next = new Set(expanded)
    if (next.has(key)) {
      next.delete(key)
    } else {
      next.add(key)
    }
    expanded = next
  }

  const formatDuration = (value) => {
    const duration = Number(value || 0)
    if (Number.isNaN(duration) || duration <= 0) return '—'
    if (duration < 1000) return `${Math.round(duration)} ms`
    return `${(duration / 1000).toFixed(2)} s`
  }

  const formatTime = (value) => {
    return formatRelativeTime(value) || '—'
  }
</script>

<section class="trace-viewer">
  <header class="trace-viewer__header">
    <div>
      <p class="eyebrow">Tracing</p>
      <h2>Trace viewer</h2>
    </div>
    <div class="trace-viewer__meta">
      <span class="label">Traces</span>
      <strong>{traces.length}</strong>
    </div>
  </header>

  {#if loading}
    <p class="muted">Loading traces…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if traces.length === 0}
    <p class="muted">{emptyLabel}</p>
  {:else}
    <ul class="trace-viewer__list">
      {#each traces as trace, index (entryKey(trace, index))}
        <li class="trace-entry">
          <button
            class="trace-entry__button"
            type="button"
            on:click={() => toggleExpanded(entryKey(trace, index))}
          >
            <div class="trace-entry__summary">
              <div class="trace-entry__meta">
                <span class={`badge badge--${trace.status || 'ok'}`}>
                  {trace.status || 'ok'}
                </span>
                <span>{formatDuration(trace.duration_ms || trace.duration || trace.durationMs)}</span>
                <span title={trace.start_time || trace.startTime || trace.timestamp || ''}>
                  {formatTime(trace.start_time || trace.startTime || trace.timestamp)}
                </span>
              </div>
              <div class="trace-entry__title">
                <strong>{trace.name || trace.root_span || trace.operation || 'Trace'}</strong>
                <span class="trace-entry__id">
                  {trace.trace_id || trace.traceId || trace.id}
                </span>
                {#if trace.service || trace.service_name || trace.serviceName}
                  <span class="trace-entry__service">
                    {trace.service || trace.service_name || trace.serviceName}
                  </span>
                {/if}
              </div>
            </div>
          </button>
          {#if expanded.has(entryKey(trace, index))}
            <pre class="trace-entry__details">{JSON.stringify(trace, null, 2)}</pre>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .trace-viewer {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  .trace-viewer__header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
  }

  .trace-viewer__meta {
    text-align: right;
  }

  .trace-viewer__list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .trace-entry {
    background: var(--color-surface);
    border-radius: 16px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    overflow: hidden;
  }

  .trace-entry__button {
    width: 100%;
    padding: 1rem 1.25rem;
    border: none;
    background: transparent;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }

  .trace-entry__summary {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .trace-entry__meta {
    display: flex;
    gap: 0.75rem;
    font-size: 0.85rem;
    color: var(--color-text-muted);
    align-items: center;
    flex-wrap: wrap;
  }

  .trace-entry__title {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    align-items: center;
    font-size: 0.95rem;
  }

  .trace-entry__id {
    font-family: var(--font-mono);
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .trace-entry__service {
    font-size: 0.8rem;
    padding: 0.2rem 0.5rem;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.08);
    color: var(--color-text-muted);
  }

  .trace-entry__details {
    margin: 0;
    padding: 1rem 1.25rem 1.25rem;
    background: rgba(10, 12, 16, 0.7);
    border-top: 1px solid rgba(255, 255, 255, 0.08);
    font-family: var(--font-mono);
    font-size: 0.8rem;
    color: var(--color-text-muted);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .badge {
    padding: 0.2rem 0.6rem;
    border-radius: 999px;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    background: rgba(255, 255, 255, 0.12);
    color: var(--color-text);
  }

  .badge--error {
    background: rgba(255, 88, 88, 0.18);
    color: #ffb4b4;
  }
  .badge--warning {
    background: rgba(255, 195, 66, 0.18);
    color: #ffd58a;
  }
  .badge--ok {
    background: rgba(83, 199, 155, 0.18);
    color: #b2f0d3;
  }
</style>
