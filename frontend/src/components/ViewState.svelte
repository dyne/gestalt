<script>
  export let loading = false
  export let error = ''
  export let hasContent = false
  export let loadingLabel = 'Loading...'
  export let emptyLabel = 'No data.'
  export let showEmpty = true
</script>

{#if loading && !hasContent}
  <p class="muted">{loadingLabel}</p>
{:else if error && !hasContent}
  <p class="error">{error}</p>
{:else if showEmpty && !hasContent}
  <p class="muted">{emptyLabel}</p>
{:else}
  {#if error}
    <p class="error error--inline">{error}</p>
  {/if}
  <slot />
{/if}

<style>
  .muted {
    color: var(--color-text-subtle);
    margin: 0.5rem 0 0;
  }

  .error {
    color: var(--color-danger);
    margin: 0.5rem 0 0;
  }

  .error--inline {
    margin: 0 0 1rem;
  }
</style>
