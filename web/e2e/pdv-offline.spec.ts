import { expect, test } from '@playwright/test'

test('login, sale offline queue and reconnect sync', async ({ page, context }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)

  await page.getByRole('link', { name: 'PDV' }).click()
  await expect(page.getByRole('heading', { name: 'PDV' })).toBeVisible()
  await page.getByRole('button', { name: 'Abrir' }).click()
  await expect(page.getByText(/cash_session_id/)).toBeVisible()

  await page.getByLabel('Produto').selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Adicionar' }).click()

  await context.setOffline(true)
  await page.getByRole('button', { name: 'Finalizar' }).click()
  await expect(page.getByText(/Venda registrada offline/)).toBeVisible()
  await expect(page.getByText(/Pendências: 1/)).toBeVisible()

  await context.setOffline(false)
  await expect(page.getByText(/Pendências: 0/)).toBeVisible({ timeout: 15000 })
})
