import { expect, test } from './fixtures'

test('nota fiscal não expõe erro do servidor na lista de vendas', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  let unavailable = true
  await page.route('**/api/v1/fiscal/obligations?*', async (route) => {
    await route.fulfill({
      status: unavailable ? 503 : 200,
      contentType: 'application/json',
      body: unavailable ? '{"message":"internal certificate or database secret"}'
        : '{"items":[],"total":0,"limit":20,"offset":0}',
    })
  })
  await page.goto('/fiscal')
  const warning = page.getByRole('alert').filter({ hasText: 'Não foi possível conferir as notas das vendas' })
  await expect(warning).toBeVisible()
  await expect(warning).not.toContainText('internal certificate')
  await expect(page.getByText('Esta tela não transmite notas para a SEFAZ.')).toBeVisible()
  unavailable = false
  await page.getByRole('button', { name: 'Atualizar pendências' }).click()
  await expect(warning).toHaveCount(0)
})
