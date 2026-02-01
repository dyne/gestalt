<script>
  export let displayTitle = ''
  export let promptFilesLabel = ''
  export let statusLabel = ''
  export let terminalId = ''
  export let historyStatus = 'idle'
  export let canReconnect = false
  export let temporalUrl = ''
  export let showBottomButton = false
  export let onReconnect = () => {}
  export let onScrollToBottom = () => {}
  export let onRequestClose = () => {}
</script>

<section class="terminal-shell">
  <header class="terminal-shell__header">
    <div class="header-line">
      <span class="label">{displayTitle}</span>
      {#if terminalId && terminalId !== displayTitle}
        <span class="separator">|</span>
        <span class="terminal-id">ID: {terminalId}</span>
      {/if}
      {#if promptFilesLabel}
        <span class="separator">|</span>
        <span class="subtitle">Prompts: {promptFilesLabel}</span>
      {/if}
      <span class="separator">|</span>
      <span class="status">{statusLabel}</span>
      {#if historyStatus === 'loading' || historyStatus === 'slow'}
        <span class="separator">|</span>
        <span class="status history-status">Loading history...</span>
      {/if}
      {#if canReconnect}
        <button class="reconnect" type="button" on:click={onReconnect}>
          Reconnect
        </button>
      {/if}
    </div>
    <div class="header-actions">
      {#if temporalUrl}
        <a
          class="header-button"
          href={temporalUrl}
          target="_blank"
          rel="noopener noreferrer"
        >
          Temporal
        </a>
      {:else}
        <button class="header-button header-button--disabled" type="button" disabled>
          Temporal
        </button>
      {/if}
      {#if showBottomButton}
        <button class="header-button" type="button" on:click={onScrollToBottom}>
          Bottom
        </button>
      {/if}
      <button class="terminal-close" type="button" on:click={onRequestClose}>
        Close
      </button>
    </div>
  </header>
  <slot name="canvas"></slot>
  <slot name="input"></slot>
</section>

<style>
  .terminal-shell {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr) auto;
    height: 100%;
    min-height: 0;
    width: 100%;
    min-width: 0;
    background: var(--terminal-bg);
    border-radius: 20px;
    border: 1px solid rgba(var(--terminal-border-rgb), 0.12);
    box-shadow: 0 20px 50px rgba(var(--shadow-color-rgb), 0.35);
    overflow: hidden;
    position: relative;
    padding-bottom: env(safe-area-inset-bottom);
  }

  .terminal-shell__header {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    padding: 0.9rem 1.2rem;
    background: var(--terminal-panel);
    border-bottom: 1px solid rgba(var(--terminal-border-rgb), 0.12);
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
    color: rgba(var(--color-text-rgb), 0.7);
    overflow-wrap: anywhere;
  }

  .subtitle {
    margin: 0;
    display: inline-flex;
    font-size: 0.7rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: rgba(var(--color-text-rgb), 0.5);
    overflow-wrap: anywhere;
  }

  .terminal-id {
    margin: 0;
    display: inline-flex;
    font-size: 0.7rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: rgba(var(--color-text-rgb), 0.55);
    overflow-wrap: anywhere;
  }

  .status {
    margin: 0;
    display: inline-flex;
    font-size: 0.75rem;
    color: rgba(var(--color-text-rgb), 0.5);
    text-transform: uppercase;
    letter-spacing: 0.12em;
  }

  .separator {
    color: rgba(var(--color-text-rgb), 0.25);
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .reconnect {
    border: 0;
    border-radius: 999px;
    padding: 0.2rem 0.7rem;
    background: rgba(var(--color-text-rgb), 0.16);
    color: rgba(var(--color-text-rgb), 0.95);
    font-size: 0.65rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    cursor: pointer;
  }

  .reconnect:hover {
    background: rgba(var(--color-text-rgb), 0.24);
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

  .header-button {
    border: 1px solid rgba(var(--color-text-rgb), 0.18);
    border-radius: 999px;
    padding: 0.4rem 0.9rem;
    background: rgba(var(--color-text-rgb), 0.08);
    color: rgba(var(--color-text-rgb), 0.9);
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    cursor: pointer;
    white-space: nowrap;
    text-decoration: none;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }

  .header-button:hover {
    background: rgba(var(--color-text-rgb), 0.16);
  }

  .header-button--disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .terminal-close {
    border: 1px solid rgba(var(--color-text-rgb), 0.18);
    border-radius: 999px;
    padding: 0.4rem 0.9rem;
    background: rgba(var(--color-text-rgb), 0.08);
    color: rgba(var(--color-text-rgb), 0.9);
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    cursor: pointer;
    white-space: nowrap;
  }

  .terminal-close:hover {
    background: rgba(var(--color-text-rgb), 0.16);
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
