<script>
  import ViewState from '../../src/components/ViewState.svelte'

  export let loading = false
  export let error = ''
  export let hasContent = false
  export let showEmpty = true
  export let loadingLabel = 'Loading...'
  export let emptyLabel = 'No data.'
  export let content = 'Content'
</script>

<ViewState {loading} {error} {hasContent} {showEmpty} {loadingLabel} {emptyLabel}>
  <div>{content}</div>
</ViewState>
