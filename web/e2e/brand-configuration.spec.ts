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
