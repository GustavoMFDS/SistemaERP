import { expect, test } from '@playwright/test'

const brandName = process.env.VITE_APP_BRAND_NAME?.trim() || 'Esterium ERP'

test('marca configurada aparece no login e no manifesto PWA', async ({ page }) => {
  await page.goto('/login')
  await expect(page.getByText(brandName)).toBeVisible()
  await expect(page).toHaveTitle('Entrar — ' + brandName)

  const manifest = await page.evaluate(async () => {
    const response = await fetch('/manifest.webmanifest')
    if (!response.ok) throw new Error('manifest request failed (' + response.status + ')')
    return response.json() as Promise<{ name: string; short_name: string; description: string }>
  })
  expect(manifest.name).toBe(brandName + ' — Gestão Comercial')
  expect(manifest.short_name).toBe(brandName)
  expect(manifest.description.startsWith(brandName + ' — ')).toBe(true)
})

test('comprovante não fiscal escapa conteúdo dinâmico', async ({ page }) => {
  await page.goto('/login')
  const markup = await page.evaluate(async () => {
    const { buildNonFiscalReceiptHtml } = await import('/src/lib/nonFiscalReceipt.ts')
    return buildNonFiscalReceiptHtml({
      brandName: '<img src=x onerror=alert(1)>',
      saleId: '<script>alert(1)</script>',
      total: 1,
      items: [{ name: '<img src=x>', quantity: 1, unit: 'un' }],
      paymentMethods: ['pix'],
      paperWidthMM: 80,
    })
  })

  expect(markup).not.toMatch(/<img|<script/i)
  expect(markup).toContain('&lt;img src=x onerror=alert(1)&gt;')
  expect(markup).toContain('&lt;script&gt;alert(1)&lt;/script&gt;')
  expect(markup).toContain('COMPROVANTE NÃO FISCAL')
})
