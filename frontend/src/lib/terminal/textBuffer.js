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

    const text = String(chunk)
    let buffer = this.carry
    for (let i = 0; i < text.length; i += 1) {
      const char = text[i]
      if (char === '\r') {
        if (text[i + 1] === '\n') {
          this.lines.push(buffer)
          buffer = ''
          i += 1
          continue
        }
        buffer = ''
        continue
      }
      if (char === '\n') {
        this.lines.push(buffer)
        buffer = ''
        continue
      }
      buffer += char
    }
    this.carry = buffer

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
