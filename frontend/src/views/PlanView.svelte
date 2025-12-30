<script>
  import { onMount } from 'svelte'
  import { apiFetch } from '../lib/api.js'
  import OrgViewer from '../components/OrgViewer.svelte'

  let loading = false
  let error = ''
  let content = ''

  const loadPlan = async () => {
    loading = true
    error = ''
    try {
      const response = await apiFetch('/api/plan')
      const payload = await response.json()
      content = payload?.content || ''
    } catch (err) {
      error = err?.message || 'Failed to load plan.'
    } finally {
      loading = false
    }
  }

  onMount(loadPlan)
</script>

<section class="plan-view">
  <header class="plan-view__header">
    <div>
      <p class="eyebrow">Project plan</p>
      <h1>PLAN.org</h1>
    </div>
    <button class="refresh" type="button" on:click={loadPlan} disabled={loading}>
      {loading ? 'Refreshing...' : 'Refresh'}
    </button>
  </header>

  {#if loading && !content}
    <p class="muted">Loading plan...</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else}
    <OrgViewer orgText={content} />
  {/if}
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

  .eyebrow {
    text-transform: uppercase;
    letter-spacing: 0.24em;
    font-size: 0.7rem;
    color: #6d6a61;
    margin: 0 0 0.6rem;
  }

  h1 {
    margin: 0;
    font-size: clamp(2rem, 3.5vw, 3rem);
    font-weight: 600;
    color: #161616;
  }

  .refresh {
    border: 1px solid rgba(20, 20, 20, 0.2);
    border-radius: 999px;
    padding: 0.6rem 1.2rem;
    background: #ffffff;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }

  .muted {
    color: #7d7a73;
    margin: 0.5rem 0 0;
  }

  .error {
    color: #b04a39;
    margin: 0.5rem 0 0;
  }

  @media (max-width: 720px) {
    .plan-view__header {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
