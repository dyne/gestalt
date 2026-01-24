import { get, writable } from 'svelte/store'
import { subscribe as subscribeAgentEvents } from './agentEventStore.js'
import { subscribe as subscribeConfigEvents } from './configEventStore.js'
import { subscribe as subscribeEvents } from './eventStore.js'
import { fetchAgentSkills, fetchAgents, fetchLogs } from './apiClient.js'
import { getErrorMessage } from './errorUtils.js'
import { notificationStore } from './notificationStore.js'

const refreshIntervalMs = 5000

export const createDashboardStore = () => {
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
    configExtractionCount: 0,
    configExtractionLast: '',
    gitOrigin: '',
    gitBranch: '',
    gitContext: 'not a git repo',
  })

  let terminals = []
  let logsRefreshTimer = null
  let logsMounted = false
  let lastLogErrorMessage = ''
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

  const loadLogs = async () => {
    state.update((current) => ({ ...current, logsLoading: true, logsError: '' }))
    try {
      const { logLevelFilter } = get(state)
      const nextLogs = await fetchLogs({ level: logLevelFilter })
      state.update((current) => ({
        ...current,
        logs: nextLogs,
        logsLoading: false,
      }))
      lastLogErrorMessage = ''
    } catch (err) {
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
    }
  }

  const resetLogRefresh = () => {
    if (logsRefreshTimer) {
      clearInterval(logsRefreshTimer)
      logsRefreshTimer = null
    }
    if (!logsMounted) return
    if (get(state).logsAutoRefresh) {
      logsRefreshTimer = setInterval(loadLogs, refreshIntervalMs)
    }
  }

  const setLogLevelFilter = (level) => {
    state.update((current) => ({ ...current, logLevelFilter: level }))
    loadLogs()
  }

  const setLogsAutoRefresh = (enabled) => {
    state.update((current) => ({ ...current, logsAutoRefresh: Boolean(enabled) }))
    resetLogRefresh()
  }

  const start = async () => {
    if (started) return
    started = true
    logsMounted = true
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
    resetLogRefresh()
  }

  const stop = () => {
    started = false
    logsMounted = false
    if (logsRefreshTimer) {
      clearInterval(logsRefreshTimer)
      logsRefreshTimer = null
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
    setLogLevelFilter,
    setLogsAutoRefresh,
    start,
    stop,
  }
}
