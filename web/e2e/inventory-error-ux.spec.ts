import { expect, test } from './fixtures'

test('estoque com consulta indisponível não apresenta saldo zero falso', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  let unavailable = true
  await page.route('**/api/v1/inventory/low-stock?*', async (route) => {
    await route.fulfill({
      status: unavailable ? 503 : 200,
      contentType: 'application/json',
      body: unavailable ? '{"message":"erro interno de estoque"}' : '{"items":[],"total":0}',
    })
  })
  await page.goto('/inventory')
  const warning = page.getByRole('alert').filter({ hasText: 'Não foi possível consultar o estoque' })
  await expect(warning).toBeVisible()
  await expect(warning).not.toContainText('erro interno')
  await expect(page.getByText('Nenhum item com baixo estoque.')).toHaveCount(0)
  unavailable = false
  await page.getByRole('button', { name: 'Tentar novamente' }).click()
  await expect(warning).toHaveCount(0)
  await expect(page.getByText('Nenhum item com baixo estoque.')).toBeVisible()
})
