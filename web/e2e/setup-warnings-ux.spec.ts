import { expect, test } from './fixtures'

test('configuração mostra avisos simples quando uma conferência falha', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  await page.route('**/api/v1/fiscal/nfce/readiness', async (route) => {
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: '{"message":"detalhe técnico interno e não público"}',
    })
  })

  await page.goto('/setup')
  const warning = page.getByRole('alert').filter({ hasText: 'Nota ao consumidor' })
  await expect(warning).toContainText('não foi possível conferir os dados de preparação')
  await expect(warning).not.toContainText('detalhe técnico')
  await expect(page.getByRole('heading', { name: 'Vamos preparar sua loja' })).toBeVisible()
})
