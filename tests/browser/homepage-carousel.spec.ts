import { expect, test, type Page } from '@playwright/test'

const position = (page: Page) => page.locator('.home-carousel__position')

test('supports controls, seamless looping, swipe, and timed advancement', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')

  const carousel = page.getByRole('region', { name: 'Gestalt Mobile journey' })
  await expect(carousel).toBeVisible()
  await expect(position(page)).toContainText('1 / 6')
  expect((await carousel.boundingBox())!.height).toBeLessThan(650)

  await page.getByRole('button', { name: 'Previous screenshot' }).click()
  await expect(position(page)).toContainText('6 / 6')
  await page.waitForTimeout(500)
  await page.getByRole('button', { name: 'Next screenshot' }).click()
  await expect(position(page)).toContainText('1 / 6')
  await page.waitForTimeout(500)

  const viewport = page.locator('.home-carousel__viewport')
  const box = await viewport.boundingBox()
  expect(box).not.toBeNull()
  await page.mouse.move(box!.x + box!.width * 0.8, box!.y + box!.height / 2)
  await page.mouse.down()
  await page.mouse.move(box!.x + box!.width * 0.2, box!.y + box!.height / 2, { steps: 6 })
  await page.mouse.up()
  await expect(position(page)).toContainText('2 / 6')
  await page.waitForTimeout(500)

  await page.getByRole('button', { name: 'Show slide 6: Take Gestalt to another device' }).click()
  await expect(position(page)).toContainText('6 / 6')
  await page.locator('body').click({ position: { x: 2, y: 2 } })
  await page.mouse.move(2, 2)
  await expect(position(page)).toContainText('1 / 6', { timeout: 6000 })
})

test('starts paused when reduced motion is requested', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto('/')

  await expect(page.getByRole('button', { name: 'Start automatic slideshow' })).toBeVisible()
  await expect(position(page)).toContainText('1 / 6')
  await page.waitForTimeout(5200)
  await expect(position(page)).toContainText('1 / 6')
})
