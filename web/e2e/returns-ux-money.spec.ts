import { expect, test } from './fixtures'

test('devoluções mostram valores em reais com vírgula e centavos', async ({ page }) => {
  await page.route('**/api/v1/sales?limit=30&offset=0', async (route) => {
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        items: [{
          id: '00000000-0000-4000-8000-000000000200',
          status: 'finalized', total: 12.5,
        }],
      }),
    })
  })
  await page.route('**/api/v1/returns?limit=50&offset=0', async (route) => {
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        items: [{
          id: '00000000-0000-4000-8000-000000000300',
          sale_id: '00000000-0000-4000-8000-000000000200',
          kind: 'return', reason: 'Tamanho incorreto',
          refund_due: 3.2, created_at: '2026-10-20T15:00:00Z',
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
  await page.goto('/returns')

  await expect(page.getByRole('combobox', { name: 'Vendas recentes' }).locator('option').nth(1))
    .toContainText('12,50')
  await expect(page.getByRole('cell', { name: /3,20/ })).toBeVisible()
})
