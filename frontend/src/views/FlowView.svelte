<script>
  import { parseEventFilterQuery, matchesEventTrigger } from '../lib/eventFilterQuery.js'

  const seedTriggers = [
    {
      id: 'workflow-paused',
      label: 'Workflow paused',
      event_type: 'workflow_paused',
      where: { terminal_id: 't1', agent_name: 'Codex' },
    },
    {
      id: 'file-changed',
      label: 'File changed',
      event_type: 'file_changed',
      where: { path: 'README.md' },
    },
    {
      id: 'terminal-resized',
      label: 'Terminal resized',
      event_type: 'terminal_resized',
      where: { terminal_id: 't2' },
    },
  ]

  let query = ''
  let triggers = seedTriggers
  let selectedTriggerId = triggers[0]?.id || ''

  $: parsed = parseEventFilterQuery(query)
  $: filteredTriggers = triggers.filter((trigger) => matchesEventTrigger(trigger, parsed))
  $: if (filteredTriggers.length === 0) {
    selectedTriggerId = ''
  } else if (!filteredTriggers.some((trigger) => trigger.id === selectedTriggerId)) {
    selectedTriggerId = filteredTriggers[0].id
  }
  $: selectedTrigger = filteredTriggers.find((trigger) => trigger.id === selectedTriggerId) || null
  $: selectedWhereEntries = selectedTrigger ? Object.entries(selectedTrigger.where || {}) : []

  const selectTrigger = (id) => {
    selectedTriggerId = id
  }

  const removeToken = (raw) => {
    const tokens = parsed.tokens.map((token) => token.raw)
    const index = tokens.indexOf(raw)
    if (index === -1) return
    tokens.splice(index, 1)
    query = tokens.join(' ')
  }
</script>

<section class="flow-view">
  <header class="flow-view__header">
    <div>
      <p class="eyebrow">Event-driven automations</p>
      <h1>Flow</h1>
    </div>
  </header>

  <div class="flow-view__body">
    <aside class="flow-rail">
      <label class="field-label" for="flow-query">Search / filters</label>
      <input
        id="flow-query"
        class="field-input"
        type="text"
        placeholder="Search / filters"
        bind:value={query}
      />
      <p class="field-hint">Try `event_type:workflow_paused terminal_id:t1`</p>
      {#if parsed.tokens.length > 0}
        <div class="chip-row" aria-label="Active filters">
          {#each parsed.tokens as token (token.raw)}
            <button class="chip" type="button" on:click={() => removeToken(token.raw)}>
              <span>{token.raw}</span>
              <span class="chip__close" aria-hidden="true">×</span>
            </button>
          {/each}
        </div>
      {/if}

      <div class="trigger-list">
        {#if filteredTriggers.length === 0}
          <p class="empty-state">No triggers match this query.</p>
        {:else}
          {#each filteredTriggers as trigger (trigger.id)}
            <button
              class="trigger-card"
              type="button"
              data-active={selectedTriggerId === trigger.id}
              on:click={() => selectTrigger(trigger.id)}
            >
              <div class="trigger-card__title">{trigger.label}</div>
              <div class="trigger-card__meta">
                <span class="badge badge--accent">{trigger.event_type}</span>
                {#each Object.entries(trigger.where || {}).slice(0, 3) as [key, value]}
                  <span class="badge badge--muted">{key}={value}</span>
                {/each}
              </div>
            </button>
          {/each}
        {/if}
      </div>
    </aside>

    <section class="flow-main">
      {#if selectedTrigger}
        <div class="flow-detail">
          <p class="eyebrow">Selected trigger</p>
          <h2>{selectedTrigger.label}</h2>
          <p class="flow-detail__event">
            Event type: <span>{selectedTrigger.event_type}</span>
          </p>
          <div class="where-grid">
            {#if selectedWhereEntries.length === 0}
              <p class="empty-state">No key/value filters yet.</p>
            {:else}
              {#each selectedWhereEntries as [key, value]}
                <div class="where-row">
                  <span class="where-key">{key}</span>
                  <span class="where-value">{value}</span>
                </div>
              {/each}
            {/if}
          </div>
          <div class="flow-placeholder">
            <p>Activity assignments will appear here.</p>
          </div>
        </div>
      {:else}
        <div class="flow-empty">
          <p>Select a trigger to configure activities.</p>
        </div>
      {/if}
    </section>
  </div>
</section>

<style>
  .flow-view {
    padding: 2.5rem clamp(1.5rem, 4vw, 3.5rem) 3.5rem;
    display: flex;
    flex-direction: column;
    gap: 1.75rem;
  }

  .flow-view__header {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    gap: 1.5rem;
  }

  .eyebrow {
    text-transform: uppercase;
    letter-spacing: 0.24em;
    font-size: 0.7rem;
    color: var(--color-text-muted);
    margin: 0 0 0.6rem;
  }

  h1 {
    margin: 0;
    font-size: clamp(2rem, 3.5vw, 3rem);
    font-weight: 600;
    color: var(--color-text);
  }

  h2 {
    margin: 0;
    font-size: clamp(1.4rem, 2.4vw, 2rem);
    color: var(--color-text);
  }

  .flow-view__body {
    display: grid;
    grid-template-columns: minmax(240px, 320px) minmax(0, 1fr);
    gap: 1.75rem;
  }

  .flow-rail {
    padding: 1.25rem;
    border-radius: 16px;
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    background: rgba(var(--color-surface-rgb), 0.6);
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    min-height: 420px;
  }

  .field-label {
    font-weight: 600;
    font-size: 0.85rem;
    color: var(--color-text);
  }

  .field-input {
    border-radius: 12px;
    padding: 0.7rem 0.85rem;
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    background: rgba(var(--color-surface-rgb), 0.9);
    color: var(--color-text);
    font-size: 0.9rem;
  }

  .field-hint {
    margin: 0;
    font-size: 0.75rem;
    color: var(--color-text-muted);
  }

  .chip-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .chip {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.3rem 0.6rem;
    font-size: 0.72rem;
    display: inline-flex;
    gap: 0.4rem;
    align-items: center;
    color: var(--color-text);
    background: rgba(var(--color-surface-rgb), 0.9);
    cursor: pointer;
  }

  .chip__close {
    font-size: 0.9rem;
  }

  .trigger-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-top: 0.5rem;
  }

  .trigger-card {
    border: 1px solid rgba(var(--color-text-rgb), 0.1);
    border-radius: 14px;
    padding: 0.9rem;
    text-align: left;
    background: rgba(var(--color-surface-rgb), 0.85);
    cursor: pointer;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    color: var(--color-text);
  }

  .trigger-card[data-active='true'] {
    border-color: rgba(var(--color-accent-rgb), 0.6);
    box-shadow: 0 0 0 1px rgba(var(--color-accent-rgb), 0.4);
  }

  .trigger-card__title {
    font-weight: 600;
    font-size: 0.95rem;
  }

  .trigger-card__meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .badge {
    font-size: 0.7rem;
    padding: 0.2rem 0.5rem;
    border-radius: 999px;
    border: 1px solid rgba(var(--color-text-rgb), 0.15);
  }

  .badge--accent {
    background: rgba(var(--color-accent-rgb), 0.15);
    color: var(--color-text);
  }

  .badge--muted {
    background: rgba(var(--color-surface-rgb), 0.8);
    color: var(--color-text-muted);
  }

  .flow-main {
    padding: 1.5rem;
    border-radius: 18px;
    border: 1px solid rgba(var(--color-text-rgb), 0.1);
    background: rgba(var(--color-surface-rgb), 0.55);
    min-height: 420px;
  }

  .flow-detail {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .flow-detail__event {
    margin: 0;
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .flow-detail__event span {
    color: var(--color-text);
    font-weight: 600;
  }

  .where-grid {
    display: grid;
    gap: 0.6rem;
  }

  .where-row {
    display: flex;
    justify-content: space-between;
    gap: 0.6rem;
    padding: 0.55rem 0.75rem;
    border-radius: 10px;
    background: rgba(var(--color-surface-rgb), 0.9);
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
  }

  .where-key {
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    color: var(--color-text-muted);
  }

  .where-value {
    font-size: 0.85rem;
    color: var(--color-text);
    font-family: var(--font-mono);
  }

  .flow-placeholder {
    padding: 1rem;
    border-radius: 14px;
    border: 1px dashed rgba(var(--color-text-rgb), 0.2);
    color: var(--color-text-muted);
    font-size: 0.85rem;
  }

  .flow-empty {
    display: grid;
    place-items: center;
    height: 100%;
    color: var(--color-text-muted);
  }

  .empty-state {
    margin: 0;
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  @media (max-width: 900px) {
    .flow-view__body {
      grid-template-columns: 1fr;
    }

    .flow-main,
    .flow-rail {
      min-height: 0;
    }
  }

  @media (max-width: 720px) {
    .flow-view__header {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
