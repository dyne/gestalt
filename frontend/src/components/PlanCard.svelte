<script>
  export let plan = {}

  const toCount = (value) => (Number.isFinite(value) ? value : 0)
  const toText = (value) => (value ? String(value) : '')
  const hasValue = (value) => Boolean(value && String(value).trim())

  const keywordClass = (keyword) => {
    const normalized = String(keyword || '').toLowerCase()
    if (normalized === 'done') return 'status--done'
    if (normalized === 'wip') return 'status--wip'
    if (normalized === 'todo') return 'status--todo'
    return 'status--neutral'
  }

  const priorityClass = (priority) => {
    const normalized = String(priority || '').toLowerCase()
    if (normalized === 'a') return 'priority--a'
    if (normalized === 'b') return 'priority--b'
    if (normalized === 'c') return 'priority--c'
    return 'priority--neutral'
  }

  $: title = plan?.title || plan?.filename || 'Untitled plan'
  $: subtitle = toText(plan?.subtitle)
  $: date = toText(plan?.date)
  $: l1Count = toCount(plan?.l1_count)
  $: l2Count = toCount(plan?.l2_count)
  $: priorityA = toCount(plan?.priority_a)
  $: priorityB = toCount(plan?.priority_b)
  $: priorityC = toCount(plan?.priority_c)
  $: headings = Array.isArray(plan?.headings) ? plan.headings : []
</script>

<details class="plan-card">
  <summary class="plan-summary">
    <div class="plan-summary__head">
      <div class="plan-summary__title">
        <h2>{title}</h2>
        {#if hasValue(subtitle)}
          <p class="plan-subtitle">{subtitle}</p>
        {/if}
      </div>
      {#if hasValue(date)}
        <span class="plan-date">{date}</span>
      {/if}
    </div>
    <div class="plan-summary__stats">
      <span class="stat">L1 <strong>{l1Count}</strong></span>
      <span class="stat">L2 <strong>{l2Count}</strong></span>
      {#if priorityA > 0}
        <span class="priority priority--a">[#A] {priorityA}</span>
      {/if}
      {#if priorityB > 0}
        <span class="priority priority--b">[#B] {priorityB}</span>
      {/if}
      {#if priorityC > 0}
        <span class="priority priority--c">[#C] {priorityC}</span>
      {/if}
    </div>
  </summary>
  <div class="plan-content">
    {#each headings as l1 (l1.text)}
      <details class="heading heading--l1">
        <summary class="heading-summary heading-summary--l1">
          {#if hasValue(l1.keyword)}
            <span class={`status ${keywordClass(l1.keyword)}`}>{l1.keyword}</span>
          {/if}
          {#if hasValue(l1.priority)}
            <span class={`priority ${priorityClass(l1.priority)}`}>[#${l1.priority}]</span>
          {/if}
          <span class="heading-text">{l1.text}</span>
        </summary>
        <div class="heading-body">
          {#if hasValue(l1.body)}
            <pre>{l1.body}</pre>
          {/if}
          {#each l1.children || [] as l2 (l2.text)}
            <details class="heading heading--l2">
              <summary class="heading-summary heading-summary--l2">
                {#if hasValue(l2.keyword)}
                  <span class={`status ${keywordClass(l2.keyword)}`}>{l2.keyword}</span>
                {/if}
                {#if hasValue(l2.priority)}
                  <span class={`priority ${priorityClass(l2.priority)}`}>[#${l2.priority}]</span>
                {/if}
                <span class="heading-text">{l2.text}</span>
              </summary>
              <div class="heading-body">
                {#if hasValue(l2.body)}
                  <pre>{l2.body}</pre>
                {/if}
              </div>
            </details>
          {/each}
        </div>
      </details>
    {/each}
  </div>
</details>

<style>
  .plan-card {
    border: 1px solid rgba(var(--color-text-rgb), 0.12);
    border-radius: 16px;
    background: rgba(var(--color-surface-rgb), 0.8);
    overflow: hidden;
  }

  .plan-summary {
    list-style: none;
    cursor: pointer;
    padding: 1.25rem 1.5rem;
    display: grid;
    gap: 0.75rem;
  }

  .plan-summary::-webkit-details-marker {
    display: none;
  }

  .plan-summary__head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1.5rem;
  }

  .plan-summary__title h2 {
    margin: 0;
    font-size: 1.4rem;
    color: var(--color-text);
  }

  .plan-subtitle {
    margin: 0.35rem 0 0;
    color: var(--color-text-muted);
    font-size: 0.9rem;
  }

  .plan-date {
    background: var(--color-contrast-bg);
    color: var(--color-contrast-text);
    padding: 0.35rem 0.7rem;
    border-radius: 999px;
    font-size: 0.75rem;
    font-weight: 600;
    white-space: nowrap;
  }

  .plan-summary__stats {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem 1rem;
    font-size: 0.75rem;
    color: var(--color-text-subtle);
    align-items: center;
  }

  .stat strong {
    color: var(--color-text);
    margin-left: 0.2rem;
  }

  .priority {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.2rem 0.6rem;
    border-radius: 999px;
    font-weight: 600;
    font-size: 0.7rem;
    background: rgba(var(--color-text-rgb), 0.08);
    color: var(--color-text);
  }

  .priority--a {
    background: rgba(var(--color-danger-rgb), 0.18);
    color: var(--color-danger);
  }

  .priority--b {
    background: rgba(var(--color-warning-rgb), 0.18);
    color: var(--color-warning);
  }

  .priority--c {
    background: rgba(var(--color-info-rgb), 0.18);
    color: var(--color-info);
  }

  .plan-content {
    padding: 0 1.5rem 1.5rem;
    display: grid;
    gap: 0.8rem;
  }

  .heading {
    border: 1px solid rgba(var(--color-text-rgb), 0.08);
    border-radius: 12px;
    background: rgba(var(--color-surface-rgb), 0.6);
    padding: 0.6rem 0.9rem;
  }

  .heading-summary {
    list-style: none;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  .heading-summary::-webkit-details-marker {
    display: none;
  }

  .heading-summary--l2 {
    padding-left: 0.4rem;
  }

  .heading-text {
    color: var(--color-text);
    font-weight: 600;
  }

  .heading-body {
    margin-top: 0.6rem;
    display: grid;
    gap: 0.6rem;
  }

  pre {
    margin: 0;
    padding: 0.75rem 0.9rem;
    background: rgba(var(--color-text-rgb), 0.06);
    border-radius: 10px;
    color: var(--color-text);
    font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
    font-size: 0.8rem;
    white-space: pre-wrap;
  }

  .status {
    text-transform: uppercase;
    font-size: 0.65rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    padding: 0.15rem 0.45rem;
    border-radius: 999px;
    background: rgba(var(--color-text-rgb), 0.12);
    color: var(--color-text);
  }

  .status--todo {
    background: rgba(var(--color-warning-rgb), 0.18);
    color: var(--color-warning);
  }

  .status--wip {
    background: rgba(var(--color-info-rgb), 0.18);
    color: var(--color-info);
  }

  .status--done {
    background: rgba(var(--color-success-rgb), 0.18);
    color: var(--color-success);
  }

  @media (max-width: 720px) {
    .plan-summary__head {
      flex-direction: column;
      align-items: flex-start;
    }

    .plan-date {
      align-self: flex-start;
    }
  }
</style>
