import { formatRelativeTime } from './timeUtils.js'

export const formatWorkflowTime = (value) => {
  return formatRelativeTime(value) || '-'
}

export const workflowStatusLabel = (status = '') => {
  switch (status) {
    case 'running':
      return 'Running'
    case 'paused':
      return 'Paused'
    case 'stopped':
      return 'Stopped'
    default:
      return 'Unknown'
  }
}

export const workflowStatusClass = (status = '') => {
  switch (status) {
    case 'running':
      return 'running'
    case 'paused':
      return 'paused'
    case 'stopped':
      return 'stopped'
    default:
      return 'unknown'
  }
}

export const workflowTaskSummary = (workflow) => {
  const l1 = workflow?.current_l1 || 'No L1 set'
  const l2 = workflow?.current_l2 || 'No L2 set'
  return `${l1} / ${l2}`
}

export const timestampValue = (value) => {
  const parsed = new Date(value)
  const time = parsed.getTime()
  return Number.isNaN(time) ? 0 : time
}

export const formatDuration = (milliseconds) => {
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return '-'
  const totalSeconds = Math.floor(milliseconds / 1000)
  const seconds = totalSeconds % 60
  const totalMinutes = Math.floor(totalSeconds / 60)
  const minutes = totalMinutes % 60
  const hours = Math.floor(totalMinutes / 60)
  const parts = []
  if (hours > 0) parts.push(`${hours}h`)
  if (minutes > 0 || hours > 0) parts.push(`${minutes}m`)
  parts.push(`${seconds}s`)
  return parts.join(' ')
}

export const truncateText = (text, maxLength = 160) => {
  if (!text) return ''
  if (text.length <= maxLength) return text
  return `${text.slice(0, maxLength)}...`
}

export const buildTemporalUrl = (workflowId, runId, baseUrl) => {
  if (!workflowId) return ''
  const base = (baseUrl || '').trim().replace(/\/+$/, '')
  if (!base) return ''
  try {
    new URL(base)
  } catch {
    return ''
  }
  const namespace = 'default'
  const encodedId = encodeURIComponent(workflowId)
  const encodedRun = runId ? `/${encodeURIComponent(runId)}` : ''
  return `${base}/namespaces/${namespace}/workflows/${encodedId}${encodedRun}`
}
