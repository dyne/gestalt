const normalizeTimestamp = (value) => {
  if (!value) return ''
  if (value instanceof Date) return value.toISOString()
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) return ''
    if (/^\d+$/.test(trimmed)) {
      const numeric = Number(trimmed)
      if (!Number.isNaN(numeric)) {
        return normalizeTimestamp(numeric)
      }
    }
    return value
  }
  if (typeof value === 'number') {
    if (value > 1e12) {
      return new Date(Math.floor(value / 1e6)).toISOString()
    }
    if (value > 1e10) {
      return new Date(value).toISOString()
    }
    return new Date(value * 1000).toISOString()
  }
  return value
}

let logSequence = 0

const normalizeLevel = (value) => {
  if (value === null || value === undefined || value === '') {
    return 'info'
  }
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (/^\d+$/.test(trimmed)) {
      const numeric = Number(trimmed)
      if (!Number.isNaN(numeric)) {
        return normalizeLevel(numeric)
      }
    }
  }
  if (typeof value === 'number') {
    if (value >= 17) return 'error'
    if (value >= 13) return 'warning'
    if (value >= 9) return 'info'
    return 'debug'
  }
  const normalized = String(value).toLowerCase()
  if (normalized.startsWith('warn')) return 'warning'
  if (normalized.startsWith('err') || normalized.startsWith('fatal')) return 'error'
  if (normalized.startsWith('debug') || normalized.startsWith('trace')) return 'debug'
  return normalized
}

const readAttributeValue = (value) => {
  if (value && typeof value === 'object') {
    if (value.stringValue !== undefined) return value.stringValue
    if (value.StringValue !== undefined) return value.StringValue
    if (value.boolValue !== undefined) return value.boolValue
    if (value.BoolValue !== undefined) return value.BoolValue
    if (value.intValue !== undefined) return value.intValue
    if (value.IntValue !== undefined) return value.IntValue
    if (value.doubleValue !== undefined) return value.doubleValue
    if (value.DoubleValue !== undefined) return value.DoubleValue
    if (value.value !== undefined) return value.value
    if (value.Value !== undefined) return value.Value
  }
  return value
}

const stringifyContextValue = (value) => {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') {
    return String(value)
  }
  if (value instanceof Date) return value.toISOString()
  const extracted = readAttributeValue(value)
  if (extracted !== value) {
    return stringifyContextValue(extracted)
  }
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

const normalizeContext = (...sources) => {
  const merged = {}
  sources.forEach((source) => {
    if (!source) return
    if (Array.isArray(source)) {
      source.forEach((entry) => {
        if (!entry || typeof entry !== 'object') return
        const key = entry.key || entry.Key
        if (!key) return
        const rawValue = entry.value ?? entry.Value ?? entry.val
        merged[key] = stringifyContextValue(rawValue)
      })
      return
    }
    if (typeof source === 'object') {
      Object.entries(source).forEach(([key, value]) => {
        merged[key] = stringifyContextValue(value)
      })
    }
  })
  return merged
}

const normalizeMessage = (entry) => {
  if (!entry) return ''
  const body = entry.body ?? entry
  if (typeof body === 'string') {
    return body
  }
  if (body && typeof body === 'object') {
    const extracted = readAttributeValue(body)
    if (extracted !== body) {
      return stringifyContextValue(extracted)
    }
    try {
      return JSON.stringify(body)
    } catch {
      return String(body)
    }
  }
  return ''
}

export const normalizeLogEntry = (entry) => {
  if (!entry || typeof entry !== 'object') return null
  const raw = entry.raw ?? entry
  const rawTime = entry?.timeUnixNano ?? entry?.observedTimeUnixNano
  const timestampISO = normalizeTimestamp(
    entry?.timeUnixNano ?? entry?.observedTimeUnixNano,
  )
  const level = normalizeLevel(
    entry?.severityNumber ?? entry?.severityText,
  )
  const message = normalizeMessage(entry?.body)
  const attributes = normalizeContext(entry?.attributes)
  const resourceAttributes = normalizeContext(entry?.resource?.attributes)
  const scopeName = entry?.scope?.name || ''
  const id = entry?.id || (rawTime ? `${rawTime}-${level}-${message}` : `${timestampISO}-${level}-${message}-${logSequence += 1}`)
  return {
    id,
    level,
    timestampISO,
    message,
    attributes,
    resourceAttributes,
    scopeName,
    raw,
  }
}

export const formatLogEntryForClipboard = (entry, { format = 'json' } = {}) => {
  if (!entry) return ''
  const payload = {
    id: entry.id,
    level: entry.level,
    timestampISO: entry.timestampISO,
    message: entry.message,
    attributes: entry.attributes,
    resourceAttributes: entry.resourceAttributes,
    scopeName: entry.scopeName,
    raw: entry.raw,
  }
  if (format === 'text') {
    const lines = []
    lines.push(`[${payload.level?.toUpperCase() || 'INFO'}] ${payload.message || ''}`)
    if (payload.timestampISO) {
      lines.push(`timestamp: ${payload.timestampISO}`)
    }
    if (payload.attributes && Object.keys(payload.attributes).length > 0) {
      lines.push('attributes:')
      Object.entries(payload.attributes).forEach(([key, value]) => {
        lines.push(`  ${key}: ${value}`)
      })
    }
    if (payload.resourceAttributes && Object.keys(payload.resourceAttributes).length > 0) {
      lines.push('resource:')
      Object.entries(payload.resourceAttributes).forEach(([key, value]) => {
        lines.push(`  ${key}: ${value}`)
      })
    }
    if (payload.scopeName) {
      lines.push(`scope: ${payload.scopeName}`)
    }
    return lines.join('\n')
  }
  return JSON.stringify(payload, null, 2)
}
