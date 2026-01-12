<script>
  import { onMount } from 'svelte'
  import Dashboard from './views/Dashboard.svelte'
  import FlowView from './views/FlowView.svelte'
  import PlanView from './views/PlanView.svelte'
  import TerminalView from './views/TerminalView.svelte'
  import TabBar from './components/TabBar.svelte'
  import ToastContainer from './components/ToastContainer.svelte'
  import { apiFetch } from './lib/api.js'
  import { subscribe as subscribeEvents } from './lib/eventStore.js'
  import { subscribe as subscribeTerminalEvents } from './lib/terminalEventStore.js'
  import { formatTerminalLabel } from './lib/terminalTabs.js'
  import { releaseTerminalState } from './lib/terminalStore.js'
  import { notificationStore } from './lib/notificationStore.js'

  let tabs = [
    { id: 'dashboard', label: 'Dashboard', isHome: true },
    { id: 'plan', label: 'Plan', isHome: true },
    { id: 'flow', label: 'Status', isHome: true },
  ]
  let activeId = 'dashboard'

  let terminals = []
  let status = null
  let loading = false
  let error = ''
  let watchErrorNotified = false
  let terminalErrorUnsubscribe = null

  const handleMenuNewTerminal = () => {
    void createTerminal().catch(() => {})
  }

  const handleMenuToggleDevtools = () => {
    const runtime = typeof window !== 'undefined' ? window.wails : null
    if (!runtime || !runtime.Window || typeof runtime.Window.OpenDevTools !== 'function') {
      console.info('[desktop] devtools unavailable')
      return
    }
    runtime.Window.OpenDevTools()
  }

  $: activeView =
    activeId === 'dashboard'
      ? 'dashboard'
      : activeId === 'plan'
        ? 'plan'
        : activeId === 'flow'
          ? 'flow'
          : 'terminal'

  const syncTabs = (terminalList) => {
    tabs = [
      { id: 'dashboard', label: 'Dashboard', isHome: true },
      { id: 'plan', label: 'Plan', isHome: true },
      { id: 'flow', label: 'Status', isHome: true },
      ...terminalList.map((terminal) => ({
        id: terminal.id,
        label: formatTerminalLabel(terminal),
        isHome: false,
      })),
    ]

    if (!tabs.find((tab) => tab.id === activeId)) {
      activeId = 'dashboard'
    }
  }

  const refresh = async () => {
    loading = true
    error = ''
    try {
      const [statusResponse, terminalsResponse] = await Promise.all([
        apiFetch('/api/status'),
        apiFetch('/api/terminals'),
      ])
      status = await statusResponse.json()
      terminals = await terminalsResponse.json()
      syncTabs(terminals)
    } catch (err) {
      const message = err?.message || 'Failed to load dashboard data.'
      error = message
      notificationStore.addNotification('error', message)
    } finally {
      loading = false
    }
  }

  const createTerminal = async (agentId = '', useWorkflow) => {
    error = ''
    try {
      const payload = agentId ? { agent: agentId } : {}
      if (typeof useWorkflow === 'boolean') {
        payload.workflow = useWorkflow
      }
      const response = await apiFetch('/api/terminals', {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      const created = await response.json()
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
        notificationStore.addNotification(
          'info',
          err?.message || `Agent ${agentId} is already running.`
        )
        return
      }
      const message = err?.message || 'Failed to create terminal.'
      notificationStore.addNotification('error', message)
      throw err
    }
  }

  const deleteTerminal = async (id) => {
    error = ''
    try {
      await apiFetch(`/api/terminals/${id}`, { method: 'DELETE' })
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
      const message = err?.message || 'Failed to close terminal.'
      notificationStore.addNotification('error', message)
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
    window.addEventListener('gestalt:menu:new-terminal', handleMenuNewTerminal)
    window.addEventListener('gestalt:menu:toggle-devtools', handleMenuToggleDevtools)
    return () => {
      window.removeEventListener('gestalt:menu:new-terminal', handleMenuNewTerminal)
      window.removeEventListener('gestalt:menu:toggle-devtools', handleMenuToggleDevtools)
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
