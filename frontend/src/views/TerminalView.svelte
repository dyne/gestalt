<script>
  import Terminal from '../components/Terminal.svelte'
  import PlanSidebar from '../components/PlanSidebar.svelte'

  export let sessionId = ''
  export let title = ''
  export let promptFiles = []
  export let visible = true
  export let sessionInterface = ''
  export let sessionRunner = ''
  export let tmuxSessionName = ''
  export let guiModules = []
  export let onDelete = () => {}

  let closeDialog
  let confirmButton
  let lastTerminalId = ''
  let planSidebarState = {}

  const openCloseDialog = () => {
    if (!closeDialog || closeDialog.open) return
    closeDialog.showModal()
    requestAnimationFrame(() => confirmButton?.focus())
  }

  const confirmClose = () => {
    if (!sessionId) return
    closeDialog?.close()
    onDelete(sessionId)
  }

  const cancelClose = () => {
    closeDialog?.close()
  }

  const togglePlanSidebar = () => {
    if (!sessionId) return
    planSidebarState = {
      ...planSidebarState,
      [sessionId]: !planSidebarState[sessionId],
    }
  }

  $: planSidebarOpen = sessionId ? Boolean(planSidebarState[sessionId]) : false
  $: hasTerminalModule =
    Array.isArray(guiModules) &&
    guiModules.some((entry) => String(entry || '').trim().toLowerCase() === 'console')
  $: hasPlanModule =
    Array.isArray(guiModules) &&
    guiModules.some((entry) => String(entry || '').trim().toLowerCase() === 'plan-progress')

  $: if (sessionId && sessionId !== lastTerminalId) {
    lastTerminalId = sessionId
  } else if (!sessionId && lastTerminalId) {
    lastTerminalId = ''
  }
</script>

<section class="terminal-view">
  {#if sessionId}
    <div class="terminal-view__layout" data-plan-open={planSidebarOpen}>
      {#if hasTerminalModule}
        <Terminal
          sessionId={sessionId}
          {title}
          {promptFiles}
          {visible}
          {sessionInterface}
          {sessionRunner}
          {tmuxSessionName}
          {guiModules}
          {planSidebarOpen}
          onTogglePlan={togglePlanSidebar}
          onRequestClose={openCloseDialog}
        />
      {/if}
      {#if hasPlanModule && planSidebarOpen}
        <PlanSidebar
          sessionId={sessionId}
          open={planSidebarOpen}
          onClose={togglePlanSidebar}
        />
      {/if}
    </div>
    <dialog id="close-confirm-dialog" class="close-dialog" bind:this={closeDialog}>
      <h2>Close Session?</h2>
      <p>This will stop the session. Any unsaved work will be lost.</p>
      <div class="close-dialog__actions">
        <button
          class="close-dialog__confirm"
          type="button"
          on:click={confirmClose}
          bind:this={confirmButton}
        >
          Close
        </button>
        <button class="close-dialog__cancel" type="button" on:click={cancelClose}>
          Cancel
        </button>
      </div>
    </dialog>
  {:else}
    <div class="empty">
      <h1>No session selected</h1>
      <p>Create a session from the dashboard to begin.</p>
    </div>
  {/if}
</section>

<style>
  .terminal-view {
    width: 100%;
    height: calc(100vh - 64px);
    height: calc(100dvh - 64px);
    min-height: 0;
    position: relative;
    box-sizing: border-box;
  }

  .terminal-view__layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 1rem;
    height: 100%;
    min-height: 0;
  }

  .terminal-view__layout[data-plan-open='true'] {
    grid-template-columns: minmax(0, 1fr) minmax(260px, 340px);
  }

  .close-dialog {
    background: var(--color-surface);
    border: 1px solid rgba(var(--color-text-rgb), 0.1);
    border-radius: 20px;
    padding: 1.5rem;
    width: min(420px, 92vw);
    box-shadow: 0 24px 60px rgba(var(--shadow-color-rgb), 0.25);
  }

  .close-dialog::backdrop {
    background: rgba(var(--shadow-color-rgb), 0.6);
  }

  .close-dialog h2 {
    margin: 0 0 0.5rem;
    font-size: 1.2rem;
  }

  .close-dialog p {
    margin: 0;
    color: var(--color-text-subtle);
  }

  .close-dialog__actions {
    margin-top: 1.5rem;
    display: flex;
    justify-content: flex-end;
    gap: 0.75rem;
  }

  .close-dialog__confirm {
    border: none;
    border-radius: 999px;
    padding: 0.45rem 1.1rem;
    background: var(--color-danger);
    color: var(--color-contrast-text);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }

  .close-dialog__cancel {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.45rem 1.1rem;
    background: transparent;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }

  @media (max-width: 900px) {
    .terminal-view__layout[data-plan-open='true'] {
      grid-template-columns: 1fr;
    }
  }

  .empty {
    padding: 2.5rem;
    border-radius: 24px;
    background: var(--color-surface);
    border: 1px solid rgba(var(--color-text-rgb), 0.1);
    max-width: 420px;
  }

  h1 {
    margin: 0 0 0.5rem;
    font-size: 1.8rem;
  }

  p {
    margin: 0;
    color: var(--color-text-muted);
  }
</style>
