<script>
  import { onDestroy, onMount } from 'svelte'

  import CommandInput from '../CommandInput.svelte'
  import TerminalShell from '../TerminalShell.svelte'
  import TerminalTextView from '../TerminalTextView.svelte'
  import { apiFetch, buildApiPath } from '../../lib/api.js'
  import { getTerminalState } from '../../lib/terminalStore.js'
  import { createTerminalService } from '../../lib/terminal/service_mcp.js'

  export let sessionId = ''
  export let title = ''
  export let promptFiles = []
  export let visible = true
  export let temporalUrl = ''
  export let guiModules = []
  export let sessionInterface = ''
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
  let attachedSessionId = ''
  let attachedInterface = ''
  let attachedHasTerminal = false
  let usingSharedState = false
  let wasVisible = false
  let pendingFocus = false
  let displayTitle = ''
  let interfaceValue = ''
  let promptFilesLabel = ''
  let hasTerminalModule = false

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
    if (state && !usingSharedState) {
      state.dispose?.()
    }
    state = null
    usingSharedState = false
  }

  const attachState = () => {
    if (!sessionId) return
    usingSharedState = interfaceValue !== 'cli' && !hasTerminalModule
    state = usingSharedState
      ? getTerminalState(sessionId, interfaceValue)
      : createTerminalService({ terminalId: sessionId })
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
    if (!trimmed || !sessionId) return
    apiFetch(buildApiPath('/api/sessions', sessionId, 'input-history'), {
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

  $:
    interfaceValue =
      typeof sessionInterface === 'string' ? sessionInterface.trim().toLowerCase() : ''
  $: if (!sessionId) {
    if (state) {
      detachState()
    }
    attachedSessionId = ''
    attachedInterface = ''
    attachedHasTerminal = false
  } else if (
    sessionId !== attachedSessionId ||
    interfaceValue !== attachedInterface ||
    hasTerminalModule !== attachedHasTerminal
  ) {
    if (state) {
      detachState()
    }
    attachedSessionId = sessionId
    attachedInterface = interfaceValue
    attachedHasTerminal = hasTerminalModule
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
        if (!sessionId || status !== 'connected') return
        commandInput?.focusInput?.()
        pendingFocus = false
      })
    }
    wasVisible = visible
  }

  $: statusLabel = statusLabels[status] || status
  $: inputDisabled = status !== 'connected' || !sessionId
  $: displayTitle = sessionId ? sessionId : 'Session —'
  $: promptFilesLabel =
    Array.isArray(promptFiles) && promptFiles.length > 0
      ? promptFiles.filter(Boolean).join(', ')
      : ''
  $: hasPlanModule =
    Array.isArray(guiModules) &&
    guiModules.some((entry) => String(entry || '').trim().toLowerCase() === 'plan-progress')
  $: hasTerminalModule =
    Array.isArray(guiModules) &&
    guiModules.some((entry) => String(entry || '').trim().toLowerCase() === 'terminal')

  onMount(() => {
    if (sessionId) {
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
  sessionId={sessionId}
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
      {sessionId}
    agentName={title}
    bind:this={commandInput}
    onSubmit={handleSubmit}
    disabled={inputDisabled}
    directInput={false}
    showDirectInputToggle={false}
    onDirectInputChange={handleDirectInputChange}
  />
</TerminalShell>
