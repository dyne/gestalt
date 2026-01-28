import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const testDir = path.dirname(fileURLToPath(import.meta.url))
const fixturePath = path.resolve(testDir, '..', '..', '..', 'testdata', 'otel', 'ui-log.json')

export const loadUiLogFixture = () => {
  const raw = fs.readFileSync(fixturePath, 'utf-8')
  return JSON.parse(raw)
}

export const buildUiLogPayload = (overrides = {}) => {
  return { ...loadUiLogFixture(), ...overrides }
}
