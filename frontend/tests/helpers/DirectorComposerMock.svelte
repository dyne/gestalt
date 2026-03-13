<script>
  import { createEventDispatcher } from 'svelte'
  export let disabled = false
  const dispatch = createEventDispatcher()
</script>

<button
  type="button"
  disabled={disabled}
  on:click={() => dispatch('submit', { text: 'mock text', source: 'text' })}
>
  Mock submit
</button>
