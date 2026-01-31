<script>
  import { onDestroy, onMount } from 'svelte'
  import TerminalTextView from './TerminalTextView.svelte'
  import CommandInput from './CommandInput.svelte'
  import TerminalShell from './TerminalShell.svelte'
  import { apiFetch, buildApiPath } from '../lib/api.js'
  import { getTerminalState } from '../lib/terminalStore.js'

  export let terminalId = ''
  export let title = ''
  export let promptFiles = []
  export let visible = true
  export let onRequestClose = () => {}

  let state
  let bellCount = 0
  let status = 'disconnected'
  let canReconnect = false
  let historyStatus = 'idle'
  let statusLabel = ''
  let inputDisabled = true
  let atBottom = true
  let outputText = ''
  let commandInput
  let textView
  let unsubscribeStatus
  let unsubscribeHistory
  let unsubscribeBell
  let unsubscribeReconnect
  let unsubscribeAtBottom
  let unsubscribeText
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
    if (state.text) {
      unsubscribeText = state.text.subscribe((value) => {
        outputText = value
      })
    }
  }

  const handleReconnect = () => {
    if (state) {
      state.reconnect()
    }
  }

  const handleScrollToBottom = () => {
    textView?.scrollToBottom?.()
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
    apiFetch(buildApiPath('/api/sessions', terminalId, 'input-history'), {
      method: 'POST',
      body: JSON.stringify({ command: trimmed }),
    }).catch((err) => {
      console.warn('failed to record input history', err)
    })
  }

  const handleAtBottomChange = (value) => {
    state?.setAtBottom?.(value)
  }

  const resizeHandler = () => {
    if (!visible || !state) return
    state.scheduleFit()
  }

  onMount(() => {
    attachState()
    if (visible) {
      pendingFocus = true
    }
    window.addEventListener('resize', resizeHandler)
  })

  $: if (state) {
    state.setVisible?.(visible)
  }

  $: {
    if (visible && !wasVisible) {
      pendingFocus = true
    }
    if (visible && pendingFocus) {
      requestAnimationFrame(() => {
        if (!visible || !pendingFocus) return
        if (inputDisabled) return
        commandInput?.focusInput?.()
        pendingFocus = false
      })
    }
    wasVisible = visible
  }

  $: statusLabel = statusLabels[status] || status
  $: inputDisabled = status !== 'connected' || !terminalId
  $: displayTitle = terminalId ? terminalId : 'Session —'
  $: promptFilesLabel =
    Array.isArray(promptFiles) && promptFiles.length > 0
      ? promptFiles.filter(Boolean).join(', ')
      : ''

  onDestroy(() => {
    window.removeEventListener('resize', resizeHandler)
    state?.setVisible?.(false)
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
    if (unsubscribeText) {
      unsubscribeText()
    }
  })
</script>

<TerminalShell
  {displayTitle}
  {promptFilesLabel}
  {statusLabel}
  {terminalId}
  {historyStatus}
  {canReconnect}
  {bellCount}
  onReconnect={handleReconnect}
  onRequestClose={onRequestClose}
>
  <TerminalTextView
    slot="canvas"
    bind:this={textView}
    text={outputText}
    onAtBottomChange={handleAtBottomChange}
  />
  <CommandInput
    slot="input"
    {terminalId}
    agentName={title}
    bind:this={commandInput}
    onSubmit={handleSubmit}
    disabled={inputDisabled}
    showScrollButton={!atBottom}
    onScrollToBottom={handleScrollToBottom}
  />
</TerminalShell>
