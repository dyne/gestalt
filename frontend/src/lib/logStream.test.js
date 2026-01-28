import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createLogStream } from './logStream.js'

vi.mock('./api.js', () => ({
  buildEventSourceUrl: (path, params = {}) => {
    const search = new URLSearchParams(params).toString()
    return `http://test${path}${search ? `?${search}` : ''}`
  },
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

  addEventListener(type, listener) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set())
    }
    this.listeners.get(type).add(listener)
  }

  close() {
    if (this.readyState === MockEventSource.CLOSED) return
    this.readyState = MockEventSource.CLOSED
  }

  dispatch(type, event) {
    const listeners = this.listeners.get(type)
    if (!listeners) return
    listeners.forEach((listener) => listener(event))
  }

  open() {
    this.readyState = MockEventSource.OPEN
    this.dispatch('open', {})
  }

  message(data) {
    this.dispatch('message', { data })
  }

  error() {
    this.dispatch('error', {})
  }
}

describe('createLogStream', () => {
  beforeEach(() => {
    MockEventSource.instances = []
    vi.useFakeTimers()
    vi.stubGlobal('EventSource', MockEventSource)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('connects and dispatches entries', () => {
    const onEntry = vi.fn()
    const onStatus = vi.fn()
    const onOpen = vi.fn()
    const onError = vi.fn()

    const stream = createLogStream({ onEntry, onStatus, onOpen, onError })
    stream.start()

    const source = MockEventSource.instances[0]
    expect(source).toBeDefined()
    expect(onStatus).toHaveBeenCalledWith('connecting')

    source.open()
    expect(onStatus).toHaveBeenCalledWith('connected')
    expect(onOpen).toHaveBeenCalled()

    source.message(JSON.stringify({ message: 'hello' }))
    expect(onEntry).toHaveBeenCalledWith({ message: 'hello' })

    source.message(JSON.stringify({ type: 'error', message: 'boom' }))
    expect(onError).toHaveBeenCalled()

    stream.stop()
  })

  it('reconnects after errors when active', () => {
    const onStatus = vi.fn()
    const stream = createLogStream({ onStatus })

    stream.start()
    const source = MockEventSource.instances[0]
    source.open()
    source.error()

    expect(onStatus).toHaveBeenCalledWith('disconnected')
    expect(MockEventSource.instances).toHaveLength(1)

    vi.advanceTimersByTime(500)
    expect(MockEventSource.instances).toHaveLength(2)

    stream.stop()
  })

  it('restarts when the level changes', () => {
    const stream = createLogStream({ level: '' })

    stream.start()
    const source = MockEventSource.instances[0]
    source.open()

    stream.setLevel('warning')
    expect(MockEventSource.instances).toHaveLength(2)
    const nextSource = MockEventSource.instances[1]
    expect(new URL(nextSource.url).searchParams.get('level')).toBe('warning')

    stream.stop()
  })
})
