<script>
  import { onDestroy, onMount } from 'svelte'
  import { fetchPlansList } from '../lib/apiClient.js'
  import { subscribe as subscribeEvents } from '../lib/eventStore.js'
  import { getErrorMessage } from '../lib/errorUtils.js'
  import { formatRelativeTime } from '../lib/timeUtils.js'
  import PlanCard from '../components/PlanCard.svelte'
  import ViewState from '../components/ViewState.svelte'

  let plans = []
  let loading = false
  let error = ''
  let updateNotice = false
  let updateNoticeTimer = null
  let refreshTimer = null
  let refreshQueued = false
  let refreshInFlight = false
  let queuedSilent = true
  let eventUnsubscribe = null
  const refreshDebounceMs = 250

  const normalizeKeyword = (value) => String(value || '').trim().toUpperCase()

  const hasActiveHeading = (heading) => {
    if (!heading) return false
    const keyword = normalizeKeyword(heading.keyword)
    if (keyword === 'TODO' || keyword === 'WIP') {
      return true
    }
    const children = Array.isArray(heading.children) ? heading.children : []
    return children.some(hasActiveHeading)
  }

  const isActivePlan = (plan) => {
    const entries = Array.isArray(plan?.headings) ? plan.headings : []
    return entries.some(hasActiveHeading)
  }

  const isDoneHeading = (heading) => {
    if (!heading) return false
    const keyword = normalizeKeyword(heading.keyword)
    if (keyword !== 'DONE') {
      return false
    }
    const children = Array.isArray(heading.children) ? heading.children : []
    return children.every(isDoneHeading)
  }

  const isDonePlan = (plan) => {
    const entries = Array.isArray(plan?.headings) ? plan.headings : []
    if (entries.length === 0) return false
    return entries.every(isDoneHeading)
  }

  const sortPlansByDate = (entries = []) =>
    entries
      .slice()
      .sort((a, b) => Date.parse(b?.date || '') - Date.parse(a?.date || ''))

  const planKey = (plan, index) => {
    const name = plan?.filename ? String(plan.filename) : ''
    if (name) {
      return `${name}:${index}`
    }
    return `plan-${index}`
  }

  const withRelativeDate = (plan) => ({
    ...plan,
    date: formatRelativeTime(plan?.date),
  })

  const showUpdateNotice = () => {
    updateNotice = true
    if (updateNoticeTimer) {
      clearTimeout(updateNoticeTimer)
    }
    updateNoticeTimer = setTimeout(() => {
      updateNotice = false
    }, 2000)
  }

  const queuePlansRefresh = (silent = true) => {
    refreshQueued = true
    if (!silent) {
      queuedSilent = false
    }
    if (refreshTimer || refreshInFlight) return
    refreshTimer = setTimeout(() => {
      refreshTimer = null
      if (refreshInFlight || !refreshQueued) return
      const nextSilent = queuedSilent
      refreshQueued = false
      queuedSilent = true
      void loadPlans({ silent: nextSilent })
    }, refreshDebounceMs)
  }

  const loadPlans = async ({ silent = false } = {}) => {
    if (refreshInFlight) {
      queuePlansRefresh(silent)
      return
    }
    refreshInFlight = true
    if (!silent) {
      loading = true
      error = ''
    }
    try {
      const result = await fetchPlansList()
      plans = Array.isArray(result?.plans) ? result.plans : []
    } catch (err) {
      error = getErrorMessage(err, 'Failed to load plans.')
    } finally {
      if (!silent) {
        loading = false
      }
      refreshInFlight = false
      if (refreshQueued) {
        queuePlansRefresh(queuedSilent)
      }
    }
  }

  $: activePlans = sortPlansByDate(plans.filter(isActivePlan))
  $: donePlans = sortPlansByDate(plans.filter(isDonePlan))

  onMount(() => {
    loadPlans()
    eventUnsubscribe = subscribeEvents('file_changed', (payload) => {
      const rawPath = String(payload?.path || '')
      const normalized = rawPath.replaceAll('\\', '/')
      if (!normalized.includes('/.gestalt/plans/')) return
      if (!normalized.endsWith('.org')) return
      queuePlansRefresh(true)
      showUpdateNotice()
    })
  })

  onDestroy(() => {
    if (updateNoticeTimer) {
      clearTimeout(updateNoticeTimer)
      updateNoticeTimer = null
    }
    if (refreshTimer) {
      clearTimeout(refreshTimer)
      refreshTimer = null
    }
    if (eventUnsubscribe) {
      eventUnsubscribe()
      eventUnsubscribe = null
    }
  })
</script>

<section class="plan-view">
  <header class="plan-view__header">
    <div>
      <p class="eyebrow">Project plans</p>
      <div class="plan-heading">
        <h1>Plans</h1>
        <span class="plan-count">{activePlans.length + donePlans.length}</span>
      </div>
      <p class="plan-path">.gestalt/plans/</p>
    </div>
    <div class="refresh-actions">
      {#if updateNotice}
        <span class="updated">Plans updated</span>
      {/if}
      {#if loading}
        <span class="refreshing">Updating...</span>
      {/if}
      <button class="refresh" type="button" on:click={loadPlans} disabled={loading}>
        {loading ? 'Refreshing...' : 'Refresh'}
      </button>
    </div>
  </header>

  <ViewState
    {loading}
    {error}
    hasContent={activePlans.length + donePlans.length > 0}
    loadingLabel="Loading plans..."
    emptyLabel="No plans found in .gestalt/plans/"
  >
    <div class="plan-list">
      {#each activePlans as plan, planIndex (planKey(plan, planIndex))}
        <PlanCard plan={withRelativeDate(plan)} />
      {/each}
      {#if donePlans.length > 0}
        <div class="section-divider">Done</div>
      {/if}
      {#each donePlans as plan, planIndex (planKey(plan, planIndex))}
        <PlanCard plan={withRelativeDate(plan)} />
      {/each}
    </div>
  </ViewState>
</section>

<style>
  .plan-view {
    padding: 2.5rem clamp(1.5rem, 4vw, 3.5rem) 3.5rem;
    display: flex;
    flex-direction: column;
    gap: 2rem;
  }

  .plan-view__header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1.5rem;
  }

  .plan-heading {
    display: flex;
    align-items: center;
    gap: 0.75rem;
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

  .plan-count {
    min-width: 2rem;
    text-align: center;
    padding: 0.2rem 0.7rem;
    border-radius: 999px;
    background: rgba(var(--color-text-rgb), 0.12);
    color: var(--color-text);
    font-size: 0.8rem;
    font-weight: 600;
  }

  .plan-path {
    margin: 0.5rem 0 0;
    color: var(--color-text-subtle);
    font-size: 0.85rem;
  }

  .refresh {
    border: 1px solid rgba(var(--color-text-rgb), 0.2);
    border-radius: 999px;
    padding: 0.6rem 1.2rem;
    background: var(--color-surface);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }

  .plan-list {
    display: grid;
    gap: 1.2rem;
  }

  .section-divider {
    border-top: 1px solid rgba(var(--color-text-rgb), 0.12);
    padding: 1rem 0 0;
    color: var(--color-text-muted);
    text-transform: uppercase;
    letter-spacing: 0.2em;
    font-size: 0.7rem;
  }

  .refreshing {
    color: var(--color-text-muted);
    font-size: 0.8rem;
  }

  .updated {
    color: var(--color-success);
    font-size: 0.8rem;
    font-weight: 600;
  }
  
  @media (max-width: 720px) {
    .plan-view__header {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
