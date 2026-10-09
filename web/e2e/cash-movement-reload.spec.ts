import { expect, test } from '@playwright/test'

test('ambiguous cash movement key survives PDV reload', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await page.getByRole('link', { name: 'PDV' }).click()
  await page.getByRole('button', { name: 'Abrir' }).click()

  const keys: string[] = []
  let drop = true
  await page.route(/\/api\/v1\/cash\/sessions\/[^/]+\/movements$/, async (route) => {
    if (route.request().method() !== 'POST') return route.continue()
    keys.push(route.request().headers()['idempotency-key'] ?? '')
    if (drop) {
      drop = false
      const response = await route.fetch()
      expect(response.ok()).toBeTruthy()
      await route.abort('failed')
      return
    }
    await route.continue()
  })

  await page.getByLabel('Movimento (R$)').fill('1')
  await page.getByRole('button', { name: 'Suprimento' }).click()
  await expect.poll(() => keys.length).toBe(1)

  await page.reload()
  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await expect(page.getByText(/resposta pendente/)).toBeVisible()

  await page.getByLabel('Movimento (R$)').fill('1')
  await page.getByRole('button', { name: 'Suprimento' }).click()
  await expect.poll(() => keys.length).toBe(2)
  expect(keys[1]).toBe(keys[0])

  await page.getByLabel('Dinheiro declarado').fill('1')
  await page.getByRole('button', { name: 'Fechar caixa' }).click()
})
