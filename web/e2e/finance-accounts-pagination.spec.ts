import { test, expect } from '@playwright/test'

test('paginacao de contas usa saldos totais do servidor', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
  await page.getByRole('link', { name: 'Financeiro' }).click()
  await expect(page.getByRole('heading', { name: 'Contas da loja' })).toBeVisible()

  const before = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ total: number; summary: { payable_open: number } }>('/api/v1/finance/accounts?limit=1')
  })
  for (let n = 0; n < 2; n++) {
    await page.getByRole('button', { name: 'Nova conta' }).click()
    await page.getByLabel('Descrição').fill('Teste de paginação ' + crypto.randomUUID())
    await page.getByLabel('Valor (R$)').fill('25.00')
    await page.getByRole('button', { name: 'Cadastrar conta' }).click()
    await expect(page.getByRole('button', { name: 'Nova conta' })).toBeVisible()
  }
  const after = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ items: unknown[]; total: number; truncated: boolean; summary: { payable_open: number } }>('/api/v1/finance/accounts?kind=all&limit=1&offset=0')
  })
  expect(after.items).toHaveLength(1)
  expect(after.total).toBe(before.total + 2)
  expect(after.truncated).toBe(true)
  expect(after.summary.payable_open).toBeCloseTo(before.summary.payable_open + 50, 2)
  await expect(page.getByRole('navigation', { name: 'Páginas de contas' })).toBeVisible()
  await expect(page.getByText(/Exibindo 1–/)).toBeVisible()
})

test('navegar entre páginas preserva saldo global calculado no servidor', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)

  await page.route('**/api/v1/finance/accounts?*', async (route) => {
    const offset = Number(new URL(route.request().url()).searchParams.get('offset') ?? '0')
    await route.fulfill({
      status: 200,
      json: {
        items: [{
          id: '90909090-9090-4090-8090-909090909090', kind: 'payable',
          description: offset === 0 ? 'Primeira conta' : 'Conta de outra página',
          amount: 35, due_date: '2026-11-15', status: 'open',
          settled_at: null, settlement_method: null,
        }],
        total: 161, limit: 80, offset, truncated: true,
        summary: { payable_open: 1234.56, receivable_open: 765.43, overdue_open: 42 },
      },
    })
  })
  await page.getByRole('link', { name: 'Financeiro' }).click()
  await expect(page.getByText('Primeira conta')).toBeVisible()
  await expect(page.getByText('Exibindo 1–1 de 161 contas')).toBeVisible()
  await expect(page.getByText('R$ 1.234,56')).toBeVisible()
  await page.getByRole('button', { name: 'Próxima' }).click()
  await expect(page.getByText('Conta de outra página')).toBeVisible()
  await expect(page.getByText('Exibindo 81–81 de 161 contas')).toBeVisible()
  await expect(page.getByText('R$ 1.234,56')).toBeVisible()
  await page.getByRole('button', { name: 'Anterior' }).click()
  await expect(page.getByText('Primeira conta')).toBeVisible()
})

test('servidor antigo não quebra a página nem inventa totais financeiros', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
  await page.route('**/api/v1/finance/accounts?*', (route) =>
    route.fulfill({ status: 200, json: { items: [], total: 0, truncated: false } }))
  await page.getByRole('link', { name: 'Financeiro' }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'O servidor precisa ser atualizado' })).toBeVisible()
  await expect(page.getByText('Os totais estão indisponíveis')).toBeVisible()
  expect(errors).toEqual([])
})
