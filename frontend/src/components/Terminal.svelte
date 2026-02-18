<script>
  import { onDestroy, onMount } from 'svelte'
  import TerminalCanvas from './TerminalCanvas.svelte'
  import TerminalTextView from './TerminalTextView.svelte'
  import CommandInput from './CommandInput.svelte'
  import TerminalShell from './TerminalShell.svelte'
  import { apiFetch, buildApiPath } from '../lib/api.js'
  import { getTerminalState } from '../lib/terminalStore.js'

  export let sessionId = ''
  export let title = ''
  export let promptFiles = []
  export let visible = true
  export let sessionInterface = ''
  export let sessionRunner = ''
  export let tmuxSessionName = ''
  export let guiModules = []
  export let planSidebarOpen = false
  export let onTogglePlan = () => {}
  export let onRequestClose = () => {}
  export let showInput = true
  export let forceDirectInput = false
  export let allowMouseReporting = false
  export let showCloseButton = true

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
  let attachedSessionId = ''
  let attachedInterface = ''
  let attachedRunner = ''
  let attachedAllowMouseReporting = false
  let wasVisible = false
  let pendingFocus = false
  let displayTitle = ''
  let promptFilesLabel = ''
  let interfaceValue = ''
  let runnerValue = ''
  let tmuxSessionValue = ''
  let isCLI = false
  let isExternal = false
  let hasPlanModule = false
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
    if (!sessionId) return
    state = getTerminalState(sessionId, interfaceValue, runnerValue, { allowMouseReporting })
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

  const handleDirectInputChange = (enabled) => {
    if (forceDirectInput) return
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
  $: runnerValue = typeof sessionRunner === 'string' ? sessionRunner.trim().toLowerCase() : ''
  $: tmuxSessionValue = typeof tmuxSessionName === 'string' ? tmuxSessionName.trim() : ''
  $: isCLI = interfaceValue === 'cli'
  $: isExternal = runnerValue === 'external'
  $: hasPlanModule =
    Array.isArray(guiModules) &&
    guiModules.some((entry) => String(entry || '').trim().toLowerCase() === 'plan-progress')

  $: {
    if (!sessionId) {
      if (state) {
        detachState()
      }
      attachedSessionId = ''
      attachedInterface = ''
      attachedRunner = ''
      attachedAllowMouseReporting = false
    } else if (
      sessionId !== attachedSessionId ||
      interfaceValue !== attachedInterface ||
      runnerValue !== attachedRunner ||
      Boolean(allowMouseReporting) !== attachedAllowMouseReporting
    ) {
      if (state) {
        detachState()
      }
      attachedSessionId = sessionId
      attachedInterface = interfaceValue
      attachedRunner = runnerValue
      attachedAllowMouseReporting = Boolean(allowMouseReporting)
      attachState()
    }
  }
  $: if (forceDirectInput && !directInputEnabled) {
    directInputEnabled = true
    state?.setDirectInput?.(true)
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
  $: inputDisabled = status !== 'connected' || !sessionId || isExternal || !showInput
  $: displayTitle = sessionId ? sessionId : 'Session —'
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
  sessionId={sessionId}
  {historyStatus}
  {canReconnect}
  showPlanButton={hasPlanModule}
  planButtonActive={planSidebarOpen}
  showBottomButton={!atBottom}
  {showCloseButton}
  onReconnect={handleReconnect}
  onScrollToBottom={handleScrollToBottom}
  onPlanToggle={onTogglePlan}
  onRequestClose={onRequestClose}
>
  <svelte:fragment slot="canvas">
    {#if isExternal}
      <div class="terminal-external">
        <p>This session is managed in tmux.</p>
        {#if tmuxSessionValue}
          <p>Attach with: <code>tmux attach -t "{tmuxSessionValue}"</code></p>
          <p>Then switch with: <code>tmux select-window -t "{sessionId}"</code></p>
        {:else}
          <p>Attach with: <code>tmux attach</code></p>
          <p>If needed, list sessions first: <code>tmux ls</code></p>
        {/if}
      </div>
    {:else if isCLI}
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
  <svelte:fragment slot="input">
    {#if showInput}
      <CommandInput
        sessionId={sessionId}
        agentName={title}
        bind:this={commandInput}
        onSubmit={handleSubmit}
        disabled={inputDisabled}
        directInput={directInputEnabled}
        showDirectInputToggle={isCLI && !isExternal && !forceDirectInput}
        onDirectInputChange={handleDirectInputChange}
      />
    {/if}
  </svelte:fragment>
</TerminalShell>

<style>
  .terminal-external {
    display: grid;
    gap: 0.5rem;
    padding: 1rem;
    border: 1px solid rgba(var(--color-text-rgb), 0.12);
    border-radius: 12px;
    background: rgba(var(--color-surface-rgb), 0.6);
  }

  .terminal-external p {
    margin: 0;
  }
</style>
