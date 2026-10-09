import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('histórico de produtos mostra apenas recibos confirmados e permite paginar', async ({ page }) => {
  const offsets: number[] = []
  await page.route('**/api/v1/products/import-batches/history?*', async (route) => {
    const url = new URL(route.request().url())
    const offset = Number(url.searchParams.get('offset'))
    offsets.push(offset)
    const count = offset === 0 ? 10 : 1
    const entries = Array.from({ length: count }, (_, i) => ({
      batch_id: '00000000-0000-4000-8000-' + String(offset + i + 1).padStart(12, '0'),
      item_count: offset + i + 1,
      actor_name: offset === 0 ? 'Gerente da loja' : 'Dona da loja',
      created_at: '2026-10-08T18:00:00Z',
    }))
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ items: entries, limit: 10, offset, has_more: offset === 0 }),
    })
  })
  await login(page, 'gerente@sistema.local')
  const history = page.getByRole('region', { name: 'Histórico de importações confirmadas' })
  await expect(history.getByText('Gerente da loja').first()).toBeVisible()
  await expect(history).toContainText('Página 1')
  await history.getByRole('button', { name: 'Próxima página' }).click()
  await expect(history).toContainText('Página 2')
  await expect(history.getByText('Dona da loja')).toBeVisible()
  await expect(history.getByRole('button', { name: 'Próxima página' })).toBeDisabled()
  await history.getByRole('button', { name: 'Página anterior' }).click()
  await expect(history).toContainText('Página 1')
  expect(offsets).toContain(0)
  expect(offsets).toContain(10)
})

test('estoque inicial possui histórico próprio de recibos, sem CSV', async ({ page }) => {
  await page.route('**/api/v1/inventory/opening-stock/batches/history?*', (route) =>
    route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        items: [{
          batch_id: '00000000-0000-4000-8000-000000000200',
          item_count: 3, actor_name: 'Responsável estoque',
          created_at: '2026-10-08T18:00:00Z',
        }],
        limit: 10, offset: 0, has_more: false,
      }),
    }))
  await login(page, 'gerente@sistema.local')
  await page.goto('/inventory')
  const history = page.getByRole('region', { name: 'Histórico de importações confirmadas' })
  await expect(history).toContainText('Responsável estoque')
  await expect(history).toContainText('00000000-0000-4000-8000-000000000200')
  await expect(history.getByRole('button', { name: 'Próxima página' })).toBeDisabled()
})

test('caixa recebe 403 para ambos históricos e não vê os quadros administrativos', async ({ page }) => {
  await login(page, 'caixa@sistema.local')
  await expect(page.getByRole('region', { name: 'Histórico de importações confirmadas' })).toHaveCount(0)
  const result = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    const status = async (path: string) => {
      try {
        await apiJson(path)
        return 200
      } catch (err) {
        if (err instanceof APIError) return err.status
        throw err
      }
    }
    return [
      await status('/api/v1/products/import-batches/history?limit=10&offset=0'),
      await status('/api/v1/inventory/opening-stock/batches/history?limit=10&offset=0'),
    ]
  })
  expect(result).toEqual([403, 403])
  await page.goto('/inventory')
  await expect(page.getByRole('region', { name: 'Histórico de importações confirmadas' })).toHaveCount(0)
})

test('consulta de histórico rejeita limites e paginação inválidos', async ({ page }) => {
  await login(page, 'gerente@sistema.local')
  const errors = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    const status = async (path: string) => {
      try {
        await apiJson(path)
        return 200
      } catch (err) {
        if (err instanceof APIError) return err.status
        throw err
      }
    }
    return [
      await status('/api/v1/products/import-batches/history?limit=51'),
      await status('/api/v1/products/import-batches/history?offset=-1'),
      await status('/api/v1/inventory/opening-stock/batches/history?limit=abc'),
      await status('/api/v1/inventory/opening-stock/batches/history?limit=10&limit=20'),
      await status('/api/v1/products/import-batches/history?offset=5001'),
    ]
  })
  expect(errors).toEqual([422, 422, 422, 422, 422])
})
