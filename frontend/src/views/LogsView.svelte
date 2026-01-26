<script>
  import { onDestroy, onMount } from 'svelte'
  import { notificationStore } from '../lib/notificationStore.js'
  import { getErrorMessage } from '../lib/errorUtils.js'
  import { formatRelativeTime } from '../lib/timeUtils.js'
  import { formatLogEntryForClipboard, normalizeLogEntry } from '../lib/logEntry.js'
  import ViewState from '../components/ViewState.svelte'
  import { createViewStateMachine } from '../lib/viewStateMachine.js'
  import { createLogStream } from '../lib/logStream.js'

  const viewState = createViewStateMachine()
  const maxLogEntries = 1000

  let logs = []
  let orderedLogs = []
  let loading = false
  let error = ''
  let levelFilter = 'info'
  let autoRefresh = true
  let lastUpdated = null
  let lastErrorMessage = ''
  let logStream = null
  let streamActive = false
  let pendingStop = false
  let stopTimer = null

  const levelOptions = [
    { value: 'debug', label: 'Debug' },
    { value: 'info', label: 'Info' },
    { value: 'warning', label: 'Warning' },
    { value: 'error', label: 'Error' },
  ]

  const formatTime = (value) => {
    return formatRelativeTime(value) || '—'
  }

  const clearStopTimer = () => {
    if (stopTimer) {
      clearTimeout(stopTimer)
      stopTimer = null
    }
  }

  const stopStream = () => {
    clearStopTimer()
    pendingStop = false
    if (logStream) {
      logStream.stop()
    }
    streamActive = false
    viewState.finish()
  }

  const appendLogEntry = (entry) => {
    if (!entry) return
    const normalized = normalizeLogEntry(entry)
    if (!normalized) return
    logs = [...logs, normalized]
    if (logs.length > maxLogEntries) {
      logs = logs.slice(logs.length - maxLogEntries)
    }
    lastUpdated = new Date().toISOString()
    viewState.finish()
    lastErrorMessage = ''
  }

  const ensureLogStream = () => {
    if (logStream) return logStream
    logStream = createLogStream({
      level: levelFilter,
      onEntry: appendLogEntry,
      onOpen: () => {
        viewState.finish()
        if (pendingStop) {
          pendingStop = false
          clearStopTimer()
          stopTimer = setTimeout(() => {
            stopStream()
          }, 1500)
        }
        lastErrorMessage = ''
      },
      onError: (err) => {
        const message = getErrorMessage(err, 'Failed to load logs.')
        viewState.setError(message)
        viewState.finish()
        if (message !== lastErrorMessage) {
          notificationStore.addNotification('error', message)
          lastErrorMessage = message
        }
      },
    })
    return logStream
  }

  const loadLogs = ({ reset = true } = {}) => {
    pendingStop = !autoRefresh
    clearStopTimer()
    if (reset) {
      logs = []
    }
    viewState.start()
    const stream = ensureLogStream()
    stream.setLevel(levelFilter)
    if (!streamActive) {
      streamActive = true
      stream.start()
      return
    }
    stream.restart()
  }

  const handleFilterChange = (event) => {
    levelFilter = event.target.value
    loadLogs()
  }

  const handleAutoRefreshChange = (event) => {
    autoRefresh = event.target.checked
    if (autoRefresh) {
      loadLogs({ reset: false })
      return
    }
    stopStream()
  }

  const entryKey = (entry, index) => entry?.id || `${entry.timestamp}-${entry.message}-${index}`

  const contextEntriesFor = (entry) => {
    return Object.entries(entry?.context || {}).sort(([left], [right]) =>
      left.localeCompare(right),
    )
  }

  const copyText = async (text, successMessage) => {
    if (!text) {
      notificationStore.addNotification('error', 'Nothing to copy.')
      return
    }
    const clipboard = navigator?.clipboard
    if (!clipboard?.writeText) {
      notificationStore.addNotification('error', 'Clipboard is unavailable.')
      return
    }
    try {
      await clipboard.writeText(text)
      notificationStore.addNotification('info', successMessage)
    } catch (err) {
      notificationStore.addNotification('error', 'Failed to copy to clipboard.')
    }
  }

  const copyLogJson = async (entry) => {
    if (!entry) return
    const text = entry.raw ? JSON.stringify(entry.raw, null, 2) : formatLogEntryForClipboard(entry)
    await copyText(text, 'Copied log JSON.')
  }

  $: orderedLogs = [...logs].reverse()
 
  onMount(() => {
    loadLogs()
  })

  onDestroy(() => {
    stopStream()
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
        <input type="checkbox" bind:checked={autoRefresh} on:change={handleAutoRefreshChange} />
        <span>Live updates</span>
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
            <details class="log-entry__details">
              <summary class="log-entry__summary">
                <div class="log-entry__meta">
                  <span class="badge">{entry.level}</span>
                  <span title={entry.timestamp || ''}>{formatTime(entry.timestamp)}</span>
                </div>
                <p>{entry.message}</p>
              </summary>
              <div class="log-entry__details-body">
                <div class="log-entry__detail-section">
                  <span class="log-entry__label">Context</span>
                  {#if contextEntriesFor(entry).length === 0}
                    <p class="log-entry__empty">No context fields.</p>
                  {:else}
                    <div class="log-entry__context">
                      <table>
                        <tbody>
                          {#each contextEntriesFor(entry) as [key, value]}
                            <tr>
                              <th scope="row">{key}</th>
                              <td>{value}</td>
                            </tr>
                          {/each}
                        </tbody>
                      </table>
                    </div>
                  {/if}
                </div>
                {#if entry.raw}
                  <details class="log-entry__raw">
                    <summary>Raw JSON</summary>
                    <div class="log-entry__raw-actions">
                      <button type="button" on:click={() => copyLogJson(entry)}>
                        Copy JSON
                      </button>
                    </div>
                    <pre>{JSON.stringify(entry.raw, null, 2)}</pre>
                  </details>
                {/if}
              </div>
            </details>
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

  .log-entry__details {
    border-radius: 16px;
  }

  .log-entry__summary {
    width: 100%;
    padding: 0.9rem 1rem;
    border-radius: 16px;
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    background: rgba(var(--color-surface-rgb), 0.55);
    cursor: pointer;
    text-align: left;
    font: inherit;
    color: inherit;
    list-style: none;
  }

  .log-entry__summary::-webkit-details-marker {
    display: none;
  }

  .log-entry__summary::marker {
    content: '';
  }

  .log-entry__summary:focus-visible {
    outline: 2px solid rgba(var(--color-text-rgb), 0.4);
    outline-offset: 2px;
  }

  .log-entry__details[open] .log-entry__summary {
    border-bottom-left-radius: 0;
    border-bottom-right-radius: 0;
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

  .log-entry__details-body {
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    border-top: none;
    padding: 0.85rem;
    border-bottom-left-radius: 16px;
    border-bottom-right-radius: 16px;
    background: rgba(var(--color-surface-rgb), 0.7);
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .log-entry__detail-section {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .log-entry__label {
    font-size: 0.7rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .log-entry__empty {
    margin: 0;
    color: var(--color-text-subtle);
  }

  .log-entry__context {
    max-height: 220px;
    overflow: auto;
    border-radius: 12px;
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    background: rgba(var(--color-surface-rgb), 0.7);
  }

  .log-entry__context table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.8rem;
  }

  .log-entry__context th,
  .log-entry__context td {
    padding: 0.5rem 0.7rem;
    border-bottom: 1px solid rgba(var(--color-text-rgb), 0.08);
    text-align: left;
  }

  .log-entry__context th {
    width: 30%;
    font-weight: 600;
    color: var(--color-text-muted);
  }

  .log-entry__context td {
    font-family: "IBM Plex Mono", "SFMono-Regular", Menlo, monospace;
    color: var(--color-text);
    word-break: break-word;
  }

  .log-entry__raw {
    border-top: 1px solid rgba(var(--color-text-rgb), 0.08);
    padding-top: 0.75rem;
  }

  .log-entry__raw summary {
    cursor: pointer;
    font-weight: 600;
  }

  .log-entry__raw-actions {
    margin-top: 0.5rem;
    display: flex;
    justify-content: flex-end;
  }

  .log-entry__raw-actions button {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.35rem 0.9rem;
    background: transparent;
    font-size: 0.75rem;
    font-weight: 600;
    cursor: pointer;
    color: var(--color-text);
  }

  .log-entry__raw-actions button:focus-visible {
    outline: 2px solid rgba(var(--color-text-rgb), 0.4);
    outline-offset: 2px;
  }

  .log-entry__raw pre {
    margin: 0.6rem 0 0;
    background: rgba(var(--color-text-rgb), 0.05);
    padding: 0.6rem;
    border-radius: 12px;
    font-size: 0.75rem;
    white-space: pre-wrap;
    word-break: break-word;
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

  @media (max-width: 720px) {
    .logs__header {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
