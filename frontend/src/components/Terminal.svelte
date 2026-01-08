<script>
  import { onDestroy, onMount } from 'svelte'
  import '@xterm/xterm/css/xterm.css'

  import CommandInput from './CommandInput.svelte'
  import { apiFetch } from '../lib/api.js'
  import { getTerminalState } from '../lib/terminalStore.js'

  export let terminalId = ''
  export let title = ''
  export let skills = []
  export let visible = true

  let container
  let state
  let bellCount = 0
  let status = 'disconnected'
  let canReconnect = false
  let historyStatus = 'idle'
  let statusLabel = ''
  let inputDisabled = true
  let directInputEnabled = false
  let atBottom = true
  let commandInput
  let unsubscribeStatus
  let unsubscribeHistory
  let unsubscribeBell
  let unsubscribeReconnect
  let unsubscribeAtBottom
  let displayTitle = ''
  let skillsLabel = ''

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
    if (state.atBottom) {
      unsubscribeAtBottom = state.atBottom.subscribe((value) => {
        atBottom = value
      })
    }
  }

  const handleReconnect = () => {
    if (state) {
      state.reconnect()
    }
  }

  const handleScrollToBottom = () => {
    state?.scrollToBottom?.()
  }

  const handleSubmit = (command) => {
    if (!state) return
    const payload = typeof command === 'string' ? command : ''
    if (state.sendCommand) {
      state.sendCommand(payload)
    } else {
      state.sendData?.(`${payload}\r\n`)
    }
    const trimmed = payload.trim()
    if (!trimmed || !terminalId) return
    apiFetch(`/api/terminals/${terminalId}/input-history`, {
      method: 'POST',
      body: JSON.stringify({ command: trimmed }),
    }).catch((err) => {
      console.warn('failed to record input history', err)
    })
  }

  const handleDirectInputChange = (enabled) => {
    directInputEnabled = enabled
    state?.setDirectInput?.(enabled)
    if (enabled) {
      requestAnimationFrame(() => state?.focus?.())
    } else {
      requestAnimationFrame(() => commandInput?.focusInput?.())
    }
  }

  const resizeHandler = () => {
    if (!visible || !state) return
    state.scheduleFit()
  }

  onMount(() => {
    attachState()
    if (state) {
      state.setDirectInput?.(directInputEnabled)
    }
    window.addEventListener('resize', resizeHandler)
  })

  $: if (visible) {
    if (state) {
      state.scheduleFit()
    }
  }

  $: statusLabel = statusLabels[status] || status
  $: inputDisabled = status !== 'connected' || !terminalId
  $: displayTitle = title?.trim()
    ? title.trim()
    : terminalId
      ? `Terminal ${terminalId}`
      : 'Terminal —'
  $: skillsLabel =
    Array.isArray(skills) && skills.length > 0 ? skills.filter(Boolean).join(', ') : ''

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
    if (unsubscribeAtBottom) {
      unsubscribeAtBottom()
    }
    if (state) {
      state.detach()
    }
  })
</script>

<section class="terminal-shell">
  <header class="terminal-shell__header">
    <div>
      <p class="label">{displayTitle}</p>
      {#if skillsLabel}
        <p class="subtitle">Skills: {skillsLabel}</p>
      {/if}
      <div class="status-row">
        <p class="status">{statusLabel}</p>
        {#if historyStatus === 'loading' || historyStatus === 'slow'}
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
  <CommandInput
    {terminalId}
    bind:this={commandInput}
    onSubmit={handleSubmit}
    disabled={inputDisabled}
    directInput={directInputEnabled}
    onDirectInputChange={handleDirectInputChange}
    showScrollButton={!atBottom}
    onScrollToBottom={handleScrollToBottom}
  />
</section>

<style>
  .terminal-shell {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr) auto;
    height: calc(100vh - 64px);
    background: #101010;
    border-radius: 20px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    box-shadow: 0 20px 50px rgba(10, 10, 10, 0.35);
    overflow: hidden;
    position: relative;
  }

  .terminal-shell__body {
    min-height: 0;
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

  .subtitle {
    margin: 0.2rem 0 0;
    font-size: 0.7rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: rgba(242, 239, 233, 0.5);
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
