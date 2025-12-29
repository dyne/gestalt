<script>
  import Toast from './Toast.svelte'

  export let notifications = []
  export let onDismiss = () => {}
  export let position = 'top-right'
</script>

<section class={`toast-container toast-container--${position}`} aria-live="polite">
  {#each notifications as notification (notification.id)}
    <Toast {notification} {onDismiss} />
  {/each}
</section>

<style>
  .toast-container {
    position: fixed;
    z-index: 20;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    max-width: min(320px, 90vw);
    pointer-events: none;
  }

  .toast-container :global(.toast) {
    pointer-events: auto;
  }

  .toast-container--top-right {
    top: 1.5rem;
    right: 1.5rem;
  }

  .toast-container--bottom-right {
    bottom: 1.5rem;
    right: 1.5rem;
  }

  @media (max-width: 720px) {
    .toast-container {
      left: 1.5rem;
      right: 1.5rem;
    }
  }
</style>
