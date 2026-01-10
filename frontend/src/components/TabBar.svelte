<script>
  import { VERSION } from '../lib/version.js'
  import dyneIcon from '../assets/dyne-icon-black.svg'
  import dyneLogotype from '../assets/dyne-logotype-black.svg'
  import gestaltLogo from '../assets/gestalt-logo.svg'

  export let tabs = []
  export let activeId = ''
  export let onSelect = () => {}
</script>

<nav class="tabbar" aria-label="App tabs">
  <button
    class="tabbar__brand"
    type="button"
    on:click={() => onSelect('dashboard')}
    aria-label="Open dashboard"
  >
    <img class="tabbar__brand-logo" src={gestaltLogo} alt="Gestalt" />
    <span class="tabbar__brand-by">v{VERSION}</span>
  </button>
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
    </div>
  {/each}
    <div class="tabbar__logos">
      <a href="https://dyne.org" target="_blank" rel="noopener noreferrer">
        <img
          class="tabbar__logo tabbar__logo--type"
          src={dyneLogotype}
          alt="Dyne.org"
          />
      </a>
      <a href="https://dyne.org" target="_blank" rel="noopener noreferrer">
        <img
          class="tabbar__logo tabbar__logo--icon"
          src={dyneIcon}
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
    border-bottom: 1px solid rgba(20, 20, 20, 0.08);
    background: rgba(255, 255, 255, 0.9);
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
    color: #151515;
    white-space: nowrap;
  }

  .tabbar__brand-logo {
    height: 40px;
    width: auto;
    display: block;
  }

  .tabbar__brand-by {
    font-size: 0.65rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: #6d6a61;
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
    width: 28px;
    height: 28px;
  }

  .tabbar__logo--type {
    height: 20px;
    width: auto;
  }

  .tabbar__logos a:hover .tabbar__logo {
    opacity: 0.7;
  }

  @media (max-width: 720px) {
    .tabbar {
      flex-wrap: wrap;
    }
  }
</style>
