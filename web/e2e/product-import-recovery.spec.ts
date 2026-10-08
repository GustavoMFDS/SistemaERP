import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

const sample = 'sku;nome;preco;unidade\nE2E-RECOVER-PROD;12,50;un\n'
const changed = 'sku;nome;preco;unidade\nE2E-DIFFERENT;20,00;un\n'

async function upload(page: import('@playwright/test').Page, csv: string) {
  await page.getByLabel('Escolher arquivo CSV').setInputFiles({
    name: 'catalogo.csv', mimeType: 'text/csv', buffer: Buffer.from(csv, 'utf8'),
  })
}

test('lote mantém idempotency key entre recarga, bloqueia CSV alterado e não duplica', async ({ page }) => {
  const keys: string[] = []
  let postCount = 0
  await page.route('**/api/v1/products/import-batches', async (route) => {
    if (route.request().method() !== 'POST') return route.continue()
    keys.push(route.request().headers()['idempotency-key'])
    postCount += 1
    if (postCount === 1) {
      await route.abort('failed')
      return
    }
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        batch_id: '00000000-0000-4000-8000-000000000032', item_count: 1, replayed: true,
      }),
    })
  })
  await page.route('**/api/v1/products/import-batches/*', (route) =>
    route.fulfill({ status: 404, contentType: 'application/json', body: '{"code":"not_found"}' }))
  page.on('dialog', (dialog) => void dialog.accept())

  await login(page, 'gerente@sistema.local')
  await upload(page, sample)
  await expect(page.getByText(/1 válida\(s\)/)).toBeVisible()
  await page.getByRole('button', { name: /Confirmar importação de 1 produto/ }).click()
  await expect(page.getByRole('region', { name: 'Importação de produtos pendente' })).toBeVisible()
  await expect(page.getByText(/chave original foi preservada/i)).toBeVisible()
  const stored = await page.evaluate(() =>
    Object.entries(localStorage).filter(([key]) => key.startsWith('sistemaemgo:pendingProductImport:v1')),
  )
  expect(stored).toHaveLength(1)
  expect(stored[0][1]).not.toContain('E2E-RECOVER-PROD')
  expect(stored[0][1]).not.toContain('12.50')
  await page.reload()
  const section = page.getByRole('region', { name: 'Importação de produtos pendente' })
  await expect(section).toBeVisible()
  await section.getByRole('button', { name: 'Conferir lote no servidor' }).click()
  await expect(page.getByText(/ainda não aparece como confirmado/i)).toBeVisible()

  await upload(page, changed)
  await expect(page.getByText(/importação pendente/i).last()).toBeVisible()
  await expect(page.getByRole('button', { name: /Confirmar importação de 1 produto/ })).toHaveCount(0)
  await upload(page, sample)
  await page.getByRole('button', { name: /Confirmar importação de 1 produto/ }).click()
  await expect(page.getByText(/Nenhum produto foi duplicado/)).toBeVisible()
  expect(keys).toHaveLength(2)
  expect(keys[0]).toBe(keys[1])
  await expect(section).toHaveCount(0)
  const remaining = await page.evaluate(() =>
    Object.keys(localStorage).filter((key) => key.startsWith('sistemaemgo:pendingProductImport:v1')),
  )
  expect(remaining).toEqual([])
})

test('consulta do lote confirmado evita novo POST depois de queda de conexão', async ({ page }) => {
  let posts = 0
  await page.route('**/api/v1/products/import-batches', (route) => {
    posts += 1
    return route.abort('failed')
  })
  await page.route('**/api/v1/products/import-batches/*', (route) =>
    route.fulfill({
      status: 200, contentType: 'application/json',
      body: '{"batch_id":"00000000-0000-4000-8000-000000000032","item_count":1,"replayed":true}',
    }))
  page.on('dialog', (dialog) => void dialog.accept())
  await login(page, 'gerente@sistema.local')
  await upload(page, sample)
  await page.getByRole('button', { name: /Confirmar importação de 1 produto/ }).click()
  await expect(page.getByRole('region', { name: 'Importação de produtos pendente' })).toBeVisible()
  await page.reload()
  await page.getByRole('button', { name: 'Conferir lote no servidor' }).click()
  await expect(page.getByText(/servidor confirmou o lote/i)).toBeVisible()
  expect(posts).toBe(1)
  await expect(page.getByRole('region', { name: 'Importação de produtos pendente' })).toHaveCount(0)
})

test('operador de caixa não pode consultar nem enviar lote de produtos', async ({ page }) => {
  await login(page, 'caixa@sistema.local')
  const statuses = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    const check = async (path: string, init?: { method: string; headers: Record<string, string>; body: unknown }) => {
      try {
        await apiJson(path, init)
        return 200
      } catch (error) {
        if (error instanceof APIError) return error.status
        throw error
      }
    }
    return {
      get: await check('/api/v1/products/import-batches/12345678-0000-4000-8000-000000000032'),
      post: await check('/api/v1/products/import-batches', {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: { items: [] },
      }),
    }
  })
  expect(statuses).toEqual({ get: 403, post: 403 })
})
