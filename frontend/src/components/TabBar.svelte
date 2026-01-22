<script>
  import { VERSION } from '../lib/version.js'
  import dyneIconLight from '../assets/dyne-icon-black.svg'
  import dyneIconDark from '../assets/border-white-Icon.svg'
  import dyneLogotypeLight from '../assets/dyne-logotype-black.svg'
  import dyneLogotypeDark from '../assets/white-Logotype.svg'
  import gestaltIconLight from '../assets/p_glogo_grey.svg'
  import gestaltIconDark from '../assets/p_glogo_white.svg'
  import gestaltLogotypeLight from '../assets/t_glogo_grey.svg'
  import gestaltLogotypeDark from '../assets/t_glogo_white.svg'

  export let tabs = []
  export let activeId = ''
  export let onSelect = () => {}

  $: visibleTabs = tabs.filter((tab) => tab.id !== 'dashboard')
</script>

<nav class="tabbar" aria-label="App tabs">
  <button
    class="tabbar__brand"
    type="button"
    on:click={() => onSelect('dashboard')}
    aria-label="Open dashboard"
  >
    <img
      class="tabbar__brand-icon tabbar__brand-icon--light"
      src={gestaltIconLight}
      alt="Gestalt icon"
    />
    <img
      class="tabbar__brand-icon tabbar__brand-icon--dark"
      src={gestaltIconDark}
      alt="Gestalt icon"
    />
    <img
      class="tabbar__brand-logotype tabbar__brand-logotype--light"
      src={gestaltLogotypeLight}
      alt="Gestalt"
    />
    <img
      class="tabbar__brand-logotype tabbar__brand-logotype--dark"
      src={gestaltLogotypeDark}
      alt="Gestalt"
    />
    <span class="tabbar__brand-by">v{VERSION}</span>
  </button>
  {#each visibleTabs as tab}
    <div class="tabbar__item" data-active={tab.id === activeId}>
      <button
        class="tabbar__button"
        type="button"
        on:click={() => onSelect(tab.id)}
        aria-current={tab.id === activeId ? 'page' : undefined}
      >
        <span class="tabbar__label">{tab.label}</span>
      </button>
    </div>
  {/each}
    <div class="tabbar__logos">
      <a href="https://dyne.org" target="_blank" rel="noopener noreferrer">
        <img
          class="tabbar__logo tabbar__logo--type tabbar__logo--light"
          src={dyneLogotypeLight}
          alt="Dyne.org"
        />
        <img
          class="tabbar__logo tabbar__logo--type tabbar__logo--dark"
          src={dyneLogotypeDark}
          alt="Dyne.org"
        />
      </a>
      <a href="https://dyne.org" target="_blank" rel="noopener noreferrer">
        <img
          class="tabbar__logo tabbar__logo--icon tabbar__logo--light"
          src={dyneIconLight}
          alt="Dyne.org"
        />
        <img
          class="tabbar__logo tabbar__logo--icon tabbar__logo--dark"
          src={dyneIconDark}
          alt="Dyne.org"
        />
      </a>
    </div>
</nav>

<style>
  .tabbar {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.75rem clamp(1.5rem, 4vw, 3.5rem);
    border-bottom: 1px solid rgba(var(--color-text-rgb), 0.08);
    background: rgba(var(--color-surface-rgb), 0.9);
    backdrop-filter: blur(12px);
    position: sticky;
    top: 0;
    z-index: 10;
  }

  .tabbar__brand {
    border: none;
    background: transparent;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0;
    cursor: pointer;
    color: var(--color-text);
    white-space: nowrap;
  }

  .tabbar__brand-icon {
    height: 36px;
    width: auto;
    display: block;
  }

  .tabbar__brand-logotype {
    height: 40px;
    width: auto;
    display: block;
  }

  .tabbar__brand-by {
    font-size: 0.65rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--color-text-muted);
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
    background: var(--color-contrast-bg);
    border-color: var(--color-contrast-bg);
  }

  .tabbar__button {
    border: none;
    background: transparent;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--color-text);
    cursor: pointer;
    padding: 0 0.4rem;
  }

  .tabbar__item[data-active='true'] .tabbar__button {
    color: var(--color-contrast-text);
  }

  .tabbar__logos {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 1.2rem;
  }

  .tabbar__logo {
    display: block;
    transition: opacity 160ms ease;
  }

  .tabbar__logo--icon {
    width: 32px;
    height: 32px;
  }

  .tabbar__logo--type {
    height: 28px;
    width: auto;
  }

  .tabbar__brand-icon--dark,
  .tabbar__brand-logotype--dark,
  .tabbar__logo--dark {
    display: none;
  }

  .tabbar__brand-icon--light,
  .tabbar__brand-logotype--light,
  .tabbar__logo--light {
    display: block;
  }

  .tabbar__logos a:hover .tabbar__logo {
    opacity: 0.7;
  }

  @media (prefers-color-scheme: dark) {
    .tabbar__brand-icon--light,
    .tabbar__brand-logotype--light,
    .tabbar__logo--light {
      display: none;
    }

    .tabbar__brand-icon--dark,
    .tabbar__brand-logotype--dark,
    .tabbar__logo--dark {
      display: block;
    }
  }

  @media (max-width: 720px) {
    .tabbar {
      flex-wrap: wrap;
    }
  }
</style>
