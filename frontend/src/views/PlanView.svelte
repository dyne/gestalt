<script>
  import { onDestroy, onMount } from 'svelte'
  import { apiFetch, buildWebSocketUrl } from '../lib/api.js'
  import OrgViewer from '../components/OrgViewer.svelte'

  let loading = false
  let refreshing = false
  let error = ''
  let content = ''
  let lastContent = ''
  let etag = ''
  let fallbackTimer = null
  let socket = null
  let reconnectTimer = null
  let reconnectAttempts = 0
  let updateNotice = false
  let updateNoticeTimer = null

  const fallbackIntervalMs = 10000
  const baseReconnectDelayMs = 500
  const maxReconnectDelayMs = 10000

  const showUpdateNotice = () => {
    updateNotice = true
    if (updateNoticeTimer) {
      clearTimeout(updateNoticeTimer)
    }
    updateNoticeTimer = setTimeout(() => {
      updateNotice = false
    }, 2000)
  }

  const loadPlan = async ({ silent = false, notify = false } = {}) => {
    if (loading || refreshing) return
    if (silent) {
      refreshing = true
    } else {
      loading = true
    }
    error = ''
    try {
      const response = await apiFetch('/api/plan', {
        allowNotModified: true,
        headers: etag ? { 'If-None-Match': etag } : {},
      })
      const responseEtag = response.headers?.get?.('ETag')
      if (responseEtag) {
        etag = responseEtag
      }
      if (response.status === 304) {
        return
      }
      const payload = await response.json()
      const nextContent = payload?.content || ''
      if (nextContent !== lastContent) {
        content = nextContent
        lastContent = nextContent
        if (notify) {
          showUpdateNotice()
        }
      }
    } catch (err) {
      error = err?.message || 'Failed to load plan.'
    } finally {
      loading = false
      refreshing = false
    }
  }

  const stopFallbackPolling = () => {
    if (fallbackTimer) {
      clearInterval(fallbackTimer)
      fallbackTimer = null
    }
  }

  const startFallbackPolling = () => {
    if (fallbackTimer) return
    fallbackTimer = setInterval(() => {
      loadPlan({ silent: true })
    }, fallbackIntervalMs)
  }

  const scheduleReconnect = () => {
    if (reconnectTimer) return
    const delay = Math.min(maxReconnectDelayMs, baseReconnectDelayMs * 2 ** reconnectAttempts)
    reconnectAttempts += 1
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      connectEvents()
    }, delay)
  }

  const connectEvents = () => {
    if (socket) {
      socket.close()
      socket = null
    }

    try {
      socket = new WebSocket(buildWebSocketUrl('/ws/events'))
    } catch {
      startFallbackPolling()
      scheduleReconnect()
      return
    }

    socket.addEventListener('open', () => {
      reconnectAttempts = 0
      stopFallbackPolling()
      socket?.send(JSON.stringify({ subscribe: ['file_changed'] }))
    })

    socket.addEventListener('message', (event) => {
      let payload = null
      try {
        payload = JSON.parse(event.data)
      } catch {
        return
      }
      if (payload?.type !== 'file_changed' || !payload?.path) {
        return
      }
      if (payload.path !== 'PLAN.org' && !payload.path.endsWith('/PLAN.org')) {
        return
      }
      loadPlan({ silent: true, notify: true })
    })

    socket.addEventListener('close', () => {
      socket = null
      startFallbackPolling()
      scheduleReconnect()
    })

    socket.addEventListener('error', () => {
      socket?.close()
    })
  }

  onMount(() => {
    loadPlan()
    connectEvents()
  })

  onDestroy(() => {
    stopFallbackPolling()
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (updateNoticeTimer) {
      clearTimeout(updateNoticeTimer)
      updateNoticeTimer = null
    }
    if (socket) {
      socket.close()
      socket = null
    }
  })
</script>

<section class="plan-view">
  <header class="plan-view__header">
    <div>
      <p class="eyebrow">Project plan</p>
      <h1>PLAN.org</h1>
    </div>
    <div class="refresh-actions">
      {#if updateNotice}
        <span class="updated">Plan updated</span>
      {/if}
      {#if refreshing}
        <span class="refreshing">Updating...</span>
      {/if}
      <button class="refresh" type="button" on:click={loadPlan} disabled={loading}>
        {loading ? 'Refreshing...' : 'Refresh'}
      </button>
    </div>
  </header>

  {#if loading && !content}
    <p class="muted">Loading plan...</p>
  {:else if error && !content}
    <p class="error">{error}</p>
  {:else}
    {#if error}
      <p class="error error--inline">{error}</p>
    {/if}
    <OrgViewer orgText={content} />
  {/if}
</section>

<style>
  .plan-view {
    padding: 2.5rem clamp(1.5rem, 4vw, 3.5rem) 3.5rem;
    display: flex;
    flex-direction: column;
    gap: 2rem;
  }

  .plan-view__header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1.5rem;
  }

  .eyebrow {
    text-transform: uppercase;
    letter-spacing: 0.24em;
    font-size: 0.7rem;
    color: #6d6a61;
    margin: 0 0 0.6rem;
  }

  h1 {
    margin: 0;
    font-size: clamp(2rem, 3.5vw, 3rem);
    font-weight: 600;
    color: #161616;
  }

  .refresh {
    border: 1px solid rgba(20, 20, 20, 0.2);
    border-radius: 999px;
    padding: 0.6rem 1.2rem;
    background: #ffffff;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }

  .refresh-actions {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .refreshing {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    color: #6f6b62;
  }

  .updated {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    color: #2e6b46;
  }

  .muted {
    color: #7d7a73;
    margin: 0.5rem 0 0;
  }

  .error {
    color: #b04a39;
    margin: 0.5rem 0 0;
  }

  .error--inline {
    margin: 0 0 1rem;
  }

  @media (max-width: 720px) {
    .plan-view__header {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
