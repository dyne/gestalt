// isCliSession reports whether a session uses the CLI interface.
export const isCliSession = (session) => {
  if (!session) return false
  const sessionInterface = String(session.interface || '').trim().toLowerCase()
  return sessionInterface === 'cli'
}
