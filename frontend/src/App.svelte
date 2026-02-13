<script>
  import { onMount } from 'svelte'
  import Dashboard from './views/Dashboard.svelte'
  import TabBar from './components/TabBar.svelte'
  import ToastContainer from './components/ToastContainer.svelte'
  import {
    createTerminal as createTerminalSession,
    deleteTerminal as deleteTerminalSession,
    fetchStatus,
    fetchTerminals,
  } from './lib/apiClient.js'
  import { apiFetch, buildApiPath } from './lib/api.js'
  import { setServerTimeOffset } from './lib/timeUtils.js'
  import { subscribe as subscribeEvents } from './lib/eventStore.js'
  import { subscribe as subscribeTerminalEvents } from './lib/terminalEventStore.js'
  import { buildTabs, ensureActiveTab, resolveActiveView } from './lib/tabRouting.js'
  import { canUseClipboard } from './lib/clipboard.js'
  import { notificationStore } from './lib/notificationStore.js'
  import { subscribe as subscribeNotificationEvents } from './lib/notificationEventStore.js'
  import { getErrorMessage, notifyError } from './lib/errorUtils.js'
  import {
    buildTerminalStyle,
    sessionUiConfig,
    setSessionUiConfigFromStatus,
  } from './lib/sessionUiConfig.js'
  import { resolveGuiModules } from './lib/guiModules/resolve.js'
  import {
    appHealthStore,
    forceReload,
    recordRefresh,
    reportCrash,
    setActiveTabId,
    setActiveView,
  } from './lib/appHealthStore.js'
  import { isExternalCliSession } from './lib/sessionSelection.js'

  let tabs = buildTabs([])
  let activeId = 'dashboard'

  let terminals = []
  let status = null
  let loading = false
  let error = ''
  let watchErrorNotified = false
  let terminalErrorUnsubscribe = null
  let notificationUnsubscribe = null
  let agentsHubUnsubscribe = null
  let crashState = null
  let clipboardAvailable = false
  let terminalStyle = ''
  let flowViewComponent = null
  let flowViewPromise = null
  let planViewComponent = null
  let planViewPromise = null
  let agentsViewComponent = null
  let agentsViewPromise = null
  let terminalViewComponent = null
  let terminalViewPromise = null

  const buildTitle = (workingDir) => {
    if (!workingDir) {
      return 'gestalt'
    }
    const trimmed = workingDir.replace(/[\\/]+$/, '')
    const parts = trimmed.split(/[\\/]/).filter(Boolean)
    return parts[parts.length - 1] || trimmed || 'gestalt'
  }

  $: activeView = resolveActiveView(activeId)
  $: setActiveTabId(activeId)
  $: setActiveView(activeView)
  $: crashState = $appHealthStore
  $: clipboardAvailable = canUseClipboard()
  $: activeTerminal = terminals.find((terminal) => terminal.id === activeId) || null
  $: tmuxSessionName =
    status?.working_dir && String(status.working_dir).trim()
      ? `Gestalt ${buildTitle(status.working_dir)}`
      : ''
  $: if (status) {
    setSessionUiConfigFromStatus(status)
  }
  $: terminalStyle = buildTerminalStyle($sessionUiConfig)

  $: if (typeof document !== 'undefined') {
    const projectName = buildTitle(status?.working_dir || '')
    document.title = `${projectName} | gestalt`
  }

  const hasAgentsTab = (nextStatus) => {
    const sessionId = String(nextStatus?.agents_session_id || '').trim()
    const tmuxSession = String(nextStatus?.agents_tmux_session || '').trim()
    return Boolean(sessionId || tmuxSession)
  }

  const syncTabs = (terminalList, nextStatus = status) => {
    tabs = buildTabs(terminalList, { showAgents: hasAgentsTab(nextStatus) })
    activeId = ensureActiveTab(activeId, tabs, 'dashboard')
  }

  function loadFlowView() {
    if (flowViewComponent) return flowViewComponent
    if (!flowViewPromise) {
      flowViewPromise = import('./views/FlowView.svelte')
        .then((module) => {
          flowViewComponent = module.default
          return flowViewComponent
        })
        .catch((err) => {
          flowViewPromise = null
          notifyError(err, 'Failed to load the Flow view.')
          return null
        })
    }
    return flowViewPromise
  }

  function loadPlanView() {
    if (planViewComponent) return planViewComponent
    if (!planViewPromise) {
      planViewPromise = import('./views/PlanView.svelte')
        .then((module) => {
          planViewComponent = module.default
          return planViewComponent
        })
        .catch((err) => {
          planViewPromise = null
          notifyError(err, 'Failed to load the Plans view.')
          return null
        })
    }
    return planViewPromise
  }

  function loadAgentsView() {
    if (agentsViewComponent) return agentsViewComponent
    if (!agentsViewPromise) {
      agentsViewPromise = import('./views/AgentsView.svelte')
        .then((module) => {
          agentsViewComponent = module.default
          return agentsViewComponent
        })
        .catch((err) => {
          agentsViewPromise = null
          notifyError(err, 'Failed to load the Agents view.')
          return null
        })
    }
    return agentsViewPromise
  }

  function loadTerminalView() {
    if (terminalViewComponent) return terminalViewComponent
    if (!terminalViewPromise) {
      terminalViewPromise = import('./views/TerminalView.svelte')
        .then((module) => {
          terminalViewComponent = module.default
          return terminalViewComponent
        })
        .catch((err) => {
          terminalViewPromise = null
          notifyError(err, 'Failed to load the Terminal view.')
          return null
        })
    }
    return terminalViewPromise
  }

  $: if (activeView === 'plan') {
    loadPlanView()
  }
  $: if (activeView === 'agents') {
    loadAgentsView()
  }
  $: if (activeView === 'flow') {
    loadFlowView()
  }
  $: if (activeView === 'terminal') {
    loadTerminalView()
  }

  const refresh = async () => {
    loading = true
    error = ''
    try {
      const [nextStatus, nextTerminals] = await Promise.all([
        fetchStatus(),
        fetchTerminals(),
      ])
      setServerTimeOffset(nextStatus?.server_time)
      status = nextStatus
      terminals = nextTerminals
      recordRefresh('status')
      recordRefresh('terminals')
      syncTabs(terminals, nextStatus)
    } catch (err) {
      const message = notifyError(err, 'Failed to load dashboard data.')
      error = message
    } finally {
      loading = false
    }
  }

  const createTerminal = async (agentId = '', useWorkflow) => {
    error = ''
    try {
      const created = await createTerminalSession({ agentId, workflow: useWorkflow })
      terminals = [...terminals, created]
      if (status) {
        status = { ...status, session_count: status.session_count + 1 }
      }
      syncTabs(terminals, status)
      if (isExternalCliSession(created)) {
        try {
          await apiFetch(buildApiPath('/api/sessions', created.id, 'activate'), {
            method: 'POST',
          })
        } catch (err) {
          notifyError(err, 'Failed to activate tmux window.')
        }
        activeId = 'agents'
        return
      }
      activeId = created.id
      console.info('session created', {
        id: created.id,
        title: created.title,
        agentId: created.agent_id,
      })
    } catch (err) {
      if (err?.status === 409 && err?.data?.session_id) {
        const existingId = err.data.session_id
        let existing = terminals.find((terminal) => terminal.id === existingId)
        if (!existing) {
          await refresh()
          existing = terminals.find((terminal) => terminal.id === existingId)
        }
        if (existing && isExternalCliSession(existing)) {
          try {
            await apiFetch(buildApiPath('/api/sessions', existingId, 'activate'), {
              method: 'POST',
            })
          } catch (activateErr) {
            notifyError(activateErr, 'Failed to activate tmux window.')
          }
          activeId = 'agents'
        } else {
          activeId = existingId
        }
        notificationStore.addNotification('info', getErrorMessage(err, `Agent ${agentId} is already running.`))
        return
      }
      const message = notifyError(err, 'Failed to create session.')
      throw err
    }
  }

  const deleteTerminal = async (id) => {
    error = ''
    try {
      await deleteTerminalSession(id)
      const { releaseTerminalState } = await import('./lib/terminalStore.js')
      releaseTerminalState(id)
      terminals = terminals.filter((terminal) => terminal.id !== id)
      if (status) {
        status = { ...status, session_count: Math.max(0, status.session_count - 1) }
      }
      syncTabs(terminals)
      if (activeId === id) {
        activeId = 'dashboard'
      }
      notificationStore.addNotification('info', `Session ${id} closed.`)
    } catch (err) {
      const message = notifyError(err, 'Failed to close session.')
      throw err
    }
  }

  const handleSelect = async (id) => {
    const selected = terminals.find((terminal) => terminal.id === id)
    if (selected) {
      if (isExternalCliSession(selected)) {
        try {
          await apiFetch(buildApiPath('/api/sessions', id, 'activate'), {
            method: 'POST',
          })
          activeId = 'agents'
          return
        } catch (err) {
          notifyError(err, 'Failed to activate tmux window.')
          return
        }
      }
    }
    activeId = id
  }

  const copyCrashId = async () => {
    const crashId = crashState?.crash?.id
    if (!crashId) return
    if (!clipboardAvailable) {
      notificationStore.addNotification('error', 'Clipboard requires HTTPS.')
      return
    }
    const clipboard = navigator?.clipboard
    if (!clipboard?.writeText) {
      notificationStore.addNotification('error', 'Clipboard is unavailable.')
      return
    }
    try {
      await clipboard.writeText(crashId)
      notificationStore.addNotification('info', 'Copied crash id.')
    } catch {
      notificationStore.addNotification('error', 'Failed to copy crash id.')
    }
  }

  const reloadAfterCrash = () => {
    forceReload()
  }

  const handleBoundaryError = (viewName, error) => {
    reportCrash(error, { source: 'view-boundary', view: viewName })
  }

  onMount(() => {
    refresh()
    const unsubscribe = subscribeEvents('watch_error', () => {
      if (watchErrorNotified) return
      watchErrorNotified = true
      notificationStore.addNotification('warning', 'File watching unavailable.')
    })
    agentsHubUnsubscribe = subscribeEvents('agents_hub_ready', (payload) => {
      const data = payload?.data || {}
      const sessionId = String(data.agents_session_id || '').trim()
      const tmuxSession = String(data.agents_tmux_session || '').trim()
      if (!sessionId && !tmuxSession) return
      status = {
        ...(status || {}),
        agents_session_id: sessionId,
        agents_tmux_session: tmuxSession,
      }
      syncTabs(terminals, status)
    })
    terminalErrorUnsubscribe = subscribeTerminalEvents('terminal_error', (payload) => {
      const sessionId = payload?.session_id || 'unknown'
      const detail = payload?.data?.error
      const message = detail
        ? `Session ${sessionId} error: ${detail}`
        : `Session ${sessionId} error.`
      notificationStore.addNotification('error', message)
    })
    notificationUnsubscribe = subscribeNotificationEvents('toast', (payload) => {
      if (!payload) return
      const message = payload.message || ''
      if (!String(message).trim()) return
      notificationStore.addNotification(payload.level || 'info', message)
    })
    return () => {
      unsubscribe()
      if (terminalErrorUnsubscribe) {
        terminalErrorUnsubscribe()
        terminalErrorUnsubscribe = null
      }
      if (notificationUnsubscribe) {
        notificationUnsubscribe()
        notificationUnsubscribe = null
      }
      if (agentsHubUnsubscribe) {
        agentsHubUnsubscribe()
        agentsHubUnsubscribe = null
      }
    }
  })
</script>

<TabBar
  {tabs}
  {activeId}
  onSelect={handleSelect}
/>
<ToastContainer notifications={$notificationStore} onDismiss={notificationStore.dismiss} />

{#if crashState?.crash}
  <div class="crash-overlay" role="alert" aria-live="assertive">
    <div class="crash-card">
      <h2>UI crash detected</h2>
      {#if crashState.crashLoop}
        <p>Crash loop detected. Auto-reload paused.</p>
      {:else}
        <p>Reloading...</p>
      {/if}
      <div class="crash-meta">
        <span>Crash id</span>
        <code>{crashState.crash.id}</code>
      </div>
      <div class="crash-actions">
        {#if crashState.crashLoop}
          <button class="crash-button" type="button" on:click={reloadAfterCrash}>
            Reload
          </button>
        {/if}
        {#if clipboardAvailable}
          <button class="crash-button crash-button--ghost" type="button" on:click={copyCrashId}>
            Copy crash id
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}

{#snippet viewFailed(error, reset)}
  <div class="view-fallback">
    <p>Reloading...</p>
  </div>
{/snippet}

<main class="app" style={terminalStyle}>
  <section class="view" data-active={activeView === 'dashboard'}>
    <svelte:boundary onerror={(error) => handleBoundaryError('dashboard', error)} failed={viewFailed}>
      <Dashboard
        {terminals}
        {status}
        {loading}
        {error}
        onCreate={createTerminal}
        onSelect={handleSelect}
      />
    </svelte:boundary>
  </section>
  <section class="view" data-active={activeView === 'plan'}>
    <svelte:boundary onerror={(error) => handleBoundaryError('plan', error)} failed={viewFailed}>
      {#if planViewComponent}
        <svelte:component this={planViewComponent} />
      {:else}
        <div class="view-fallback">
          <p>Loading...</p>
        </div>
      {/if}
    </svelte:boundary>
  </section>
  <section class="view" data-active={activeView === 'flow'}>
    <svelte:boundary onerror={(error) => handleBoundaryError('flow', error)} failed={viewFailed}>
      {#if flowViewComponent}
        <svelte:component this={flowViewComponent} />
      {:else}
        <div class="view-fallback">
          <p>Loading...</p>
        </div>
      {/if}
    </svelte:boundary>
  </section>
  <section class="view" data-active={activeView === 'agents'}>
    <svelte:boundary onerror={(error) => handleBoundaryError('agents', error)} failed={viewFailed}>
      {#if agentsViewComponent}
        <svelte:component
          this={agentsViewComponent}
          status={status}
          visible={activeView === 'agents'}
        />
      {:else}
        <div class="view-fallback">
          <p>Loading...</p>
        </div>
      {/if}
    </svelte:boundary>
  </section>
  <section class="view view--terminals" data-active={activeView === 'terminal'}>
    {#if activeTerminal}
      <div class="terminal-tab" data-active="true">
        <svelte:boundary onerror={(error) => handleBoundaryError('terminal', error)} failed={viewFailed}>
          {#if terminalViewComponent}
            <svelte:component
              this={terminalViewComponent}
              sessionId={activeTerminal.id}
              title={activeTerminal.title}
              promptFiles={activeTerminal.prompt_files || []}
              visible={true}
              sessionInterface={activeTerminal.interface || ''}
              sessionRunner={activeTerminal.runner || ''}
              tmuxSessionName={tmuxSessionName}
              guiModules={resolveGuiModules(activeTerminal.gui_modules, activeTerminal.runner)}
              onDelete={deleteTerminal}
            />
          {:else}
            <div class="view-fallback">
              <p>Loading...</p>
            </div>
          {/if}
        </svelte:boundary>
      </div>
    {/if}
  </section>
</main>

<style>
  .app {
    min-height: calc(100vh - 64px);
    display: flex;
    flex-direction: column;
  }

  .view {
    flex: 1 1 auto;
    display: none;
  }

  .view[data-active='true'] {
    display: block;
  }

  .view--terminals {
    display: block;
  }

  .terminal-tab {
    display: none;
  }

  .terminal-tab[data-active='true'] {
    display: block;
  }

  .view-fallback {
    padding: 1.5rem;
    border-radius: 16px;
    border: 1px solid rgba(var(--color-danger-rgb), 0.35);
    background: rgba(var(--color-surface-rgb), 0.8);
    color: var(--color-text);
  }

  .crash-overlay {
    position: fixed;
    inset: 0;
    z-index: 50;
    background: rgba(6, 8, 12, 0.72);
    display: grid;
    place-items: center;
    padding: 1.5rem;
  }

  .crash-card {
    width: min(480px, 90vw);
    background: rgba(var(--color-surface-rgb), 0.95);
    border: 1px solid rgba(var(--color-danger-rgb), 0.45);
    border-radius: 18px;
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
    color: var(--color-text);
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.45);
  }

  .crash-card h2 {
    margin: 0;
    font-size: 1.2rem;
  }

  .crash-meta {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .crash-meta code {
    font-family: var(--font-mono);
    color: var(--color-text);
  }

  .crash-actions {
    display: flex;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .crash-button {
    border: none;
    border-radius: 999px;
    padding: 0.55rem 1.1rem;
    font-weight: 600;
    font-size: 0.85rem;
    cursor: pointer;
    background: var(--color-danger);
    color: var(--color-contrast-text);
  }

  .crash-button--ghost {
    background: transparent;
    border: 1px solid rgba(var(--color-text-rgb), 0.3);
    color: var(--color-text);
  }
</style>
