export const isExternalCliSession = (session) => {
  if (!session) return false
  const runner = String(session.runner || '').trim().toLowerCase()
  const sessionInterface = String(session.interface || '').trim().toLowerCase()
  return runner === 'external' && sessionInterface === 'cli'
}
