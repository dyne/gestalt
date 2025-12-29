<script>
  import { fly, fade } from 'svelte/transition'

  export let notification
  export let onDismiss = () => {}

  const levelLabels = {
    info: 'Info',
    warning: 'Warning',
    error: 'Error',
  }

  const handleDismiss = () => {
    if (notification?.id) {
      onDismiss(notification.id)
    }
  }

  const handleKeydown = (event) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      handleDismiss()
    }
  }
</script>

<div
  class={`toast toast--${notification?.level ?? 'info'} ${
    notification?.autoClose ? 'toast--timed' : ''
  }`}
  style={`--duration: ${notification?.duration ?? 0}ms`}
  role="button"
  tabindex="0"
  on:click={handleDismiss}
  on:keydown={handleKeydown}
  in:fly={{ x: 24, duration: 180 }}
  out:fade={{ duration: 120 }}
>
  <header class="toast__header">
    <strong class="toast__title">
      {levelLabels[notification?.level] || 'Info'}
    </strong>
    <button
      class="toast__close"
      type="button"
      on:click|stopPropagation={handleDismiss}
      aria-label="Dismiss notification"
    >
      ×
    </button>
  </header>
  <p class="toast__message">{notification?.message}</p>
  {#if notification?.autoClose}
    <div class="toast__progress" aria-hidden="true"></div>
  {/if}
</div>

<style>
  .toast {
    position: relative;
    padding: 0.9rem 1rem 1rem;
    border-radius: 16px;
    background: var(--toast-bg, #1b1b1b);
    color: #f4f1eb;
    border: 1px solid rgba(255, 255, 255, 0.12);
    box-shadow: 0 18px 40px rgba(10, 10, 10, 0.3);
    cursor: pointer;
    overflow: hidden;
  }

  .toast--info {
    --toast-bg: #1a2333;
    --toast-accent: #66a2ff;
  }

  .toast--warning {
    --toast-bg: #2a2416;
    --toast-accent: #f6c15d;
  }

  .toast--error {
    --toast-bg: #2d1917;
    --toast-accent: #ff8a7a;
  }

  .toast__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    margin-bottom: 0.4rem;
  }

  .toast__title {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.16em;
    color: var(--toast-accent, #f4f1eb);
  }

  .toast__message {
    margin: 0;
    font-size: 0.95rem;
    color: #f4f1eb;
  }

  .toast__close {
    border: 0;
    background: rgba(255, 255, 255, 0.1);
    color: inherit;
    width: 1.4rem;
    height: 1.4rem;
    border-radius: 999px;
    display: grid;
    place-items: center;
    cursor: pointer;
    font-size: 0.9rem;
  }

  .toast__progress {
    position: absolute;
    left: 0;
    bottom: 0;
    height: 3px;
    background: var(--toast-accent, #f4f1eb);
    animation: toast-progress var(--duration) linear forwards;
  }

  @keyframes toast-progress {
    from {
      width: 100%;
    }
    to {
      width: 0%;
    }
  }
</style>
