import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('barcode lookup and PDV scanner add and increment the product', async ({ page }) => {
  await login(page)

  const lookup = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ sku: string; barcode?: string | null; name: string }>(
      '/api/v1/products/barcode/7890000000000',
    )
  })

  expect(lookup.sku).toBe('SKU-COCA-2L')
  expect(lookup.barcode).toBe('7890000000000')

  await page.getByRole('link', { name: 'PDV' }).click()
  await expect(page).toHaveURL(/\/pdv$/)
  const scanner = page.getByLabel('Código de barras')
  await scanner.fill('7890000000000')
  await scanner.press('Enter')

  const row = page.locator('tbody tr').filter({ hasText: 'Coca-Cola 2L' })
  await expect(row).toHaveCount(1)
  await expect(row).toContainText('1.00')

  await scanner.fill('7890000000000')
  await scanner.press('Enter')

  await expect(row).toHaveCount(1)
  await expect(row).toContainText('2.00')
})
