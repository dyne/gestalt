<script>
  import { onMount } from 'svelte'
  import { apiFetch } from '../lib/api.js'

  export let terminals = []
  export let status = null
  export let loading = false
  export let error = ''
  export let onCreate = () => {}

  let creating = false
  let localError = ''
  let agents = []
  let agentsLoading = false
  let agentsError = ''
  let skills = []
  let skillsLoading = false
  let skillsError = ''
  let agentSkills = {}
  let agentSkillsLoading = false
  let agentSkillsError = ''

  const createTerminal = async (agentId = '') => {
    creating = true
    localError = ''
    try {
      await onCreate(agentId)
    } catch (err) {
      localError = err?.message || 'Failed to create terminal.'
    } finally {
      creating = false
    }
  }

  const loadAgents = async () => {
    agentsLoading = true
    agentsError = ''
    try {
      const response = await apiFetch('/api/agents')
      agents = await response.json()
      await loadAgentSkills(agents)
    } catch (err) {
      agentsError = err?.message || 'Failed to load agents.'
    } finally {
      agentsLoading = false
    }
  }

  const loadSkills = async () => {
    skillsLoading = true
    skillsError = ''
    try {
      const response = await apiFetch('/api/skills')
      skills = await response.json()
    } catch (err) {
      skillsError = err?.message || 'Failed to load skills.'
    } finally {
      skillsLoading = false
    }
  }

  const loadAgentSkills = async (agentList) => {
    if (!agentList || agentList.length === 0) {
      agentSkills = {}
      return
    }
    agentSkillsLoading = true
    agentSkillsError = ''
    try {
      const entries = await Promise.all(
        agentList.map(async (agent) => {
          try {
            const response = await apiFetch(`/api/skills?agent=${encodeURIComponent(agent.id)}`)
            const data = await response.json()
            return [agent.id, data.map((skill) => skill.name)]
          } catch {
            return [agent.id, []]
          }
        }),
      )
      agentSkills = Object.fromEntries(entries)
    } catch (err) {
      agentSkillsError = err?.message || 'Failed to load agent skills.'
      agentSkills = {}
    } finally {
      agentSkillsLoading = false
    }
  }

  const agentNamesForSkill = (skillName) => {
    if (!skillName || agents.length === 0) return []
    return agents
      .filter((agent) => (agentSkills[agent.id] || []).includes(skillName))
      .map((agent) => agent.name)
  }

  const formatTime = (value) => {
    if (!value) return '—'
    const parsed = new Date(value)
    if (Number.isNaN(parsed.getTime())) return '—'
    return parsed.toLocaleString()
  }

  onMount(() => {
    loadAgents()
    loadSkills()
  })
</script>

<section class="dashboard">
  <header class="dashboard__header">
    <div>
      <p class="eyebrow">Dyne.org presents...</p>
      <h1>Gestalt</h1>
    </div>
  </header>

  <section class="dashboard__status">
    <div class="status-card">
      <span class="label">Active terminals</span>
      <strong class="value">{status?.terminal_count ?? '—'}</strong>
    </div>
    <div class="status-card">
      <span class="label">Server time</span>
      <strong class="value">{formatTime(status?.server_time)}</strong>
    </div>
    <div class="status-card">
      <span class="label">Skills available</span>
      <strong class="value">{skillsLoading ? '…' : skills.length}</strong>
    </div>
  </section>

  <section class="dashboard__agents">
    <div class="list-header">
      <h2>Agent terminals</h2>
      <p class="subtle">{agents.length} profile(s)</p>
    </div>

    {#if agentsLoading}
      <p class="muted">Loading agents…</p>
    {:else if agentsError}
      <p class="error">{agentsError}</p>
    {:else if agents.length === 0}
      <p class="muted">No agent profiles found.</p>
    {:else}
      <div class="agent-grid">
        {#each agents as agent}
          <button
            class="agent-button"
            on:click={() => createTerminal(agent.id)}
            disabled={creating || loading}
          >
            <span class="agent-name">{agent.name}</span>
            {#if agentSkillsLoading}
              <span class="agent-skills muted">Loading skills…</span>
            {:else if agentSkillsError}
              <span class="agent-skills error">Skills unavailable</span>
            {:else if (agentSkills[agent.id] || []).length === 0}
              <span class="agent-skills muted">No skills assigned</span>
            {:else}
              <span class="agent-skills">{agentSkills[agent.id].join(', ')}</span>
            {/if}
          </button>
        {/each}
      </div>
    {/if}
  </section>

  <section class="dashboard__skills">
    <div class="list-header">
      <h2>Skills</h2>
      <p class="subtle">{skills.length} available</p>
    </div>

    {#if skillsLoading}
      <p class="muted">Loading skills…</p>
    {:else if skillsError}
      <p class="error">{skillsError}</p>
    {:else if skills.length === 0}
      <p class="muted">No skills found.</p>
    {:else}
      <div class="skill-grid">
        {#each skills as skill}
          <article class="skill-card">
            <div class="skill-card__header">
              <h3>{skill.name}</h3>
              {#if skill.license}
                <span class="chip chip--muted">{skill.license}</span>
              {/if}
            </div>
            <p class="skill-desc">{skill.description}</p>
            <div class="skill-meta">
              <span class="meta-label">Agents</span>
              <span class="meta-value">
                {agentNamesForSkill(skill.name).join(', ') || 'None'}
              </span>
            </div>
            <div class="skill-tags">
              {#if skill.has_scripts}
                <span class="tag">scripts</span>
              {/if}
              {#if skill.has_references}
                <span class="tag">references</span>
              {/if}
              {#if skill.has_assets}
                <span class="tag">assets</span>
              {/if}
            </div>
          </article>
        {/each}
      </div>
    {/if}
  </section>

  <section class="dashboard__list">
    <div class="list-header">
      <h2>Live terminals</h2>
      <p class="subtle">{terminals.length} session(s)</p>
    </div>

    {#if loading}
      <p class="muted">Loading sessions…</p>
    {:else if error || localError}
      <p class="error">{error || localError}</p>
    {:else if terminals.length === 0}
      <p class="muted">No terminals running. Create one to get started.</p>
    {:else}
      <ul class="terminal-list">
        {#each terminals as terminal}
          <li class="terminal-row">
            <div>
              <p class="terminal-title">
                {terminal.title || 'Untitled terminal'}
              </p>
              <p class="terminal-meta">
                ID {terminal.id} · {terminal.role || 'general'} · {formatTime(terminal.created_at)}
              </p>
            </div>
            <span class="chip">{terminal.status}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</section>

<style>
  :global(body) {
    background: radial-gradient(circle at top left, #f6f0e4, #f1f4f6 60%, #f6f6f1);
  }

  .dashboard {
    padding: 2.5rem clamp(1.5rem, 4vw, 3.5rem) 3.5rem;
    display: flex;
    flex-direction: column;
    gap: 2.5rem;
  }

  .dashboard__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1.5rem;
  }

  .eyebrow {
    text-transform: uppercase;
    letter-spacing: 0.24em;
    font-size: 0.7rem;
    color: #6d6a61;
    margin: 0 0 0.6rem;
  }

  h1 {
    margin: 0;
    font-size: clamp(2rem, 3.5vw, 3rem);
    font-weight: 600;
    color: #161616;
  }

  .cta {
    border: none;
    border-radius: 999px;
    padding: 0.85rem 1.6rem;
    font-size: 0.95rem;
    font-weight: 600;
    background: #151515;
    color: #f6f3ed;
    cursor: pointer;
    transition: transform 160ms ease, box-shadow 160ms ease, opacity 160ms ease;
    box-shadow: 0 10px 30px rgba(10, 10, 10, 0.2);
  }

  .cta:disabled {
    cursor: not-allowed;
    opacity: 0.6;
    transform: none;
    box-shadow: none;
  }

  .cta:not(:disabled):hover {
    transform: translateY(-2px);
  }

  .dashboard__status {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 1rem;
  }

  .status-card {
    padding: 1.5rem;
    border-radius: 20px;
    background: #ffffffd9;
    border: 1px solid rgba(20, 20, 20, 0.08);
    box-shadow: 0 20px 50px rgba(20, 20, 20, 0.08);
  }

  .label {
    display: block;
    font-size: 0.8rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: #6c6860;
  }

  .value {
    display: block;
    margin-top: 0.35rem;
    font-size: 1.6rem;
    color: #141414;
  }

  .dashboard__list {
    padding: 1.5rem;
    border-radius: 24px;
    background: #ffffff;
    border: 1px solid rgba(20, 20, 20, 0.08);
  }

  .dashboard__agents {
    padding: 1.5rem;
    border-radius: 24px;
    background: #fff6ec;
    border: 1px solid rgba(20, 20, 20, 0.08);
  }

  .dashboard__skills {
    padding: 1.5rem;
    border-radius: 24px;
    background: #eef3f2;
    border: 1px solid rgba(20, 20, 20, 0.08);
  }

  .agent-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: 0.75rem;
  }

  .agent-button {
    border: 1px solid rgba(20, 20, 20, 0.2);
    border-radius: 14px;
    padding: 0.75rem 1rem;
    background: #ffffff;
    font-weight: 600;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.35rem;
    cursor: pointer;
    transition: transform 160ms ease, box-shadow 160ms ease, opacity 160ms ease;
  }

  .agent-name {
    font-size: 0.95rem;
  }

  .agent-skills {
    font-size: 0.8rem;
    font-weight: 500;
    color: #5f5b54;
    text-align: left;
  }

  .skill-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 0.9rem;
  }

  .skill-card {
    background: #ffffff;
    border-radius: 18px;
    border: 1px solid rgba(20, 20, 20, 0.08);
    padding: 1rem 1.1rem 1.1rem;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .skill-card__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .skill-card__header h3 {
    margin: 0;
    font-size: 1rem;
  }

  .skill-desc {
    margin: 0;
    font-size: 0.9rem;
    color: #5c5851;
  }

  .skill-meta {
    display: flex;
    gap: 0.4rem;
    font-size: 0.8rem;
    color: #5c5851;
  }

  .meta-label {
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-size: 0.7rem;
    color: #6c6860;
  }

  .meta-value {
    font-weight: 600;
  }

  .skill-tags {
    display: flex;
    gap: 0.4rem;
    flex-wrap: wrap;
  }

  .tag {
    padding: 0.2rem 0.5rem;
    border-radius: 999px;
    background: #f0ede7;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: #5e5a53;
  }

  .agent-button:disabled {
    cursor: not-allowed;
    opacity: 0.6;
    transform: none;
    box-shadow: none;
  }

  .agent-button:not(:disabled):hover {
    transform: translateY(-2px);
    box-shadow: 0 12px 20px rgba(10, 10, 10, 0.12);
  }

  .list-header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 1rem;
  }

  .subtle {
    margin: 0;
    font-size: 0.9rem;
    color: #7a776f;
  }

  .terminal-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .terminal-row {
    padding: 1rem 1.2rem;
    border-radius: 16px;
    border: 1px solid rgba(20, 20, 20, 0.06);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    background: #f9f7f2;
  }

  .terminal-title {
    margin: 0 0 0.3rem;
    font-size: 1rem;
    font-weight: 600;
  }

  .terminal-meta {
    margin: 0;
    font-size: 0.85rem;
    color: #6f6b62;
  }

  .chip {
    padding: 0.3rem 0.75rem;
    border-radius: 999px;
    background: #151515;
    color: #f4f1eb;
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.1em;
  }

  .chip--muted {
    background: #f0ede7;
    color: #5b5851;
  }

  .muted {
    color: #7d7a73;
    margin: 0.5rem 0 0;
  }

  .error {
    color: #b04a39;
    margin: 0.5rem 0 0;
  }

  @media (max-width: 720px) {
    .dashboard__header {
      flex-direction: column;
      align-items: flex-start;
    }

    .list-header {
      flex-direction: column;
      align-items: flex-start;
      gap: 0.4rem;
    }

    .terminal-row {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
