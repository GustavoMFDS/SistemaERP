import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email = 'admin@sistema.local') {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('estoque baixa CSV sem preços e sem escolher outra empresa', async ({ page }) => {
  let requested = 0
  await page.route('**/api/v1/inventory/stock-report.csv', async (route) => {
    requested += 1
    expect(route.request().url()).not.toContain('tenant_id')
    await route.fulfill({
      status: 200,
      contentType: 'text/csv;charset=utf-8',
      headers: { 'Content-Disposition': 'attachment; filename="relatorio-estoque-loja.csv"' },
      body: '\uFEFFSKU;Produto;Unidade;Saldo;Estoque minimo;Situacao do estoque;Cadastro\nA1;Produto;un;2,500;3,000;Baixo;Ativo\n',
    })
  })
  await login(page)
  await page.goto('/inventory')
  const downloadEvent = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Exportar estoque CSV' }).click()
  const download = await downloadEvent
  expect(download.suggestedFilename()).toBe('relatorio-estoque-loja.csv')
  expect(requested).toBe(1)
})

test('limite do relatório não permite arquivo truncado', async ({ page }) => {
  await page.route('**/api/v1/inventory/stock-report.csv', (route) =>
    route.fulfill({
      status: 422, contentType: 'application/json',
      body: '{"code":"validation_error","message":"mais de 5000 produtos; solicite relatorio segmentado"}',
    }))
  await login(page)
  await page.goto('/inventory')
  await page.getByRole('button', { name: 'Exportar estoque CSV' }).click()
  await expect(page.getByText(/mais de 5000 produtos/)).toBeVisible()
})

test('API não aceita parâmetro de seleção de outro CNPJ', async ({ page }) => {
  await login(page)
  const result = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/inventory/stock-report.csv?tenant_id=other-company')
      return 200
    } catch (error) {
      return error instanceof APIError ? error.status : 0
    }
  })
  expect(result).toBe(422)
})
