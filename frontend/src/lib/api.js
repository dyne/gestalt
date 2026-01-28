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

export function buildEventSourceUrl(path, params = {}) {
  const token = getToken()
  const url = new URL(path, window.location.origin)

  Object.entries(params || {}).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return
    if (Array.isArray(value)) {
      value.forEach((entry) => {
        if (entry === undefined || entry === null || entry === '') return
        url.searchParams.append(key, entry)
      })
      return
    }
    url.searchParams.set(key, value)
  })

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

  const response = await fetch(path, {
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
        if (payload?.message) {
          message = payload.message
        } else if (payload?.error) {
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
