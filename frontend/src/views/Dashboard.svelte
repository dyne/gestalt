<script>
  import { onDestroy, onMount } from 'svelte'
  import { apiFetch } from '../lib/api.js'
  import { subscribe as subscribeEvents } from '../lib/eventStore.js'
  import { notificationStore } from '../lib/notificationStore.js'

  export let terminals = []
  export let status = null
  export let loading = false
  export let error = ''
  export let onCreate = () => {}
  export let onDelete = () => {}

  let actionPending = false
  let localError = ''
  let agents = []
  let agentsLoading = false
  let agentsError = ''
  let agentSkills = {}
  let agentSkillsLoading = false
  let agentSkillsError = ''
  let agentWorkflow = {}
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
      const useWorkflow =
        agentId && typeof agentWorkflow[agentId] === 'boolean'
          ? agentWorkflow[agentId]
          : undefined
      await onCreate(agentId, useWorkflow)
      await loadAgents()
    } catch (err) {
      localError = err?.message || 'Failed to create terminal.'
    } finally {
      actionPending = false
    }
  }

  const stopTerminal = async (terminalId) => {
    if (!terminalId) {
      localError = 'No running terminal found.'
      return
    }
    actionPending = true
    localError = ''
    try {
      await onDelete(terminalId)
      await loadAgents()
    } catch (err) {
      localError = err?.message || 'Failed to stop terminal.'
    } finally {
      actionPending = false
    }
  }

  const loadAgents = async () => {
    agentsLoading = true
    agentsError = ''
    try {
      const response = await apiFetch('/api/agents')
      agents = await response.json()
      syncAgentWorkflow(agents)
      await loadAgentSkills(agents)
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

  const syncAgentWorkflow = (agentList) => {
    const next = {}
    if (agentList && agentList.length > 0) {
      agentList.forEach((agent) => {
        if (typeof agentWorkflow[agent.id] === 'boolean') {
          next[agent.id] = agentWorkflow[agent.id]
        } else {
          next[agent.id] = agent.use_workflow !== false
        }
      })
    }
    agentWorkflow = next
  }

  const formatLogTime = (value) => {
    if (!value) return '—'
    const parsed = new Date(value)
    if (Number.isNaN(parsed.getTime())) return '—'
    return parsed.toLocaleString()
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

  onMount(() => {
    loadAgents()
    logsMounted = true
    fetchLogs()
    resetLogRefresh()
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
                agent.running ? stopTerminal(agent.terminal_id) : createTerminal(agent.id)
              }
              disabled={actionPending || loading}
            >
              <span class="agent-name">{agent.name}</span>
              <span class="agent-action">{agent.running ? 'Stop' : 'Start'}</span>
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
            <label class="agent-toggle">
              <input
                type="checkbox"
                bind:checked={agentWorkflow[agent.id]}
                disabled={actionPending || loading}
              />
              <span>Enable workflow tracking</span>
            </label>
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
                <span class="log-time">{formatLogTime(entry.timestamp)}</span>
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
    background: radial-gradient(circle at top left, #f6f0e4, #f1f4f6 60%, #f6f6f1);
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
    background: #151515;
    color: #f6f3ed;
    cursor: pointer;
    transition: transform 160ms ease, box-shadow 160ms ease, opacity 160ms ease;
    box-shadow: 0 10px 30px rgba(10, 10, 10, 0.2);
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
    background: #ffffffd9;
    border: 1px solid rgba(20, 20, 20, 0.08);
    box-shadow: 0 20px 50px rgba(20, 20, 20, 0.08);
  }

  .label {
    display: block;
    font-size: 0.8rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: #6c6860;
  }

  .value {
    display: block;
    margin-top: 0.35rem;
    font-size: 1.6rem;
    color: #141414;
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
    background: rgba(255, 255, 255, 0.85);
    border: 1px solid rgba(20, 20, 20, 0.12);
    font-family: "IBM Plex Mono", "SFMono-Regular", Menlo, monospace;
    font-size: 0.85rem;
    word-break: break-all;
  }

  .status-pill--path {
    font-size: 0.95rem;
  }

  .status-pill--git {
    color: #5a5a54;
  }

  .dashboard__agents {
    padding: 1.5rem;
    border-radius: 24px;
    background: #fff6ec;
    border: 1px solid rgba(20, 20, 20, 0.08);
  }

  .dashboard__logs {
    padding: 1.5rem;
    border-radius: 24px;
    background: #f5f7fb;
    border: 1px solid rgba(20, 20, 20, 0.08);
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
    color: #6f6b62;
  }

  .logs-control select {
    border: 1px solid rgba(20, 20, 20, 0.16);
    border-radius: 10px;
    padding: 0.35rem 0.6rem;
    font-size: 0.85rem;
    background: #ffffff;
  }

  .logs-control--toggle {
    flex-direction: row;
    align-items: center;
    gap: 0.5rem;
  }

  .logs-refresh {
    border: 1px solid rgba(20, 20, 20, 0.2);
    border-radius: 999px;
    padding: 0.45rem 0.95rem;
    background: #ffffff;
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
    background: #ffffff;
    border: 1px solid rgba(20, 20, 20, 0.06);
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .log-entry__meta {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    font-size: 0.75rem;
    color: #5f5b54;
  }

  .log-badge {
    padding: 0.2rem 0.5rem;
    border-radius: 999px;
    background: #151515;
    color: #f4f1eb;
    font-size: 0.65rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .log-entry--info .log-badge {
    background: #2b4b76;
  }

  .log-entry--warning .log-badge {
    background: #b07219;
  }

  .log-entry--error .log-badge {
    background: #9a3535;
  }

  .log-message {
    margin: 0;
    font-size: 0.85rem;
    color: #2b2b2b;
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
    border: 1px solid rgba(20, 20, 20, 0.2);
    border-radius: 14px;
    padding: 0.75rem 1rem;
    background: #ffffff;
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

  .agent-toggle {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    font-size: 0.65rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: #6c6860;
  }

  .agent-toggle input {
    margin: 0;
  }

  .agent-button--running {
    background: #e4f4ef;
    border-color: rgba(22, 94, 66, 0.35);
    box-shadow: 0 12px 20px rgba(22, 94, 66, 0.12);
  }

  .agent-button--stopped {
    background: #ffffff;
    border-color: rgba(20, 20, 20, 0.16);
  }

  .agent-name {
    font-size: 0.95rem;
  }

  .agent-button--running .agent-name::before {
    content: '';
    display: inline-block;
    width: 0.45rem;
    height: 0.45rem;
    border-radius: 999px;
    margin-right: 0.4rem;
    background: #1f7a5f;
    box-shadow: 0 0 0 0 rgba(31, 122, 95, 0.4);
    animation: pulseDot 2.4s ease-in-out infinite;
  }

  .agent-action {
    font-size: 0.7rem;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    color: #5f5b54;
  }

  @keyframes pulseDot {
    0% {
      box-shadow: 0 0 0 0 rgba(31, 122, 95, 0.4);
    }
    70% {
      box-shadow: 0 0 0 0.4rem rgba(31, 122, 95, 0);
    }
    100% {
      box-shadow: 0 0 0 0 rgba(31, 122, 95, 0);
    }
  }

  .agent-skills {
    font-size: 0.8rem;
    font-weight: 500;
    color: #5f5b54;
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
    box-shadow: 0 12px 20px rgba(10, 10, 10, 0.12);
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
    color: #7a776f;
  }

  .muted {
    color: #7d7a73;
    margin: 0.5rem 0 0;
  }

  .error {
    color: #b04a39;
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
