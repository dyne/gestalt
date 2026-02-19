<script>
  import { onDestroy, onMount } from 'svelte'
  import { createDashboardStore } from '../lib/dashboardStore.js'
  import { getErrorMessage } from '../lib/errorUtils.js'
  import { formatRelativeTime } from '../lib/timeUtils.js'
  import { formatLogEntryForClipboard } from '../lib/logEntry.js'
  import { parseConventionalCommit } from '../lib/conventionalCommit.js'
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
  let gitLog = { branch: '', commits: [] }
  let gitLogLoading = false
  let gitLogError = ''
  let gitLogAutoRefresh = true
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

  const formatCount = (value) => {
    const numeric = Number(value)
    if (Number.isNaN(numeric)) {
      return '—'
    }
    return numberFormatter.format(numeric)
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

  const handleGitLogAutoRefreshChange = (event) => {
    dashboardStore.setGitLogAutoRefresh(event.target.checked)
  }

  const refreshGitLog = () => {
    dashboardStore.loadGitLog()
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

  const formatLineDelta = (value, prefix) => {
    const numeric = Number(value)
    if (Number.isNaN(numeric)) return `${prefix}0`
    return `${prefix}${numberFormatter.format(numeric)}`
  }

  const formatGitLogTime = (value) => {
    return formatRelativeTime(value) || '—'
  }

  const gitBranchDisplay = (value) => {
    if (!value) return 'not a git repo'
    return value
  }

  const gitFileDelta = (file) => {
    if (!file || file.binary) return 'binary'
    return `${formatLineDelta(file.added, '+')} / ${formatLineDelta(file.deleted, '-')}`
  }

  const commitConventional = (commit) => {
    return parseConventionalCommit(commit?.subject || '')
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
    gitLog,
    gitLogLoading,
    gitLogError,
    gitLogAutoRefresh,
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
              <span class="agent-name" title={agent.name}>{agent.name}</span>
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

    <section class="dashboard__gitlog">
      <div class="list-header">
        <div>
          <h2>Git log</h2>
          <p class="subtle">{gitBranchDisplay(gitLog?.branch)}</p>
        </div>
        <div class="gitlog-controls">
          <label class="gitlog-control gitlog-control--toggle">
            <input
              type="checkbox"
              bind:checked={gitLogAutoRefresh}
              on:change={handleGitLogAutoRefreshChange}
            />
            <span>Auto refresh</span>
          </label>
          <button
            class="gitlog-refresh"
            type="button"
            on:click={refreshGitLog}
            disabled={gitLogLoading}
          >
            {gitLogLoading ? 'Refreshing…' : 'Refresh'}
          </button>
        </div>
      </div>

      {#if gitLogError}
        <p class="error">{gitLogError}</p>
      {/if}

      <div class="gitlog-list">
      {#if gitLogLoading && (gitLog?.commits || []).length === 0}
        <p class="muted">Loading commits…</p>
      {:else if !(gitLog?.branch || '').trim() && (gitLog?.commits || []).length === 0}
        <p class="muted">Not a git repo.</p>
      {:else if (gitLog?.commits || []).length === 0}
        <p class="muted">No commits found.</p>
      {:else}
        <ul>
          {#each gitLog.commits as commit (commit.sha)}
            {@const conventional = commitConventional(commit)}
            <li class="gitlog-entry">
              <details>
                <summary class="gitlog-entry__summary">
                  <div class="gitlog-entry__line">
                    <div class="gitlog-subject-wrap">
                      {#if conventional.type}
                        <span
                          class={`conventional-badge ${conventional.badgeClass}`}
                          title={conventional.type}
                        >
                          {conventional.type}
                        </span>
                      {/if}
                      <span class="gitlog-subject">{conventional.displayTitle || 'No subject'}</span>
                    </div>
                    <span class="gitlog-time" title={commit.committed_at || ''}>
                      {formatGitLogTime(commit.committed_at)}
                    </span>
                  </div>
                  <div class="gitlog-entry__meta">
                    <span class="gitlog-sha">{commit.short_sha}</span>
                    <span class="gitlog-delta">
                      {formatLineDelta(commit?.stats?.lines_added, '+')} /
                      {formatLineDelta(commit?.stats?.lines_deleted, '-')}
                    </span>
                    <span class="gitlog-files">{formatCount(commit?.stats?.files_changed)} file(s)</span>
                  </div>
                </summary>
                <div class="gitlog-entry__details">
                  {#if commit.files_truncated}
                    <p class="muted">Showing first {formatCount((commit.files || []).length)} files.</p>
                  {/if}
                  {#if (commit.files || []).length === 0}
                    <p class="muted">No file stats available.</p>
                  {:else}
                    <ul>
                      {#each commit.files as file}
                        <li class="gitlog-file">
                          <span class="gitlog-file__path">{file.path}</span>
                          <span class="gitlog-file__delta">{gitFileDelta(file)}</span>
                        </li>
                      {/each}
                    </ul>
                  {/if}
                </div>
              </details>
            </li>
          {/each}
        </ul>
      {/if}
      </div>
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

  .dashboard__gitlog {
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

  .gitlog-controls {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    flex-wrap: wrap;
  }

  .gitlog-control {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    font-size: 0.75rem;
    color: var(--color-text-muted);
  }

  .gitlog-control--toggle {
    flex-direction: row;
    align-items: center;
    gap: 0.5rem;
  }

  .gitlog-refresh {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.45rem 0.95rem;
    background: var(--color-surface);
    font-size: 0.7rem;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    cursor: pointer;
  }

  .gitlog-refresh:disabled {
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

  .gitlog-list {
    max-height: 28rem;
    overflow: auto;
  }

  .gitlog-list ul {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
  }

  .gitlog-entry {
    margin: 0;
  }

  .gitlog-entry details {
    border-radius: 16px;
  }

  .gitlog-entry__summary {
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

  .gitlog-entry__summary::-webkit-details-marker {
    display: none;
  }

  .gitlog-entry__summary::marker {
    content: '';
  }

  .gitlog-entry__summary:focus-visible {
    outline: 2px solid rgba(var(--color-text-rgb), 0.4);
    outline-offset: 2px;
  }

  .gitlog-entry details[open] .gitlog-entry__summary {
    border-bottom-left-radius: 0;
    border-bottom-right-radius: 0;
  }

  .gitlog-entry__details {
    border: 1px solid rgba(var(--color-text-rgb), 0.06);
    border-top: none;
    padding: 0.65rem 0.7rem;
    border-bottom-left-radius: 16px;
    border-bottom-right-radius: 16px;
    background: rgba(var(--color-surface-rgb), 0.7);
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
  }

  .gitlog-entry__line {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.6rem;
  }

  .gitlog-subject-wrap {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    min-width: 0;
  }

  .gitlog-subject {
    color: var(--color-text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .conventional-badge {
    border-radius: 999px;
    padding: 0.1rem 0.45rem;
    font-size: 0.66rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    border: 1px solid transparent;
    color: var(--color-text);
    background: rgba(var(--color-text-rgb), 0.1);
    white-space: nowrap;
  }

  .conventional-badge--feat {
    color: rgb(var(--color-success-rgb));
    border-color: rgba(var(--color-success-rgb), 0.4);
    background: rgba(var(--color-success-rgb), 0.14);
  }

  .conventional-badge--fix {
    color: rgb(var(--color-warning-rgb));
    border-color: rgba(var(--color-warning-rgb), 0.4);
    background: rgba(var(--color-warning-rgb), 0.16);
  }

  .conventional-badge--docs {
    color: rgb(var(--color-info-rgb));
    border-color: rgba(var(--color-info-rgb), 0.4);
    background: rgba(var(--color-info-rgb), 0.16);
  }

  .conventional-badge--refactor {
    color: #7e57c2;
    border-color: rgba(126, 87, 194, 0.4);
    background: rgba(126, 87, 194, 0.16);
  }

  .conventional-badge--chore,
  .conventional-badge--ci,
  .conventional-badge--build,
  .conventional-badge--test,
  .conventional-badge--perf,
  .conventional-badge--style,
  .conventional-badge--revert,
  .conventional-badge--default {
    color: var(--color-text-subtle);
    border-color: rgba(var(--color-text-rgb), 0.3);
    background: rgba(var(--color-text-rgb), 0.08);
  }

  .gitlog-time {
    font-size: 0.75rem;
    color: var(--color-text-subtle);
    white-space: nowrap;
  }

  .gitlog-entry__meta {
    display: flex;
    gap: 0.55rem;
    flex-wrap: wrap;
    align-items: center;
  }

  .gitlog-sha,
  .gitlog-delta,
  .gitlog-files,
  .gitlog-file__delta {
    font-family: "IBM Plex Mono", "SFMono-Regular", Menlo, monospace;
    font-size: 0.75rem;
    color: var(--color-text-subtle);
  }

  .gitlog-file {
    display: flex;
    gap: 0.6rem;
    justify-content: space-between;
    align-items: center;
    font-size: 0.85rem;
  }

  .gitlog-file__path {
    color: var(--color-text);
    overflow-wrap: anywhere;
  }

  .gitlog-file__delta {
    white-space: nowrap;
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
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 0.6rem;
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
    padding: 0.55rem 0.8rem;
    background: var(--color-surface);
    font-weight: 600;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    cursor: pointer;
    transition: transform 160ms ease, box-shadow 160ms ease, opacity 160ms ease,
      background 160ms ease, border-color 160ms ease;
    width: 100%;
    height: 100%;
    min-height: 2.5rem;
  }

  .agent-name {
    font-size: 0.9rem;
    display: inline-flex;
    align-items: center;
    min-width: 0;
    flex: 1 1 auto;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
    flex: 0 0 auto;
    white-space: nowrap;
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
