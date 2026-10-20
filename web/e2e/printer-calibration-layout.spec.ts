import { readFileSync } from 'node:fs'
import path from 'node:path'
import { expect, test } from '@playwright/test'

// The real Go-rendered non-fiscal calibration HTML, not a browser mock.
const fixtureDir = process.env.PRINTER_CALIBRATION_BROWSER_DIR
test.skip(!fixtureDir, 'needs generated disposable calibration receipts')

for (const width of [58, 80] as const) {
  test(`printer calibration ${width}mm does not clip and keeps 32mm QR and 40mm ruler`, async ({ page }) => {
    const markup = readFileSync(path.join(fixtureDir!, `printer-test-${width}.html`), 'utf8')
    await page.setViewportSize({ width: width === 58 ? 236 : 340, height: 850 })
    await page.emulateMedia({ media: 'print' })
    await page.setContent(markup, { waitUntil: 'load' })

    await expect(page.locator('body')).toHaveClass(`receipt-test-${width}`)
    await expect(page.getByText('NÃO É DOCUMENTO FISCAL')).toBeVisible()
    await expect(page.getByText(/Teste de texto: ção, café, maçã/)).toBeVisible()

    const metrics = await page.evaluate(() => {
      const body = document.body.getBoundingClientRect()
      const qr = document.querySelector('.qr img')! as HTMLImageElement
      const qrRect = qr.getBoundingClientRect()
      const ruler = document.querySelector('.ruler')!.getBoundingClientRect()
      return {
        documentWidth: document.documentElement.scrollWidth,
        viewportWidth: window.innerWidth,
        bodyWidth: body.width,
        qrWidth: qrRect.width,
        rulerWidth: ruler.width,
        qrLoaded: qr.complete && qr.naturalWidth === 256,
        qrWithinBody: qrRect.left >= body.left - 1 && qrRect.right <= body.right + 1,
        rulerWithinBody: ruler.left >= body.left - 1 && ruler.right <= body.right + 1,
      }
    })

    expect(metrics.documentWidth).toBeLessThanOrEqual(metrics.viewportWidth + 1)
    expect(metrics.bodyWidth).toBeLessThanOrEqual(width * 96 / 25.4 + 1)
    expect(metrics.qrLoaded).toBe(true)
    expect(metrics.qrWidth).toBeGreaterThan(115)
    expect(metrics.qrWidth).toBeLessThan(125)
    expect(metrics.rulerWidth).toBeGreaterThan(147)
    expect(metrics.rulerWidth).toBeLessThan(156)
    expect(metrics.qrWithinBody).toBe(true)
    expect(metrics.rulerWithinBody).toBe(true)
  })
}
