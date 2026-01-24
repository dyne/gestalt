import { get, writable } from 'svelte/store'
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
  })

  let terminals = []
  let logsRefreshTimer = null
  let logsMounted = false
  let lastLogErrorMessage = ''

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
    logsMounted = true
    await loadAgents()
    await loadLogs()
    resetLogRefresh()
  }

  const stop = () => {
    logsMounted = false
    if (logsRefreshTimer) {
      clearInterval(logsRefreshTimer)
      logsRefreshTimer = null
    }
  }

  return {
    subscribe: state.subscribe,
    setTerminals,
    loadAgents,
    loadLogs,
    setLogLevelFilter,
    setLogsAutoRefresh,
    start,
    stop,
  }
}
