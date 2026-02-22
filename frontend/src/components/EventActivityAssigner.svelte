<script>
  import { createEventDispatcher, tick } from 'svelte'

  export let trigger = null
  export let activityDefs = []
  export let bindings = []
  export let disabled = false

  const dispatch = createEventDispatcher()

  const templateTokenGroups = [
    {
      label: 'Common',
      tokens: [
        '{{summary}}',
        '{{plan_file}}',
        '{{plan_summary}}',
        '{{task_title}}',
        '{{task_state}}',
        '{{git_branch}}',
      ],
    },
    {
      label: 'Always',
      tokens: [
        '{{session_id}}',
        '{{agent_id}}',
        '{{agent_name}}',
        '{{timestamp}}',
        '{{event_id}}',
      ],
    },
    {
      label: 'Notify (advanced)',
      tokens: [
        '{{notify.type}}',
        '{{notify.event_id}}',
        '{{notify.summary}}',
        '{{notify.plan_file}}',
        '{{notify.plan_summary}}',
        '{{notify.task_title}}',
        '{{notify.task_state}}',
        '{{notify.git_branch}}',
      ],
    },
    {
      label: 'Advanced',
      tokens: [
        '{{trigger_id}}',
        '{{activity_id}}',
        '{{output_tail}}',
      ],
    },
  ]

  const baseSampleEvent = {
    session_id: 'coder-1',
    agent_id: 'coder',
    agent_name: 'Coder 1',
    timestamp: '2026-02-11T18:15:00Z',
    git_branch: 'feature/flow-notify',
  }

  const notifySampleBase = {
    ...baseSampleEvent,
    plan_file: '.gestalt/plans/flow-notify-router.org',
    plan_summary: 'Route notify events into Flow.',
    task_title: 'Add notify presets',
  }

  const notifySamples = {
    'plan-new': {
      ...notifySampleBase,
      summary: 'New plan: Flow router updates',
      task_state: 'TODO',
      event_id: 'evt_notify_1',
      'notify.type': 'plan-new',
      'notify.event_id': 'evt_notify_1',
      'notify.summary': 'New plan: Flow router updates',
      'notify.plan_file': '.gestalt/plans/flow-notify-router.org',
      'notify.plan_summary': 'Route notify events into Flow.',
      'notify.task_title': 'Add notify presets',
      'notify.task_state': 'TODO',
    },
    'plan-update': {
      ...notifySampleBase,
      summary: 'Plan update: Add notify preset',
      task_state: 'WIP',
      event_id: 'evt_notify_2',
      'notify.type': 'plan-update',
      'notify.event_id': 'evt_notify_2',
      'notify.summary': 'Plan update: Add notify preset',
      'notify.plan_file': '.gestalt/plans/flow-notify-router.org',
      'notify.plan_summary': 'Route notify events into Flow.',
      'notify.task_title': 'Add notify presets',
      'notify.task_state': 'WIP',
    },
    'work-start': {
      ...notifySampleBase,
      summary: 'Start: Notify flow',
      task_state: 'IN_PROGRESS',
      event_id: 'evt_notify_3',
      'notify.type': 'work-start',
      'notify.event_id': 'evt_notify_3',
      'notify.summary': 'Start: Notify flow',
      'notify.plan_file': '.gestalt/plans/flow-notify-router.org',
      'notify.plan_summary': 'Route notify events into Flow.',
      'notify.task_title': 'Add notify presets',
      'notify.task_state': 'IN_PROGRESS',
    },
    'work-progress': {
      ...notifySampleBase,
      summary: 'Progress: Notify flow',
      task_state: 'IN_PROGRESS',
      event_id: 'evt_notify_4',
      'notify.type': 'work-progress',
      'notify.event_id': 'evt_notify_4',
      'notify.summary': 'Progress: Notify flow',
      'notify.plan_file': '.gestalt/plans/flow-notify-router.org',
      'notify.plan_summary': 'Route notify events into Flow.',
      'notify.task_title': 'Add notify presets',
      'notify.task_state': 'IN_PROGRESS',
    },
    'work-finish': {
      ...notifySampleBase,
      summary: 'Finish: Notify flow',
      task_state: 'DONE',
      event_id: 'evt_notify_5',
      'notify.type': 'work-finish',
      'notify.event_id': 'evt_notify_5',
      'notify.summary': 'Finish: Notify flow',
      'notify.plan_file': '.gestalt/plans/flow-notify-router.org',
      'notify.plan_summary': 'Route notify events into Flow.',
      'notify.task_title': 'Add notify presets',
      'notify.task_state': 'DONE',
    },
    'agent-turn': {
      ...notifySampleBase,
      summary: 'Agent turn complete',
      event_id: 'evt_notify_6',
      'notify.type': 'agent-turn-complete',
      'notify.event_id': 'evt_notify_6',
      'notify.summary': 'Agent turn complete',
      'notify.plan_file': '.gestalt/plans/flow-notify-router.org',
      'notify.plan_summary': 'Route notify events into Flow.',
      'notify.task_title': 'Add notify presets',
    },
    'prompt-voice': {
      ...notifySampleBase,
      summary: 'Voice prompt received',
      event_id: 'evt_notify_7',
      'notify.type': 'prompt-voice',
      'notify.event_id': 'evt_notify_7',
      'notify.summary': 'Voice prompt received',
      'notify.plan_file': '.gestalt/plans/flow-notify-router.org',
      'notify.plan_summary': 'Route notify events into Flow.',
      'notify.task_title': 'Add notify presets',
    },
    'prompt-text': {
      ...notifySampleBase,
      summary: 'Text prompt received',
      event_id: 'evt_notify_8',
      'notify.type': 'prompt-text',
      'notify.event_id': 'evt_notify_8',
      'notify.summary': 'Text prompt received',
      'notify.plan_file': '.gestalt/plans/flow-notify-router.org',
      'notify.plan_summary': 'Route notify events into Flow.',
      'notify.task_title': 'Add notify presets',
    },
  }

  const sampleEventForType = (eventType) => {
    if (eventType && notifySamples[eventType]) {
      return notifySamples[eventType]
    }
    if (eventType === 'file-change') {
      return { ...baseSampleEvent, event_id: 'evt_file_1', path: 'README.md', op: 'write' }
    }
    if (eventType === 'git-branch') {
      return {
        ...baseSampleEvent,
        event_id: 'evt_git_1',
        git_branch: 'feature/notify',
        previous_branch: 'main',
      }
    }
    if (eventType === 'git-commit') {
      return {
        ...baseSampleEvent,
        event_id: 'evt_commit_1',
        git_branch: 'main',
        commit_hash: 'abc1234',
        summary: 'Commit: Add flow router',
      }
    }
    if (eventType && eventType.startsWith('workflow_')) {
      return {
        ...baseSampleEvent,
        event_id: 'evt_workflow_1',
        workflow_state: eventType.replace('workflow_', ''),
      }
    }
    return { ...baseSampleEvent, event_id: 'evt_generic_1' }
  }

  let configDialogRef = null
  let configDialogOpen = false
  let configActivity = null
  let configDraft = {}
  let dragOver = ''
  let previewJson = ''

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

  $: previewSampleEvent = sampleEventForType(trigger?.event_type)
  $: previewOverride = parsePreviewJson(previewJson)
  $: previewFields = previewOverride.fields || previewSampleEvent
  $: previewParseError = previewOverride.error
  $: previewMeta = {
    event_id: previewFields?.event_id || 'evt_123',
    trigger_id: triggerId() || 'trigger',
    activity_id: configActivity?.def?.id || 'activity',
    output_tail: previewFields?.output_tail || 'output tail sample',
  }
  $: hasTemplateFields = Boolean(configActivity?.def?.fields?.some((field) => isTemplateField(field)))

  const openConfigDialog = (item) => {
    configActivity = item
    configDraft = { ...(item?.binding?.config || {}) }
    previewJson = ''
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

  function isTemplateField(field) {
    return Boolean(field?.key && field.key.endsWith('_template'))
  }

  function normalizePreviewFields(source) {
    if (!source || typeof source !== 'object' || Array.isArray(source)) {
      return {}
    }
    const fields = {}
    Object.entries(source).forEach(([key, value]) => {
      if (value === null || value === undefined) return
      if (typeof value === 'object') return
      fields[key] = String(value)
    })
    return fields
  }

  function parsePreviewJson(raw) {
    const trimmed = raw.trim()
    if (!trimmed) return { fields: null, error: '' }
    try {
      const parsed = JSON.parse(trimmed)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        return { fields: null, error: 'Preview data must be a JSON object.' }
      }
      return { fields: normalizePreviewFields(parsed), error: '' }
    } catch {
      return { fields: null, error: 'Preview data is not valid JSON.' }
    }
  }

  function resolveTemplateToken(token, fields, meta) {
    if (!token) return ''
    if (token === 'event_id') return meta.event_id || ''
    if (token === 'trigger_id') return meta.trigger_id || ''
    if (token === 'activity_id') return meta.activity_id || ''
    if (token === 'output_tail') return meta.output_tail || ''
    if (token.startsWith('event.')) {
      const key = token.slice('event.'.length)
      return key ? fields[key] || '' : ''
    }
    return fields[token] || ''
  }

  function renderTemplate(template, fields, meta) {
    if (!template) return ''
    let output = ''
    let remaining = template
    while (true) {
      const start = remaining.indexOf('{{')
      if (start < 0) {
        output += remaining
        break
      }
      output += remaining.slice(0, start)
      remaining = remaining.slice(start + 2)
      const end = remaining.indexOf('}}')
      if (end < 0) {
        output += '{{' + remaining
        break
      }
      const token = remaining.slice(0, end).trim()
      output += resolveTemplateToken(token, fields, meta)
      remaining = remaining.slice(end + 2)
    }
    return output
  }

  const insertToken = async (fieldKey, token) => {
    const element = configDialogRef?.querySelector(`#field-${fieldKey}`)
    const current = typeof configDraft[fieldKey] === 'string' ? configDraft[fieldKey] : ''
    const start = element?.selectionStart ?? current.length
    const end = element?.selectionEnd ?? current.length
    const next = current.slice(0, start) + token + current.slice(end)
    updateConfigField(fieldKey, next)
    await tick()
    const updated = configDialogRef?.querySelector(`#field-${fieldKey}`)
    if (updated?.setSelectionRange) {
      const cursor = start + token.length
      updated.setSelectionRange(cursor, cursor)
    }
    updated?.focus()
  }

  const handleInsertToken = (fieldKey, event) => {
    const token = event?.target?.value
    if (!token) return
    insertToken(fieldKey, token)
    event.target.value = ''
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
        <div class="assigner-field">
          <label class="assigner-label" for={`field-${field.key}`}>{field.label}</label>
          {#if field.type === 'bool'}
            <input
              id={`field-${field.key}`}
              class="assigner-checkbox"
              type="checkbox"
              checked={Boolean(configDraft[field.key])}
              on:change={(event) => updateConfigField(field.key, event.target.checked)}
            />
          {:else if field.type === 'int'}
            <input
              id={`field-${field.key}`}
              class="assigner-input"
              type="number"
              value={configDraft[field.key] ?? ''}
              on:input={(event) => updateConfigField(field.key, Number(event.target.value || 0))}
            />
          {:else if isTemplateField(field)}
            <textarea
              id={`field-${field.key}`}
              class="assigner-input"
              rows="3"
              value={configDraft[field.key] ?? ''}
              on:input={(event) => updateConfigField(field.key, event.target.value)}
            ></textarea>
          {:else}
            <input
              id={`field-${field.key}`}
              class="assigner-input"
              type="text"
              value={configDraft[field.key] ?? ''}
              on:input={(event) => updateConfigField(field.key, event.target.value)}
            />
          {/if}
          {#if isTemplateField(field)}
            <div class="assigner-template-tools">
              <label class="assigner-hint" for={`insert-${field.key}`}>Insert field</label>
              <select
                id={`insert-${field.key}`}
                class="assigner-input"
                on:change={(event) => handleInsertToken(field.key, event)}
              >
                <option value="">Insert field</option>
                {#each templateTokenGroups as group}
                  <optgroup label={group.label}>
                    {#each group.tokens as token}
                      <option value={token}>{token}</option>
                    {/each}
                  </optgroup>
                {/each}
              </select>
            </div>
            <div class="assigner-template-preview">
              <span class="assigner-preview-label">Preview</span>
              <pre class="assigner-preview-output">{renderTemplate(String(configDraft[field.key] ?? ''), previewFields, previewMeta) || 'Preview will appear here.'}</pre>
            </div>
          {/if}
        </div>
      {/each}
    {:else}
      <p class="assigner-empty">No configurable fields.</p>
    {/if}
    {#if hasTemplateFields}
      <div class="assigner-preview-config">
        <label class="assigner-label" for="preview-json">Preview data (JSON)</label>
        <textarea
          id="preview-json"
          class="assigner-input"
          rows="4"
          placeholder="summary: New plan"
          bind:value={previewJson}
        ></textarea>
        {#if previewParseError}
          <p class="assigner-hint assigner-hint--error">{previewParseError}</p>
        {:else}
          <p class="assigner-hint">
            Leave blank to use sample data for {trigger?.event_type || 'this trigger'}.
          </p>
        {/if}
      </div>
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

  .assigner-field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .assigner-input {
    border-radius: 10px;
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    padding: 0.45rem 0.6rem;
    background: rgba(var(--color-surface-rgb), 0.95);
    color: var(--color-text);
    font-size: 0.8rem;
  }

  .assigner-checkbox {
    width: 1rem;
    height: 1rem;
  }

  .assigner-template-tools {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  .assigner-template-tools .assigner-input {
    flex: 1;
  }

  .assigner-template-preview {
    border: 1px solid rgba(var(--color-text-rgb), 0.12);
    border-radius: 10px;
    padding: 0.5rem 0.65rem;
    background: rgba(var(--color-surface-rgb), 0.7);
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .assigner-preview-label {
    font-size: 0.65rem;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .assigner-preview-output {
    margin: 0;
    font-size: 0.75rem;
    color: var(--color-text);
    font-family: var(--font-mono);
    white-space: pre-wrap;
  }

  .assigner-preview-config {
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
  }

  .assigner-hint {
    margin: 0;
    font-size: 0.7rem;
    color: var(--color-text-muted);
  }

  .assigner-hint--error {
    color: var(--color-danger);
  }

  .assigner-dialog__actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.6rem;
    margin-top: 0.5rem;
  }
</style>
