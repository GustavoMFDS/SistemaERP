import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('gerente confirma prévia de estoque inicial sem alterar o banco na seleção', async ({ page }) => {
  await login(page, 'gerente@sistema.local')
  await page.getByRole('link', { name: 'Estoque' }).click()
  await expect(page.getByRole('heading', { name: 'Cadastrar estoque inicial por planilha' })).toBeVisible()
  const input = page.getByLabel('Escolher planilha CSV')
  await input.setInputFiles({
    name: 'abertura.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from('sku;quantidade\nSKU-E2E-UNDEFINED;5,500\n', 'utf8'),
  })
  await expect(page.getByText(/1 válida\(s\), 0 com erro/)).toBeVisible()
  await expect(page.getByRole('button', { name: /Confirmar estoque inicial de 1 produto/ })).toBeDisabled()
  // Upload preview is read-only: it is safe to leave without applying.
  await page.getByRole('link', { name: 'Início' }).click()
  await expect(page).toHaveURL(/\/home$/)
})

test('caixa não consegue executar importação de estoque inicial', async ({ page }) => {
  await login(page, 'caixa@sistema.local')
  const response = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/inventory/opening-stock', {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: { items: [{ sku: 'ABC', quantity: 1 }] },
      })
      return 201
    } catch (error) {
      if (error instanceof APIError) return error.status
      throw error
    }
  })
  expect(response).toBe(403)
})
