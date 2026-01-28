const normalize = (value) => (value || '').toString().toLowerCase()

const buildToken = (raw) => {
  const trimmed = raw.trim()
  const separatorIndex = trimmed.indexOf(':')
  if (separatorIndex === -1) {
    return {
      raw: trimmed,
      type: 'text',
      value: trimmed,
    }
  }
  const key = trimmed.slice(0, separatorIndex)
  const value = trimmed.slice(separatorIndex + 1)
  if (key === 'event_type') {
    return {
      raw: trimmed,
      type: 'filter',
      target: 'event_type',
      key: 'event_type',
      value,
      exists: value === '',
    }
  }
  if (key.startsWith('where.')) {
    return {
      raw: trimmed,
      type: 'filter',
      target: 'where',
      key: key.slice('where.'.length),
      value,
      exists: value === '',
    }
  }
  return {
    raw: trimmed,
    type: 'filter',
    target: 'where',
    key,
    value,
    exists: value === '',
  }
}

export const parseEventFilterQuery = (query = '') => {
  const tokens = query
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .map(buildToken)
  const terms = tokens.filter((token) => token.type === 'text').map((token) => token.value)
  const filters = tokens.filter((token) => token.type === 'filter')
  return { query, tokens, terms, filters }
}

export const matchesEventTrigger = (trigger, parsed) => {
  if (!parsed) {
    return true
  }
  const label = normalize(trigger?.label)
  const eventType = normalize(trigger?.event_type)
  for (const term of parsed.terms || []) {
    const needle = normalize(term)
    if (!needle) continue
    if (!label.includes(needle) && !eventType.includes(needle)) {
      return false
    }
  }
  const where = trigger?.where || {}
  for (const filter of parsed.filters || []) {
    if (filter.target === 'event_type') {
      if (filter.exists) {
        if (!eventType) return false
      } else if (eventType !== normalize(filter.value)) {
        return false
      }
      continue
    }
    const key = filter.key
    if (!key) return false
    const rawValue = where[key]
    if (filter.exists) {
      if (!Object.prototype.hasOwnProperty.call(where, key)) {
        return false
      }
    } else if (normalize(rawValue) !== normalize(filter.value)) {
      return false
    }
  }
  return true
}
