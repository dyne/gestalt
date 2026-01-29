export const formatTerminalLabel = (terminal) => {
  const id = terminal?.id
  return id ? String(id) : ''
}
