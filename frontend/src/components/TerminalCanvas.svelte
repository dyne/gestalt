<script>
  import { onDestroy, onMount } from 'svelte'
  import '@xterm/xterm/css/xterm.css'

  export let state = null
  export let scrollSensitivity = 1
  export let visible = true

  let container
  let attachedState = null

  const attach = () => {
    if (!state || !container) return
    state.attach?.(container)
    state.setScrollSensitivity?.(scrollSensitivity)
    attachedState = state
  }

  const detach = () => {
    if (!attachedState) return
    attachedState.detach?.()
    attachedState = null
  }

  onMount(() => {
    attach()
  })

  $: if (!state && attachedState) {
    detach()
  }

  $: if (state && container && attachedState !== state) {
    detach()
    attach()
  }

  $: if (state && container) {
    state.setScrollSensitivity?.(scrollSensitivity)
  }

  $: if (visible && state) {
    state.scheduleFit?.()
  }

  onDestroy(() => {
    detach()
  })
</script>

<div class="terminal-shell__body" bind:this={container}></div>

<style>
  .terminal-shell__body {
    min-height: 0;
    touch-action: none;
    overscroll-behavior: contain;
    padding: 0.6rem;
    min-width: 0;
  }

  :global(.xterm) {
    height: 100%;
    touch-action: none;
  }

  :global(.xterm-viewport) {
    border-radius: 12px;
    -webkit-overflow-scrolling: touch;
    touch-action: pan-y;
  }
</style>
