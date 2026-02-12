<script>
  import { onMount } from 'svelte'
  import Dashboard from './views/Dashboard.svelte'
  import FlowView from './views/FlowView.svelte'
  import PlanView from './views/PlanView.svelte'
  import TerminalView from './views/TerminalView.svelte'
  import TabBar from './components/TabBar.svelte'
  import ToastContainer from './components/ToastContainer.svelte'
  import {
    createTerminal as createTerminalSession,
    deleteTerminal as deleteTerminalSession,
    fetchStatus,
    fetchTerminals,
  } from './lib/apiClient.js'
  import { setServerTimeOffset } from './lib/timeUtils.js'
  import { subscribe as subscribeEvents } from './lib/eventStore.js'
  import { subscribe as subscribeTerminalEvents } from './lib/terminalEventStore.js'
  import { buildTabs, ensureActiveTab, resolveActiveView } from './lib/tabRouting.js'
  import { releaseTerminalState } from './lib/terminalStore.js'
  import { canUseClipboard } from './lib/clipboard.js'
  import { notificationStore } from './lib/notificationStore.js'
  import { subscribe as subscribeNotificationEvents } from './lib/notificationEventStore.js'
  import { getErrorMessage, notifyError } from './lib/errorUtils.js'
  import {
    buildTerminalStyle,
    sessionUiConfig,
    setSessionUiConfigFromStatus,
  } from './lib/sessionUiConfig.js'
  import {
    appHealthStore,
    forceReload,
    recordRefresh,
    reportCrash,
    setActiveTabId,
    setActiveView,
  } from './lib/appHealthStore.js'

  let tabs = buildTabs([])
  let activeId = 'dashboard'

  let terminals = []
  let status = null
  let loading = false
  let error = ''
  let watchErrorNotified = false
  let terminalErrorUnsubscribe = null
  let notificationUnsubscribe = null
  let crashState = null
  let clipboardAvailable = false
  let terminalStyle = ''

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
  $: if (status) {
    setSessionUiConfigFromStatus(status)
  }
  $: terminalStyle = buildTerminalStyle($sessionUiConfig)

  $: if (typeof document !== 'undefined') {
    const projectName = buildTitle(status?.working_dir || '')
    document.title = `${projectName} | gestalt`
  }

  const syncTabs = (terminalList) => {
    tabs = buildTabs(terminalList)
    activeId = ensureActiveTab(activeId, tabs, 'dashboard')
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
      syncTabs(terminals)
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
      syncTabs(terminals)
      activeId = created.id
      console.info('session created', {
        id: created.id,
        title: created.title,
        agentId: created.agent_id,
      })
    } catch (err) {
      if (err?.status === 409 && err?.data?.session_id) {
        const existingId = err.data.session_id
        if (!terminals.find((terminal) => terminal.id === existingId)) {
          await refresh()
        }
        activeId = existingId
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

  const handleSelect = (id) => {
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
    terminalErrorUnsubscribe = subscribeTerminalEvents('terminal_error', (payload) => {
      const terminalId = payload?.session_id || 'unknown'
      const detail = payload?.data?.error
      const message = detail
        ? `Session ${terminalId} error: ${detail}`
        : `Session ${terminalId} error.`
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
      <PlanView />
    </svelte:boundary>
  </section>
  <section class="view" data-active={activeView === 'flow'}>
    <svelte:boundary onerror={(error) => handleBoundaryError('flow', error)} failed={viewFailed}>
      <FlowView />
    </svelte:boundary>
  </section>
  <section class="view view--terminals" data-active={activeView === 'terminal'}>
    {#if activeTerminal}
      <div class="terminal-tab" data-active="true">
        <svelte:boundary onerror={(error) => handleBoundaryError('terminal', error)} failed={viewFailed}>
          <TerminalView
            terminalId={activeTerminal.id}
            title={activeTerminal.title}
            promptFiles={activeTerminal.prompt_files || []}
            visible={true}
            sessionInterface={activeTerminal.interface || ''}
            role={activeTerminal.role || ''}
            onDelete={deleteTerminal}
          />
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
