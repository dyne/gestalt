<script>
  import { onMount } from 'svelte'
  import Terminal from '../components/Terminal.svelte'
  import { fetchStatus, fetchWorkflows } from '../lib/apiClient.js'
  import { buildTemporalUrl } from '../lib/workflowFormat.js'

  export let terminalId = ''
  export let title = ''
  export let promptFiles = []
  export let visible = true
  export let sessionInterface = ''
  export let role = ''
  export let onDelete = () => {}

  let closeDialog
  let confirmButton
  let temporalUiUrl = ''
  let workflowId = ''
  let workflowRunId = ''
  let temporalUrl = ''
  let lastTerminalId = ''
  let loadId = 0
  let planSidebarState = {}

  const resetWorkflowContext = () => {
    temporalUiUrl = ''
    workflowId = ''
    workflowRunId = ''
  }

  const loadWorkflowContext = async (sessionId) => {
    if (!sessionId) {
      resetWorkflowContext()
      return
    }
    const currentLoad = (loadId += 1)
    try {
      const [nextStatus, workflows] = await Promise.all([
        fetchStatus(),
        fetchWorkflows(),
      ])
      if (currentLoad !== loadId) return
      temporalUiUrl = nextStatus?.temporal_ui_url || ''
      const match = Array.isArray(workflows)
        ? workflows.find((workflow) => String(workflow?.session_id || '') === String(sessionId))
        : null
      workflowId = match?.workflow_id || ''
      workflowRunId = match?.workflow_run_id || ''
    } catch (err) {
      if (currentLoad !== loadId) return
      console.warn('failed to load workflow context', err)
      resetWorkflowContext()
    }
  }

  const openCloseDialog = () => {
    if (!closeDialog || closeDialog.open) return
    closeDialog.showModal()
    requestAnimationFrame(() => confirmButton?.focus())
  }

  const confirmClose = () => {
    if (!terminalId) return
    closeDialog?.close()
    onDelete(terminalId)
  }

  const cancelClose = () => {
    closeDialog?.close()
  }

  const togglePlanSidebar = () => {
    if (!terminalId) return
    planSidebarState = {
      ...planSidebarState,
      [terminalId]: !planSidebarState[terminalId],
    }
  }

  $: planSidebarOpen = terminalId ? Boolean(planSidebarState[terminalId]) : false

  $: if (terminalId && terminalId !== lastTerminalId) {
    lastTerminalId = terminalId
    loadWorkflowContext(terminalId)
  } else if (!terminalId && lastTerminalId) {
    lastTerminalId = ''
    resetWorkflowContext()
  }

  $: temporalUrl = buildTemporalUrl(workflowId, workflowRunId, temporalUiUrl)

  onMount(() => {
    if (terminalId) {
      lastTerminalId = terminalId
      loadWorkflowContext(terminalId)
    }
  })
</script>

<section class="terminal-view">
  {#if terminalId}
    <Terminal
      {terminalId}
      {title}
      {promptFiles}
      {visible}
      {temporalUrl}
      {sessionInterface}
      {role}
      {planSidebarOpen}
      onTogglePlan={togglePlanSidebar}
      onRequestClose={openCloseDialog}
    />
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
