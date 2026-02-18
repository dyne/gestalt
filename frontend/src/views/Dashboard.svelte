<script>
  import { onDestroy, onMount } from 'svelte'
  import { createDashboardStore } from '../lib/dashboardStore.js'
  import { getErrorMessage } from '../lib/errorUtils.js'
  import { formatRelativeTime } from '../lib/timeUtils.js'
  import { formatLogEntryForClipboard } from '../lib/logEntry.js'
  import { notificationStore } from '../lib/notificationStore.js'
  import { canUseClipboard } from '../lib/clipboard.js'

  export let terminals = []
  export let status = null
  export let loading = false
  export let error = ''
  export let onCreate = () => {}
  export let onSelect = () => {}

  const dashboardStore = createDashboardStore()

  let actionPending = false
  let localError = ''
  let agents = []
  let visibleAgents = []
  let agentsLoading = false
  let agentsError = ''
  let logs = []
  let orderedLogs = []
  let visibleLogs = []
  let logsLoading = false
  let logsError = ''
  let logLevelFilter = 'info'
  let logsAutoRefresh = true
  let metricsSummary = null
  let metricsLoading = false
  let metricsError = ''
  let metricsAutoRefresh = true
  let configExtractionCount = 0
  let configExtractionLast = ''
  let clipboardAvailable = false

  const logLevelOptions = [
    { value: 'debug', label: 'Debug' },
    { value: 'info', label: 'Info' },
    { value: 'warning', label: 'Warning' },
    { value: 'error', label: 'Error' },
  ]

  const numberFormatter = new Intl.NumberFormat('en-US')

  const createTerminal = async (agentId = '') => {
    actionPending = true
    localError = ''
    try {
      await onCreate(agentId)
      await dashboardStore.loadAgents()
    } catch (err) {
      localError = getErrorMessage(err, 'Failed to create session.')
    } finally {
      actionPending = false
    }
  }

  const switchToTerminal = (sessionId) => {
    if (!sessionId) {
      localError = 'No running session found.'
      return
    }
    onSelect(sessionId)
  }

  const formatLogTime = (value) => {
    return formatRelativeTime(value) || '—'
  }

  const formatMetricsTime = (value) => {
    return formatRelativeTime(value) || '—'
  }

  const formatCount = (value) => {
    const numeric = Number(value)
    if (Number.isNaN(numeric)) {
      return '—'
    }
    return numberFormatter.format(numeric)
  }

  const formatDuration = (value) => {
    const numeric = Number(value)
    if (Number.isNaN(numeric)) {
      return '—'
    }
    if (numeric < 1) {
      return `${Math.round(numeric * 1000)}ms`
    }
    if (numeric < 10) {
      return `${numeric.toFixed(2)}s`
    }
    return `${numeric.toFixed(1)}s`
  }

  const formatPercent = (value) => {
    const numeric = Number(value)
    if (Number.isNaN(numeric)) {
      return '—'
    }
    return `${numeric.toFixed(1)}%`
  }

  const handleLogFilterChange = (event) => {
    dashboardStore.setLogLevelFilter(event.target.value)
  }

  const handleLogsAutoRefreshChange = (event) => {
    dashboardStore.setLogsAutoRefresh(event.target.checked)
  }

  const refreshLogs = () => {
    dashboardStore.loadLogs()
  }

  const handleMetricsAutoRefreshChange = (event) => {
    dashboardStore.setMetricsAutoRefresh(event.target.checked)
  }

  const refreshMetrics = () => {
    dashboardStore.loadMetricsSummary()
  }

  const gitBranchName = (origin, branch) => {
    if (!branch) return ''
    const normalized = String(branch)
    const originValue = origin ? String(origin) : ''
    if (originValue && normalized.startsWith(`${originValue}/`)) {
      return normalized.slice(originValue.length + 1)
    }
    return normalized
  }

  const logEntryKey = (entry, index) => entry?.id || `${entry.timestampISO}-${entry.message}-${index}`

  const metricsKey = (prefix, entry, index, field) => {
    const raw = entry?.[field]
    const value = raw ? String(raw) : ''
    if (value) return `${prefix}:${value}`
    return `${prefix}:${index}`
  }

  $: visibleAgents = agents.filter((agent) => !agent?.hidden)

  const attributeEntriesFor = (entry) => {
    return Object.entries(entry?.attributes || {}).sort(([left], [right]) =>
      left.localeCompare(right),
    )
  }

  const resourceEntriesFor = (entry) => {
    return Object.entries(entry?.resourceAttributes || {}).sort(([left], [right]) =>
      left.localeCompare(right),
    )
  }

  const notifySummaryChips = (entry) => {
    const attributes = entry?.attributes || {}
    const chips = []
    const notifyType = attributes['notify.type']
    const sessionID = attributes['session.id'] || attributes['session_id']
    const agentID = attributes['agent.id'] || attributes['agent_id']
    if (notifyType) {
      chips.push(`notify:${notifyType}`)
    }
    if (sessionID) {
      chips.push(`session:${sessionID}`)
    }
    if (agentID) {
      chips.push(`agent:${agentID}`)
    }
    return chips
  }
  const copyText = async (text, successMessage) => {
    if (!clipboardAvailable) {
      notificationStore.addNotification('error', 'Clipboard requires HTTPS.')
      return
    }
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

  $: ({
    agents,
    agentsLoading,
    agentsError,
    logs,
    logsLoading,
    logsError,
    logLevelFilter,
    logsAutoRefresh,
    metricsSummary,
    metricsLoading,
    metricsError,
    metricsAutoRefresh,
    configExtractionCount,
    configExtractionLast,
  } = $dashboardStore)

  $: dashboardStore.setTerminals(terminals)
  $: dashboardStore.setStatus(status)
  $: orderedLogs = [...logs].reverse()
  $: visibleLogs = orderedLogs.slice(0, 30)
  $: clipboardAvailable = canUseClipboard()

  onMount(() => {
    dashboardStore.start()
  })

  onDestroy(() => {
    dashboardStore.stop()
  })
</script>

<section class="dashboard" data-terminal-count={terminals.length}>
  <section class="dashboard__status">
    <div class="status-card status-card--wide">
      <div class="status-meta">
        <div class="status-item">
          <span class="label">Workdir</span>
          {#if clipboardAvailable}
            <button
              class="status-pill status-pill--path"
              type="button"
              on:click={() => copyText(status?.working_dir || '', 'Copied workdir.')}
            >
              {status?.working_dir || '—'}
            </button>
          {:else}
            <span class="status-pill status-pill--path status-pill--static">
              {status?.working_dir || '—'}
            </span>
          {/if}
        </div>
        <div class="status-item">
          <span class="label">Git remote</span>
          {#if clipboardAvailable}
            <button
              class="status-pill status-pill--git"
              type="button"
              on:click={() => copyText(status?.git_origin || '', 'Copied git remote.')}
            >
              {status?.git_origin || '—'}
            </button>
          {:else}
            <span class="status-pill status-pill--git status-pill--static">
              {status?.git_origin || '—'}
            </span>
          {/if}
        </div>
        <div class="status-item">
          <span class="label">Git branch</span>
          {#if clipboardAvailable}
            <button
              class="status-pill status-pill--git"
              type="button"
              on:click={() =>
                copyText(gitBranchName(status?.git_origin, status?.git_branch), 'Copied git branch.')
              }
            >
              {gitBranchName(status?.git_origin, status?.git_branch) || '—'}
            </button>
          {:else}
            <span class="status-pill status-pill--git status-pill--static">
              {gitBranchName(status?.git_origin, status?.git_branch) || '—'}
            </span>
          {/if}
        </div>
      </div>
    </div>
    {#if configExtractionCount > 0}
      <div class="status-card">
        <span class="label">Config extraction</span>
        <span class="value">{configExtractionCount} file(s)</span>
        {#if configExtractionLast}
          <span class="status-pill status-pill--path">{configExtractionLast}</span>
        {/if}
      </div>
    {/if}
  </section>

  <section class="dashboard__agents">
    <div class="list-header">
      <h2>Agents</h2>
    </div>

    {#if agentsLoading}
      <p class="muted">Loading agents…</p>
    {:else if agentsError}
      <p class="error">{agentsError}</p>
    {:else if agents.length === 0}
      <p class="muted">No agent profiles found.</p>
    {:else if visibleAgents.length === 0}
      <p class="muted">All agents are hidden.</p>
    {:else}
      <div class="agent-grid">
        {#each visibleAgents as agent}
          <div class="agent-card">
            <button
              class="agent-button"
              class:agent-button--running={agent.running}
              class:agent-button--stopped={!agent.running}
              on:click={() =>
                agent.running ? switchToTerminal(agent.session_id) : createTerminal(agent.id)
              }
              disabled={actionPending || loading}
            >
              <span class="agent-name">{agent.name}</span>
              <span class="agent-action">{agent.running ? 'Open' : 'Start'}</span>
            </button>
          </div>
        {/each}
      </div>
    {/if}

    {#if error || localError}
      <p class="error">{error || localError}</p>
    {/if}
  </section>

  <section class="dashboard__intel">
    <section class="dashboard__logs">
      <div class="list-header">
        <h2>Recent logs</h2>
      </div>

      <div class="logs-controls">
        <label class="logs-control">
          <span>Level</span>
          <select on:change={handleLogFilterChange} bind:value={logLevelFilter}>
            {#each logLevelOptions as option}
              <option value={option.value}>{option.label}</option>
            {/each}
          </select>
        </label>
        <label class="logs-control logs-control--toggle">
          <input
            type="checkbox"
            bind:checked={logsAutoRefresh}
            on:change={handleLogsAutoRefreshChange}
          />
          <span>Live updates</span>
        </label>
        <button class="logs-refresh" type="button" on:click={refreshLogs} disabled={logsLoading}>
          {logsLoading ? 'Refreshing…' : 'Refresh'}
        </button>
      </div>

      <div class="logs-list">
        {#if logsLoading && logs.length === 0}
          <p class="muted">Loading logs…</p>
        {:else if logsError}
          <p class="error">{logsError}</p>
        {:else if visibleLogs.length === 0}
          <p class="muted">No logs yet.</p>
        {:else}
          <ul>
            {#each visibleLogs as entry, index (logEntryKey(entry, index))}
              <li class={`log-entry log-entry--${entry.level}`}>
                <details class="log-entry__details">
                  <summary class="log-entry__summary">
                    <div class="log-entry__meta">
                      <span class="log-badge">{entry.level}</span>
                      <span class="log-time" title={entry.timestampISO || ''}>
                        {formatLogTime(entry.timestampISO)}
                      </span>
                    </div>
                    <p class="log-message">{entry.message}</p>
                    {#if notifySummaryChips(entry).length > 0}
                      <div class="log-entry__chips">
                        {#each notifySummaryChips(entry) as chip}
                          <span class="log-chip">{chip}</span>
                        {/each}
                      </div>
                    {/if}
                  </summary>
                  <div class="log-entry__details-body">
                    <div class="log-entry__detail-section">
                      <span class="log-entry__label">Attributes</span>
                      {#if attributeEntriesFor(entry).length === 0}
                        <p class="log-entry__empty">No attributes.</p>
                      {:else}
                        <div class="log-entry__context">
                          <table>
                            <tbody>
                              {#each attributeEntriesFor(entry) as [key, value]}
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
                    <div class="log-entry__detail-section">
                      <span class="log-entry__label">Resource</span>
                      {#if resourceEntriesFor(entry).length === 0}
                        <p class="log-entry__empty">No resource attributes.</p>
                      {:else}
                        <div class="log-entry__context">
                          <table>
                            <tbody>
                              {#each resourceEntriesFor(entry) as [key, value]}
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
                    {#if entry.scopeName}
                      <div class="log-entry__detail-section">
                        <span class="log-entry__label">Scope</span>
                        <p class="log-entry__empty">{entry.scopeName}</p>
                      </div>
                    {/if}
                    {#if entry.raw}
                      <details class="log-entry__raw">
                        <summary>Raw JSON</summary>
                        <div class="log-entry__raw-body">
                          {#if clipboardAvailable}
                            <div class="log-entry__raw-actions">
                              <button type="button" on:click={() => copyLogJson(entry)}>
                                Copy JSON
                              </button>
                            </div>
                          {/if}
                          <pre>{JSON.stringify(entry.raw, null, 2)}</pre>
                        </div>
                      </details>
                    {/if}
                  </div>
                </details>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    </section>

    <section class="dashboard__metrics">
      <div class="list-header">
        <div>
          <h2>API metrics</h2>
          <p class="subtle">
            Updated {metricsSummary?.updated_at ? formatMetricsTime(metricsSummary.updated_at) : '—'}
          </p>
        </div>
        <div class="metrics-controls">
          <label class="metrics-control metrics-control--toggle">
            <input
              type="checkbox"
              bind:checked={metricsAutoRefresh}
              on:change={handleMetricsAutoRefreshChange}
            />
            <span>Auto refresh</span>
          </label>
          <button
            class="metrics-refresh"
            type="button"
            on:click={refreshMetrics}
            disabled={metricsLoading}
          >
            {metricsLoading ? 'Refreshing…' : 'Refresh'}
          </button>
        </div>
      </div>

      {#if metricsError}
        <p class="error">{metricsError}</p>
      {/if}

      {#if metricsLoading && !metricsSummary}
        <p class="muted">Loading metrics…</p>
      {:else if !metricsSummary}
        <p class="muted">No metrics yet.</p>
      {:else}
        <div class="metrics-grid">
          <details class="metrics-card">
            <summary class="metrics-card__summary">
              <div class="metrics-card__header">
                <h3>Top endpoints</h3>
                <span class="metrics-pill">Requests</span>
              </div>
            </summary>
            <div class="metrics-card__body">
              {#if (metricsSummary.top_endpoints || []).length === 0}
                <p class="muted">No traffic yet.</p>
              {:else}
                <ul class="metrics-list">
                  {#each metricsSummary.top_endpoints as entry, index (metricsKey('top-endpoints', entry, index, 'route'))}
                    <li>
                      <span class="metric-label metric-label--mono">{entry.route}</span>
                      <span class="metric-value">{formatCount(entry.count)}</span>
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          </details>

          <details class="metrics-card">
            <summary class="metrics-card__summary">
              <div class="metrics-card__header">
                <h3>Slowest endpoints</h3>
                <span class="metrics-pill">p99 latency</span>
              </div>
            </summary>
            <div class="metrics-card__body">
              {#if (metricsSummary.slowest_endpoints || []).length === 0}
                <p class="muted">No latency data yet.</p>
              {:else}
                <ul class="metrics-list">
                  {#each metricsSummary.slowest_endpoints as entry, index (metricsKey('slowest-endpoints', entry, index, 'route'))}
                    <li>
                      <div class="metric-stack">
                        <span class="metric-label metric-label--mono">{entry.route}</span>
                        <span class="metric-detail">{formatCount(entry.count)} request(s)</span>
                      </div>
                      <span class="metric-value">{formatDuration(entry.p99_seconds)}</span>
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          </details>

          <details class="metrics-card">
            <summary class="metrics-card__summary">
              <div class="metrics-card__header">
                <h3>Top agents</h3>
                <span class="metrics-pill">Requests</span>
              </div>
            </summary>
            <div class="metrics-card__body">
              {#if (metricsSummary.top_agents || []).length === 0}
                <p class="muted">No agent traffic yet.</p>
              {:else}
                <ul class="metrics-list">
                  {#each metricsSummary.top_agents as entry, index (metricsKey('top-agents', entry, index, 'name'))}
                    <li>
                      <span class="metric-label">{entry.name}</span>
                      <span class="metric-value">{formatCount(entry.count)}</span>
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          </details>

          <details class="metrics-card">
            <summary class="metrics-card__summary">
              <div class="metrics-card__header">
                <h3>Error rates</h3>
                <span class="metrics-pill">By category</span>
              </div>
            </summary>
            <div class="metrics-card__body">
              {#if (metricsSummary.error_rates || []).length === 0}
                <p class="muted">No errors recorded.</p>
              {:else}
                <ul class="metrics-list metrics-list--stacked">
                  {#each metricsSummary.error_rates as entry, index (metricsKey('error-rates', entry, index, 'category'))}
                    <li>
                      <div class="metric-row">
                        <span class="metric-label">{entry.category}</span>
                        <span class="metric-value">{formatPercent(entry.error_rate_pct)}</span>
                      </div>
                      <span class="metric-detail">
                        {formatCount(entry.errors)} errors / {formatCount(entry.total)} total
                      </span>
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          </details>
        </div>
      {/if}
    </section>
  </section>
</section>

<style>
  :global(body) {
    background: var(--gradient-bg);
  }

  .dashboard {
    padding: 2.5rem clamp(1.5rem, 4vw, 3.5rem) 3.5rem;
    display: flex;
    flex-direction: column;
    gap: 2.5rem;
  }

  .cta {
    border: none;
    border-radius: 999px;
    padding: 0.85rem 1.6rem;
    font-size: 0.95rem;
    font-weight: 600;
    background: var(--color-contrast-bg);
    color: var(--color-contrast-text);
    cursor: pointer;
    transition: transform 160ms ease, box-shadow 160ms ease, opacity 160ms ease;
    box-shadow: 0 10px 30px rgba(var(--shadow-color-rgb), 0.2);
  }

  .cta:disabled {
    cursor: not-allowed;
    opacity: 0.6;
    transform: none;
    box-shadow: none;
  }

  .cta:not(:disabled):hover {
    transform: translateY(-2px);
  }

  .dashboard__status {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 1rem;
  }

  .status-card {
    padding: 1.5rem;
    border-radius: 20px;
    background: rgba(var(--color-surface-rgb), 0.85);
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    box-shadow: 0 20px 50px rgba(var(--shadow-color-rgb), 0.08);
  }

  .label {
    display: block;
    font-size: 0.8rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .value {
    display: block;
    margin-top: 0.35rem;
    font-size: 1.6rem;
    color: var(--color-text);
  }

  .status-card--wide {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .status-meta {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 0.85rem 1rem;
    align-items: start;
  }

  .status-item {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .status-pill {
    display: inline-flex;
    align-items: center;
    padding: 0.35rem 0.6rem;
    border-radius: 999px;
    background: rgba(var(--color-surface-rgb), 0.85);
    border: 1px solid rgba(var(--color-text-rgb), 0.12);
    font-family: "IBM Plex Mono", "SFMono-Regular", Menlo, monospace;
    font-size: 0.85rem;
    word-break: break-all;
    cursor: pointer;
    text-align: left;
    font: inherit;
  }

  .status-pill:focus-visible {
    outline: 2px solid rgba(var(--color-text-rgb), 0.4);
    outline-offset: 2px;
  }

  .status-pill--static {
    cursor: text;
    user-select: text;
  }

  .status-pill--path {
    font-size: 0.95rem;
  }

  .status-pill--git {
    color: var(--color-text-subtle);
  }

  .dashboard__agents {
    padding: 1.5rem;
    border-radius: 24px;
    background: rgba(var(--color-warning-rgb), 0.12);
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
  }

  .dashboard__metrics {
    padding: 1.5rem;
    border-radius: 24px;
    background: rgba(var(--color-success-rgb), 0.08);
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .dashboard__intel {
    display: grid;
    grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.9fr);
    gap: 1.5rem;
    align-items: start;
  }

  .dashboard__logs {
    padding: 1.25rem;
    border-radius: 24px;
    background: rgba(var(--color-info-rgb), 0.08);
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
  }

  .logs-controls {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .metrics-controls {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    flex-wrap: wrap;
  }

  .metrics-control {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    font-size: 0.75rem;
    color: var(--color-text-muted);
  }

  .metrics-control--toggle {
    flex-direction: row;
    align-items: center;
    gap: 0.5rem;
  }

  .metrics-refresh {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.45rem 0.95rem;
    background: var(--color-surface);
    font-size: 0.7rem;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    cursor: pointer;
  }

  .metrics-refresh:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  .logs-control {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    font-size: 0.72rem;
    color: var(--color-text-muted);
  }

  .logs-control select {
    border: 1px solid rgba(var(--color-text-rgb), 0.16);
    border-radius: 10px;
    padding: 0.35rem 0.6rem;
    font-size: 0.85rem;
    background: var(--color-surface);
    color: var(--color-text);
  }

  .logs-control--toggle {
    flex-direction: row;
    align-items: center;
    gap: 0.5rem;
  }

  .logs-refresh {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.45rem 0.95rem;
    background: var(--color-surface);
    font-size: 0.7rem;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    cursor: pointer;
  }

  .logs-refresh:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  .logs-list ul {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
  }

  .metrics-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 1rem;
  }

  .metrics-card {
    padding: 1rem 1.1rem;
    border-radius: 16px;
    background: var(--color-surface);
    border: 1px solid rgba(var(--color-text-rgb), 0.06);
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
  }

  .metrics-card__summary {
    list-style: none;
    cursor: pointer;
  }

  .metrics-card__summary::-webkit-details-marker {
    display: none;
  }

  .metrics-card__summary::marker {
    content: '';
  }

  .metrics-card__summary:focus-visible {
    outline: 2px solid rgba(var(--color-text-rgb), 0.35);
    outline-offset: 2px;
  }

  .metrics-card__body {
    margin-top: 0.85rem;
  }

  .metrics-card__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
  }

  .metrics-card__header h3 {
    margin: 0;
    font-size: 0.95rem;
  }

  .metrics-pill {
    padding: 0.2rem 0.55rem;
    border-radius: 999px;
    border: 1px solid rgba(var(--color-text-rgb), 0.12);
    font-size: 0.65rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--color-text-subtle);
  }

  .metrics-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .metrics-list li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    font-size: 0.85rem;
  }

  .metrics-list--stacked li {
    align-items: flex-start;
    flex-direction: column;
  }

  .metric-row {
    display: flex;
    justify-content: space-between;
    gap: 0.6rem;
    width: 100%;
  }

  .metric-stack {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .metric-label {
    color: var(--color-text);
  }

  .metric-label--mono {
    font-family: "IBM Plex Mono", "SFMono-Regular", Menlo, monospace;
    font-size: 0.8rem;
  }

  .metric-value {
    font-weight: 600;
    color: var(--color-text);
  }

  .metric-detail {
    font-size: 0.75rem;
    color: var(--color-text-subtle);
  }

  .log-entry {
    margin: 0;
  }

  .log-entry__details {
    border-radius: 16px;
  }

  .log-entry__summary {
    width: 100%;
    padding: 0.55rem 0.7rem;
    border-radius: 16px;
    background: var(--color-surface);
    border: 1px solid rgba(var(--color-text-rgb), 0.06);
    display: flex;
    flex-direction: column;
    gap: 0.28rem;
    text-align: left;
    cursor: pointer;
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

  .log-entry__details-body {
    border: 1px solid rgba(var(--color-text-rgb), 0.06);
    border-top: none;
    padding: 0.65rem 0.7rem;
    border-bottom-left-radius: 16px;
    border-bottom-right-radius: 16px;
    background: rgba(var(--color-surface-rgb), 0.7);
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .log-entry__detail-section {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
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
    padding: 0.4rem 0.6rem;
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

  .log-entry__raw-body {
    margin-top: 0.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .log-entry__raw-actions {
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
    margin: 0;
    background: rgba(var(--color-text-rgb), 0.05);
    padding: 0.6rem;
    border-radius: 12px;
    font-size: 0.75rem;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .log-entry__meta {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    font-size: 0.75rem;
    color: var(--color-text-subtle);
  }

  .log-badge {
    padding: 0.16rem 0.45rem;
    border-radius: 999px;
    background: var(--color-contrast-bg);
    color: var(--color-contrast-text);
    font-size: 0.62rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .log-entry--info .log-badge {
    background: var(--color-info);
  }

  .log-entry--debug .log-badge {
    background: var(--color-border);
    color: var(--color-text);
  }

  .log-entry--warning .log-badge {
    background: var(--color-warning);
  }

  .log-entry--error .log-badge {
    background: var(--color-danger);
  }

  .log-message {
    margin: 0;
    font-size: 0.82rem;
    color: var(--color-text);
  }

  .log-entry__chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    align-items: center;
  }

  .log-chip {
    display: inline-flex;
    align-items: center;
    max-width: 100%;
    padding: 0.1rem 0.45rem;
    border-radius: 999px;
    border: 1px solid rgba(var(--color-text-rgb), 0.15);
    background: rgba(var(--color-surface-rgb), 0.72);
    color: var(--color-text-subtle);
    font-family: "IBM Plex Mono", "SFMono-Regular", Menlo, monospace;
    font-size: 0.68rem;
    line-height: 1.3;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .agent-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: 0.75rem;
  }

  .agent-card {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.5rem;
    align-items: stretch;
  }

  .agent-button {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 14px;
    padding: 0.75rem 1rem;
    background: var(--color-surface);
    font-weight: 600;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.35rem;
    cursor: pointer;
    transition: transform 160ms ease, box-shadow 160ms ease, opacity 160ms ease,
      background 160ms ease, border-color 160ms ease;
    width: 100%;
    height: 100%;
  }

  .agent-name {
    font-size: 0.95rem;
  }

  .agent-button--running {
    background: rgba(var(--color-success-rgb), 0.12);
    border-color: rgba(var(--color-success-rgb), 0.35);
    box-shadow: 0 12px 20px rgba(var(--color-success-rgb), 0.12);
  }

  .agent-button--stopped {
    background: var(--color-surface);
    border-color: rgba(var(--color-text-rgb), 0.16);
  }

  .agent-button--running .agent-name::before {
    content: '';
    display: inline-block;
    width: 0.45rem;
    height: 0.45rem;
    border-radius: 999px;
    margin-right: 0.4rem;
    background: var(--color-success);
    box-shadow: 0 0 0 0 rgba(var(--color-success-rgb), 0.4);
    animation: pulseDot 2.4s ease-in-out infinite;
  }

  .agent-action {
    font-size: 0.7rem;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    color: var(--color-text-subtle);
  }

  @keyframes pulseDot {
    0% {
      box-shadow: 0 0 0 0 rgba(var(--color-success-rgb), 0.4);
    }
    70% {
      box-shadow: 0 0 0 0.4rem rgba(var(--color-success-rgb), 0);
    }
    100% {
      box-shadow: 0 0 0 0 rgba(var(--color-success-rgb), 0);
    }
  }

  .agent-button:disabled {
    cursor: not-allowed;
    opacity: 0.6;
    transform: none;
    box-shadow: none;
  }

  .agent-button:not(:disabled):hover {
    transform: translateY(-2px);
    box-shadow: 0 12px 20px rgba(var(--shadow-color-rgb), 0.12);
  }

  .list-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 1rem;
  }

  .subtle {
    margin: 0;
    font-size: 0.9rem;
    color: var(--color-text-subtle);
  }

  .muted {
    color: var(--color-text-subtle);
    margin: 0.5rem 0 0;
  }

  .error {
    color: var(--color-danger);
    margin: 0.5rem 0 0;
  }

  @media (max-width: 720px) {
    .list-header {
      flex-direction: column;
      align-items: flex-start;
      gap: 0.4rem;
    }

    .dashboard__intel {
      grid-template-columns: 1fr;
    }

    .logs-controls {
      align-items: flex-start;
    }
  }

  @media (max-width: 640px) {
    .log-entry__summary {
      padding: 0.5rem 0.6rem;
    }

    .log-entry__details-body {
      padding: 0.55rem 0.6rem;
    }

    .log-entry__chips {
      gap: 0.3rem;
    }

    .log-chip {
      white-space: normal;
      overflow-wrap: anywhere;
    }
  }
</style>
