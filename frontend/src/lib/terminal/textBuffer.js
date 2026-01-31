const normalizeChunk = (chunk) =>
  chunk.replace(/\r\n/g, '\n').replace(/\r/g, '\n')

export class TextBuffer {
  constructor(maxLines = 2000) {
    this.maxLines = maxLines
    this.lines = []
    this.carry = ''
  }

  append(chunk) {
    if (chunk === undefined || chunk === null || chunk === '') {
      return
    }

    const text = this.carry + normalizeChunk(String(chunk))
    const parts = text.split('\n')

    if (text.endsWith('\n')) {
      this.carry = ''
    } else {
      this.carry = parts.pop() ?? ''
    }

    for (const line of parts) {
      this.lines.push(line)
    }

    this.trimToMaxLines(this.maxLines)
  }

  loadHistory(lines) {
    this.lines = Array.isArray(lines) ? [...lines] : []
    this.carry = ''
    this.trimToMaxLines(this.maxLines)
  }

  clear() {
    this.lines = []
    this.carry = ''
  }

  trimToMaxLines(maxLines) {
    if (!Number.isFinite(maxLines) || maxLines <= 0) {
      return
    }

    this.maxLines = maxLines
    if (this.lines.length > maxLines) {
      this.lines = this.lines.slice(-maxLines)
    }
  }

  text() {
    if (this.carry) {
      return [...this.lines, this.carry].join('\n')
    }
    return this.lines.join('\n')
  }

  getLines() {
    if (this.carry) {
      return [...this.lines, this.carry]
    }
    return [...this.lines]
  }
}
