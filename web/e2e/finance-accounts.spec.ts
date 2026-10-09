import { expect, test } from '@playwright/test'

test('contas financeiras integradas: cadastrar, dar baixa e consultar evolução', async ({ page }) => {
  const pageErrors: string[] = []
  page.on('pageerror', (e) => pageErrors.push(e.message))
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await page.getByRole('link', { name: 'Financeiro' }).click()
  await expect(page.getByRole('heading', { name: 'Finanças da loja' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Contas da loja' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Movimento financeiro' })).toBeVisible()
  await page.getByRole('button', { name: 'Nova conta' }).click()
  const description = 'Conta demonstração ' + Date.now()
  await page.getByLabel('Descrição').fill(description)
  await page.getByLabel('Valor (R$)').fill('124.25')
  await page.getByRole('button', { name: 'Cadastrar conta' }).click()
  await expect(page.getByText(description)).toBeVisible()
  const account = page.getByRole('article').filter({ hasText: description })
  await account.getByRole('button', { name: 'Dar baixa' }).click()
  await account.getByRole('button', { name: 'Confirmar baixa' }).click()
  await expect(account).toContainText('Paga')
  await page.getByLabel('Período do gráfico').selectOption('7')
  await expect(page.getByRole('img', { name: /Gráfico de barras/ })).toBeVisible()
  expect(pageErrors).toEqual([])
})

test('operador de caixa não acessa contas financeiras', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('caixa@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
  await expect(page.getByRole('link', { name: 'Financeiro' })).toHaveCount(0)
  const status = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/finance/accounts')
      return 200
    } catch (e) {
      if (e instanceof APIError) return e.status
      throw e
    }
  })
  expect(status).toBe(403)
})
