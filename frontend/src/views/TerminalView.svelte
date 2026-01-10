<script>
  import Terminal from '../components/Terminal.svelte'

  export let terminalId = ''
  export let title = ''
  export let promptFiles = []
  export let visible = true
  export let onDelete = () => {}

  let closeDialog
  let confirmButton

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
</script>

<section class="terminal-view">
  {#if terminalId}
    <Terminal {terminalId} {title} {promptFiles} {visible} onRequestClose={openCloseDialog} />
    <dialog id="close-confirm-dialog" class="close-dialog" bind:this={closeDialog}>
      <h2>Close Terminal?</h2>
      <p>This will stop the terminal session. Any unsaved work will be lost.</p>
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
      <h1>No terminal selected</h1>
      <p>Create a terminal from the dashboard to begin.</p>
    </div>
  {/if}
</section>

<style>
  .terminal-view {
    width: 100%;
    height: 100%;
    position: relative;
  }

  .close-dialog {
    border: 1px solid rgba(20, 20, 20, 0.1);
    border-radius: 20px;
    padding: 1.5rem;
    width: min(420px, 92vw);
    box-shadow: 0 24px 60px rgba(10, 10, 10, 0.25);
  }

  .close-dialog::backdrop {
    background: rgba(15, 15, 15, 0.6);
  }

  .close-dialog h2 {
    margin: 0 0 0.5rem;
    font-size: 1.2rem;
  }

  .close-dialog p {
    margin: 0;
    color: #5f5b54;
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
    background: #b34137;
    color: #ffffff;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }

  .close-dialog__cancel {
    border: 1px solid rgba(20, 20, 20, 0.2);
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
    background: #ffffff;
    border: 1px solid rgba(20, 20, 20, 0.1);
    max-width: 420px;
  }

  h1 {
    margin: 0 0 0.5rem;
    font-size: 1.8rem;
  }

  p {
    margin: 0;
    color: #6d6a61;
  }
</style>
