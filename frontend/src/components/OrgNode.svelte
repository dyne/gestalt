<script>
  import { slide } from 'svelte/transition'

  export let node
  export let filter = 'all'
  export let searchQuery = ''
  export let onToggle = () => {}

  const keywordClass = (keyword) => {
    if (!keyword) return ''
    return `org-node__badge--${keyword.toLowerCase()}`
  }

  const matchesFilter = (target) => {
    if (filter === 'all') return true
    if (filter === 'todo') return target.keyword === 'TODO'
    if (filter === 'wip') return target.keyword === 'WIP'
    if (filter === 'done') return target.keyword === 'DONE'
    if (filter === 'todo-wip') return target.keyword === 'TODO' || target.keyword === 'WIP'
    return true
  }

  const shouldDisplay = (target) =>
    matchesFilter(target) || target.children.some((child) => shouldDisplay(child))

  const matchesSearch = (target, query) => {
    if (!query) return false
    const heading = (target.text || '').toLowerCase()
    const body = (target.body || '').toLowerCase()
    return heading.includes(query) || body.includes(query)
  }

  const hasSearchMatch = (target, query) =>
    matchesSearch(target, query) || target.children.some((child) => hasSearchMatch(child, query))

  const buildHighlights = (text, query) => {
    const raw = text || ''
    if (!query || !raw) {
      return [{ text: raw, match: false }]
    }
    const lower = raw.toLowerCase()
    const parts = []
    let cursor = 0
    while (cursor < raw.length) {
      const index = lower.indexOf(query, cursor)
      if (index === -1) {
        parts.push({ text: raw.slice(cursor), match: false })
        break
      }
      if (index > cursor) {
        parts.push({ text: raw.slice(cursor, index), match: false })
      }
      parts.push({ text: raw.slice(index, index + query.length), match: true })
      cursor = index + query.length
    }
    return parts
  }

  $: query = searchQuery.trim().toLowerCase()
  $: visible = shouldDisplay(node)
  $: visibleChildren = node.children.filter((child) => shouldDisplay(child))
  $: expanded = !node.collapsed || (query && hasSearchMatch(node, query))
  $: headingParts = buildHighlights(node.text, query)
  $: bodyParts = buildHighlights(node.body, query)
  $: indent = `${Math.max(0, node.level - 1) * 1.4}rem`
</script>

{#if visible}
  <div class="org-node" style={`--indent:${indent}`}>
    <div class="org-node__row">
      {#if node.children.length > 0}
        <button
          class="org-node__toggle"
          type="button"
          aria-label={expanded ? 'Collapse section' : 'Expand section'}
          aria-expanded={expanded}
          on:click={() => onToggle(node)}
        >
          {expanded ? 'v' : '>'}
        </button>
      {:else}
        <span class="org-node__toggle org-node__toggle--empty" aria-hidden="true"></span>
      {/if}
      {#if node.keyword}
        <span class={`org-node__badge ${keywordClass(node.keyword)}`}>{node.keyword}</span>
      {/if}
      {#if node.priority}
        <span class="org-node__badge org-node__badge--priority">[#${node.priority}]</span>
      {/if}
      <span class="org-node__text">
        {#each headingParts as part, index (index)}
          <span class:org-node__highlight={part.match}>{part.text}</span>
        {/each}
      </span>
    </div>

    {#if expanded}
      {#if node.body}
        <div class="org-node__body" transition:slide={{ duration: 120 }}>
          {#each bodyParts as part, index (index)}
            <span class:org-node__highlight={part.match}>{part.text}</span>
          {/each}
        </div>
      {/if}

      {#if visibleChildren.length > 0}
        <div class="org-node__children" transition:slide={{ duration: 120 }}>
          {#each visibleChildren as child}
            <svelte:self node={child} {filter} {searchQuery} onToggle={onToggle} />
          {/each}
        </div>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .org-node {
    padding-left: var(--indent);
  }

  .org-node__row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.25rem 0.4rem;
    border-radius: 10px;
    color: #1b1b1b;
  }

  .org-node__row:hover {
    background: rgba(20, 20, 20, 0.05);
  }

  .org-node__toggle {
    width: 1.1rem;
    height: 1.1rem;
    border: none;
    background: rgba(20, 20, 20, 0.08);
    border-radius: 6px;
    color: #1b1b1b;
    font-size: 0.7rem;
    display: grid;
    place-items: center;
    cursor: pointer;
  }

  .org-node__toggle:focus-visible {
    outline: 2px solid rgba(31, 74, 154, 0.35);
    outline-offset: 2px;
  }

  .org-node__toggle--empty {
    background: transparent;
  }

  .org-node__badge {
    font-size: 0.65rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    padding: 0.1rem 0.4rem;
    border-radius: 999px;
    background: rgba(20, 20, 20, 0.08);
  }

  .org-node__badge--todo {
    background: rgba(36, 88, 190, 0.16);
    color: #1f4a9a;
  }

  .org-node__badge--wip {
    background: rgba(214, 143, 30, 0.18);
    color: #a05405;
  }

  .org-node__badge--done {
    background: rgba(73, 132, 90, 0.2);
    color: #2e6d3a;
  }

  .org-node__badge--priority {
    background: rgba(180, 60, 40, 0.18);
    color: #8f2a1a;
  }

  .org-node__text {
    font-size: 0.92rem;
    font-weight: 500;
  }

  .org-node__highlight {
    background: rgba(245, 214, 132, 0.7);
    border-radius: 4px;
    padding: 0 2px;
  }

  .org-node__body {
    margin: 0.2rem 0 0.4rem 1.6rem;
    color: #5f5b52;
    font-size: 0.82rem;
    white-space: pre-wrap;
  }

  .org-node__children {
    margin-bottom: 0.2rem;
  }
</style>
