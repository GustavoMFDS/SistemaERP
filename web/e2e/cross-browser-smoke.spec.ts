import { expect, test } from '@playwright/test'

test('authenticated shell and PDV render across browser engines', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()

  await expect(page).toHaveURL(/\/products$/)
  await expect(page.getByRole('heading', { name: 'Produtos' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'PDV' })).toBeVisible()

  await page.getByRole('link', { name: 'PDV' }).click()
  await expect(page.getByRole('heading', { name: 'PDV' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Abrir' })).toBeVisible()
})
