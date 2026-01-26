const MINUTE_MS = 60 * 1000
const HOUR_MS = 60 * MINUTE_MS
const DAY_MS = 24 * HOUR_MS

let serverTimeOffsetMs = 0

const shortDateFormatter = new Intl.DateTimeFormat('en-US', {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function formatUnit(value, unit) {
  return `${value} ${unit}${value === 1 ? '' : 's'} ago`
}

export function setServerTimeOffset(serverTime) {
  if (!serverTime) {
    serverTimeOffsetMs = 0
    return
  }
  const date = serverTime instanceof Date ? serverTime : new Date(serverTime)
  const time = date.getTime()
  if (Number.isNaN(time)) {
    return
  }
  serverTimeOffsetMs = time - Date.now()
}

export function resetServerTimeOffset() {
  serverTimeOffsetMs = 0
}

export function formatRelativeTime(timestamp) {
  if (!timestamp) {
    return ''
  }

  const date = timestamp instanceof Date ? timestamp : new Date(timestamp)
  if (Number.isNaN(date.getTime())) {
    return ''
  }

  const diffMs = Date.now() + serverTimeOffsetMs - date.getTime()
  if (diffMs < MINUTE_MS) {
    return 'just now'
  }

  if (diffMs < HOUR_MS) {
    return formatUnit(Math.floor(diffMs / MINUTE_MS), 'minute')
  }

  if (diffMs < DAY_MS) {
    return formatUnit(Math.floor(diffMs / HOUR_MS), 'hour')
  }

  const days = Math.floor(diffMs / DAY_MS)
  if (days < 7) {
    return formatUnit(days, 'day')
  }

  return shortDateFormatter.format(date)
}
