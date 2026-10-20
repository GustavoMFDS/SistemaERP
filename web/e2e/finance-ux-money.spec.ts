import { expect, test } from './fixtures'

test('financeiro exibe pagamentos com moeda brasileira', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  await page.route('**/api/v1/finance/payments?*', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        items: [{
          id: 'pagamento-exemplo',
          sale_id: 'venda-exemplo',
          method: 'cash',
          amount: 1234.5,
          installments: 1,
          reconciliation_status: 'not_applicable',
        }],
        total: 1,
      }),
    })
  })

  await page.goto('/finance')
  await page.getByText('Conferir pagamentos e devoluções').click()
  await expect(page.getByRole('cell', { name: /1\.234,50/ }).first()).toBeVisible()
  await expect(page.getByText('1,234.50')).toHaveCount(0)
})
