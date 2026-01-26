<script>
  import { createEventDispatcher } from 'svelte'
  import { formatRelativeTime } from '../lib/timeUtils.js'
  import { formatLogEntryForClipboard } from '../lib/logEntry.js'
  import { notificationStore } from '../lib/notificationStore.js'

  export let entry = null
  export let open = false

  const dispatch = createEventDispatcher()

  let dialog
  let rawOpen = false

  const setDialogOpen = (shouldOpen) => {
    if (!dialog) return
    if (shouldOpen) {
      if (dialog.open) return
      if (typeof dialog.showModal === 'function') {
        dialog.showModal()
      } else {
        dialog.open = true
      }
      return
    }
    if (!dialog.open) return
    if (typeof dialog.close === 'function') {
      dialog.close()
    } else {
      dialog.open = false
    }
  }

  const requestClose = () => {
    setDialogOpen(false)
    dispatch('close')
  }

  const handleBackdropClick = (event) => {
    if (event.target === dialog) {
      requestClose()
    }
  }

  const handleCancel = (event) => {
    event.preventDefault()
    requestClose()
  }

  const handleRawToggle = (event) => {
    rawOpen = Boolean(event?.currentTarget?.open)
  }

  const copyText = async (text, successMessage) => {
    if (!text) {
      notificationStore.addNotification('error', 'Nothing to copy.')
      return
    }
    const clipboard = navigator?.clipboard
    if (!clipboard?.writeText) {
      notificationStore.addNotification('error', 'Clipboard is unavailable.')
      return
    }
    try {
      await clipboard.writeText(text)
      notificationStore.addNotification('info', successMessage)
    } catch (err) {
      notificationStore.addNotification('error', 'Failed to copy to clipboard.')
    }
  }

  const handleCopyJson = async () => {
    if (!entry) return
    await copyText(formatLogEntryForClipboard(entry, { format: 'json' }), 'Copied log entry JSON.')
  }

  const handleCopyMessage = async () => {
    if (!entry) return
    await copyText(entry.message || '', 'Copied log message.')
  }

  $: if (dialog) {
    setDialogOpen(Boolean(open && entry))
  }
  $: relativeTime = entry ? formatRelativeTime(entry.timestamp) || '—' : '—'
  $: contextEntries = entry ? Object.entries(entry.context || {}).sort(([a], [b]) => a.localeCompare(b)) : []
</script>

<dialog
  class="log-details"
  bind:this={dialog}
  on:click={handleBackdropClick}
  on:cancel={handleCancel}
>
  <header class="log-details__header">
    <div class="log-details__meta">
      <span class={`log-details__badge log-details__badge--${entry?.level || 'info'}`}>
        {entry?.level || 'info'}
      </span>
      <span class="log-details__time" title={entry?.timestamp || ''}>{relativeTime}</span>
    </div>
    <button class="log-details__close" type="button" on:click={requestClose}>Close</button>
  </header>

  <div class="log-details__body">
    <section class="log-details__section">
      <span class="log-details__label">Message</span>
      <p class="log-details__message">{entry?.message || '—'}</p>
    </section>

    <section class="log-details__section">
      <span class="log-details__label">Context</span>
      {#if contextEntries.length === 0}
        <p class="log-details__empty">No context fields.</p>
      {:else}
        <div class="log-details__context">
          <table>
            <tbody>
              {#each contextEntries as [key, value]}
                <tr>
                  <th scope="row">{key}</th>
                  <td>{value}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>

    {#if entry?.raw}
      <details class="log-details__raw" on:toggle={handleRawToggle}>
        <summary>Raw JSON</summary>
        {#if rawOpen}
          <pre>{JSON.stringify(entry.raw, null, 2)}</pre>
        {/if}
      </details>
    {/if}
  </div>

  <footer class="log-details__actions">
    <button class="log-details__button" type="button" on:click={handleCopyJson} disabled={!entry}>
      Copy JSON
    </button>
    <button
      class="log-details__button log-details__button--ghost"
      type="button"
      on:click={handleCopyMessage}
      disabled={!entry || !entry?.message}
    >
      Copy message
    </button>
  </footer>
</dialog>

<style>
  .log-details {
    background: var(--color-surface);
    border: 1px solid rgba(var(--color-text-rgb), 0.1);
    border-radius: 20px;
    padding: 1.5rem;
    width: min(720px, 94vw);
    box-shadow: 0 24px 60px rgba(var(--shadow-color-rgb), 0.25);
    color: var(--color-text);
  }

  .log-details::backdrop {
    background: rgba(var(--shadow-color-rgb), 0.6);
  }

  .log-details__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }

  .log-details__meta {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .log-details__badge {
    text-transform: uppercase;
    letter-spacing: 0.12em;
    font-size: 0.7rem;
    padding: 0.25rem 0.6rem;
    border-radius: 999px;
    background: var(--color-contrast-bg);
    color: var(--color-contrast-text);
  }

  .log-details__badge--warning {
    background: var(--color-warning);
  }

  .log-details__badge--debug {
    background: var(--color-border);
    color: var(--color-text);
  }

  .log-details__badge--error {
    background: var(--color-danger);
  }

  .log-details__time {
    font-size: 0.8rem;
  }

  .log-details__close {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.35rem 0.9rem;
    background: transparent;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    color: var(--color-text);
  }

  .log-details__body {
    margin-top: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  .log-details__section {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .log-details__label {
    font-size: 0.75rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .log-details__message {
    margin: 0;
    font-size: 1rem;
    color: var(--color-text);
    user-select: text;
  }

  .log-details__empty {
    margin: 0;
    color: var(--color-text-subtle);
  }

  .log-details__context {
    max-height: 220px;
    overflow: auto;
    border-radius: 12px;
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    background: rgba(var(--color-surface-rgb), 0.7);
  }

  .log-details__context table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85rem;
  }

  .log-details__context th,
  .log-details__context td {
    padding: 0.6rem 0.8rem;
    border-bottom: 1px solid rgba(var(--color-text-rgb), 0.08);
    text-align: left;
  }

  .log-details__context th {
    width: 30%;
    font-weight: 600;
    color: var(--color-text-muted);
  }

  .log-details__context td {
    font-family: 'SFMono-Regular', Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
    color: var(--color-text);
    word-break: break-word;
  }

  .log-details__raw {
    border-top: 1px solid rgba(var(--color-text-rgb), 0.08);
    padding-top: 1rem;
  }

  .log-details__raw summary {
    cursor: pointer;
    font-weight: 600;
    color: var(--color-text);
  }

  .log-details__raw pre {
    margin: 0.75rem 0 0;
    background: rgba(var(--color-text-rgb), 0.05);
    padding: 0.75rem;
    border-radius: 12px;
    font-size: 0.75rem;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .log-details__actions {
    margin-top: 1.5rem;
    display: flex;
    justify-content: flex-end;
    gap: 0.75rem;
  }

  .log-details__button {
    border: none;
    border-radius: 999px;
    padding: 0.45rem 1.1rem;
    background: var(--color-contrast-bg);
    color: var(--color-contrast-text);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }

  .log-details__button:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .log-details__button--ghost {
    background: transparent;
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    color: var(--color-text);
  }

  @media (max-width: 720px) {
    .log-details__actions {
      flex-direction: column;
      align-items: stretch;
    }

    .log-details__close {
      align-self: flex-start;
    }
  }
</style>
