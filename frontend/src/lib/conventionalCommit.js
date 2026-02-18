const conventionalPattern = /^(?<type>[a-zA-Z0-9-]+)(\((?<scope>[^)]+)\))?(?<breaking>!)?:\s*(?<title>.+)$/

const typeClassMap = {
  feat: 'conventional-badge--feat',
  fix: 'conventional-badge--fix',
  docs: 'conventional-badge--docs',
  refactor: 'conventional-badge--refactor',
  chore: 'conventional-badge--chore',
  ci: 'conventional-badge--ci',
  build: 'conventional-badge--build',
  test: 'conventional-badge--test',
  perf: 'conventional-badge--perf',
  style: 'conventional-badge--style',
  revert: 'conventional-badge--revert',
}

export const parseConventionalCommit = (subject) => {
  const value = subject ? String(subject).trim() : ''
  if (!value) {
    return {
      type: '',
      scope: '',
      breaking: false,
      title: '',
      displayTitle: '',
      badgeClass: 'conventional-badge--default',
    }
  }
  const match = value.match(conventionalPattern)
  if (!match?.groups) {
    return {
      type: '',
      scope: '',
      breaking: false,
      title: value,
      displayTitle: value,
      badgeClass: 'conventional-badge--default',
    }
  }
  const type = String(match.groups.type || '').toLowerCase()
  const scope = String(match.groups.scope || '')
  const breaking = Boolean(match.groups.breaking)
  const title = String(match.groups.title || '').trim()
  const displayTitle = title || value
  return {
    type,
    scope,
    breaking,
    title,
    displayTitle,
    badgeClass: typeClassMap[type] || 'conventional-badge--default',
  }
}
