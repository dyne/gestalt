<script>
  import { onDestroy, onMount } from 'svelte'
  import TerminalCanvas from './TerminalCanvas.svelte'
  import TerminalTextView from './TerminalTextView.svelte'
  import CommandInput from './CommandInput.svelte'
  import TerminalShell from './TerminalShell.svelte'
  import { apiFetch, buildApiPath } from '../lib/api.js'
  import { getTerminalState } from '../lib/terminalStore.js'

  export let terminalId = ''
  export let title = ''
  export let promptFiles = []
  export let visible = true
  export let temporalUrl = ''
  export let sessionInterface = ''
  export let onRequestClose = () => {}

  let state
  let status = 'disconnected'
  let canReconnect = false
  let historyStatus = 'idle'
  let statusLabel = ''
  let inputDisabled = true
  let directInputEnabled = false
  let atBottom = true
  let outputSegments = []
  let commandInput
  let textView
  let unsubscribeStatus
  let unsubscribeHistory
  let unsubscribeReconnect
  let unsubscribeAtBottom
  let unsubscribeSegments
  let attachedTerminalId = ''
  let attachedInterface = ''
  let wasVisible = false
  let pendingFocus = false
  let displayTitle = ''
  let promptFilesLabel = ''
  let interfaceValue = ''
  let isCLI = false
  const scrollSensitivity = 1

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
    state = null
  }

  const attachState = () => {
    if (!terminalId) return
    state = getTerminalState(terminalId, interfaceValue)
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
    state.setDirectInput?.(directInputEnabled)
  }

  const handleReconnect = () => {
    if (state) {
      state.reconnect()
    }
  }

  const handleScrollToBottom = () => {
    if (isCLI) {
      state?.scrollToBottom?.()
      return
    }
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

  const handleDirectInputChange = (enabled) => {
    directInputEnabled = enabled
    state?.setDirectInput?.(enabled)
    if (enabled) {
      requestAnimationFrame(() => state?.focus?.())
      return
    }
    requestAnimationFrame(() => commandInput?.focusInput?.())
  }

  const resizeHandler = () => {
    if (!visible || !state) return
    state.scheduleFit?.()
  }

  onMount(() => {
    if (visible) {
      pendingFocus = true
    }
    if (typeof window !== 'undefined') {
      window.addEventListener('resize', resizeHandler)
    }
  })

  $: interfaceValue =
    typeof sessionInterface === 'string' ? sessionInterface.trim().toLowerCase() : ''
  $: isCLI = interfaceValue === 'cli'

  $: {
    if (!terminalId) {
      if (state) {
        detachState()
      }
      attachedTerminalId = ''
      attachedInterface = ''
    } else if (terminalId !== attachedTerminalId || interfaceValue !== attachedInterface) {
      if (state) {
        detachState()
      }
      attachedTerminalId = terminalId
      attachedInterface = interfaceValue
      attachState()
    }
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
        if (isCLI && directInputEnabled) {
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
  $: displayTitle = terminalId ? terminalId : 'Session —'
  $: promptFilesLabel =
    Array.isArray(promptFiles) && promptFiles.length > 0
      ? promptFiles.filter(Boolean).join(', ')
      : ''

  onDestroy(() => {
    detachState()
    if (typeof window !== 'undefined') {
      window.removeEventListener('resize', resizeHandler)
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
  {temporalUrl}
  showBottomButton={!atBottom}
  onReconnect={handleReconnect}
  onScrollToBottom={handleScrollToBottom}
  onRequestClose={onRequestClose}
>
  <svelte:fragment slot="canvas">
    {#if isCLI}
      <TerminalCanvas
        {state}
        {visible}
        {scrollSensitivity}
      />
    {:else}
      <TerminalTextView
        bind:this={textView}
        segments={outputSegments}
        onAtBottomChange={handleAtBottomChange}
      />
    {/if}
  </svelte:fragment>
  <CommandInput
    slot="input"
    {terminalId}
    agentName={title}
    bind:this={commandInput}
    onSubmit={handleSubmit}
    disabled={inputDisabled}
    directInput={directInputEnabled}
    showDirectInputToggle={isCLI}
    onDirectInputChange={handleDirectInputChange}
  />
</TerminalShell>
