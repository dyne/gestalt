<script>
  import { onDestroy, onMount } from 'svelte'
  import TerminalCanvas from './TerminalCanvas.svelte'
  import CommandInput from './CommandInput.svelte'
  import TerminalShell from './TerminalShell.svelte'
  import { apiFetch } from '../lib/api.js'
  import { getTerminalState } from '../lib/terminalStore.js'

  export let terminalId = ''
  export let title = ''
  export let promptFiles = []
  export let visible = true
  export let scrollSensitivity = 1
  export let onRequestClose = () => {}

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
  let promptFilesLabel = ''

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
  $: promptFilesLabel =
    Array.isArray(promptFiles) && promptFiles.length > 0
      ? promptFiles.filter(Boolean).join(', ')
      : ''

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
  })
</script>

<TerminalShell
  {displayTitle}
  {promptFilesLabel}
  {statusLabel}
  {historyStatus}
  {canReconnect}
  {bellCount}
  onReconnect={handleReconnect}
  onRequestClose={onRequestClose}
>
  <TerminalCanvas slot="canvas" {state} {visible} {scrollSensitivity} />
  <CommandInput
    slot="input"
    {terminalId}
    bind:this={commandInput}
    onSubmit={handleSubmit}
    disabled={inputDisabled}
    directInput={directInputEnabled}
    onDirectInputChange={handleDirectInputChange}
    showScrollButton={!atBottom}
    onScrollToBottom={handleScrollToBottom}
  />
</TerminalShell>
