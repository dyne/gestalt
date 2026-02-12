<script>
  import { onDestroy, onMount } from 'svelte'

  import CommandInput from '../CommandInput.svelte'
  import TerminalShell from '../TerminalShell.svelte'
  import TerminalTextView from '../TerminalTextView.svelte'
  import { apiFetch, buildApiPath } from '../../lib/api.js'
  import { createTerminalService } from '../../lib/terminal/service_mcp.js'

  export let terminalId = ''
  export let title = ''
  export let promptFiles = []
  export let visible = true
  export let temporalUrl = ''
  export let guiModules = []
  export let planSidebarOpen = false
  export let onTogglePlan = () => {}
  export let onRequestClose = () => {}

  let state
  let status = 'disconnected'
  let historyStatus = 'idle'
  let canReconnect = false
  let atBottom = true
  let outputSegments = []
  let inputDisabled = true
  let statusLabel = ''
  let commandInput
  let textView
  let unsubscribeStatus
  let unsubscribeHistory
  let unsubscribeReconnect
  let unsubscribeAtBottom
  let unsubscribeSegments
  let attachedTerminalId = ''
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

  const detachState = () => {
    state?.setVisible?.(false)
    if (unsubscribeStatus) {
      unsubscribeStatus()
      unsubscribeStatus = null
    }
    if (unsubscribeHistory) {
      unsubscribeHistory()
      unsubscribeHistory = null
    }
    if (unsubscribeReconnect) {
      unsubscribeReconnect()
      unsubscribeReconnect = null
    }
    if (unsubscribeAtBottom) {
      unsubscribeAtBottom()
      unsubscribeAtBottom = null
    }
    if (unsubscribeSegments) {
      unsubscribeSegments()
      unsubscribeSegments = null
    }
    state?.dispose?.()
    state = null
  }

  const attachState = () => {
    if (!terminalId) return
    state = createTerminalService({ terminalId })
    if (!state) return
    unsubscribeStatus = state.status.subscribe((value) => {
      status = value
    })
    unsubscribeHistory = state.historyStatus.subscribe((value) => {
      historyStatus = value
    })
    unsubscribeReconnect = state.canReconnect.subscribe((value) => {
      canReconnect = value
    })
    if (state.atBottom) {
      unsubscribeAtBottom = state.atBottom.subscribe((value) => {
        atBottom = value
      })
    }
    if (state.segments) {
      unsubscribeSegments = state.segments.subscribe((value) => {
        outputSegments = value
      })
    }
  }

  const handleReconnect = () => {
    state?.reconnect?.()
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
    state.appendPrompt?.(payload)
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

  const handleDirectInputChange = () => {}

  $: if (!terminalId) {
    if (state) {
      detachState()
    }
    attachedTerminalId = ''
  } else if (terminalId !== attachedTerminalId) {
    if (state) {
      detachState()
    }
    attachedTerminalId = terminalId
    attachState()
  }

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
        if (!terminalId || status !== 'connected') return
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
  $: hasPlanModule =
    Array.isArray(guiModules) &&
    guiModules.some((entry) => String(entry || '').trim().toLowerCase() === 'plan-progress')

  onMount(() => {
    if (terminalId) {
      pendingFocus = true
    }
  })

  onDestroy(() => {
    detachState()
  })
</script>

<TerminalShell
  {displayTitle}
  {promptFilesLabel}
  {statusLabel}
  {terminalId}
  {historyStatus}
  {canReconnect}
  {temporalUrl}
  showPlanButton={hasPlanModule}
  planButtonActive={planSidebarOpen}
  showBottomButton={!atBottom}
  onReconnect={handleReconnect}
  onScrollToBottom={handleScrollToBottom}
  onPlanToggle={onTogglePlan}
  onRequestClose={onRequestClose}
>
  <svelte:fragment slot="canvas">
    <TerminalTextView
      bind:this={textView}
      segments={outputSegments}
      onAtBottomChange={handleAtBottomChange}
    />
  </svelte:fragment>
  <CommandInput
    slot="input"
    {terminalId}
    agentName={title}
    bind:this={commandInput}
    onSubmit={handleSubmit}
    disabled={inputDisabled}
    directInput={false}
    showDirectInputToggle={false}
    onDirectInputChange={handleDirectInputChange}
  />
</TerminalShell>
