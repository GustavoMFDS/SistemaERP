import { expect, test } from './fixtures'

test('falha ao consultar equipe oferece nova tentativa sem fingir lista vazia', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  let unavailable = true
  let attempts = 0
  await page.route('**/api/v1/staff', async (route) => {
    attempts += 1
    await route.fulfill({
      status: unavailable ? 503 : 200,
      contentType: 'application/json',
      body: unavailable
        ? '{"message":"detalhes técnicos da API"}'
        : JSON.stringify({
          members: [{ id: 'operador-de-teste', name: 'Operador de teste',
            username: 'operador.teste', email: '', role: 'cashier', active: true }],
          invitations: [], password_resets: [],
        }),
    })
  })

  await page.goto('/staff')
  const warning = page.getByRole('alert').filter({ hasText: 'Não foi possível consultar a equipe' })
  await expect(warning).toBeVisible()
  await expect(warning).not.toContainText('detalhes técnicos')
  await expect(page.getByText('Ainda não há funcionários cadastrados')).toHaveCount(0)

  unavailable = false
  await page.getByRole('button', { name: 'Tentar novamente' }).click()
  await expect(page.getByRole('cell', { name: 'Operador de teste' })).toBeVisible()
  await expect(warning).toHaveCount(0)
  expect(attempts).toBeGreaterThanOrEqual(2)
})
