<script>
  import DirectorComposer from '../../src/components/DirectorComposer.svelte'

  export let onSubmit = () => {}
</script>

<DirectorComposer on:submit={(event) => onSubmit(event.detail)} />
