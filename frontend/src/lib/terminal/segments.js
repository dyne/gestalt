const normalizeChunk = (chunk) =>
  String(chunk ?? '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')

export const appendSegment = (segments, kind, text) => {
  if (!text) return segments
  const next = Array.isArray(segments) ? [...segments] : []
  const lastIndex = next.length - 1
  if (lastIndex >= 0 && next[lastIndex].kind === kind) {
    next[lastIndex] = {
      kind,
      text: `${next[lastIndex].text}${text}`,
    }
    return next
  }
  next.push({ kind, text })
  return next
}

export const appendOutputSegment = (segments, chunk) =>
  appendSegment(segments, 'output', normalizeChunk(chunk))

export const appendPromptSegment = (segments, prompt) => {
  const normalized = normalizeChunk(prompt)
  if (!normalized) return segments
  const text = normalized.endsWith('\n') ? normalized : `${normalized}\n`
  return appendSegment(segments, 'prompt', text)
}

export const historyToSegments = (lines) => {
  const text =
    Array.isArray(lines) && lines.length > 0 ? lines.join('\n') : ''
  if (!text) return []
  return [{ kind: 'output', text }]
}
