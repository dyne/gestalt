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
  import { notificationStore } from './lib/notificationStore.js'
  import { getErrorMessage, notifyError } from './lib/errorUtils.js'

  let tabs = buildTabs([])
  let activeId = 'dashboard'

  let terminals = []
  let status = null
  let loading = false
  let error = ''
  let watchErrorNotified = false
  let terminalErrorUnsubscribe = null

  const buildTitle = (workingDir) => {
    if (!workingDir) {
      return 'gestalt'
    }
    const trimmed = workingDir.replace(/[\\/]+$/, '')
    const parts = trimmed.split(/[\\/]/).filter(Boolean)
    return parts[parts.length - 1] || trimmed || 'gestalt'
  }

  $: activeView = resolveActiveView(activeId)

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
        status = { ...status, terminal_count: status.terminal_count + 1 }
      }
      syncTabs(terminals)
      activeId = created.id
      console.info('terminal created', {
        id: created.id,
        title: created.title,
        agentId: created.agent_id,
      })
    } catch (err) {
      if (err?.status === 409 && err?.data?.terminal_id) {
        const existingId = err.data.terminal_id
        if (!terminals.find((terminal) => terminal.id === existingId)) {
          await refresh()
        }
        activeId = existingId
        notificationStore.addNotification('info', getErrorMessage(err, `Agent ${agentId} is already running.`))
        return
      }
      const message = notifyError(err, 'Failed to create terminal.')
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
        status = { ...status, terminal_count: Math.max(0, status.terminal_count - 1) }
      }
      syncTabs(terminals)
      if (activeId === id) {
        activeId = 'dashboard'
      }
      notificationStore.addNotification('info', `Terminal ${id} closed.`)
    } catch (err) {
      const message = notifyError(err, 'Failed to close terminal.')
      throw err
    }
  }

  const handleSelect = (id) => {
    activeId = id
  }

  onMount(() => {
    refresh()
    const unsubscribe = subscribeEvents('watch_error', () => {
      if (watchErrorNotified) return
      watchErrorNotified = true
      notificationStore.addNotification('warning', 'File watching unavailable.')
    })
    terminalErrorUnsubscribe = subscribeTerminalEvents('terminal_error', (payload) => {
      const terminalId = payload?.terminal_id || 'unknown'
      const detail = payload?.data?.error
      const message = detail
        ? `Terminal ${terminalId} error: ${detail}`
        : `Terminal ${terminalId} error.`
      notificationStore.addNotification('error', message)
    })
    return () => {
      unsubscribe()
      if (terminalErrorUnsubscribe) {
        terminalErrorUnsubscribe()
        terminalErrorUnsubscribe = null
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

<main class="app">
  <section class="view" data-active={activeView === 'dashboard'}>
    <Dashboard
      {terminals}
      {status}
      {loading}
      {error}
      onCreate={createTerminal}
      onSelect={handleSelect}
    />
  </section>
  <section class="view" data-active={activeView === 'plan'}>
    <PlanView />
  </section>
  <section class="view" data-active={activeView === 'flow'}>
    <FlowView onViewTerminal={handleSelect} temporalUiUrl={status?.temporal_ui_url || ''} />
  </section>
  <section class="view view--terminals" data-active={activeView === 'terminal'}>
    {#each terminals as terminal (terminal.id)}
      <div class="terminal-tab" data-active={terminal.id === activeId}>
        <TerminalView
          terminalId={terminal.id}
          title={terminal.title}
          promptFiles={terminal.prompt_files || []}
          visible={terminal.id === activeId}
          onDelete={deleteTerminal}
        />
      </div>
    {/each}
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
</style>
