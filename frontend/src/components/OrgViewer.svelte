<script>
  import { onDestroy } from 'svelte'
  import OrgNode from './OrgNode.svelte'
  import { parseOrg } from '../lib/orgParser.js'

  export let orgText = ''
  export let orgTree = null

  const filterOptions = [
    { value: 'all', label: 'All' },
    { value: 'todo', label: 'TODO only' },
    { value: 'wip', label: 'WIP only' },
    { value: 'done', label: 'DONE only' },
    { value: 'todo-wip', label: 'TODO + WIP' },
  ]

  let tree = []
  let filter = 'all'
  let searchInput = ''
  let searchQuery = ''
  let debounceId = null

  const applyToAll = (nodes, updater) => {
    for (const node of nodes) {
      updater(node)
      if (node.children.length > 0) {
        applyToAll(node.children, updater)
      }
    }
  }

  const toggleNode = (node) => {
    node.collapsed = !node.collapsed
    tree = [...tree]
  }

  const expandAll = () => {
    applyToAll(tree, (node) => {
      node.collapsed = false
    })
    tree = [...tree]
  }

  const collapseAll = () => {
    applyToAll(tree, (node) => {
      node.collapsed = true
    })
    tree = [...tree]
  }

  const collapseToLevel = (level) => {
    applyToAll(tree, (node) => {
      node.collapsed = node.level > level
    })
    tree = [...tree]
  }

  const handleSearchInput = (event) => {
    searchInput = event.target.value
    if (debounceId) {
      clearTimeout(debounceId)
    }
    debounceId = setTimeout(() => {
      searchQuery = searchInput.trim()
    }, 300)
  }

  const matchesFilter = (node) => {
    if (filter === 'all') return true
    if (filter === 'todo') return node.keyword === 'TODO'
    if (filter === 'wip') return node.keyword === 'WIP'
    if (filter === 'done') return node.keyword === 'DONE'
    if (filter === 'todo-wip') return node.keyword === 'TODO' || node.keyword === 'WIP'
    return true
  }

  const hasVisibleNodes = (nodes) =>
    nodes.some((node) => matchesFilter(node) || hasVisibleNodes(node.children))

  $: {
    if (orgTree != null) {
      tree = orgTree
    } else {
      tree = parseOrg(orgText)
    }
  }

  $: filterHasMatches = tree.length > 0 && hasVisibleNodes(tree)

  onDestroy(() => {
    if (debounceId) {
      clearTimeout(debounceId)
    }
  })
</script>

<section class="org-viewer">
  <header class="org-viewer__toolbar">
    <div class="org-viewer__actions">
      <button type="button" on:click={expandAll}>Expand all</button>
      <button type="button" on:click={collapseAll}>Collapse all</button>
      <button type="button" on:click={() => collapseToLevel(1)}>Collapse to L1</button>
      <button type="button" on:click={() => collapseToLevel(2)}>Collapse to L2</button>
    </div>
    <div class="org-viewer__filters">
      <label>
        <span>Filter</span>
        <select bind:value={filter}>
          {#each filterOptions as option}
            <option value={option.value}>{option.label}</option>
          {/each}
        </select>
      </label>
      <label class="org-viewer__search">
        <span>Search</span>
        <input
          type="search"
          placeholder="Find headings or body text"
          value={searchInput}
          on:input={handleSearchInput}
        />
      </label>
    </div>
  </header>

  <div class="org-viewer__content">
    {#if tree.length === 0}
      <p class="muted">No plan entries found.</p>
    {:else if filter !== 'all' && !filterHasMatches}
      <p class="muted">No entries match this filter.</p>
    {:else}
      {#each tree as node}
        <OrgNode {node} {filter} {searchQuery} onToggle={toggleNode} />
      {/each}
    {/if}
  </div>
</section>

<style>
  .org-viewer {
    border-radius: 24px;
    border: 1px solid rgba(20, 20, 20, 0.08);
    background: rgba(255, 255, 255, 0.88);
    box-shadow: 0 25px 60px rgba(20, 20, 20, 0.08);
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 1.2rem;
  }

  .org-viewer__toolbar {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    justify-content: space-between;
    align-items: center;
  }

  .org-viewer__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .org-viewer__actions button {
    border: 1px solid rgba(20, 20, 20, 0.15);
    border-radius: 999px;
    padding: 0.35rem 0.9rem;
    background: #ffffff;
    font-size: 0.75rem;
    font-weight: 600;
    cursor: pointer;
  }

  .org-viewer__actions button:focus-visible,
  .org-viewer__filters select:focus-visible,
  .org-viewer__filters input:focus-visible {
    outline: 2px solid rgba(31, 74, 154, 0.35);
    outline-offset: 2px;
  }

  .org-viewer__filters {
    display: flex;
    flex-wrap: wrap;
    gap: 0.8rem;
    align-items: center;
  }

  .org-viewer__filters label {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.16em;
    color: #6d6a61;
  }

  .org-viewer__filters select,
  .org-viewer__filters input {
    border: 1px solid rgba(20, 20, 20, 0.15);
    border-radius: 12px;
    padding: 0.45rem 0.7rem;
    font-size: 0.85rem;
    min-width: 160px;
  }

  .org-viewer__search input {
    min-width: 200px;
  }

  .org-viewer__content {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    max-width: 100%;
  }

  .muted {
    color: #7d7a73;
    margin: 0.4rem 0 0;
  }

  @media (max-width: 720px) {
    .org-viewer {
      padding: 1.2rem;
    }

    .org-viewer__toolbar {
      align-items: stretch;
    }

    .org-viewer__filters {
      width: 100%;
      justify-content: space-between;
    }

    .org-viewer__filters select,
    .org-viewer__filters input {
      width: 100%;
      min-width: 0;
    }
  }
</style>
