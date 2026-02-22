import { get, writable } from 'svelte/store'
import { subscribe as subscribeAgentEvents } from './agentEventStore.js'
import { subscribe as subscribeConfigEvents } from './configEventStore.js'
import { subscribe as subscribeEvents } from './eventStore.js'
import { fetchAgents, fetchGitLog } from './apiClient.js'
import { getErrorMessage } from './errorUtils.js'
import { notificationStore } from './notificationStore.js'
import { createLogStream } from './logStream.js'
import { normalizeLogEntry } from './logEntry.js'

const gitLogRefreshIntervalMs = 60000
const maxLogEntries = 1000
const logBatchSize = 200
const logFlushDelayMs = 16

export const createDashboardStore = () => {
  const state = writable({
    agents: [],
    agentsLoading: false,
    agentsError: '',
    logs: [],
    logsLoading: false,
    logsError: '',
    logLevelFilter: 'info',
    logsAutoRefresh: true,
    gitLog: { branch: '', commits: [] },
    gitLogLoading: false,
    gitLogError: '',
    gitLogAutoRefresh: true,
    configExtractionCount: 0,
    configExtractionLast: '',
    gitOrigin: '',
    gitBranch: '',
    gitContext: 'not a git repo',
  })

  let terminals = []
  let logsStream = null
  let logsMounted = false
  let logsStopTimer = null
  let logsStreaming = false
  let pendingLogStop = false
  let logBuffer = []
  let logFlushTimer = null
  let lastLogErrorMessage = ''
  let gitLogRefreshTimer = null
  let gitLogMounted = false
  let lastGitLogErrorMessage = ''
  let configExtractionTimer = null
  let agentEventsUnsubscribes = []
  let configEventsUnsubscribes = []
  let gitUnsubscribe = null
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
      const terminalId = agent.session_id || ''
      const running = Boolean(terminalId && terminalIds.has(terminalId))
      const nextTerminalId = running ? terminalId : ''
      if (agent.running === running && (agent.session_id || '') === nextTerminalId) {
        return agent
      }
      changed = true
      return {
        ...agent,
        running,
        session_id: nextTerminalId,
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

  const clearLogFlush = () => {
    if (logFlushTimer) {
      clearTimeout(logFlushTimer)
      logFlushTimer = null
    }
  }

  const flushLogBuffer = () => {
    if (logBuffer.length === 0) {
      clearLogFlush()
      return
    }
    const batch = logBuffer.slice(0, logBatchSize)
    logBuffer = logBuffer.slice(batch.length)
    state.update((current) => {
      const nextLogs = [...current.logs, ...batch]
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
    clearLogFlush()
    if (logBuffer.length > 0) {
      scheduleLogFlush()
    }
  }

  const scheduleLogFlush = () => {
    if (logFlushTimer) return
    logFlushTimer = setTimeout(() => {
      flushLogBuffer()
    }, logFlushDelayMs)
  }

  const stopLogStream = () => {
    clearLogStopTimer()
    pendingLogStop = false
    if (logsStream) {
      logsStream.stop()
    }
    logsStreaming = false
    logBuffer = []
    clearLogFlush()
    state.update((current) => ({ ...current, logsLoading: false }))
  }

  const appendLogEntry = (entry) => {
    if (!entry) return
    const normalized = normalizeLogEntry(entry)
    if (!normalized) return
    logBuffer.push(normalized)
    scheduleLogFlush()
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
      logBuffer = []
      clearLogFlush()
    }
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

  const loadGitLog = async () => {
    state.update((current) => ({ ...current, gitLogLoading: true, gitLogError: '' }))
    try {
      const gitLog = await fetchGitLog({ limit: 20 })
      state.update((current) => ({
        ...current,
        gitLog,
        gitLogLoading: false,
        gitLogError: '',
      }))
      lastGitLogErrorMessage = ''
    } catch (err) {
      const message = getErrorMessage(err, 'Failed to load git log.')
      state.update((current) => ({
        ...current,
        gitLogError: message,
        gitLogLoading: false,
      }))
      if (message !== lastGitLogErrorMessage) {
        notificationStore.addNotification('error', message)
        lastGitLogErrorMessage = message
      }
    }
  }

  const resetGitLogRefresh = () => {
    if (gitLogRefreshTimer) {
      clearInterval(gitLogRefreshTimer)
      gitLogRefreshTimer = null
    }
    if (!gitLogMounted) return
    if (get(state).gitLogAutoRefresh) {
      gitLogRefreshTimer = setInterval(loadGitLog, gitLogRefreshIntervalMs)
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

  const setGitLogAutoRefresh = (enabled) => {
    state.update((current) => ({ ...current, gitLogAutoRefresh: Boolean(enabled) }))
    resetGitLogRefresh()
  }

  const start = async () => {
    if (started) return
    started = true
    logsMounted = true
    gitLogMounted = true
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
    gitUnsubscribe = subscribeEvents('git-branch', (payload) => {
      if (!payload?.path) return
      setGitBranch(payload.path)
      loadGitLog()
    })
    await loadAgents()
    await loadLogs()
    await loadGitLog()
    resetGitLogRefresh()
  }

  const stop = () => {
    started = false
    logsMounted = false
    gitLogMounted = false
    stopLogStream()
    if (gitLogRefreshTimer) {
      clearInterval(gitLogRefreshTimer)
      gitLogRefreshTimer = null
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
  }

  return {
    subscribe: state.subscribe,
    setTerminals,
    setStatus,
    loadAgents,
    loadLogs,
    loadGitLog,
    setLogLevelFilter,
    setLogsAutoRefresh,
    setGitLogAutoRefresh,
    start,
    stop,
  }
}
