import { expect, test } from './fixtures'

test('compras mostram totais em reais com vírgula e separador de milhar', async ({ page }) => {
  await page.route('**/api/v1/purchases?limit=200&offset=0', async (route) => {
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        items: [{
          id: '00000000-0000-4000-8000-000000000222',
          supplier_id: '00000000-0000-4000-8000-000000000333',
          supplier_name: 'Fornecedor exemplo', status: 'ordered',
          total: 1234.5, ordered_at: '2026-10-20T12:00:00Z',
        }],
        total: 1,
      }),
    })
  })
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
  await page.goto('/purchases')
  await expect(page.getByRole('heading', { name: 'Compras', exact: true })).toBeVisible()
  await expect(page.getByRole('cell', { name: /1\.234,50/ })).toBeVisible()
})
