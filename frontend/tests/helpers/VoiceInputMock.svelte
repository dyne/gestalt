<script>
  import { createEventDispatcher } from 'svelte'

  export let disabled = false
  const dispatch = createEventDispatcher()

  const emitTranscript = () => {
    dispatch('transcript', { text: 'Voice sentence' })
  }
</script>

<button class="voice-mock" type="button" on:click={emitTranscript} {disabled}>Voice</button>
