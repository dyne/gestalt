<script>
  import { onDestroy, onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import '@xterm/xterm/css/xterm.css'

  import { buildWebSocketUrl } from '../lib/api.js'

  export let terminalId = ''

  let container
  let term
  let fitAddon
  let socket
  let bellCount = 0
  let status = 'disconnected'

  const encoder = new TextEncoder()

  const sendResize = () => {
    if (!socket || socket.readyState !== WebSocket.OPEN || !term) return
    const payload = {
      type: 'resize',
      cols: term.cols,
      rows: term.rows,
    }
    socket.send(JSON.stringify(payload))
  }

  const connect = () => {
    if (!terminalId) return

    term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: '"IBM Plex Mono", "JetBrains Mono", monospace',
      theme: {
        background: '#101010',
        foreground: '#f2efe9',
        cursor: '#f2efe9',
        selectionBackground: '#3a3a3a',
      },
    })
    fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(container)
    fitAddon.fit()

    socket = new WebSocket(buildWebSocketUrl(`/ws/terminal/${terminalId}`))
    socket.binaryType = 'arraybuffer'
    status = 'connecting'

    socket.addEventListener('open', () => {
      status = 'connected'
      sendResize()
    })

    socket.addEventListener('message', (event) => {
      if (!term) return
      if (typeof event.data === 'string') {
        term.write(event.data)
        return
      }
      term.write(new Uint8Array(event.data))
    })

    socket.addEventListener('close', () => {
      status = 'disconnected'
    })

    socket.addEventListener('error', () => {
      status = 'error'
    })

    term.onData((data) => {
      if (socket.readyState !== WebSocket.OPEN) return
      socket.send(encoder.encode(data))
    })

    term.onBell(() => {
      bellCount += 1
    })
  }

  const resizeHandler = () => {
    if (!fitAddon || !term) return
    fitAddon.fit()
    sendResize()
  }

  onMount(() => {
    connect()
    window.addEventListener('resize', resizeHandler)
  })

  onDestroy(() => {
    window.removeEventListener('resize', resizeHandler)
    if (socket) {
      socket.close()
    }
    if (term) {
      term.dispose()
    }
  })
</script>

<section class="terminal-shell">
  <header class="terminal-shell__header">
    <div>
      <p class="label">Terminal {terminalId || '—'}</p>
      <p class="status">{status}</p>
    </div>
    <div class="bell" aria-live="polite">
      <span>Bell</span>
      <strong>{bellCount}</strong>
    </div>
  </header>
  <div class="terminal-shell__body" bind:this={container}></div>
</section>

<style>
  .terminal-shell {
    display: grid;
    grid-template-rows: auto 1fr;
    height: 100%;
    min-height: 70vh;
    background: #101010;
    border-radius: 20px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    box-shadow: 0 20px 50px rgba(10, 10, 10, 0.35);
    overflow: hidden;
  }

  .terminal-shell__header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.9rem 1.2rem;
    background: #171717;
    border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  }

  .label {
    margin: 0;
    font-size: 0.85rem;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: rgba(242, 239, 233, 0.7);
  }

  .status {
    margin: 0.2rem 0 0;
    font-size: 0.75rem;
    color: rgba(242, 239, 233, 0.5);
    text-transform: uppercase;
    letter-spacing: 0.12em;
  }

  .bell {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.4rem 0.9rem;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.08);
    color: rgba(242, 239, 233, 0.9);
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.12em;
  }

  .bell strong {
    font-size: 0.9rem;
  }

  .terminal-shell__body {
    padding: 0.6rem;
  }

  :global(.xterm) {
    height: 100%;
  }

  :global(.xterm-viewport) {
    border-radius: 12px;
  }

  @media (max-width: 720px) {
    .terminal-shell {
      min-height: 60vh;
    }

    .terminal-shell__header {
      flex-direction: column;
      align-items: flex-start;
      gap: 0.5rem;
    }
  }
</style>
