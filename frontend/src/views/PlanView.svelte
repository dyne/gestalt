<script>
  import { onMount } from 'svelte'
  import { fetchPlansList } from '../lib/apiClient.js'
  import { getErrorMessage } from '../lib/errorUtils.js'
  import PlanCard from '../components/PlanCard.svelte'
  import ViewState from '../components/ViewState.svelte'

  let plans = []
  let loading = false
  let error = ''

  const loadPlans = async () => {
    if (loading) return
    loading = true
    error = ''
    try {
      const result = await fetchPlansList()
      plans = Array.isArray(result?.plans) ? result.plans : []
    } catch (err) {
      error = getErrorMessage(err, 'Failed to load plans.')
    } finally {
      loading = false
    }
  }

  onMount(() => {
    loadPlans()
  })
</script>

<section class="plan-view">
  <header class="plan-view__header">
    <div>
      <p class="eyebrow">Project plans</p>
      <div class="plan-heading">
        <h1>Project Plans</h1>
        <span class="plan-count">{plans.length}</span>
      </div>
      <p class="plan-path">.gestalt/plans/</p>
    </div>
    <div class="refresh-actions">
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
    hasContent={plans.length > 0}
    loadingLabel="Loading plans..."
    emptyLabel="No plans found in .gestalt/plans/"
  >
    <div class="plan-list">
      {#each plans as plan (plan.filename)}
        <PlanCard {plan} />
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

  .refreshing {
    color: var(--color-text-muted);
    font-size: 0.8rem;
  }
  
  @media (max-width: 720px) {
    .plan-view__header {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
