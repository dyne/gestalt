import { describe, it, expect, vi, beforeEach } from 'vitest'

const buildEventSourceUrl = vi.hoisted(() =>
  vi.fn((path, params = {}) => {
    const search = new URLSearchParams(params).toString()
    return `http://test${path}${search ? `?${search}` : ''}`
  })
)

vi.mock('../src/lib/api.js', () => ({
  buildEventSourceUrl,
}))

class MockEventSource {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 2
  static instances = []

  constructor(url) {
    this.url = url
    this.readyState = MockEventSource.CONNECTING
    this.listeners = new Map()
    MockEventSource.instances.push(this)
  }

  addEventListener(type, handler) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set())
    }
    this.listeners.get(type).add(handler)
  }

  close() {
    this.readyState = MockEventSource.CLOSED
  }

  open() {
    this.readyState = MockEventSource.OPEN
    this.dispatch('open', {})
  }

  dispatch(type, payload) {
    const handlers = this.listeners.get(type)
    if (!handlers) return
    handlers.forEach((handler) => handler(payload))
  }

  error() {
    this.dispatch('error', {})
  }
}

beforeEach(() => {
  MockEventSource.instances = []
  vi.resetModules()
  global.EventSource = MockEventSource
})

const flush = () => Promise.resolve()

describe('eventStore', () => {
  it('subscribes and dispatches events', async () => {
    const { subscribe, eventConnectionStatus } = await import('../src/lib/eventStore.js')
    const received = []
    const statuses = []

    const unsubscribeStatus = eventConnectionStatus.subscribe((value) => {
      statuses.push(value)
    })
    const unsubscribe = subscribe('file-change', (payload) => {
      received.push(payload.path)
    })

    const source = MockEventSource.instances[0]
    source.open()
    await flush()

    source.dispatch('message', {
      data: JSON.stringify({ type: 'file-change', path: '.gestalt/PLAN.org' }),
    })

    expect(received).toEqual(['.gestalt/PLAN.org'])
    expect(statuses).toContain('connected')

    unsubscribe()
    unsubscribeStatus()
  })

  it('unsubscribes listeners', async () => {
    const { subscribe } = await import('../src/lib/eventStore.js')
    const received = []
    const unsubscribe = subscribe('git-branch', (payload) => {
      received.push(payload.path)
    })

    const source = MockEventSource.instances[0]
    source.open()
    await flush()

    unsubscribe()

    source.dispatch('message', {
      data: JSON.stringify({ type: 'git-branch', path: 'main' }),
    })

    expect(received).toEqual([])
  })

  it('updates types query on subscription changes', async () => {
    const { subscribe } = await import('../src/lib/eventStore.js')

    const stopFile = subscribe('file-change', () => {})
    let source = MockEventSource.instances[0]
    expect(new URL(source.url).searchParams.get('types')).toBe('file-change')

    const stopBranch = subscribe('git-branch', () => {})
    expect(MockEventSource.instances).toHaveLength(2)
    source = MockEventSource.instances[1]
    expect(new URL(source.url).searchParams.get('types')).toBe(
      'file-change,git-branch'
    )

    stopBranch()
    stopFile()
  })

  it('marks disconnected on error', async () => {
    const { subscribe, eventConnectionStatus } = await import('../src/lib/eventStore.js')
    const statuses = []
    const unsubscribeStatus = eventConnectionStatus.subscribe((value) => {
      statuses.push(value)
    })
    const unsubscribe = subscribe('file-change', () => {})

    const source = MockEventSource.instances[0]
    source.open()
    await flush()

    source.error()
    await flush()

    expect(statuses).toContain('disconnected')

    unsubscribe()
    unsubscribeStatus()
  })

  it('ignores malformed payloads without crashing', async () => {
    const { subscribe } = await import('../src/lib/eventStore.js')
    const received = []
    const unsubscribe = subscribe('file-change', (payload) => {
      received.push(payload.path)
    })

    const source = MockEventSource.instances[0]
    source.open()
    await flush()

    source.dispatch('message', { data: 'not-json' })
    source.dispatch('message', { data: JSON.stringify({ path: '/tmp/plan.org' }) })

    expect(received).toEqual([])
    unsubscribe()
  })

  it('handles burst file events', async () => {
    const { subscribe } = await import('../src/lib/eventStore.js')
    const received = []
    const unsubscribe = subscribe('file-change', (payload) => {
      received.push(payload.path)
    })

    const source = MockEventSource.instances[0]
    source.open()
    await flush()

    for (let index = 0; index < 5; index += 1) {
      source.dispatch('message', {
        data: JSON.stringify({ type: 'file-change', path: `/tmp/plan-${index}.org` }),
      })
    }

    expect(received).toEqual([
      '/tmp/plan-0.org',
      '/tmp/plan-1.org',
      '/tmp/plan-2.org',
      '/tmp/plan-3.org',
      '/tmp/plan-4.org',
    ])
    unsubscribe()
  })
})
