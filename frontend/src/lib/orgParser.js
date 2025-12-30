const DEFAULT_KEYWORDS = ['TODO', 'WIP', 'DONE']

const parseHeading = (raw, keywords) => {
  let text = raw.trim()
  let keyword = ''
  let priority = ''

  for (const candidate of keywords) {
    if (text === candidate || text.startsWith(`${candidate} `)) {
      keyword = candidate
      text = text.slice(candidate.length).trimStart()
      break
    }
  }

  const priorityMatch = text.match(/^\[#([A-Z])\]\s*/)
  if (priorityMatch) {
    priority = priorityMatch[1]
    text = text.slice(priorityMatch[0].length).trimStart()
  }

  return { keyword, priority, text }
}

const finalizeNodes = (nodes) => {
  for (const node of nodes) {
    const body = node.bodyLines.join('\n').trimEnd()
    node.body = body
    delete node.bodyLines
    if (node.children.length > 0) {
      finalizeNodes(node.children)
    }
  }
}

export const parseOrg = (text = '', options = {}) => {
  const keywords =
    Array.isArray(options.keywords) && options.keywords.length > 0
      ? options.keywords
      : DEFAULT_KEYWORDS

  const lines = String(text || '').split(/\r?\n/)
  const roots = []
  const stack = []
  let current = null

  for (const line of lines) {
    const match = line.match(/^(\*+)\s+(.*)$/)
    if (!match) {
      if (current) {
        current.bodyLines.push(line)
      }
      continue
    }

    const level = match[1].length
    const { keyword, priority, text: headingText } = parseHeading(match[2], keywords)
    const node = {
      level,
      keyword,
      priority,
      text: headingText,
      bodyLines: [],
      children: [],
      collapsed: false,
    }

    stack.length = Math.max(0, level - 1)
    if (level === 1) {
      roots.push(node)
    } else if (stack[level - 2]) {
      stack[level - 2].children.push(node)
    } else {
      roots.push(node)
    }

    stack[level - 1] = node
    current = node
  }

  finalizeNodes(roots)
  return roots
}

export const __test = { parseHeading }
