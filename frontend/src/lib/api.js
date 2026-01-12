const TOKEN_KEY = 'gestalt_token'
const isWails =
  typeof window !== 'undefined' && typeof window.runtime !== 'undefined'

let cachedServerURL = null

function getToken() {
  try {
    return window.localStorage.getItem(TOKEN_KEY) || ''
  } catch {
    return ''
  }
}

async function getServerURL() {
  if (cachedServerURL) {
    return cachedServerURL
  }
  if (!isWails) {
    cachedServerURL = ''
    return cachedServerURL
  }
  const modulePath = './wails/go/main/App'
  const { GetServerURL } = await import(/* @vite-ignore */ modulePath)
  cachedServerURL = await GetServerURL()
  return cachedServerURL
}

export async function buildApiUrl(path) {
  if (!isWails) {
    return path
  }
  const base = await getServerURL()
  return `${base}${path}`
}

export async function buildWebSocketUrl(path) {
  const token = getToken()
  let url = null

  if (isWails) {
    const base = await getServerURL()
    const wsURL = new URL(base)
    wsURL.protocol = wsURL.protocol === 'https:' ? 'wss:' : 'ws:'
    wsURL.pathname = path
    url = wsURL
  } else {
    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
    url = new URL(`${protocol}://${window.location.host}${path}`)
  }

  if (token) {
    url.searchParams.set('token', token)
  }

  return url.toString()
}

export async function apiFetch(path, options = {}) {
  const { allowNotModified, ...fetchOptions } = options
  const headers = new Headers(fetchOptions.headers || {})
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  if (fetchOptions.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const url = await buildApiUrl(path)
  const response = await fetch(url, {
    ...fetchOptions,
    headers,
  })

  if (!response.ok && !(allowNotModified && response.status === 304)) {
    const bodyText = await response.text()
    let payload = null
    let message = bodyText
    if (bodyText) {
      try {
        payload = JSON.parse(bodyText)
        if (payload?.error) {
          message = payload.error
        }
      } catch {
        // Ignore JSON parsing errors.
      }
    }
    const error = new Error(message || `Request failed: ${response.status}`)
    error.status = response.status
    if (payload) {
      error.data = payload
    }
    throw error
  }

  return response
}
