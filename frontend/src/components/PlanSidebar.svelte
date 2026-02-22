<script>
  import { onDestroy, onMount } from 'svelte'
  import { fetchPlansList, fetchSessionProgress } from '../lib/apiClient.js'
  import { subscribe as subscribeTerminalEvents } from '../lib/terminalEventStore.js'
  import { formatRelativeTime } from '../lib/timeUtils.js'

  const planCache = new Map()

  export let sessionId = ''
  export let open = false
  export let onClose = () => {}

  let progress = null
  let plan = null
  let loading = false
  let planLoading = false
  let progressError = ''
  let planError = ''
  let lastSessionId = ''
  let lastPlanFile = ''
  let unsubscribe = null

  const normalizeText = (value) => String(value || '').trim().toLowerCase()
  const normalizeKeyword = (value) => String(value || '').trim().toUpperCase()
  const isActiveKeyword = (value) => {
    const keyword = normalizeKeyword(value)
    return keyword === 'TODO' || keyword === 'WIP'
  }
  const isDoneHeading = (heading) => {
    if (!heading) return false
    if (normalizeKeyword(heading.keyword) !== 'DONE') return false
    const children = Array.isArray(heading.children) ? heading.children : []
    return children.every((child) => isDoneHeading(child))
  }
  const normalizeProgressFromRest = (payload) => ({
    planFile: String(payload?.plan_file || ''),
    l1: String(payload?.l1 || ''),
    l2: String(payload?.l2 || ''),
    taskLevel: Number(payload?.task_level || 0),
    taskState: String(payload?.task_state || ''),
    timestamp: payload?.updated_at || '',
  })
  const normalizeProgressFromEvent = (payload) => {
    const data = payload?.data || {}
    return {
      planFile: String(data?.plan_file || ''),
      l1: String(data?.l1 || ''),
      l2: String(data?.l2 || ''),
      taskLevel: Number(data?.task_level || 0),
      taskState: String(data?.task_state || ''),
      timestamp: payload?.timestamp || data?.timestamp || '',
    }
  }
  const statusClass = (value) => {
    const normalized = normalizeKeyword(value).toLowerCase()
    return normalized || 'neutral'
  }

  const findMatchingHeading = (headings, label) => {
    const normalized = normalizeText(label)
    if (!normalized) return null
    return headings.find((heading) => normalizeText(heading?.text) === normalized) || null
  }

  const findFirstActiveHeading = (headings) => {
    return headings.find((heading) => isActiveKeyword(heading?.keyword)) || null
  }

  const loadProgress = async (id) => {
    if (!id) return
    loading = true
    progressError = ''
    try {
      const payload = await fetchSessionProgress(id)
      if (payload?.has_progress) {
        updateProgress(normalizeProgressFromRest(payload))
      } else {
        progress = null
        plan = null
        lastPlanFile = ''
      }
    } catch {
      progressError = 'Unable to load progress.'
    } finally {
      loading = false
    }
  }

  const loadPlan = async (filename) => {
    if (!filename) {
      plan = null
      return
    }
    if (planCache.has(filename)) {
      plan = planCache.get(filename)
      return
    }
    planLoading = true
    planError = ''
    try {
      const payload = await fetchPlansList()
      const plans = Array.isArray(payload?.plans) ? payload.plans : []
      const match = plans.find((entry) => entry?.filename === filename) || null
      planCache.set(filename, match)
      plan = match
    } catch {
      planError = 'Unable to load plan outline.'
      plan = null
    } finally {
      planLoading = false
    }
  }

  const updateProgress = (next) => {
    progress = next
    if (!next?.planFile) {
      plan = null
      lastPlanFile = ''
      return
    }
    if (next.planFile !== lastPlanFile) {
      lastPlanFile = next.planFile
      void loadPlan(next.planFile)
    }
  }

  const l1Key = (heading, index) => `${heading?.text || 'l1'}:${index}`
  const l2Key = (heading, index) => `${heading?.text || 'l2'}:${index}`

  $: headings = Array.isArray(plan?.headings) ? plan.headings : []
  $: currentL1 = findMatchingHeading(headings, progress?.l1) || findFirstActiveHeading(headings)
  $: currentL2 =
    currentL1 && Array.isArray(currentL1.children)
      ? findMatchingHeading(currentL1.children, progress?.l2) ||
        findFirstActiveHeading(currentL1.children)
      : null
  $: currentL1Key = normalizeText(currentL1?.text)
  $: currentL2Key = normalizeText(currentL2?.text)
  $: planTitle = plan?.title || progress?.planFile || 'Plan'
  $: planFilename = plan?.filename || progress?.planFile || ''
  $: updatedLabel = progress?.timestamp ? formatRelativeTime(progress.timestamp) : ''

  onMount(() => {
    if (open && sessionId) {
      void loadProgress(sessionId)
    }
    unsubscribe = subscribeTerminalEvents('plan-update', (payload) => {
      if (!payload) return
      if (!sessionId) return
      if (String(payload?.session_id || '') !== String(sessionId)) return
      updateProgress(normalizeProgressFromEvent(payload))
    })
  })

  onDestroy(() => {
    if (unsubscribe) {
      unsubscribe()
      unsubscribe = null
    }
  })

  $: if (open && sessionId && sessionId !== lastSessionId) {
    lastSessionId = sessionId
    void loadProgress(sessionId)
  }
</script>

<aside class="plan-sidebar" data-open={open}>
  <header class="plan-sidebar__header">
    <div class="plan-sidebar__title">
      <p class="plan-sidebar__eyebrow">Plan</p>
      <h2>{planTitle}</h2>
      {#if planFilename}
        <p class="plan-sidebar__filename">{planFilename}</p>
      {/if}
    </div>
    <button class="plan-sidebar__close" type="button" on:click={onClose}>
      Close
    </button>
  </header>
  <div class="plan-sidebar__meta">
    {#if updatedLabel}
      <span>Updated {updatedLabel}</span>
    {/if}
    {#if progress?.taskState}
      <span class="meta-divider">|</span>
      <span>{progress.taskState}</span>
    {/if}
  </div>

  {#if loading || planLoading}
    <p class="plan-sidebar__status">Loading plan progress...</p>
  {:else if progressError || planError}
    <p class="plan-sidebar__status">{progressError || planError}</p>
  {:else if !progress || !plan}
    <p class="plan-sidebar__status">No plan progress yet.</p>
  {:else}
    <ul class="plan-outline">
      {#each headings as heading, index (l1Key(heading, index))}
        {#if heading}
          {@const headingKey = normalizeText(heading.text)}
          {@const isCurrentL1 = headingKey && headingKey === currentL1Key}
          {@const doneHeading = isDoneHeading(heading)}
          {@const showChildren = isCurrentL1 || !doneHeading}
          <li
            class="plan-outline__l1"
            class:is-current={isCurrentL1}
            class:is-done={doneHeading}
            data-current={isCurrentL1}
          >
            <div class="plan-outline__l1-header">
              <span class="status-pill status-pill--{statusClass(heading.keyword)}">
                {heading.keyword || '—'}
              </span>
              <span class="plan-outline__title">{heading.text}</span>
            </div>
            {#if showChildren && Array.isArray(heading.children) && heading.children.length > 0}
              <ul class="plan-outline__l2">
                {#each heading.children as child, childIndex (l2Key(child, childIndex))}
                  {#if child}
                    {@const childKey = normalizeText(child.text)}
                    {@const isCurrentL2 = childKey && childKey === currentL2Key && isCurrentL1}
                    {@const childDone = normalizeKeyword(child.keyword) === 'DONE'}
                    <li
                      class="plan-outline__l2-item"
                      class:is-current={isCurrentL2}
                      class:is-done={childDone}
                      data-current={isCurrentL2}
                    >
                      <span class="plan-outline__l2-status">{child.keyword || '—'}</span>
                      <span class="plan-outline__l2-title">{child.text}</span>
                    </li>
                  {/if}
                {/each}
              </ul>
            {/if}
          </li>
        {/if}
      {/each}
    </ul>
  {/if}
</aside>

<style>
  .plan-sidebar {
    height: 100%;
    min-height: 0;
    background: var(--terminal-panel);
    border-radius: 18px;
    border: 1px solid rgba(var(--terminal-border-rgb), 0.18);
    box-shadow: 0 16px 32px rgba(var(--shadow-color-rgb), 0.3);
    padding: 1.2rem 1.2rem 1.4rem;
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .plan-sidebar__header {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    align-items: flex-start;
  }

  .plan-sidebar__eyebrow {
    text-transform: uppercase;
    letter-spacing: 0.2em;
    font-size: 0.6rem;
    color: rgba(var(--color-text-rgb), 0.5);
    margin: 0 0 0.35rem;
  }

  .plan-sidebar__title h2 {
    margin: 0;
    font-size: 1.05rem;
    color: rgba(var(--color-text-rgb), 0.95);
  }

  .plan-sidebar__filename {
    margin: 0.35rem 0 0;
    font-size: 0.75rem;
    color: rgba(var(--color-text-rgb), 0.6);
  }

  .plan-sidebar__close {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.35rem 0.8rem;
    background: rgba(var(--color-text-rgb), 0.08);
    color: rgba(var(--color-text-rgb), 0.85);
    font-size: 0.65rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    cursor: pointer;
  }

  .plan-sidebar__meta {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    color: rgba(var(--color-text-rgb), 0.5);
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .meta-divider {
    color: rgba(var(--color-text-rgb), 0.25);
  }

  .plan-sidebar__status {
    margin: 0;
    color: rgba(var(--color-text-rgb), 0.6);
    font-size: 0.8rem;
  }

  .plan-outline {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    overflow: auto;
  }

  .plan-outline__l1 {
    border-radius: 14px;
    padding: 0.75rem 0.8rem;
    background: rgba(var(--color-text-rgb), 0.06);
    border: 1px solid transparent;
  }

  .plan-outline__l1.is-current {
    border-color: rgba(var(--color-primary-rgb), 0.4);
    background: rgba(var(--color-primary-rgb), 0.14);
  }

  .plan-outline__l1-header {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  .plan-outline__title {
    font-size: 0.85rem;
    color: rgba(var(--color-text-rgb), 0.9);
  }

  .status-pill {
    text-transform: uppercase;
    letter-spacing: 0.12em;
    font-size: 0.55rem;
    padding: 0.2rem 0.45rem;
    border-radius: 999px;
    background: rgba(var(--color-text-rgb), 0.16);
    color: rgba(var(--color-text-rgb), 0.75);
  }

  .status-pill--done {
    background: rgba(var(--color-success-rgb), 0.2);
    color: rgba(var(--color-success-rgb), 0.95);
  }

  .status-pill--wip {
    background: rgba(var(--color-primary-rgb), 0.2);
    color: rgba(var(--color-primary-rgb), 0.95);
  }

  .status-pill--todo {
    background: rgba(var(--color-warning-rgb), 0.22);
    color: rgba(var(--color-warning-rgb), 0.95);
  }

  .status-pill--neutral {
    background: rgba(var(--color-text-rgb), 0.16);
    color: rgba(var(--color-text-rgb), 0.75);
  }

  .plan-outline__l1.is-done {
    opacity: 0.65;
  }

  .plan-outline__l2 {
    list-style: none;
    margin: 0.6rem 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .plan-outline__l2-item {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    font-size: 0.75rem;
    color: rgba(var(--color-text-rgb), 0.75);
  }

  .plan-outline__l2-item.is-current {
    color: rgba(var(--color-primary-rgb), 0.95);
    font-weight: 600;
  }

  .plan-outline__l2-item.is-done {
    opacity: 0.5;
  }

  .plan-outline__l2-status {
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-size: 0.55rem;
    color: rgba(var(--color-text-rgb), 0.5);
  }

  @media (max-width: 900px) {
    .plan-sidebar {
      border-radius: 16px;
    }
  }
</style>
