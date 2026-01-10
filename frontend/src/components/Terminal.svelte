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
  export let onRequestClose = () => {}

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
  let wasVisible = false
  let pendingFocus = false
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
    if (visible) {
      pendingFocus = true
    }
    window.addEventListener('resize', resizeHandler)
  })

  $: if (visible) {
    if (state) {
      state.scheduleFit()
    }
  }

  $: {
    if (visible && !wasVisible) {
      pendingFocus = true
    }
    if (visible && pendingFocus) {
      requestAnimationFrame(() => {
        if (!visible || !pendingFocus) return
        if (directInputEnabled) {
          if (status !== 'connected') return
          state?.focus?.()
          pendingFocus = false
          return
        }
        if (inputDisabled) return
        commandInput?.focusInput?.()
        pendingFocus = false
      })
    }
    wasVisible = visible
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
    <div class="header-line">
      <span class="label">{displayTitle}</span>
      {#if skillsLabel}
        <span class="separator">|</span>
        <span class="subtitle">Skills: {skillsLabel}</span>
      {/if}
      <span class="separator">|</span>
      <span class="status">{statusLabel}</span>
      {#if historyStatus === 'loading' || historyStatus === 'slow'}
        <span class="separator">|</span>
        <span class="status history-status">Loading history...</span>
      {/if}
      {#if canReconnect}
        <button class="reconnect" type="button" on:click={handleReconnect}>
          Reconnect
        </button>
      {/if}
    </div>
    <div class="header-actions">
      <div class="bell" aria-live="polite">
        <span>Bell</span>
        <strong>{bellCount}</strong>
      </div>
      <button class="terminal-close" type="button" on:click={onRequestClose}>
        Close
      </button>
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
    width: 100%;
    min-width: 0;
    background: #101010;
    border-radius: 20px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    box-shadow: 0 20px 50px rgba(10, 10, 10, 0.35);
    overflow: hidden;
    position: relative;
  }

  .terminal-shell__body {
    min-height: 0;
    touch-action: pan-y;
    overscroll-behavior: contain;
  }

  .terminal-shell__header {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    padding: 0.9rem 1.2rem;
    background: #171717;
    border-bottom: 1px solid rgba(255, 255, 255, 0.06);
    gap: 1rem;
  }

  .header-line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.35rem;
    min-width: 0;
    flex: 1 1 240px;
  }

  .label {
    margin: 0;
    display: inline-flex;
    font-size: 0.85rem;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: rgba(242, 239, 233, 0.7);
    overflow-wrap: anywhere;
  }

  .subtitle {
    margin: 0;
    display: inline-flex;
    font-size: 0.7rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: rgba(242, 239, 233, 0.5);
    overflow-wrap: anywhere;
  }

  .status {
    margin: 0;
    display: inline-flex;
    font-size: 0.75rem;
    color: rgba(242, 239, 233, 0.5);
    text-transform: uppercase;
    letter-spacing: 0.12em;
  }

  .separator {
    color: rgba(242, 239, 233, 0.25);
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
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

  .header-actions {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex: 0 1 auto;
    flex-wrap: wrap;
    justify-content: flex-end;
    min-width: 0;
    justify-self: end;
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

  .terminal-close {
    border: 1px solid rgba(255, 255, 255, 0.18);
    border-radius: 999px;
    padding: 0.4rem 0.9rem;
    background: rgba(255, 255, 255, 0.08);
    color: rgba(242, 239, 233, 0.9);
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    cursor: pointer;
    white-space: nowrap;
  }

  .terminal-close:hover {
    background: rgba(255, 255, 255, 0.16);
  }

  .terminal-shell__body {
    padding: 0.6rem;
    min-width: 0;
  }

  :global(.xterm) {
    height: 100%;
  }

  :global(.xterm-viewport) {
    border-radius: 12px;
    -webkit-overflow-scrolling: touch;
  }

  @media (max-width: 720px) {
    .terminal-shell {
      min-height: 60vh;
    }

    .terminal-shell__header {
      grid-template-columns: 1fr;
      align-items: flex-start;
      gap: 0.75rem;
    }

    .header-actions {
      width: 100%;
      justify-content: flex-start;
    }
  }
</style>
