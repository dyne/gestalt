<script>
  import { onDestroy, onMount } from 'svelte'
  import '@xterm/xterm/css/xterm.css'

  import { getTerminalState } from '../lib/terminalStore.js'

  export let terminalId = ''
  export let visible = true

  let container
  let state
  let bellCount = 0
  let status = 'disconnected'
  let canReconnect = false
  let historyStatus = 'idle'
  let statusLabel = ''
  let unsubscribeStatus
  let unsubscribeHistory
  let unsubscribeBell
  let unsubscribeReconnect

  const statusLabels = {
    connecting: 'Connecting...',
    connected: 'Connected',
    retrying: 'Connection lost, retrying...',
    disconnected: 'Disconnected',
    unauthorized: 'Authentication required',
  }

  const attachState = () => {
    if (!terminalId) return
    state = getTerminalState(terminalId)
    if (!state) return
    state.attach(container)
    unsubscribeStatus = state.status.subscribe((value) => {
      status = value
    })
    unsubscribeHistory = state.historyStatus.subscribe((value) => {
      historyStatus = value
    })
    unsubscribeBell = state.bellCount.subscribe((value) => {
      bellCount = value
    })
    unsubscribeReconnect = state.canReconnect.subscribe((value) => {
      canReconnect = value
    })
  }

  const handleReconnect = () => {
    if (state) {
      state.reconnect()
    }
  }

  const resizeHandler = () => {
    if (!visible || !state) return
    state.scheduleFit()
  }

  onMount(() => {
    attachState()
    window.addEventListener('resize', resizeHandler)
  })

  $: if (visible) {
    if (state) {
      state.scheduleFit()
    }
  }

  $: statusLabel = statusLabels[status] || status

  onDestroy(() => {
    window.removeEventListener('resize', resizeHandler)
    if (unsubscribeStatus) {
      unsubscribeStatus()
    }
    if (unsubscribeHistory) {
      unsubscribeHistory()
    }
    if (unsubscribeBell) {
      unsubscribeBell()
    }
    if (unsubscribeReconnect) {
      unsubscribeReconnect()
    }
    if (state) {
      state.detach()
    }
  })
</script>

<section class="terminal-shell">
  <header class="terminal-shell__header">
    <div>
      <p class="label">Terminal {terminalId || '—'}</p>
      <div class="status-row">
        <p class="status">{statusLabel}</p>
        {#if historyStatus === 'loading'}
          <p class="status history-status">Loading history...</p>
        {/if}
        {#if canReconnect}
          <button class="reconnect" type="button" on:click={handleReconnect}>
            Reconnect
          </button>
        {/if}
      </div>
    </div>
    <div class="bell" aria-live="polite">
      <span>Bell</span>
      <strong>{bellCount}</strong>
    </div>
  </header>
  <div class="terminal-shell__body" bind:this={container}></div>
</section>

<style>
  .terminal-shell {
    display: grid;
    grid-template-rows: auto 1fr;
    height: 100%;
    min-height: 70vh;
    background: #101010;
    border-radius: 20px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    box-shadow: 0 20px 50px rgba(10, 10, 10, 0.35);
    overflow: hidden;
  }

  .terminal-shell__header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.9rem 1.2rem;
    background: #171717;
    border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  }

  .label {
    margin: 0;
    font-size: 0.85rem;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: rgba(242, 239, 233, 0.7);
  }

  .status {
    margin: 0.2rem 0 0;
    font-size: 0.75rem;
    color: rgba(242, 239, 233, 0.5);
    text-transform: uppercase;
    letter-spacing: 0.12em;
  }

  .status-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  .reconnect {
    border: 0;
    border-radius: 999px;
    padding: 0.2rem 0.7rem;
    background: rgba(242, 239, 233, 0.16);
    color: rgba(242, 239, 233, 0.95);
    font-size: 0.65rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    cursor: pointer;
  }

  .reconnect:hover {
    background: rgba(242, 239, 233, 0.24);
  }

  .bell {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.4rem 0.9rem;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.08);
    color: rgba(242, 239, 233, 0.9);
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
  }

  .bell strong {
    font-size: 0.9rem;
  }

  .terminal-shell__body {
    padding: 0.6rem;
  }

  :global(.xterm) {
    height: 100%;
  }

  :global(.xterm-viewport) {
    border-radius: 12px;
  }

  @media (max-width: 720px) {
    .terminal-shell {
      min-height: 60vh;
    }

    .terminal-shell__header {
      flex-direction: column;
      align-items: flex-start;
      gap: 0.5rem;
    }
  }
</style>
