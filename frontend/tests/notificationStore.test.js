import assert from 'node:assert/strict'
import test from 'node:test'
import { notificationPreferences, notificationStore } from '../src/lib/notificationStore.js'

const resetPreferences = () => {
  notificationPreferences.set({
    enabled: true,
    durationMs: 0,
    levelFilter: 'all',
  })
}

test('notificationStore adds and dismisses entries', async () => {
  resetPreferences()
  notificationStore.clear()
  let current = []
  const unsubscribe = notificationStore.subscribe((value) => {
    current = value
  })

  const id = notificationStore.addNotification('info', 'hello', { duration: 10 })
  assert.equal(current.length, 1)
  assert.equal(current[0].id, id)
  assert.equal(current[0].level, 'info')

  await new Promise((resolve) => setTimeout(resolve, 30))
  assert.equal(current.length, 0)
  unsubscribe()
})

test('notificationStore does not auto-dismiss errors by default', async () => {
  resetPreferences()
  notificationStore.clear()
  let current = []
  const unsubscribe = notificationStore.subscribe((value) => {
    current = value
  })

  const id = notificationStore.addNotification('error', 'boom')
  assert.equal(current.length, 1)

  await new Promise((resolve) => setTimeout(resolve, 30))
  assert.equal(current.length, 1)

  notificationStore.dismiss(id)
  assert.equal(current.length, 0)
  unsubscribe()
})

test('notificationStore clear removes active timers', async () => {
  resetPreferences()
  notificationStore.clear()
  let current = []
  const unsubscribe = notificationStore.subscribe((value) => {
    current = value
  })

  notificationStore.addNotification('warning', 'heads up', { duration: 100 })
  assert.equal(current.length, 1)

  notificationStore.clear()
  assert.equal(current.length, 0)

  await new Promise((resolve) => setTimeout(resolve, 120))
  assert.equal(current.length, 0)
  unsubscribe()
})

test('notificationStore honors level filter', async () => {
  resetPreferences()
  notificationStore.clear()
  notificationPreferences.set({
    enabled: true,
    durationMs: 0,
    levelFilter: 'warning',
  })

  let current = []
  const unsubscribe = notificationStore.subscribe((value) => {
    current = value
  })

  const ignored = notificationStore.addNotification('info', 'skip me')
  assert.equal(ignored, null)
  assert.equal(current.length, 0)

  const id = notificationStore.addNotification('warning', 'show me', { duration: 10 })
  assert.equal(current.length, 1)
  assert.equal(current[0].id, id)

  await new Promise((resolve) => setTimeout(resolve, 30))
  assert.equal(current.length, 0)
  unsubscribe()
})

test('notificationStore applies duration override', async () => {
  resetPreferences()
  notificationStore.clear()
  notificationPreferences.set({
    enabled: true,
    durationMs: 5,
    levelFilter: 'all',
  })

  let current = []
  const unsubscribe = notificationStore.subscribe((value) => {
    current = value
  })

  notificationStore.addNotification('info', 'short')
  assert.equal(current.length, 1)

  await new Promise((resolve) => setTimeout(resolve, 20))
  assert.equal(current.length, 0)
  unsubscribe()
})
