import { expect, test } from './fixtures'

test('consulta de produtos falha sem mostrar catálogo vazio e permite tentar de novo', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  let unavailable = true
  await page.route('**/api/v1/products?*', async (route) => {
    await route.fulfill({
      status: unavailable ? 503 : 200,
      contentType: 'application/json',
      body: unavailable ? '{"message":"internal database failure"}' : '{"items":[],"total":0}',
    })
  })
  await page.goto('/products')
  const alert = page.getByRole('alert').filter({ hasText: 'Não foi possível consultar os produtos' })
  await expect(alert).toBeVisible()
  await expect(alert).not.toContainText('internal database')
  await expect(page.getByText('Ainda não há produtos cadastrados nesta loja.')).toHaveCount(0)

  unavailable = false
  await page.getByRole('button', { name: 'Tentar novamente' }).click()
  await expect(alert).toHaveCount(0)
  await expect(page.getByText('Ainda não há produtos cadastrados nesta loja.')).toBeVisible()
})
