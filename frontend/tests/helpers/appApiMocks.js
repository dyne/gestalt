export const defaultMetricsSummary = {
  updated_at: '',
  top_endpoints: [],
  slowest_endpoints: [],
  top_agents: [],
  error_rates: [],
}

export const createAppApiMocks = (apiFetch, overrides = {}) => {
  return (url) => {
    if (url === '/api/status') {
      return Promise.resolve({
        json: () => Promise.resolve({ terminal_count: 0, ...(overrides.status ?? {}) }),
      })
    }
    if (url === '/api/terminals') {
      return Promise.resolve({
        json: () => Promise.resolve(overrides.terminals ?? []),
      })
    }
    if (url === '/api/agents') {
      return Promise.resolve({
        json: () => Promise.resolve(overrides.agents ?? []),
      })
    }
    if (url.startsWith('/api/skills')) {
      return Promise.resolve({
        json: () => Promise.resolve(overrides.skills ?? []),
      })
    }
    if (url === '/api/metrics/summary') {
      return Promise.resolve({
        json: () => Promise.resolve(overrides.metricsSummary ?? defaultMetricsSummary),
      })
    }
    if (url === '/api/otel/logs') {
      return Promise.resolve({
        json: () => Promise.resolve(overrides.otelLogs ?? { ok: true }),
      })
    }
    return Promise.resolve({ json: () => Promise.resolve(overrides.fallback ?? {}) })
  }
}

export const createLogStreamStub = (options = {}) => {
  return {
    start: options.start ?? (() => {}),
    stop: options.stop ?? (() => {}),
    restart: options.restart ?? (() => {}),
    setLevel: options.setLevel ?? (() => {}),
  }
}
