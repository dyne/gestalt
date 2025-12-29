<script>
  import { onMount } from 'svelte'
  import Dashboard from './views/Dashboard.svelte'
  import TerminalView from './views/TerminalView.svelte'
  import TabBar from './components/TabBar.svelte'
  import { apiFetch } from './lib/api.js'

  let tabs = [{ id: 'dashboard', label: 'Dashboard', isHome: true }]
  let activeId = 'dashboard'

  let terminals = []
  let status = null
  let loading = false
  let error = ''

  $: activeView = activeId === 'dashboard' ? 'dashboard' : 'terminal'

  const syncTabs = (terminalList) => {
    tabs = [
      { id: 'dashboard', label: 'Dashboard', isHome: true },
      ...terminalList.map((terminal) => ({
        id: terminal.id,
        label: terminal.title || `Terminal ${terminal.id}`,
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
      error = err?.message || 'Failed to load dashboard data.'
    } finally {
      loading = false
    }
  }

  const createTerminal = async () => {
    error = ''
    const response = await apiFetch('/api/terminals', {
      method: 'POST',
      body: JSON.stringify({}),
    })
    const created = await response.json()
    terminals = [...terminals, created]
    if (status) {
      status = { ...status, terminal_count: status.terminal_count + 1 }
    }
    syncTabs(terminals)
    activeId = created.id
  }

  const deleteTerminal = async (id) => {
    error = ''
    await apiFetch(`/api/terminals/${id}`, { method: 'DELETE' })
    terminals = terminals.filter((terminal) => terminal.id !== id)
    if (status) {
      status = { ...status, terminal_count: Math.max(0, status.terminal_count - 1) }
    }
    syncTabs(terminals)
    if (activeId === id) {
      activeId = 'dashboard'
    }
  }

  const handleSelect = (id) => {
    activeId = id
  }

  const handleClose = (id) => {
    if (id === 'dashboard') return
    deleteTerminal(id).catch((err) => {
      error = err?.message || 'Failed to close terminal.'
    })
  }

  onMount(refresh)
</script>

<TabBar {tabs} {activeId} onSelect={handleSelect} onClose={handleClose} />

<main class="app">
  <section class="view" data-active={activeView === 'dashboard'}>
    <Dashboard
      {terminals}
      {status}
      {loading}
      {error}
      onCreate={createTerminal}
    />
  </section>
  <section class="view view--terminals" data-active={activeView === 'terminal'}>
    {#each terminals as terminal (terminal.id)}
      <div class="terminal-tab" data-active={terminal.id === activeId}>
        <TerminalView terminalId={terminal.id} visible={terminal.id === activeId} />
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
