import { expect, test } from './fixtures'

test('histórico de importações oferece nova tentativa e não inventa registros vazios', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  let unavailable = true
  await page.route('**/api/v1/imports/history?*', async (route) => {
    await route.fulfill({
      status: unavailable ? 503 : 200,
      contentType: 'application/json',
      body: unavailable ? '{"message":"internal service detail"}'
        : '{"items":[],"limit":10,"offset":0,"has_more":false}',
    })
  })
  await page.goto('/imports')
  const warning = page.getByRole('alert').filter({ hasText: 'Não foi possível carregar as importações' })
  await expect(warning).toBeVisible()
  await expect(warning).not.toContainText('internal service')
  await expect(page.getByText('Ainda não há importações confirmadas nesta loja.')).toHaveCount(0)

  unavailable = false
  await page.getByRole('button', { name: 'Tentar novamente' }).click()
  await expect(warning).toHaveCount(0)
  await expect(page.getByText('Ainda não há importações confirmadas nesta loja.')).toBeVisible()
})
