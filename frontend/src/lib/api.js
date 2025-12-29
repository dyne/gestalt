const TOKEN_KEY = 'gestalt_token'

function getToken() {
  try {
    return window.localStorage.getItem(TOKEN_KEY) || ''
  } catch {
    return ''
  }
}

export function buildWebSocketUrl(path) {
  const token = getToken()
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const url = new URL(`${protocol}://${window.location.host}${path}`)

  if (token) {
    url.searchParams.set('token', token)
  }

  return url.toString()
}

export async function apiFetch(path, options = {}) {
  const headers = new Headers(options.headers || {})
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(path, {
    ...options,
    headers,
  })

  if (!response.ok) {
    const message = await response.text()
    throw new Error(message || `Request failed: ${response.status}`)
  }

  return response
}
