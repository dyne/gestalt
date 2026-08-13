import { expect, test, type Page } from '@playwright/test'

const position = (page: Page) => page.locator('.home-carousel__position')

test('keeps every desktop slide aligned with the viewport', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/')

  for (let index = 1; index <= 6; index += 1) {
    await page.getByRole('button', { name: new RegExp(`^Show slide ${index}:`) }).click()
    await page.waitForTimeout(500)
    const alignment = await page.evaluate(() => {
      const viewport = document.querySelector<HTMLElement>('.home-carousel__viewport')!
      const active = document.querySelector<HTMLElement>(
        '.home-carousel__slide[aria-hidden="false"]'
      )!
      const viewportRect = viewport.getBoundingClientRect()
      const activeRect = active.getBoundingClientRect()
      const imageRect = active.querySelector('img')!.getBoundingClientRect()
      return {
        left: activeRect.left - viewportRect.left - viewport.clientLeft,
        right: viewportRect.right - viewport.clientLeft - activeRect.right,
        imageTop: imageRect.top - viewportRect.top - viewport.clientTop,
        imageRight: viewportRect.right - viewport.clientLeft - imageRect.right,
        imageBottom: viewportRect.bottom - viewport.clientTop - imageRect.bottom,
        imageLeft: imageRect.left - viewportRect.left - viewport.clientLeft
      }
    })
    expect(alignment.left, `slide ${index} left drift`).toBeCloseTo(0, 1)
    expect(alignment.right, `slide ${index} right drift`).toBeCloseTo(0, 1)
    expect(alignment.imageTop, `slide ${index} image top`).toBeGreaterThanOrEqual(0)
    expect(alignment.imageRight, `slide ${index} image right`).toBeGreaterThanOrEqual(0)
    expect(alignment.imageBottom, `slide ${index} image bottom`).toBeGreaterThanOrEqual(0)
    expect(alignment.imageLeft, `slide ${index} image left`).toBeGreaterThanOrEqual(0)
  }
})

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
