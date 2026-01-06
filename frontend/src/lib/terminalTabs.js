export const formatTerminalLabel = (terminal) => {
  const title = terminal?.title?.trim()
  if (title) {
    return title
  }
  return `Terminal ${terminal?.id ?? ''}`.trim()
}
