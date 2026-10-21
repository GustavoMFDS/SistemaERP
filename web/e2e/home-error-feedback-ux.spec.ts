import { expect, test } from './fixtures'

test('início informa indisponibilidade de estoque sem apresentar resultado vazio', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  let unavailable = true
  await page.route('**/api/v1/inventory/low-stock?limit=8', async (route) => {
    await route.fulfill({
      status: unavailable ? 503 : 200,
      contentType: 'application/json',
      body: unavailable ? '{"message":"detalhe interno"}' : '{"items":[],"total":0}',
    })
  })

  await page.reload()
  const warning = page.getByRole('alert').filter({ hasText: 'Não foi possível consultar o estoque' })
  await expect(warning).toBeVisible()
  await expect(page.getByText('Seu estoque está dentro dos mínimos cadastrados.')).toHaveCount(0)
  await expect(warning).not.toContainText('detalhe interno')

  unavailable = false
  await warning.getByRole('button', { name: 'Tentar novamente' }).click()
  await expect(warning).toHaveCount(0)
  await expect(page.getByText('Seu estoque está dentro dos mínimos cadastrados.')).toBeVisible()
})
