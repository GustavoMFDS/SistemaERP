import { expect, test } from './fixtures'

test('relatórios oferecem retorno claro quando acesso não está disponível', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
  await page.getByText('Mais áreas do sistema').click()
  await page.getByRole('link', { name: 'Relatórios', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Relatórios da loja' })).toBeVisible()
})

test('permite repetir a verificação sem expor detalhes internos', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
  await expect(page.getByRole('link', { name: /Ir para o caixa/ })).toBeVisible()
  let count = 0
  let accessUnavailable = true
  await page.route('**/api/v1/auth/me', async (route) => {
    count += 1
    await route.fulfill({
      status: accessUnavailable ? 503 : 200,
      contentType: 'application/json',
      body: accessUnavailable ? '{"message":"erro interno"}' : '{"permissions":["finance:read"]}',
    })
  })
  await page.getByText('Mais áreas do sistema').click()
  await page.getByRole('link', { name: 'Relatórios', exact: true }).click()
  const notice = page.getByRole('alert').filter({ hasText: 'Não foi possível verificar seu acesso aos relatórios' })
  await expect(notice).toBeVisible()
  await expect(notice).not.toContainText('erro interno')
  accessUnavailable = false
  await page.getByRole('button', { name: 'Tentar novamente' }).click()
  await expect(page.getByRole('button', { name: 'Tentar novamente' })).toHaveCount(0)
  expect(count).toBeGreaterThanOrEqual(2)
})
