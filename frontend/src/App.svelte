<script>
  import { onMount } from 'svelte'
  import Dashboard from './views/Dashboard.svelte'
  import PlanView from './views/PlanView.svelte'
  import TerminalView from './views/TerminalView.svelte'
  import TabBar from './components/TabBar.svelte'
  import ToastContainer from './components/ToastContainer.svelte'
  import NotificationSettings from './components/NotificationSettings.svelte'
  import { apiFetch } from './lib/api.js'
  import { formatTerminalLabel } from './lib/terminalTabs.js'
  import { releaseTerminalState } from './lib/terminalStore.js'
  import { notificationStore } from './lib/notificationStore.js'

  let tabs = [
    { id: 'dashboard', label: 'Dashboard', isHome: true },
    { id: 'plan', label: 'Plan', isHome: true },
  ]
  let activeId = 'dashboard'

  let terminals = []
  let status = null
  let loading = false
  let error = ''
  let showSettings = false

  $: activeView =
    activeId === 'dashboard'
      ? 'dashboard'
      : activeId === 'plan'
        ? 'plan'
        : 'terminal'

  const syncTabs = (terminalList) => {
    tabs = [
      { id: 'dashboard', label: 'Dashboard', isHome: true },
      { id: 'plan', label: 'Plan', isHome: true },
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

  const createTerminal = async (agentId = '') => {
    error = ''
    try {
      const response = await apiFetch('/api/terminals', {
        method: 'POST',
        body: JSON.stringify(agentId ? { agent: agentId } : {}),
      })
      const created = await response.json()
      terminals = [...terminals, created]
      if (status) {
        status = { ...status, terminal_count: status.terminal_count + 1 }
      }
      syncTabs(terminals)
      activeId = created.id
      notificationStore.addNotification(
        'info',
        `Terminal ${created.title || created.id} created.`
      )
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

  const handleClose = (id) => {
    if (id === 'dashboard' || id === 'plan') return
    deleteTerminal(id).catch((err) => {
      error = err?.message || 'Failed to close terminal.'
    })
  }

  const openSettings = () => {
    showSettings = true
  }

  const closeSettings = () => {
    showSettings = false
  }

  onMount(refresh)
</script>

<TabBar
  {tabs}
  {activeId}
  onSelect={handleSelect}
  onClose={handleClose}
  onOpenSettings={openSettings}
/>
<ToastContainer notifications={$notificationStore} onDismiss={notificationStore.dismiss} />
<NotificationSettings open={showSettings} onClose={closeSettings} />

<main class="app">
  <section class="view" data-active={activeView === 'dashboard'}>
    <Dashboard
      {terminals}
      {status}
      {loading}
      {error}
      onCreate={createTerminal}
      onDelete={deleteTerminal}
    />
  </section>
  <section class="view" data-active={activeView === 'plan'}>
    <PlanView />
  </section>
  <section class="view view--terminals" data-active={activeView === 'terminal'}>
    {#each terminals as terminal (terminal.id)}
      <div class="terminal-tab" data-active={terminal.id === activeId}>
        <TerminalView
          terminalId={terminal.id}
          title={terminal.title}
          skills={terminal.skills || []}
          visible={terminal.id === activeId}
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
