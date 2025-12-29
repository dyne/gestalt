<script>
  import { notificationPreferences } from '../lib/notificationStore.js'

  export let open = false
  export let onClose = () => {}

  const levelOptions = [
    { value: 'all', label: 'All' },
    { value: 'info', label: 'Info and higher' },
    { value: 'warning', label: 'Warning and higher' },
    { value: 'error', label: 'Errors only' },
  ]

  const updatePreferences = (changes) => {
    notificationPreferences.update((prefs) => ({
      ...prefs,
      ...changes,
    }))
  }

  const handleDurationChange = (event) => {
    const value = Number(event.target.value)
    updatePreferences({ durationMs: Number.isFinite(value) ? value : 0 })
  }

  const handleOverlayKeydown = (event) => {
    if (event.key === 'Escape' || event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      onClose()
    }
  }

  const handleOverlayClick = (event) => {
    if (event.target === event.currentTarget) {
      onClose()
    }
  }
</script>

{#if open}
  <div
    class="settings-overlay"
    role="button"
    tabindex="0"
    aria-label="Close notification settings"
    on:click={handleOverlayClick}
    on:keydown={handleOverlayKeydown}
  >
    <section
      class="settings-panel"
      role="dialog"
      aria-modal="true"
      tabindex="-1"
    >
      <header>
        <h2>Notification preferences</h2>
        <button class="close" type="button" on:click={onClose}>Close</button>
      </header>

      <div class="settings-row">
        <label class="checkbox">
          <input
            type="checkbox"
            checked={$notificationPreferences.enabled}
            on:change={(event) => updatePreferences({ enabled: event.target.checked })}
          />
          Enable toast notifications
        </label>
      </div>

      <div class="settings-row">
        <label>
          Auto-dismiss duration (ms)
          <input
            type="number"
            min="0"
            step="500"
            value={$notificationPreferences.durationMs}
            on:input={handleDurationChange}
          />
        </label>
        <p class="hint">Use 0 to keep per-level defaults.</p>
      </div>

      <div class="settings-row">
        <label>
          Toast level filter
          <select
            value={$notificationPreferences.levelFilter}
            on:change={(event) => updatePreferences({ levelFilter: event.target.value })}
          >
            {#each levelOptions as option}
              <option value={option.value}>{option.label}</option>
            {/each}
          </select>
        </label>
      </div>
    </section>
  </div>
{/if}

<style>
  .settings-overlay {
    position: fixed;
    inset: 0;
    background: rgba(15, 15, 15, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 30;
    padding: 1.5rem;
  }

  .settings-panel {
    background: #ffffff;
    border-radius: 20px;
    padding: 1.5rem;
    width: min(420px, 100%);
    border: 1px solid rgba(20, 20, 20, 0.1);
    box-shadow: 0 24px 60px rgba(10, 10, 10, 0.25);
  }

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1rem;
  }

  h2 {
    margin: 0;
    font-size: 1.2rem;
  }

  .close {
    border: 0;
    border-radius: 999px;
    padding: 0.4rem 0.9rem;
    background: #151515;
    color: #f6f3ed;
    font-size: 0.75rem;
    cursor: pointer;
  }

  .settings-row {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    font-size: 0.9rem;
    color: #3b3832;
  }

  .checkbox {
    flex-direction: row;
    align-items: center;
  }

  input[type='number'],
  select {
    border-radius: 12px;
    border: 1px solid rgba(20, 20, 20, 0.2);
    padding: 0.5rem 0.6rem;
    background: #fff;
  }

  input[type='checkbox'] {
    margin-right: 0.5rem;
  }

  .hint {
    margin: 0;
    font-size: 0.75rem;
    color: #6f6b62;
  }
</style>
