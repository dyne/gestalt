<script>
  import { createEventDispatcher } from 'svelte'

  export let trigger = null
  export let activityDefs = []
  export let bindings = []
  export let disabled = false

  const dispatch = createEventDispatcher()

  let configDialogRef = null
  let configDialogOpen = false
  let configActivity = null
  let configDraft = {}
  let dragOver = ''

  const triggerId = () => trigger?.id || ''

  $: boundIds = new Set((bindings || []).map((binding) => binding.activity_id))
  $: availableActivities = (activityDefs || []).filter((def) => !boundIds.has(def.id))
  $: assignedActivities = (bindings || [])
    .map((binding) => {
      const def = (activityDefs || []).find((item) => item.id === binding.activity_id)
      return {
        binding,
        def: def || { id: binding.activity_id, label: binding.activity_id, fields: [] },
      }
    })
    .sort((a, b) => {
      const left = a.def.label || a.def.id
      const right = b.def.label || b.def.id
      return left.localeCompare(right)
    })

  const openConfigDialog = (item) => {
    configActivity = item
    configDraft = { ...(item?.binding?.config || {}) }
    configDialogOpen = true
    if (configDialogRef?.showModal) {
      configDialogRef.showModal()
    }
  }

  const closeConfigDialog = () => {
    configDialogOpen = false
    if (configDialogRef?.open && typeof configDialogRef.close === 'function') {
      configDialogRef.close()
    }
  }

  const updateConfigField = (key, value) => {
    configDraft = { ...configDraft, [key]: value }
  }

  const saveConfig = () => {
    if (!configActivity) return
    dispatch('update_activity_config', {
      trigger_id: triggerId(),
      activity_id: configActivity.def.id,
      config: configDraft,
    })
    closeConfigDialog()
  }

  const assignActivity = (activityId, via) => {
    const id = triggerId()
    if (!id) return
    dispatch('assign_activity', { trigger_id: id, activity_id: activityId, via })
  }

  const unassignActivity = (activityId, via) => {
    const id = triggerId()
    if (!id) return
    dispatch('unassign_activity', { trigger_id: id, activity_id: activityId, via })
  }

  const handleDragStart = (event, activityId, source) => {
    if (!event?.dataTransfer) return
    const payload = JSON.stringify({ activity_id: activityId, source })
    event.dataTransfer.setData('application/json', payload)
    event.dataTransfer.setData('text/plain', payload)
    event.dataTransfer.effectAllowed = 'move'
  }

  const parseDragPayload = (event) => {
    const raw =
      event?.dataTransfer?.getData('application/json') ||
      event?.dataTransfer?.getData('text/plain')
    if (!raw) return null
    try {
      const parsed = JSON.parse(raw)
      return {
        activityId: parsed.activity_id || parsed.activityId,
        source: parsed.source || parsed.from || '',
      }
    } catch {
      return { activityId: raw, source: '' }
    }
  }

  const handleDrop = (event, target) => {
    event.preventDefault()
    const payload = parseDragPayload(event)
    dragOver = ''
    if (!payload?.activityId) return
    const activityId = payload.activityId
    if (target === 'assigned' && !boundIds.has(activityId)) {
      assignActivity(activityId, 'dnd')
    }
    if (target === 'available' && boundIds.has(activityId)) {
      unassignActivity(activityId, 'dnd')
    }
  }

  const handleDragOver = (event, target) => {
    event.preventDefault()
    dragOver = target
  }

  const handleDragLeave = () => {
    dragOver = ''
  }
</script>

<section class="assigner">
  {#if !trigger}
    <div class="assigner-empty">Select a trigger to assign activities.</div>
  {:else}
    <div class="assigner-columns">
      <div
        class="assigner-column"
        class:assigner-column--dragover={dragOver === 'available'}
        data-dropzone="available"
        role="group"
        aria-label="Available activities dropzone"
        on:dragover={(event) => handleDragOver(event, 'available')}
        on:dragleave={handleDragLeave}
        on:drop={(event) => handleDrop(event, 'available')}
      >
        <header>
          <h3>Available</h3>
          <p>Activities you can add.</p>
        </header>
        {#if availableActivities.length === 0}
          <p class="assigner-empty">All activities are assigned.</p>
        {:else}
          <ul class="assigner-list">
            {#each availableActivities as def (def.id)}
              <li
                class="assigner-card"
                data-activity-id={def.id}
                data-source="available"
                draggable={!disabled}
                on:dragstart={(event) => handleDragStart(event, def.id, 'available')}
              >
                <div>
                  <strong>{def.label}</strong>
                  {#if def.description}
                    <p>{def.description}</p>
                  {/if}
                </div>
                <button
                  type="button"
                  class="assigner-action"
                  on:click={() => assignActivity(def.id, 'button')}
                  disabled={disabled}
                  aria-label={`Add ${def.label}`}
                >
                  Add
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>

      <div
        class="assigner-column"
        class:assigner-column--dragover={dragOver === 'assigned'}
        data-dropzone="assigned"
        role="group"
        aria-label="Assigned activities dropzone"
        on:dragover={(event) => handleDragOver(event, 'assigned')}
        on:dragleave={handleDragLeave}
        on:drop={(event) => handleDrop(event, 'assigned')}
      >
        <header>
          <h3>Assigned</h3>
          <p>Activities that will run on this trigger.</p>
        </header>
        {#if assignedActivities.length === 0}
          <p class="assigner-empty">No activities assigned.</p>
        {:else}
          <ul class="assigner-list">
            {#each assignedActivities as item (item.def.id)}
              <li
                class="assigner-card"
                data-activity-id={item.def.id}
                data-source="assigned"
                draggable={!disabled}
                on:dragstart={(event) => handleDragStart(event, item.def.id, 'assigned')}
              >
                <div>
                  <strong>{item.def.label}</strong>
                  {#if item.def.description}
                    <p>{item.def.description}</p>
                  {/if}
                </div>
                <div class="assigner-actions">
                  <button
                    type="button"
                    class="assigner-action assigner-action--ghost"
                    on:click={() => openConfigDialog(item)}
                    disabled={disabled}
                    aria-label={`Configure ${item.def.label}`}
                  >
                    Configure
                  </button>
                  <button
                    type="button"
                    class="assigner-action"
                    on:click={() => unassignActivity(item.def.id, 'button')}
                    disabled={disabled}
                    aria-label={`Remove ${item.def.label}`}
                  >
                    Remove
                  </button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    </div>
  {/if}
</section>

<dialog class="assigner-dialog" bind:this={configDialogRef} open={configDialogOpen} on:close={() => (configDialogOpen = false)}>
  <form class="assigner-dialog__form" on:submit|preventDefault={saveConfig}>
    <header>
      <h3>Configure activity</h3>
      <p>{configActivity?.def?.label}</p>
    </header>
    {#if configActivity?.def?.fields?.length}
      {#each configActivity.def.fields as field (field.key)}
        <label class="assigner-label" for={`field-${field.key}`}>{field.label}</label>
        {#if field.type === 'bool'}
          <input
            id={`field-${field.key}`}
            type="checkbox"
            checked={Boolean(configDraft[field.key])}
            on:change={(event) => updateConfigField(field.key, event.target.checked)}
          />
        {:else if field.type === 'int'}
          <input
            id={`field-${field.key}`}
            type="number"
            value={configDraft[field.key] ?? ''}
            on:input={(event) => updateConfigField(field.key, Number(event.target.value || 0))}
          />
        {:else}
          <input
            id={`field-${field.key}`}
            type="text"
            value={configDraft[field.key] ?? ''}
            on:input={(event) => updateConfigField(field.key, event.target.value)}
          />
        {/if}
      {/each}
    {:else}
      <p class="assigner-empty">No configurable fields.</p>
    {/if}
    <div class="assigner-dialog__actions">
      <button type="submit" class="assigner-action">Save</button>
      <button type="button" class="assigner-action assigner-action--ghost" on:click={closeConfigDialog}>
        Cancel
      </button>
    </div>
  </form>
</dialog>

<style>
  .assigner {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .assigner-columns {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 1rem;
  }

  .assigner-column {
    padding: 1rem;
    border-radius: 14px;
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    background: rgba(var(--color-surface-rgb), 0.6);
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .assigner-column h3 {
    margin: 0;
    font-size: 1rem;
  }

  .assigner-column p {
    margin: 0.25rem 0 0;
    font-size: 0.75rem;
    color: var(--color-text-muted);
  }

  .assigner-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .assigner-card {
    display: flex;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.75rem;
    border-radius: 12px;
    border: 1px solid rgba(var(--color-text-rgb), 0.1);
    background: rgba(var(--color-surface-rgb), 0.85);
  }

  .assigner-column--dragover {
    border-color: rgba(var(--color-accent-rgb), 0.6);
    box-shadow: 0 0 0 1px rgba(var(--color-accent-rgb), 0.4);
  }

  .assigner-card strong {
    display: block;
    font-size: 0.85rem;
  }

  .assigner-card p {
    margin: 0.35rem 0 0;
    font-size: 0.7rem;
    color: var(--color-text-muted);
  }

  .assigner-actions {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .assigner-action {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.35rem 0.75rem;
    font-size: 0.7rem;
    font-weight: 600;
    background: rgba(var(--color-surface-rgb), 0.95);
    color: var(--color-text);
    cursor: pointer;
  }

  .assigner-action--ghost {
    background: transparent;
  }

  .assigner-empty {
    margin: 0;
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .assigner-dialog {
    border: none;
    border-radius: 18px;
    padding: 0;
    background: rgba(8, 12, 18, 0.96);
    color: var(--color-text);
    width: min(420px, 90vw);
  }

  .assigner-dialog__form {
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
  }

  .assigner-dialog__form header h3 {
    margin: 0;
  }

  .assigner-label {
    font-size: 0.8rem;
    font-weight: 600;
  }

  .assigner-dialog__form input[type='text'],
  .assigner-dialog__form input[type='number'] {
    border-radius: 10px;
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    padding: 0.45rem 0.6rem;
    background: rgba(var(--color-surface-rgb), 0.95);
    color: var(--color-text);
  }

  .assigner-dialog__actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.6rem;
    margin-top: 0.5rem;
  }
</style>
