<script>
  import { onDestroy, onMount } from 'svelte'
  import { apiFetch } from '../lib/api.js'
  import { subscribe as subscribeAgentEvents } from '../lib/agentEventStore.js'
  import { subscribe as subscribeConfigEvents } from '../lib/configEventStore.js'
  import { subscribe as subscribeEvents } from '../lib/eventStore.js'
  import { notificationStore } from '../lib/notificationStore.js'
  import { formatRelativeTime } from '../lib/timeUtils.js'

  export let terminals = []
  export let status = null
  export let loading = false
  export let error = ''
  export let onCreate = () => {}
  export let onSelect = () => {}

  let actionPending = false
  let localError = ''
  let agents = []
  let agentsLoading = false
  let agentsError = ''
  let agentSkills = {}
  let agentSkillsLoading = false
  let agentSkillsError = ''
  let logs = []
  let orderedLogs = []
  let visibleLogs = []
  let logsLoading = false
  let logsError = ''
  let logLevelFilter = 'all'
  let logsAutoRefresh = true
  let logsRefreshTimer = null
  let logsMounted = false
  let lastLogErrorMessage = ''
  let agentEventsUnsubscribes = []
  let configEventsUnsubscribes = []
  let configExtractionCount = 0
  let configExtractionLast = ''
  let configExtractionTimer = null
  let gitOrigin = ''
  let gitBranch = ''
  let gitContext = 'not a git repo'
  let gitUnsubscribe = null

  const logLevelOptions = [
    { value: 'all', label: 'All' },
    { value: 'info', label: 'Info' },
    { value: 'warning', label: 'Warning' },
    { value: 'error', label: 'Error' },
  ]

  const createTerminal = async (agentId = '') => {
    actionPending = true
    localError = ''
    try {
      await onCreate(agentId)
      await loadAgents()
    } catch (err) {
      localError = err?.message || 'Failed to create terminal.'
    } finally {
      actionPending = false
    }
  }

  const switchToTerminal = (terminalId) => {
    if (!terminalId) {
      localError = 'No running terminal found.'
      return
    }
    onSelect(terminalId)
  }

  const loadAgents = async () => {
    agentsLoading = true
    agentsError = ''
    try {
      const response = await apiFetch('/api/agents')
      const fetched = await response.json()
      const nextAgents = syncAgentRunning(fetched, terminals)
      agents = nextAgents
      await loadAgentSkills(nextAgents)
    } catch (err) {
      agentsError = err?.message || 'Failed to load agents.'
    } finally {
      agentsLoading = false
    }
  }

  const loadAgentSkills = async (agentList) => {
    if (!agentList || agentList.length === 0) {
      agentSkills = {}
      return
    }
    agentSkillsLoading = true
    agentSkillsError = ''
    try {
      const entries = await Promise.all(
        agentList.map(async (agent) => {
          try {
            const response = await apiFetch(`/api/skills?agent=${encodeURIComponent(agent.id)}`)
            const data = await response.json()
            return [agent.id, data.map((skill) => skill.name)]
          } catch {
            return [agent.id, []]
          }
        }),
      )
      agentSkills = Object.fromEntries(entries)
    } catch (err) {
      agentSkillsError = err?.message || 'Failed to load agent skills.'
      agentSkills = {}
    } finally {
      agentSkillsLoading = false
    }
  }

  const syncAgentRunning = (agentList, terminalList) => {
    if (!Array.isArray(agentList)) return []
    const terminalIds = new Set((terminalList || []).map((terminal) => terminal?.id).filter(Boolean))
    let changed = false
    const nextAgents = agentList.map((agent) => {
      const terminalId = agent.terminal_id || ''
      const running = Boolean(terminalId && terminalIds.has(terminalId))
      const nextTerminalId = running ? terminalId : ''
      if (agent.running === running && (agent.terminal_id || '') === nextTerminalId) {
        return agent
      }
      changed = true
      return {
        ...agent,
        running,
        terminal_id: nextTerminalId,
      }
    })
    return changed ? nextAgents : agentList
  }

  const formatLogTime = (value) => {
    return formatRelativeTime(value) || '—'
  }

  const buildLogQuery = () => {
    const params = new URLSearchParams()
    if (logLevelFilter !== 'all') {
      params.set('level', logLevelFilter)
    }
    return params.toString()
  }

  const fetchLogs = async () => {
    logsLoading = true
    logsError = ''
    try {
      const query = buildLogQuery()
      const response = await apiFetch(`/api/logs${query ? `?${query}` : ''}`)
      logs = await response.json()
      lastLogErrorMessage = ''
    } catch (err) {
      const message = err?.message || 'Failed to load logs.'
      logsError = message
      if (message !== lastLogErrorMessage) {
        notificationStore.addNotification('error', message)
        lastLogErrorMessage = message
      }
    } finally {
      logsLoading = false
    }
  }

  const resetLogRefresh = () => {
    if (logsRefreshTimer) {
      clearInterval(logsRefreshTimer)
      logsRefreshTimer = null
    }
    if (logsAutoRefresh) {
      logsRefreshTimer = setInterval(fetchLogs, 5000)
    }
  }

  const handleLogFilterChange = (event) => {
    logLevelFilter = event.target.value
    fetchLogs()
  }

  const logEntryKey = (entry, index) => `${entry.timestamp}-${entry.message}-${index}`

  const resetConfigExtraction = () => {
    configExtractionCount = 0
    configExtractionLast = ''
    if (configExtractionTimer) {
      clearTimeout(configExtractionTimer)
      configExtractionTimer = null
    }
  }

  const noteConfigExtraction = (payload) => {
    configExtractionCount += 1
    configExtractionLast = payload?.path || ''
    if (configExtractionTimer) {
      clearTimeout(configExtractionTimer)
    }
    configExtractionTimer = setTimeout(() => {
      resetConfigExtraction()
    }, 5000)
  }

  $: orderedLogs = [...logs].reverse()
  $: visibleLogs = orderedLogs.slice(0, 15)
  $: if (status && !gitOrigin) {
    gitOrigin = status.git_origin || ''
  }
  $: if (status && !gitBranch) {
    gitBranch = status.git_branch || ''
  }
  $: gitContext =
    gitOrigin && gitBranch
      ? `${gitOrigin}/${gitBranch}`
      : gitOrigin || gitBranch || 'not a git repo'

  $: if (logsMounted) {
    logsAutoRefresh
    resetLogRefresh()
  }
  $: {
    const nextAgents = syncAgentRunning(agents, terminals)
    if (nextAgents !== agents) {
      agents = nextAgents
    }
  }

  onMount(() => {
    loadAgents()
    logsMounted = true
    fetchLogs()
    resetLogRefresh()
    const handleAgentEvent = () => {
      loadAgents()
    }
    agentEventsUnsubscribes = [
      subscribeAgentEvents('agent_started', handleAgentEvent),
      subscribeAgentEvents('agent_stopped', handleAgentEvent),
      subscribeAgentEvents('agent_error', handleAgentEvent),
    ]
    configEventsUnsubscribes = [
      subscribeConfigEvents('config_extracted', noteConfigExtraction),
      subscribeConfigEvents('config_conflict', (payload) => {
        const path = payload?.path || 'config file'
        notificationStore.addNotification('warning', `Config conflict: ${path}`)
      }),
      subscribeConfigEvents('config_validation_error', (payload) => {
        const path = payload?.path || 'config file'
        notificationStore.addNotification('error', `Config validation failed: ${path}`)
      }),
    ]
    gitUnsubscribe = subscribeEvents('git_branch_changed', (payload) => {
      if (!payload?.path) return
      gitBranch = payload.path
    })
  })

  onDestroy(() => {
    logsMounted = false
    if (logsRefreshTimer) {
      clearInterval(logsRefreshTimer)
      logsRefreshTimer = null
    }
    if (agentEventsUnsubscribes.length > 0) {
      agentEventsUnsubscribes.forEach((unsubscribe) => unsubscribe())
      agentEventsUnsubscribes = []
    }
    if (configEventsUnsubscribes.length > 0) {
      configEventsUnsubscribes.forEach((unsubscribe) => unsubscribe())
      configEventsUnsubscribes = []
    }
    if (configExtractionTimer) {
      clearTimeout(configExtractionTimer)
      configExtractionTimer = null
    }
    if (gitUnsubscribe) {
      gitUnsubscribe()
      gitUnsubscribe = null
    }
  })
</script>

<section class="dashboard" data-terminal-count={terminals.length}>
  <section class="dashboard__status">
    <div class="status-card status-card--wide">
      <div class="status-meta">
        <div class="status-item">
          <span class="label">Working directory</span>
          <span class="status-pill status-pill--path">{status?.working_dir || '—'}</span>
        </div>
        <div class="status-item">
          <span class="label">Git</span>
          <span class="status-pill status-pill--git">{gitContext}</span>
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
      <h2>Agent terminals</h2>
      <p class="subtle">{agents.length} profile(s)</p>
    </div>

    {#if agentsLoading}
      <p class="muted">Loading agents…</p>
    {:else if agentsError}
      <p class="error">{agentsError}</p>
    {:else if agents.length === 0}
      <p class="muted">No agent profiles found.</p>
    {:else}
      <div class="agent-grid">
        {#each agents as agent}
          <div class="agent-card">
            <button
              class="agent-button"
              class:agent-button--running={agent.running}
              class:agent-button--stopped={!agent.running}
              on:click={() =>
                agent.running ? switchToTerminal(agent.terminal_id) : createTerminal(agent.id)
              }
              disabled={actionPending || loading}
            >
              <span class="agent-name">{agent.name}</span>
              <span class="agent-action">{agent.running ? 'Open' : 'Start'}</span>
              {#if agentSkillsLoading}
                <span class="agent-skills muted">Loading skills…</span>
              {:else if agentSkillsError}
                <span class="agent-skills error">Skills unavailable</span>
              {:else if (agentSkills[agent.id] || []).length === 0}
                <span class="agent-skills muted">No skills assigned</span>
              {:else}
                <span class="agent-skills">{agentSkills[agent.id].join(', ')}</span>
              {/if}
            </button>
          </div>
        {/each}
      </div>
    {/if}

    {#if error || localError}
      <p class="error">{error || localError}</p>
    {/if}
  </section>

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
        <input type="checkbox" bind:checked={logsAutoRefresh} />
        <span>Auto refresh</span>
      </label>
      <button class="logs-refresh" type="button" on:click={fetchLogs} disabled={logsLoading}>
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
              <div class="log-entry__meta">
                <span class="log-badge">{entry.level}</span>
                <span class="log-time" title={entry.timestamp || ''}>
                  {formatLogTime(entry.timestamp)}
                </span>
              </div>
              <p class="log-message">{entry.message}</p>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
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

  .dashboard__logs {
    padding: 1.5rem;
    border-radius: 24px;
    background: rgba(var(--color-info-rgb), 0.08);
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .logs-controls {
    display: flex;
    align-items: center;
    gap: 1rem;
    flex-wrap: wrap;
  }

  .logs-control {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    font-size: 0.75rem;
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
    gap: 0.6rem;
  }

  .log-entry {
    padding: 0.65rem 0.85rem;
    border-radius: 16px;
    background: var(--color-surface);
    border: 1px solid rgba(var(--color-text-rgb), 0.06);
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .log-entry__meta {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    font-size: 0.75rem;
    color: var(--color-text-subtle);
  }

  .log-badge {
    padding: 0.2rem 0.5rem;
    border-radius: 999px;
    background: var(--color-contrast-bg);
    color: var(--color-contrast-text);
    font-size: 0.65rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .log-entry--info .log-badge {
    background: var(--color-info);
  }

  .log-entry--warning .log-badge {
    background: var(--color-warning);
  }

  .log-entry--error .log-badge {
    background: var(--color-danger);
  }

  .log-message {
    margin: 0;
    font-size: 0.85rem;
    color: var(--color-text);
  }

  .agent-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: 0.75rem;
  }

  .agent-card {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
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

  .agent-skills {
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--color-text-subtle);
    text-align: left;
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
  }
</style>
