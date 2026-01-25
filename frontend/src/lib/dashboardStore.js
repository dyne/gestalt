import { get, writable } from 'svelte/store'
import { subscribe as subscribeAgentEvents } from './agentEventStore.js'
import { subscribe as subscribeConfigEvents } from './configEventStore.js'
import { subscribe as subscribeEvents } from './eventStore.js'
import { fetchAgentSkills, fetchAgents, fetchMetricsSummary } from './apiClient.js'
import { getErrorMessage } from './errorUtils.js'
import { notificationStore } from './notificationStore.js'
import { createLogStream } from './logStream.js'
import { createScipStore, initialScipStatus } from './scipStore.js'

const metricsRefreshIntervalMs = 60000
const maxLogEntries = 1000

export const createDashboardStore = () => {
  const scipStore = createScipStore()
  const state = writable({
    agents: [],
    agentsLoading: false,
    agentsError: '',
    agentSkills: {},
    agentSkillsLoading: false,
    agentSkillsError: '',
    logs: [],
    logsLoading: false,
    logsError: '',
    logLevelFilter: 'info',
    logsAutoRefresh: true,
    metricsSummary: null,
    metricsLoading: false,
    metricsError: '',
    metricsAutoRefresh: true,
    configExtractionCount: 0,
    configExtractionLast: '',
    gitOrigin: '',
    gitBranch: '',
    gitContext: 'not a git repo',
    scipStatus: { ...initialScipStatus },
  })

  let terminals = []
  let logsStream = null
  let logsMounted = false
  let logsStopTimer = null
  let logsStreaming = false
  let pendingLogStop = false
  let lastLogErrorMessage = ''
  let metricsRefreshTimer = null
  let metricsMounted = false
  let lastMetricsErrorMessage = ''
  let configExtractionTimer = null
  let agentEventsUnsubscribes = []
  let configEventsUnsubscribes = []
  let gitUnsubscribe = null
  let scipUnsubscribe = null
  let started = false

  const buildGitContext = (origin, branch) => {
    if (origin && branch) {
      return `${origin}/${branch}`
    }
    return origin || branch || 'not a git repo'
  }

  const resetConfigExtraction = () => {
    if (configExtractionTimer) {
      clearTimeout(configExtractionTimer)
      configExtractionTimer = null
    }
    state.update((current) => {
      if (!current.configExtractionCount && !current.configExtractionLast) {
        return current
      }
      return {
        ...current,
        configExtractionCount: 0,
        configExtractionLast: '',
      }
    })
  }

  const noteConfigExtraction = (payload) => {
    const path = payload?.path || ''
    state.update((current) => ({
      ...current,
      configExtractionCount: current.configExtractionCount + 1,
      configExtractionLast: path,
    }))
    if (configExtractionTimer) {
      clearTimeout(configExtractionTimer)
    }
    configExtractionTimer = setTimeout(() => {
      resetConfigExtraction()
    }, 5000)
  }

  const syncAgentRunning = (agentList, terminalList) => {
    if (!Array.isArray(agentList)) return []
    const terminalIds = new Set((terminalList || []).map((terminal) => terminal?.id).filter(Boolean))
    let changed = false
    const nextAgents = agentList.map((agent) => {
      const terminalId = agent.terminal_id || ''
      const running = Boolean(terminalId && terminalIds.has(terminalId))
      const nextTerminalId = running ? terminalId : ''
      if (agent.running === running && (agent.terminal_id || '') === nextTerminalId) {
        return agent
      }
      changed = true
      return {
        ...agent,
        running,
        terminal_id: nextTerminalId,
      }
    })
    return changed ? nextAgents : agentList
  }

  const setTerminals = (terminalList) => {
    terminals = Array.isArray(terminalList) ? terminalList : []
    state.update((current) => {
      const nextAgents = syncAgentRunning(current.agents, terminals)
      if (nextAgents === current.agents) return current
      return { ...current, agents: nextAgents }
    })
  }

  const setStatus = (status) => {
    if (!status) return
    const origin = status.git_origin || ''
    const branch = status.git_branch || ''
    state.update((current) => {
      const nextOrigin = current.gitOrigin || origin
      const nextBranch = current.gitBranch || branch
      const nextContext = buildGitContext(nextOrigin, nextBranch)
      if (
        nextOrigin === current.gitOrigin &&
        nextBranch === current.gitBranch &&
        nextContext === current.gitContext
      ) {
        return current
      }
      return {
        ...current,
        gitOrigin: nextOrigin,
        gitBranch: nextBranch,
        gitContext: nextContext,
      }
    })
  }

  const setGitBranch = (branch) => {
    const nextBranch = branch || ''
    if (!nextBranch) return
    state.update((current) => {
      if (current.gitBranch === nextBranch) return current
      const nextContext = buildGitContext(current.gitOrigin, nextBranch)
      return {
        ...current,
        gitBranch: nextBranch,
        gitContext: nextContext,
      }
    })
  }

  const loadAgentSkills = async (agentList) => {
    if (!agentList || agentList.length === 0) {
      state.update((current) => ({
        ...current,
        agentSkills: {},
        agentSkillsLoading: false,
        agentSkillsError: '',
      }))
      return
    }
    state.update((current) => ({ ...current, agentSkillsLoading: true, agentSkillsError: '' }))
    try {
      const entries = await Promise.all(
        agentList.map(async (agent) => {
          try {
            const data = await fetchAgentSkills(agent.id)
            return [agent.id, data.map((skill) => skill.name)]
          } catch {
            return [agent.id, []]
          }
        }),
      )
      state.update((current) => ({
        ...current,
        agentSkills: Object.fromEntries(entries),
        agentSkillsLoading: false,
      }))
    } catch (err) {
      state.update((current) => ({
        ...current,
        agentSkillsError: getErrorMessage(err, 'Failed to load agent skills.'),
        agentSkills: {},
        agentSkillsLoading: false,
      }))
    }
  }

  const loadAgents = async () => {
    state.update((current) => ({ ...current, agentsLoading: true, agentsError: '' }))
    try {
      const fetched = await fetchAgents()
      const nextAgents = syncAgentRunning(fetched, terminals)
      state.update((current) => ({
        ...current,
        agents: nextAgents,
        agentsLoading: false,
      }))
      await loadAgentSkills(nextAgents)
    } catch (err) {
      state.update((current) => ({
        ...current,
        agentsError: getErrorMessage(err, 'Failed to load agents.'),
        agentsLoading: false,
      }))
    }
  }

  const clearLogStopTimer = () => {
    if (logsStopTimer) {
      clearTimeout(logsStopTimer)
      logsStopTimer = null
    }
  }

  const stopLogStream = () => {
    clearLogStopTimer()
    pendingLogStop = false
    if (logsStream) {
      logsStream.stop()
    }
    logsStreaming = false
    state.update((current) => ({ ...current, logsLoading: false }))
  }

  const appendLogEntry = (entry) => {
    if (!entry) return
    state.update((current) => {
      const nextLogs = [...current.logs, entry]
      if (nextLogs.length > maxLogEntries) {
        nextLogs.splice(0, nextLogs.length - maxLogEntries)
      }
      return {
        ...current,
        logs: nextLogs,
        logsLoading: false,
        logsError: '',
      }
    })
  }

  const ensureLogStream = () => {
    if (logsStream) return logsStream
    logsStream = createLogStream({
      level: get(state).logLevelFilter,
      onEntry: appendLogEntry,
      onOpen: () => {
        state.update((current) => ({ ...current, logsLoading: false, logsError: '' }))
        if (pendingLogStop) {
          pendingLogStop = false
          clearLogStopTimer()
          logsStopTimer = setTimeout(() => {
            stopLogStream()
          }, 1500)
        }
        lastLogErrorMessage = ''
      },
      onError: (err) => {
        const message = getErrorMessage(err, 'Failed to load logs.')
        state.update((current) => ({
          ...current,
          logsError: message,
          logsLoading: false,
        }))
        if (message !== lastLogErrorMessage) {
          notificationStore.addNotification('error', message)
          lastLogErrorMessage = message
        }
      },
    })
    return logsStream
  }

  const loadLogs = async ({ reset = true } = {}) => {
    if (!logsMounted) return
    pendingLogStop = !get(state).logsAutoRefresh
    clearLogStopTimer()
    if (reset) {
      state.update((current) => ({ ...current, logs: [] }))
    }
    state.update((current) => ({ ...current, logsLoading: true, logsError: '' }))
    const stream = ensureLogStream()
    stream.setLevel(get(state).logLevelFilter)
    if (!logsStreaming) {
      logsStreaming = true
      stream.start()
      return
    }
    stream.restart()
  }

  const loadMetricsSummary = async () => {
    state.update((current) => ({ ...current, metricsLoading: true, metricsError: '' }))
    try {
      const summary = await fetchMetricsSummary()
      state.update((current) => ({
        ...current,
        metricsSummary: summary,
        metricsLoading: false,
        metricsError: '',
      }))
      lastMetricsErrorMessage = ''
    } catch (err) {
      const message = getErrorMessage(err, 'Failed to load metrics summary.')
      state.update((current) => ({
        ...current,
        metricsError: message,
        metricsLoading: false,
      }))
      if (message !== lastMetricsErrorMessage) {
        notificationStore.addNotification('error', message)
        lastMetricsErrorMessage = message
      }
    }
  }

  const resetMetricsRefresh = () => {
    if (metricsRefreshTimer) {
      clearInterval(metricsRefreshTimer)
      metricsRefreshTimer = null
    }
    if (!metricsMounted) return
    if (get(state).metricsAutoRefresh) {
      metricsRefreshTimer = setInterval(loadMetricsSummary, metricsRefreshIntervalMs)
    }
  }

  const setLogLevelFilter = (level) => {
    state.update((current) => ({ ...current, logLevelFilter: level }))
    loadLogs()
  }

  const setLogsAutoRefresh = (enabled) => {
    const nextValue = Boolean(enabled)
    state.update((current) => ({ ...current, logsAutoRefresh: nextValue }))
    if (!logsMounted) return
    if (nextValue) {
      loadLogs({ reset: false })
    } else {
      stopLogStream()
    }
  }

  const setMetricsAutoRefresh = (enabled) => {
    state.update((current) => ({ ...current, metricsAutoRefresh: Boolean(enabled) }))
    resetMetricsRefresh()
  }

  const reindexScip = async () => {
    try {
      await scipStore.reindex()
    } catch (err) {
      notificationStore.addNotification(
        'error',
        getErrorMessage(err, 'Failed to start SCIP indexing.')
      )
    }
  }

  const start = async () => {
    if (started) return
    started = true
    logsMounted = true
    metricsMounted = true
    scipUnsubscribe = scipStore.status.subscribe((nextStatus) => {
      state.update((current) => ({ ...current, scipStatus: nextStatus }))
    })
    void scipStore.start()
    agentEventsUnsubscribes = [
      subscribeAgentEvents('agent_started', () => loadAgents()),
      subscribeAgentEvents('agent_stopped', () => loadAgents()),
      subscribeAgentEvents('agent_error', () => loadAgents()),
    ]
    configEventsUnsubscribes = [
      subscribeConfigEvents('config_extracted', noteConfigExtraction),
      subscribeConfigEvents('config_conflict', (payload) => {
        const path = payload?.path || 'config file'
        notificationStore.addNotification('warning', `Config conflict: ${path}`)
      }),
      subscribeConfigEvents('config_validation_error', (payload) => {
        const detail = payload?.message || payload?.path || 'config file'
        notificationStore.addNotification('error', `Config validation failed: ${detail}`)
      }),
    ]
    gitUnsubscribe = subscribeEvents('git_branch_changed', (payload) => {
      if (!payload?.path) return
      setGitBranch(payload.path)
    })
    await loadAgents()
    await loadLogs()
    await loadMetricsSummary()
    resetMetricsRefresh()
  }

  const stop = () => {
    started = false
    logsMounted = false
    metricsMounted = false
    stopLogStream()
    if (metricsRefreshTimer) {
      clearInterval(metricsRefreshTimer)
      metricsRefreshTimer = null
    }
    resetConfigExtraction()
    if (agentEventsUnsubscribes.length > 0) {
      agentEventsUnsubscribes.forEach((unsubscribe) => unsubscribe())
      agentEventsUnsubscribes = []
    }
    if (configEventsUnsubscribes.length > 0) {
      configEventsUnsubscribes.forEach((unsubscribe) => unsubscribe())
      configEventsUnsubscribes = []
    }
    if (gitUnsubscribe) {
      gitUnsubscribe()
      gitUnsubscribe = null
    }
    if (scipUnsubscribe) {
      scipUnsubscribe()
      scipUnsubscribe = null
    }
    scipStore.stop()
  }

  return {
    subscribe: state.subscribe,
    setTerminals,
    setStatus,
    loadAgents,
    loadLogs,
    loadMetricsSummary,
    setLogLevelFilter,
    setLogsAutoRefresh,
    setMetricsAutoRefresh,
    reindexScip,
    start,
    stop,
  }
}
