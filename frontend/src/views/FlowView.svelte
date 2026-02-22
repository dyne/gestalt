<script>
  import { onMount } from 'svelte'
  import EventActivityAssigner from '../components/EventActivityAssigner.svelte'
  import { exportFlowConfig, fetchTerminals, importFlowConfig } from '../lib/apiClient.js'
  import { canUseClipboard, copyToClipboard } from '../lib/clipboard.js'
  import { parseEventFilterQuery, matchesEventTrigger } from '../lib/eventFilterQuery.js'
  import { getErrorMessage } from '../lib/errorUtils.js'
  import { flowConfigStore } from '../lib/flowConfigStore.js'

  const fallbackEventTypeOptions = [
    'file-change',
    'git-branch',
    'git-commit',
    'plan-new',
    'plan-update',
    'work-start',
    'work-progress',
    'work-finish',
    'agent-turn',
    'prompt-voice',
    'prompt-text',
  ]

  let query = ''
  let selectedTriggerId = ''
  let dialogRef = null
  let dialogMode = 'create'
  let dialogOpen = false
  let dialogError = ''
  let draftLabel = ''
  let draftEventType = fallbackEventTypeOptions[0]
  let draftWhere = ''
  let draftSessionId = ''
  let importInputRef = null
  let clipboardAvailable = false
  let storageCopied = false
  let exportError = ''
  let importError = ''
  let exportInProgress = false
  let importInProgress = false
  let sessionOptions = []

  onMount(() => {
    clipboardAvailable = canUseClipboard()
    flowConfigStore.load()
    void loadSessions()
  })

  $: flowState = $flowConfigStore
  $: eventTypeOptions =
    Array.isArray(flowState?.eventTypes) && flowState.eventTypes.length > 0
      ? flowState.eventTypes
      : fallbackEventTypeOptions
  $: eventTypeOptionsWithDraft =
    draftEventType && !eventTypeOptions.includes(draftEventType)
      ? [...eventTypeOptions, draftEventType]
      : eventTypeOptions
  $: triggers = flowState?.config?.triggers || []
  $: bindingsByTriggerId = flowState?.config?.bindings_by_trigger_id || {}
  $: activityDefs = flowState?.activities || []
  $: isBusy = Boolean(flowState?.loading || flowState?.saving)

  $: parsed = parseEventFilterQuery(query)
  $: filteredTriggers = triggers.filter((trigger) => matchesEventTrigger(trigger, parsed))
  $: if (filteredTriggers.length === 0) {
    selectedTriggerId = ''
  } else if (!filteredTriggers.some((trigger) => trigger.id === selectedTriggerId)) {
    selectedTriggerId = filteredTriggers[0].id
  }
  $: selectedTrigger = filteredTriggers.find((trigger) => trigger.id === selectedTriggerId) || null
  $: selectedBindings = selectedTrigger ? bindingsByTriggerId[selectedTrigger.id] || [] : []
  $: selectedWhereEntries = selectedTrigger ? Object.entries(selectedTrigger.where || {}) : []

  const parseExportFilename = (header) => {
    if (!header) return ''
    const match = /filename="?([^\";]+)"?/i.exec(header)
    return match ? match[1] : ''
  }

  const selectTrigger = (id) => {
    selectedTriggerId = id
  }

  $: if (eventTypeOptions.length && !eventTypeOptions.includes(draftEventType)) {
    draftEventType = eventTypeOptions[0]
  }

  const showDialog = () => {
    dialogOpen = true
    if (dialogRef?.showModal) {
      dialogRef.showModal()
    }
  }

  const closeDialog = () => {
    dialogOpen = false
    if (dialogRef?.open && typeof dialogRef.close === 'function') {
      dialogRef.close()
    }
  }

  const parseWhereText = (value) => {
    const where = {}
    value
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
      .forEach((line) => {
        const [rawKey, ...rest] = line.split('=')
        const key = rawKey?.trim()
        if (!key) return
        where[key] = rest.join('=').trim()
      })
    return where
  }

  const isSessionKey = (value) => {
    const normalized = String(value || '').trim().toLowerCase()
    return normalized === 'session.id' || normalized === 'session_id'
  }

  const extractSessionId = (where = {}) => {
    for (const [key, value] of Object.entries(where)) {
      if (!isSessionKey(key)) continue
      const trimmed = String(value || '').trim()
      if (trimmed) return trimmed
    }
    return ''
  }

  const serializeWhere = (where = {}) =>
    Object.entries(where)
      .filter(([key]) => !isSessionKey(key))
      .map(([key, value]) => `${key}=${value}`)
      .join('\n')

  const buildTriggerId = (label) => {
    const base = label
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
    let id = base || 'trigger'
    let suffix = 1
    while (triggers.some((trigger) => trigger.id === id)) {
      id = `${base || 'trigger'}-${suffix}`
      suffix += 1
    }
    return id
  }

  const openCreateDialog = () => {
    dialogMode = 'create'
    draftLabel = ''
    draftEventType = eventTypeOptions[0]
    draftWhere = ''
    draftSessionId = ''
    dialogError = ''
    showDialog()
  }

  const openEditDialog = () => {
    if (!selectedTrigger) return
    dialogMode = 'edit'
    draftLabel = selectedTrigger.label
    draftEventType = selectedTrigger.event_type
    draftWhere = serializeWhere(selectedTrigger.where)
    draftSessionId = extractSessionId(selectedTrigger.where)
    dialogError = ''
    showDialog()
  }

  const saveTrigger = () => {
    dialogError = ''
    const label = draftLabel.trim()
    if (!label) {
      dialogError = 'Label is required.'
      return
    }
    const where = parseWhereText(draftWhere)
    let sessionId = draftSessionId.trim()
    if (!sessionId) {
      sessionId = String(where['session.id'] || where['session_id'] || '').trim()
    }
    Object.keys(where).forEach((key) => {
      if (isSessionKey(key)) {
        delete where[key]
      }
    })
    if (sessionId) {
      where['session.id'] = sessionId
    }
    if (dialogMode === 'edit' && selectedTrigger) {
      flowConfigStore.updateConfig((config) => ({
        ...config,
        triggers: (config.triggers || []).map((trigger) =>
          trigger.id === selectedTrigger.id
            ? { ...trigger, label, event_type: draftEventType, where }
            : trigger,
        ),
      }))
      selectedTriggerId = selectedTrigger.id
      closeDialog()
      return
    }
    const id = buildTriggerId(label)
    flowConfigStore.updateConfig((config) => {
      const bindingsByTrigger = { ...(config.bindings_by_trigger_id || {}) }
      if (!bindingsByTrigger[id]) {
        bindingsByTrigger[id] = []
      }
      return {
        ...config,
        triggers: [...(config.triggers || []), { id, label, event_type: draftEventType, where }],
        bindings_by_trigger_id: bindingsByTrigger,
      }
    })
    selectedTriggerId = id
    closeDialog()
  }

  const copyStoragePath = async () => {
    const path = flowState?.storagePath
    if (!path) return
    const ok = await copyToClipboard(path)
    storageCopied = ok
    if (ok) {
      setTimeout(() => {
        storageCopied = false
      }, 1500)
    }
  }

  const handleExport = async () => {
    exportError = ''
    exportInProgress = true
    try {
      const response = await exportFlowConfig()
      const blob = await response.blob()
      const filename =
        parseExportFilename(response.headers.get('Content-Disposition')) || 'flows.yaml'
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = filename
      document.body.appendChild(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)
    } catch (err) {
      exportError = getErrorMessage(err, 'Failed to export Flow configuration.')
    } finally {
      exportInProgress = false
    }
  }

  const openImportDialog = () => {
    importError = ''
    if (!importInputRef) return
    importInputRef.value = ''
    importInputRef.click()
  }

  const handleImportFile = async (event) => {
    const file = event?.target?.files?.[0]
    if (!file) return
    importError = ''
    importInProgress = true
    try {
      const text = await file.text()
      await importFlowConfig(text)
      await flowConfigStore.load()
    } catch (err) {
      importError = getErrorMessage(err, 'Failed to import Flow configuration.')
    } finally {
      importInProgress = false
    }
  }

  const loadSessions = async () => {
    try {
      const sessions = await fetchTerminals()
      sessionOptions = Array.isArray(sessions) ? sessions : []
    } catch (err) {
      sessionOptions = []
    }
  }

  const saveChanges = () => {
    flowConfigStore.save()
  }

  const handleAssign = (event) => {
    const { trigger_id, activity_id } = event.detail || {}
    if (!trigger_id || !activity_id) return
    flowConfigStore.updateConfig((config) => {
      const bindingsByTrigger = { ...(config.bindings_by_trigger_id || {}) }
      const current = Array.isArray(bindingsByTrigger[trigger_id])
        ? bindingsByTrigger[trigger_id]
        : []
      if (current.some((binding) => binding.activity_id === activity_id)) {
        return config
      }
      bindingsByTrigger[trigger_id] = [
        ...current,
        { activity_id, config: {} },
      ]
      return { ...config, bindings_by_trigger_id: bindingsByTrigger }
    })
  }

  const handleUnassign = (event) => {
    const { trigger_id, activity_id } = event.detail || {}
    if (!trigger_id || !activity_id) return
    flowConfigStore.updateConfig((config) => {
      const bindingsByTrigger = { ...(config.bindings_by_trigger_id || {}) }
      const current = Array.isArray(bindingsByTrigger[trigger_id])
        ? bindingsByTrigger[trigger_id]
        : []
      bindingsByTrigger[trigger_id] = current.filter(
        (binding) => binding.activity_id !== activity_id,
      )
      return { ...config, bindings_by_trigger_id: bindingsByTrigger }
    })
  }

  const handleConfigUpdate = (event) => {
    const { trigger_id, activity_id, config } = event.detail || {}
    if (!trigger_id || !activity_id) return
    flowConfigStore.updateConfig((current) => {
      const bindingsByTrigger = { ...(current.bindings_by_trigger_id || {}) }
      const list = Array.isArray(bindingsByTrigger[trigger_id])
        ? bindingsByTrigger[trigger_id]
        : []
      bindingsByTrigger[trigger_id] = list.map((binding) =>
        binding.activity_id === activity_id ? { ...binding, config: config || {} } : binding,
      )
      return { ...current, bindings_by_trigger_id: bindingsByTrigger }
    })
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
    <div class="flow-actions">
      {#if flowState?.dirty}
        <span class="flow-status">Unsaved changes</span>
      {/if}
      <button
        class="rail-button"
        type="button"
        on:click={saveChanges}
        disabled={!flowState?.dirty || isBusy}
      >
        {flowState?.saving ? 'Saving...' : 'Save changes'}
      </button>
      <button
        class="rail-button rail-button--ghost"
        type="button"
        on:click={handleExport}
        disabled={isBusy || exportInProgress}
      >
        {exportInProgress ? 'Exporting...' : 'Export'}
      </button>
      <button
        class="rail-button rail-button--ghost"
        type="button"
        on:click={openImportDialog}
        disabled={isBusy || importInProgress}
      >
        {importInProgress ? 'Importing...' : 'Import'}
      </button>
      <input
        class="flow-import-input"
        type="file"
        accept=".yaml,.yml"
        bind:this={importInputRef}
        on:change={handleImportFile}
      />
    </div>
  </header>

  {#if flowState?.storagePath}
    <div class="flow-storage">
      <div class="flow-storage__path">
        <span class="flow-storage__label">Saved to</span>
        <span class="flow-storage__value">{flowState.storagePath}</span>
      </div>
      {#if clipboardAvailable}
        <button class="rail-button rail-button--ghost" type="button" on:click={copyStoragePath}>
          {storageCopied ? 'Copied' : 'Copy path'}
        </button>
      {/if}
    </div>
  {/if}

  {#if flowState?.error}
    <div class="flow-alert flow-alert--error">{flowState.error}</div>
  {/if}
  {#if flowState?.saveError}
    <div class="flow-alert flow-alert--error">{flowState.saveError}</div>
  {/if}
  {#if exportError}
    <div class="flow-alert flow-alert--error">{exportError}</div>
  {/if}
  {#if importError}
    <div class="flow-alert flow-alert--error">{importError}</div>
  {/if}
  <div class="flow-view__body">
    <aside class="flow-rail">
      <div class="rail-header">
        <label class="field-label" for="flow-query">Search / filters</label>
        <button class="rail-button" type="button" on:click={openCreateDialog} disabled={isBusy}>
          Add trigger
        </button>
      </div>
      <input
        id="flow-query"
        class="field-input"
        type="text"
        placeholder="Search / filters"
        bind:value={query}
        disabled={flowState?.loading}
      />
      <p class="field-hint">Try `event_type:file-change path:README.md`</p>
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
        {#if flowState?.loading && triggers.length === 0}
          <p class="empty-state">Loading triggers...</p>
        {:else if filteredTriggers.length === 0}
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
          <div class="detail-header">
            <p class="eyebrow">Selected trigger</p>
            <button
              class="rail-button rail-button--ghost"
              type="button"
              on:click={openEditDialog}
              disabled={isBusy}
            >
              Edit trigger
            </button>
          </div>
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
          <EventActivityAssigner
            {activityDefs}
            trigger={selectedTrigger}
            bindings={selectedBindings}
            disabled={isBusy}
            on:assign_activity={handleAssign}
            on:unassign_activity={handleUnassign}
            on:update_activity_config={handleConfigUpdate}
          />
        </div>
      {:else}
        <div class="flow-empty">
          <p>{flowState?.loading ? 'Loading Flow configuration...' : 'Select a trigger to configure activities.'}</p>
        </div>
      {/if}
    </section>
  </div>
</section>

<dialog class="flow-dialog" bind:this={dialogRef} open={dialogOpen} on:close={() => (dialogOpen = false)}>
  <form class="dialog-form" on:submit|preventDefault={saveTrigger}>
    <header class="dialog-header">
      <h2>{dialogMode === 'edit' ? 'Edit trigger' : 'Add trigger'}</h2>
    </header>
    <label class="field-label" for="trigger-label">Label</label>
    <input
      id="trigger-label"
      class="field-input"
      type="text"
      placeholder="Trigger label"
      bind:value={draftLabel}
    />
    <label class="field-label" for="trigger-event-type">Event type</label>
    <select
      id="trigger-event-type"
      class="field-input"
      bind:value={draftEventType}
    >
      {#each eventTypeOptionsWithDraft as eventType}
        <option value={eventType}>{eventType}</option>
      {/each}
    </select>
    <label class="field-label" for="trigger-session-id">Session ID</label>
    <input
      id="trigger-session-id"
      class="field-input"
      type="text"
      list="session-id-options"
      placeholder="coder 1"
      bind:value={draftSessionId}
    />
    <datalist id="session-id-options">
      {#each sessionOptions as session}
        <option value={session.id}>{session.title ? `${session.id} — ${session.title}` : session.id}</option>
      {/each}
    </datalist>
    <p class="field-hint">
      Leave blank to match any session. Use the name only (no number) to match all sessions for that name.
    </p>
    <label class="field-label" for="trigger-where">Where (one per line)</label>
    <textarea
      id="trigger-where"
      class="field-input"
      rows="4"
      placeholder="session.id=coder-1"
      bind:value={draftWhere}
    ></textarea>
    {#if dialogError}
      <p class="dialog-error">{dialogError}</p>
    {/if}
    <div class="dialog-actions">
      <button class="rail-button" type="submit">Save trigger</button>
      <button class="rail-button rail-button--ghost" type="button" on:click={closeDialog}>
        Cancel
      </button>
    </div>
  </form>
</dialog>

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

  .flow-actions {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .flow-status {
    font-size: 0.75rem;
    color: var(--color-text-muted);
  }

  .flow-alert {
    border-radius: 12px;
    padding: 0.75rem 1rem;
    font-size: 0.85rem;
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    background: rgba(var(--color-surface-rgb), 0.6);
  }

  .flow-alert--error {
    border-color: rgba(var(--color-danger-rgb), 0.4);
    color: var(--color-danger);
    background: rgba(var(--color-danger-rgb), 0.08);
  }

  .flow-storage {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.75rem 1rem;
    border-radius: 12px;
    border: 1px solid rgba(var(--color-text-rgb), 0.12);
    background: rgba(var(--color-surface-rgb), 0.6);
  }

  .flow-storage__path {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }

  .flow-storage__label {
    text-transform: uppercase;
    letter-spacing: 0.2em;
    font-size: 0.65rem;
    color: var(--color-text-muted);
  }

  .flow-storage__value {
    font-size: 0.85rem;
    font-family: var(--font-mono);
    color: var(--color-text);
  }

  .flow-import-input {
    display: none;
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

  .rail-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.75rem;
  }

  .rail-button {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.4rem 0.9rem;
    font-size: 0.75rem;
    font-weight: 600;
    background: rgba(var(--color-surface-rgb), 0.95);
    color: var(--color-text);
    cursor: pointer;
  }

  .rail-button--ghost {
    background: transparent;
    border-color: rgba(var(--color-text-rgb), 0.25);
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

  .detail-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.75rem;
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

  .flow-dialog {
    border: none;
    border-radius: 18px;
    padding: 0;
    background: rgba(8, 12, 18, 0.95);
    color: var(--color-text);
    width: min(520px, 90vw);
  }

  .dialog-form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    padding: 1.5rem;
  }

  .dialog-header h2 {
    margin: 0;
    font-size: 1.2rem;
  }

  .dialog-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
  }

  .dialog-error {
    margin: 0;
    color: var(--color-danger);
    font-size: 0.8rem;
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

    .detail-header {
      flex-direction: column;
      align-items: flex-start;
    }

    .rail-header {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
