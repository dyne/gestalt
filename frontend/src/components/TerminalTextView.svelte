<script>
  import { onMount } from 'svelte'

  export let segments = []
  export let onAtBottomChange = () => {}
  export let autoFollow = true

  let container
  let atBottom = true
  let lastKey = ''

  const isAtBottom = () => {
    if (!container) return true
    const threshold = 4
    return (
      container.scrollTop + container.clientHeight >=
      container.scrollHeight - threshold
    )
  }

  const updateAtBottom = () => {
    const next = isAtBottom()
    if (next === atBottom) return
    atBottom = next
    onAtBottomChange(next)
  }

  const scrollToBottom = () => {
    if (!container) return
    container.scrollTop = container.scrollHeight
    updateAtBottom()
  }

  const handleScroll = () => {
    updateAtBottom()
  }

  export { scrollToBottom }

  $: {
    const nextKey = `${segments?.length || 0}:${
      segments && segments.length > 0
        ? segments[segments.length - 1].text.length
        : 0
    }`
    if (nextKey !== lastKey) {
      lastKey = nextKey
      if (autoFollow && atBottom) {
        requestAnimationFrame(scrollToBottom)
      }
    }
  }

  onMount(() => {
    updateAtBottom()
    if (autoFollow) {
      requestAnimationFrame(scrollToBottom)
    }
  })
</script>

<div class="terminal-text__body" bind:this={container} on:scroll={handleScroll}>
  <pre class="terminal-text__content">
    {#each segments as segment}
      <span class={`terminal-text__segment terminal-text__segment--${segment.kind}`}>
        {segment.text}
      </span>
    {/each}
  </pre>
</div>

<style>
  .terminal-text__body {
    min-height: 0;
    min-width: 0;
    padding: 0.6rem;
    overflow: auto;
    overscroll-behavior: contain;
  }

  .terminal-text__content {
    margin: 0;
    white-space: pre-wrap;
    font-family: var(--terminal-font-family, ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace);
    font-size: var(--terminal-font-size, 0.95rem);
    line-height: var(--terminal-line-height, 1.4);
  }

  .terminal-text__segment {
    color: var(--terminal-text);
  }

  .terminal-text__segment--prompt {
    color: var(--terminal-prompt, var(--terminal-text));
    font-weight: 600;
  }
</style>
