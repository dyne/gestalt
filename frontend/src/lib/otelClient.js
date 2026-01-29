import { apiFetch } from './api.js'

const toQueryValue = (value) => {
  if (value instanceof Date) {
    return value.toISOString()
  }
  return value
}

const buildQuery = (params) => {
  const search = new URLSearchParams()
  Object.entries(params || {}).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return
    search.set(key, String(toQueryValue(value)))
  })
  const query = search.toString()
  return query ? `?${query}` : ''
}

export const fetchOtelTraces = async ({
  traceId,
  spanName,
  since,
  until,
  limit,
  query,
} = {}) => {
  const response = await apiFetch(
    `/api/otel/traces${buildQuery({
      trace_id: traceId,
      span_name: spanName,
      since,
      until,
      limit,
      query,
    })}`,
  )
  return response.json()
}

export const fetchOtelMetrics = async ({ name, since, until, step, query } = {}) => {
  const response = await apiFetch(
    `/api/otel/metrics${buildQuery({ name, since, until, step, query })}`,
  )
  return response.json()
}
