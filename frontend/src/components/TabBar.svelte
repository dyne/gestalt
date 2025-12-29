<script>
  export let tabs = []
  export let activeId = ''
  export let onSelect = () => {}
  export let onClose = () => {}
  export let onOpenSettings = () => {}
</script>

<nav class="tabbar" aria-label="Terminal tabs">
  {#each tabs as tab}
    <div class="tabbar__item" data-active={tab.id === activeId}>
      <button
        class="tabbar__button"
        type="button"
        on:click={() => onSelect(tab.id)}
        aria-current={tab.id === activeId ? 'page' : undefined}
      >
        <span class="tabbar__label">{tab.label}</span>
      </button>
      {#if !tab.isHome}
        <button
          class="tabbar__close"
          type="button"
          on:click|stopPropagation={() => onClose(tab.id)}
          aria-label={`Close ${tab.label}`}
        >
          ×
        </button>
      {/if}
    </div>
  {/each}
  <div class="tabbar__actions">
    <button class="tabbar__settings" type="button" on:click={onOpenSettings}>
      Notifications
    </button>
  </div>
</nav>

<style>
  .tabbar {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.75rem clamp(1.5rem, 4vw, 3.5rem);
    border-bottom: 1px solid rgba(20, 20, 20, 0.08);
    background: rgba(255, 255, 255, 0.9);
    backdrop-filter: blur(12px);
    position: sticky;
    top: 0;
    z-index: 10;
  }

  .tabbar__item {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.3rem 0.5rem 0.3rem 0.4rem;
    border-radius: 999px;
    border: 1px solid transparent;
    background: transparent;
    transition: border-color 160ms ease, background 160ms ease;
  }

  .tabbar__item[data-active='true'] {
    background: #151515;
    border-color: #151515;
  }

  .tabbar__button {
    border: none;
    background: transparent;
    font-size: 0.85rem;
    font-weight: 600;
    color: #151515;
    cursor: pointer;
    padding: 0 0.4rem;
  }

  .tabbar__item[data-active='true'] .tabbar__button {
    color: #f6f3ed;
  }

  .tabbar__close {
    border: none;
    background: rgba(255, 255, 255, 0.2);
    color: inherit;
    width: 1.4rem;
    height: 1.4rem;
    border-radius: 999px;
    cursor: pointer;
    display: grid;
    place-items: center;
    font-size: 0.9rem;
  }

  .tabbar__actions {
    margin-left: auto;
  }

  .tabbar__settings {
    border: 1px solid rgba(20, 20, 20, 0.2);
    border-radius: 999px;
    padding: 0.4rem 0.9rem;
    background: #ffffff;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }

  .tabbar__item[data-active='true'] .tabbar__close {
    color: #f6f3ed;
    background: rgba(255, 255, 255, 0.2);
  }

  @media (max-width: 720px) {
    .tabbar {
      flex-wrap: wrap;
    }
  }
</style>
