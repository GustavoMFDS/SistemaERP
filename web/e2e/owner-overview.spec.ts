import { expect, test } from '@playwright/test'

async function signIn(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  // Retain the existing login landing route while the new home is reviewed.
  await expect(page).toHaveURL(/\/products$/)
  await page.getByRole('link', { name: 'Início' }).click()
  await expect(page).toHaveURL(/\/home$/)
}

test('proprietário autorizado vê resumo e exporta somente seu período', async ({ page }) => {
  await signIn(page, 'admin@sistema.local')
  await expect(page.getByRole('heading', { name: 'Resumo das vendas' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Produtos que precisam de atenção' })).toBeVisible()
  await expect(page.getByText(/produto\(s\) ativos no estoque mínimo ou abaixo/)).toBeVisible()
  const downloadPromise = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Exportar CSV' }).click()
  const download = await downloadPromise
  expect(download.suggestedFilename()).toMatch(/^resumo-loja-\d{4}-\d{2}-\d{2}-a-\d{4}-\d{2}-\d{2}\.csv$/)
})

test('caixa não vê indicadores financeiros, nem acessa endpoint proprietário', async ({ page }) => {
  await signIn(page, 'caixa@sistema.local')
  await expect(page.getByRole('link', { name: /Abrir o caixa/ })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Resumo das vendas' })).toHaveCount(0)
  const response = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/finance/overview?from=2026-01-01&to=2026-12-31')
      return 200
    } catch (error) {
      if (error instanceof APIError) return error.status
      throw error
    }
  })
  expect(response).toBe(403)
})
