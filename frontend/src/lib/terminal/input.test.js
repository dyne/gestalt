import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import {
  isCopyKey,
  isPasteKey,
  isMouseReport,
  shouldSuppressMouseMode,
  writeClipboardText,
  readClipboardText,
} from './input.js'

const buildKeyEvent = ({ key = '', ctrlKey = false, metaKey = false, altKey = false } = {}) => ({
  key,
  ctrlKey,
  metaKey,
  altKey,
})

describe('terminal input helpers', () => {
  let originalClipboard
  let originalExecCommand

  beforeEach(() => {
    originalClipboard = navigator.clipboard
    originalExecCommand = document.execCommand
  })

  afterEach(() => {
    if (originalClipboard) {
      Object.defineProperty(navigator, 'clipboard', {
        value: originalClipboard,
        configurable: true,
      })
    } else {
      delete navigator.clipboard
    }
    document.execCommand = originalExecCommand
  })

  it('detects copy and paste key combos', () => {
    expect(isCopyKey(buildKeyEvent({ key: 'c', ctrlKey: true }))).toBe(true)
    expect(isCopyKey(buildKeyEvent({ key: 'C', metaKey: true }))).toBe(true)
    expect(isCopyKey(buildKeyEvent({ key: 'c', ctrlKey: true, altKey: true }))).toBe(false)
    expect(isCopyKey(buildKeyEvent({ key: 'v', ctrlKey: true }))).toBe(false)

    expect(isPasteKey(buildKeyEvent({ key: 'v', ctrlKey: true }))).toBe(true)
    expect(isPasteKey(buildKeyEvent({ key: 'V', metaKey: true }))).toBe(true)
    expect(isPasteKey(buildKeyEvent({ key: 'v', ctrlKey: true, altKey: true }))).toBe(false)
    expect(isPasteKey(buildKeyEvent({ key: 'c', ctrlKey: true }))).toBe(false)
  })

  it('recognizes mouse reports and mouse mode suppression', () => {
    expect(isMouseReport('\x1b[<0;1;2m')).toBe(true)
    expect(isMouseReport('\x1b[<10;20;30M')).toBe(true)
    expect(isMouseReport('\x1b[Mabc')).toBe(true)
    expect(isMouseReport('\x1b[Mab')).toBe(false)
    expect(isMouseReport('plain')).toBe(false)

    expect(shouldSuppressMouseMode([])).toBe(false)
    expect(shouldSuppressMouseMode([1000])).toBe(true)
    expect(shouldSuppressMouseMode([[1000, 1002], [1003]])).toBe(true)
    expect(shouldSuppressMouseMode([1000, 2000])).toBe(false)
  })

  it('writes to clipboard with modern API and falls back when needed', async () => {
    const writeText = async () => {}
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    await expect(writeClipboardText('hello')).resolves.toBe(true)
    await expect(writeClipboardText('')).resolves.toBe(false)

    const failingClipboard = {
      writeText: async () => {
        throw new Error('denied')
      },
    }
    Object.defineProperty(navigator, 'clipboard', {
      value: failingClipboard,
      configurable: true,
    })
    document.execCommand = () => true
    await expect(writeClipboardText('fallback')).resolves.toBe(true)
  })

  it('reads from clipboard with modern API and falls back to document paste', async () => {
    const readText = async () => 'modern'
    Object.defineProperty(navigator, 'clipboard', {
      value: { readText },
      configurable: true,
    })
    await expect(readClipboardText()).resolves.toBe('modern')

    Object.defineProperty(navigator, 'clipboard', {
      value: { readText: async () => {
        throw new Error('blocked')
      } },
      configurable: true,
    })

    document.execCommand = (command) => {
      if (command === 'paste' && document.activeElement) {
        document.activeElement.value = 'fallback'
      }
      return true
    }

    await expect(readClipboardText()).resolves.toBe('fallback')
  })
})
