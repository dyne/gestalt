const normalizePromptChunk = (chunk) =>
  String(chunk ?? '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')

const applyOutputChunk = (currentText, chunk) => {
  const input = String(chunk ?? '')
  if (!input) return currentText

  const lastNewline = currentText.lastIndexOf('\n')
  let prefix = ''
  let buffer = currentText
  if (lastNewline >= 0) {
    prefix = currentText.slice(0, lastNewline + 1)
    buffer = currentText.slice(lastNewline + 1)
  }

  for (let i = 0; i < input.length; i += 1) {
    const char = input[i]
    if (char === '\r') {
      if (input[i + 1] === '\n') {
        prefix += `${buffer}\n`
        buffer = ''
        i += 1
        continue
      }
      buffer = ''
      continue
    }
    if (char === '\n') {
      prefix += `${buffer}\n`
      buffer = ''
      continue
    }
    buffer += char
  }

  return `${prefix}${buffer}`
}

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

export const appendOutputSegment = (segments, chunk) => {
  const text = String(chunk ?? '')
  if (!text) return segments
  const next = Array.isArray(segments) ? [...segments] : []
  const lastIndex = next.length - 1
  if (lastIndex >= 0 && next[lastIndex].kind === 'output') {
    const updated = applyOutputChunk(next[lastIndex].text, text)
    next[lastIndex] = {
      kind: 'output',
      text: updated,
    }
    return next
  }
  const processed = applyOutputChunk('', text)
  if (!processed) return next
  next.push({ kind: 'output', text: processed })
  return next
}

export const appendPromptSegment = (segments, prompt) => {
  const normalized = normalizePromptChunk(prompt)
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
