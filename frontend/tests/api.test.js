import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { apiFetch, buildApiPath, buildWebSocketUrl } from '../src/lib/api.js'

const mockFetch = (response) => {
  const fetchMock = vi.fn().mockResolvedValue(response)
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const ensureLocalStorage = () => {
  if (
    !globalThis.localStorage ||
    typeof globalThis.localStorage.setItem !== 'function'
  ) {
    const store = new Map()
    vi.stubGlobal('localStorage', {
      getItem: (key) => (store.has(key) ? store.get(key) : null),
      setItem: (key, value) => {
        store.set(key, String(value))
      },
      removeItem: (key) => {
        store.delete(key)
      },
      clear: () => {
        store.clear()
      },
    })
  }
}

describe('api helpers', () => {
  beforeEach(() => {
    ensureLocalStorage()
  })

  afterEach(() => {
    if (typeof localStorage?.clear === 'function') {
      localStorage.clear()
    }
    vi.unstubAllGlobals()
  })

  it('buildWebSocketUrl appends token', () => {
    localStorage.setItem('gestalt_token', 'abc123')
    const url = buildWebSocketUrl('/ws/session/1')
    expect(url.startsWith('ws://')).toBe(true)
    expect(url).toContain('/ws/session/1')
    expect(url).toContain('token=abc123')
  })

  it('buildApiPath encodes path segments', () => {
    const path = buildApiPath('/api/sessions', 'Architect (Codex) 1', 'history')
    expect(path).toBe('/api/sessions/Architect%20(Codex)%201/history')
  })

  it('apiFetch attaches auth and content headers', async () => {
    localStorage.setItem('gestalt_token', 'secret')
    const fetchMock = mockFetch({ ok: true, status: 200, text: vi.fn().mockResolvedValue('') })

    await apiFetch('/api/status', {
      method: 'POST',
      body: JSON.stringify({ ok: true }),
    })

    const [, options] = fetchMock.mock.calls[0]
    expect(options.headers.get('Authorization')).toBe('Bearer secret')
    expect(options.headers.get('Content-Type')).toBe('application/json')
  })

  it('apiFetch throws on non-ok responses', async () => {
    mockFetch({
      ok: false,
      status: 500,
      text: vi.fn().mockResolvedValue('boom'),
    })

    await expect(apiFetch('/api/status')).rejects.toMatchObject({
      message: 'boom',
      status: 500,
    })
  })

  it('apiFetch allows 304 when allowNotModified is set', async () => {
    const response = {
      ok: false,
      status: 304,
      text: vi.fn().mockResolvedValue(''),
    }
    mockFetch(response)

    const result = await apiFetch('/api/plans', { allowNotModified: true })
    expect(result).toBe(response)
  })
})
