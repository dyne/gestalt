import { apiFetch } from './api.js'

const normalizeLevel = (level) => {
  const raw = String(level || '').trim().toLowerCase()
  if (!raw) return 'info'
  if (raw.startsWith('warn')) return 'warning'
  if (raw.startsWith('err') || raw.startsWith('fatal')) return 'error'
  if (raw.startsWith('debug') || raw.startsWith('trace')) return 'debug'
  return raw
}

const mergeAttributes = (attributes) => {
  const merged = { ...(attributes || {}) }
  if (!('gestalt.source' in merged)) {
    merged['gestalt.source'] = 'frontend'
  }
  if (!('gestalt.category' in merged)) {
    merged['gestalt.category'] = 'ui'
  }
  return merged
}

export const logUI = ({ level = 'info', body = '', attributes } = {}) => {
  if (typeof window === 'undefined') return
  const payload = {
    severity_text: normalizeLevel(level),
    body,
  }
  const mergedAttributes = mergeAttributes(attributes)
  if (Object.keys(mergedAttributes).length > 0) {
    payload.attributes = mergedAttributes
  }
  void apiFetch('/api/otel/logs', {
    method: 'POST',
    body: JSON.stringify(payload),
  }).catch(() => {
    // Ignore log transport errors to avoid loops.
  })
}
