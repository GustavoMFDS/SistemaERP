import { expect, test } from './fixtures'

test('histórico do estoque diferencia falha e ausência de movimentos por produto', async ({ page }) => {
  let calls = 0
  await page.route('**/api/v1/inventory/movements?*', async (route) => {
    calls += 1
    const url = new URL(route.request().url())
    if (calls === 1) {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: '{"message":"erro técnico interno"}',
      })
      return
    }
    const filtered = url.searchParams.has('product_id')
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        items: filtered ? [] : [{
          id: 'registro-ficticio', product_id: 'produto-ficticio',
          product_sku: 'ARROZ-5', product_name: 'Arroz 5kg',
          movement_type: 'purchase', delta: 3, qty_before: 0, qty_after: 3,
          reason: 'Recebimento de teste', created_at: '2026-10-08T21:00:00Z',
        }],
        total: filtered ? 0 : 1,
      }),
    })
  })

  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('gerente@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
  await page.goto('/stock-movements')

  const error = page.getByRole('alert')
  await expect(error).toContainText('Não foi possível carregar as movimentações')
  await expect(error).not.toContainText('erro técnico')
  await expect(page.getByText(/Página 1 — 0 movimentações/)).toHaveCount(0)

  await page.getByRole('button', { name: 'Tentar novamente' }).click()
  await expect(page.getByRole('button', { name: 'Arroz 5kg' })).toBeVisible()
  await page.getByRole('button', { name: 'Arroz 5kg' }).click()
  await expect(page.getByText('Este produto ainda não tem movimentações registradas.')).toBeVisible()
  expect(calls).toBeGreaterThanOrEqual(3)
})
