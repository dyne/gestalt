<script>
  import { onDestroy, onMount } from 'svelte'
  import { fetchOtelLogs } from '../lib/otelClient.js'
  import { notificationStore } from '../lib/notificationStore.js'
  import { getErrorMessage } from '../lib/errorUtils.js'
  import { formatRelativeTime } from '../lib/timeUtils.js'
  import ViewState from '../components/ViewState.svelte'
  import { createViewStateMachine } from '../lib/viewStateMachine.js'

  const viewState = createViewStateMachine()

  let logs = []
  let orderedLogs = []
  let loading = false
  let error = ''
  let levelFilter = 'info'
  let autoRefresh = true
  let lastUpdated = null
  let refreshTimer = null
  let mounted = false
  let expanded = new Set()
  let lastErrorMessage = ''

  const levelOptions = [
    { value: 'debug', label: 'Debug' },
    { value: 'info', label: 'Info' },
    { value: 'warning', label: 'Warning' },
    { value: 'error', label: 'Error' },
  ]

  const formatTime = (value) => {
    return formatRelativeTime(value) || '—'
  }

  const normalizeTimestamp = (value) => {
    if (!value) return ''
    if (value instanceof Date) return value.toISOString()
    if (typeof value === 'string') {
      const trimmed = value.trim()
      if (!trimmed) return ''
      if (/^\d+$/.test(trimmed)) {
        const numeric = Number(trimmed)
        if (!Number.isNaN(numeric)) {
          return normalizeTimestamp(numeric)
        }
      }
      return value
    }
    if (typeof value === 'number') {
      if (value > 1e12) {
        return new Date(Math.floor(value / 1e6)).toISOString()
      }
      if (value > 1e10) {
        return new Date(value).toISOString()
      }
      return new Date(value * 1000).toISOString()
    }
    return value
  }

  const normalizeLevel = (value) => {
    if (value === null || value === undefined || value === '') {
      return 'info'
    }
    if (typeof value === 'number') {
      if (value >= 17) return 'error'
      if (value >= 13) return 'warning'
      if (value >= 9) return 'info'
      return 'debug'
    }
    const normalized = String(value).toLowerCase()
    if (normalized.startsWith('warn')) return 'warning'
    if (normalized.startsWith('err') || normalized.startsWith('fatal')) return 'error'
    if (normalized.startsWith('debug') || normalized.startsWith('trace')) return 'debug'
    return normalized
  }

  const normalizeAttributes = (value) => {
    if (!value) return {}
    if (Array.isArray(value)) {
      return value.reduce((acc, entry) => {
        if (!entry || typeof entry !== 'object') return acc
        const key = entry.key || entry.Key
        if (!key) return acc
        const rawValue = entry.value ?? entry.Value ?? entry.val
        acc[key] = rawValue
        return acc
      }, {})
    }
    if (typeof value === 'object') {
      return value
    }
    return {}
  }

  const normalizeMessage = (entry) => {
    if (!entry) return ''
    const body = entry.body ?? entry.Body
    if (typeof body === 'string') {
      return body
    }
    if (body && typeof body === 'object') {
      if (body.stringValue) return body.stringValue
      if (body.StringValue) return body.StringValue
      if (body.value) return body.value
      if (body.Value) return body.Value
      try {
        return JSON.stringify(body)
      } catch {
        return String(body)
      }
    }
    return entry.message || entry.Message || entry.event_name || entry.eventName || ''
  }

  const extractLogs = (payload) => {
    if (Array.isArray(payload)) return payload
    if (Array.isArray(payload?.logs)) return payload.logs
    if (Array.isArray(payload?.records)) return payload.records
    return []
  }

  const normalizeLogEntry = (entry) => {
    const timestamp = normalizeTimestamp(
      entry?.timestamp ??
        entry?.time ??
        entry?.time_unix_nano ??
        entry?.timeUnixNano ??
        entry?.observed_timestamp ??
        entry?.observedTimestamp ??
        entry?.observed_time_unix_nano ??
        entry?.observedTimeUnixNano,
    )
    const level = normalizeLevel(
      entry?.severity ??
        entry?.severity_number ??
        entry?.severityNumber ??
        entry?.severity_text ??
        entry?.severityText ??
        entry?.level,
    )
    const message = normalizeMessage(entry)
    const attributes = normalizeAttributes(entry?.attributes ?? entry?.attrs ?? entry?.context)
    return {
      level,
      message,
      timestamp,
      attributes,
      raw: entry,
    }
  }

  const loadLogs = async () => {
    viewState.start()
    try {
      const payload = await fetchOtelLogs({ level: levelFilter })
      logs = extractLogs(payload).map(normalizeLogEntry)
      lastUpdated = new Date().toISOString()
      lastErrorMessage = ''
    } catch (err) {
      const message = getErrorMessage(err, 'Failed to load logs.')
      viewState.setError(message)
      if (message !== lastErrorMessage) {
        notificationStore.addNotification('error', message)
        lastErrorMessage = message
      }
    } finally {
      viewState.finish()
    }
  }

  const resetAutoRefresh = () => {
    if (refreshTimer) {
      clearInterval(refreshTimer)
      refreshTimer = null
    }
    if (autoRefresh) {
      refreshTimer = setInterval(loadLogs, 5000)
    }
  }

  const handleFilterChange = (event) => {
    levelFilter = event.target.value
    loadLogs()
  }

  const toggleExpanded = (entryId) => {
    const next = new Set(expanded)
    if (next.has(entryId)) {
      next.delete(entryId)
    } else {
      next.add(entryId)
    }
    expanded = next
  }

  const entryKey = (entry, index) => `${entry.timestamp}-${entry.message}-${index}`

  $: orderedLogs = [...logs].reverse()

  $: if (mounted) {
    resetAutoRefresh()
  }

  onMount(async () => {
    mounted = true
    await loadLogs()
    resetAutoRefresh()
  })

  onDestroy(() => {
    mounted = false
    if (refreshTimer) {
      clearInterval(refreshTimer)
    }
  })

  $: ({ loading, error } = $viewState)
</script>

<section class="logs">
  <header class="logs__header">
    <div>
      <p class="eyebrow">System Logs</p>
      <h1>Activity stream</h1>
    </div>
    <div class="controls">
      <label class="control">
        <span>Level</span>
        <select on:change={handleFilterChange} bind:value={levelFilter}>
          {#each levelOptions as option}
            <option value={option.value}>{option.label}</option>
          {/each}
        </select>
      </label>
      <label class="control control--toggle">
        <input type="checkbox" bind:checked={autoRefresh} />
        <span>Auto refresh</span>
      </label>
      <button class="refresh" type="button" on:click={loadLogs} disabled={loading}>
        {loading ? 'Refreshing…' : 'Refresh'}
      </button>
    </div>
  </header>

  <section class="logs__meta">
    <div>
      <span class="label">Entries</span>
      <strong>{logs.length}</strong>
    </div>
    <div>
      <span class="label">Last updated</span>
      <strong title={lastUpdated || ''}>{formatTime(lastUpdated)}</strong>
    </div>
  </section>

  <section class="logs__list">
    <ViewState
      {loading}
      {error}
      hasContent={orderedLogs.length > 0}
      loadingLabel="Loading logs…"
      emptyLabel="No logs yet."
    >
      <ul>
        {#each orderedLogs as entry, index (entryKey(entry, index))}
          <li class={`log-entry log-entry--${entry.level}`}>
            <button
              class="log-entry__button"
              type="button"
              on:click={() => toggleExpanded(entryKey(entry, index))}
            >
              <div class="log-entry__summary">
                <div class="log-entry__meta">
                  <span class="badge">{entry.level}</span>
                  <span title={entry.timestamp || ''}>{formatTime(entry.timestamp)}</span>
                </div>
                <p>{entry.message}</p>
              </div>
            </button>
            {#if expanded.has(entryKey(entry, index))}
              <pre class="log-entry__context">{JSON.stringify(entry, null, 2)}</pre>
            {/if}
          </li>
        {/each}
      </ul>
    </ViewState>
  </section>
</section>

<style>
  .logs {
    padding: 2.5rem clamp(1.5rem, 4vw, 3.5rem) 3.5rem;
    display: flex;
    flex-direction: column;
    gap: 2rem;
  }

  .logs__header {
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

  .controls {
    display: flex;
    align-items: center;
    gap: 1rem;
    flex-wrap: wrap;
  }

  .control {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  select {
    border-radius: 12px;
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    padding: 0.4rem 0.6rem;
    background: var(--color-surface);
    color: var(--color-text);
  }

  .control--toggle {
    flex-direction: row;
    align-items: center;
    gap: 0.5rem;
  }

  .refresh {
    border: none;
    border-radius: 999px;
    padding: 0.7rem 1.4rem;
    font-size: 0.85rem;
    font-weight: 600;
    background: var(--color-contrast-bg);
    color: var(--color-contrast-text);
    cursor: pointer;
  }

  .refresh:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  .logs__meta {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 1rem;
  }

  .logs__meta > div {
    background: rgba(var(--color-surface-rgb), 0.85);
    padding: 1rem 1.2rem;
    border-radius: 18px;
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
  }

  .label {
    display: block;
    font-size: 0.75rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--color-text-muted);
    margin-bottom: 0.35rem;
  }

  .logs__list {
    background: var(--color-surface);
    border-radius: 24px;
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    padding: 1.5rem;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .log-entry {
    margin: 0;
  }

  .log-entry__button {
    width: 100%;
    padding: 0.9rem 1rem;
    border-radius: 16px;
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    background: rgba(var(--color-surface-rgb), 0.55);
    cursor: pointer;
    text-align: left;
    font: inherit;
    color: inherit;
  }

  .log-entry__button:focus-visible {
    outline: 2px solid rgba(var(--color-text-rgb), 0.4);
    outline-offset: 2px;
  }

  .log-entry__summary {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .log-entry__meta {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    font-size: 0.75rem;
    color: var(--color-text-muted);
  }

  .badge {
    text-transform: uppercase;
    letter-spacing: 0.12em;
    font-size: 0.7rem;
    padding: 0.2rem 0.5rem;
    border-radius: 999px;
    background: var(--color-contrast-bg);
    color: var(--color-contrast-text);
  }

  .log-entry--warning .badge {
    background: var(--color-warning);
  }

  .log-entry--debug .badge {
    background: var(--color-border);
    color: var(--color-text);
  }

  .log-entry--error .badge {
    background: var(--color-danger);
  }

  .log-entry__context {
    margin: 0.6rem 0 0;
    background: rgba(var(--color-text-rgb), 0.05);
    padding: 0.6rem;
    border-radius: 12px;
    font-size: 0.75rem;
    white-space: pre-wrap;
  }

  @media (max-width: 720px) {
    .logs__header {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
