import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

const sampleCSV = 'sku;quantidade\nSTOCK-RESUME-TEST;5,500\n'
const differentCSV = 'sku;quantidade\nSTOCK-DIFFERENT;8\n'

async function selectFile(page: import('@playwright/test').Page, csv: string) {
  await page.getByLabel('Escolher planilha CSV').setInputFiles({
    name: 'estoque.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from(csv, 'utf8'),
  })
}

test('resposta ambígua mantém chave; recarga só aceita CSV idêntico e replay não duplica', async ({ page }) => {
  const submittedKeys: string[] = []
  let successful = false
  await page.route('**/api/v1/inventory/opening-stock', async (route) => {
    if (route.request().method() !== 'POST') return route.continue()
    submittedKeys.push(route.request().headers()['idempotency-key'])
    if (!successful) {
      successful = true
      await route.abort('failed')
      return
    }
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ batch_id: '00000000-0000-4000-8000-000000000010', item_count: 1, replayed: true }),
    })
  })
  await page.route('**/api/v1/inventory/opening-stock/batches/*', (route) =>
    route.fulfill({ status: 404, contentType: 'application/json', body: '{"code":"not_found"}' }))
  page.on('dialog', (dialog) => void dialog.accept())
  await login(page, 'gerente@sistema.local')
  await page.goto('/inventory')
  await selectFile(page, sampleCSV)
  await expect(page.getByText(/1 válida\(s\), 0 com erro/)).toBeVisible()
  await page.getByLabel('Conferi os códigos e as quantidades').check()
  await page.getByRole('button', { name: /Confirmar estoque inicial de 1 produto/ }).click()
  await expect(page.getByRole('region', { name: 'Importação pendente de confirmação' })).toBeVisible()
  await expect(page.getByText(/referência original foi preservada/i)).toBeVisible()

  const pendingRaw = await page.evaluate(() =>
    Object.entries(localStorage).filter(([key]) => key.startsWith('sistemaemgo:pendingOpeningStock:v1')),
  )
  expect(pendingRaw).toHaveLength(1)
  expect(pendingRaw[0][1]).not.toContain('STOCK-RESUME-TEST')
  expect(pendingRaw[0][1]).not.toContain('5.500')

  await page.reload()
  const region = page.getByRole('region', { name: 'Importação pendente de confirmação' })
  await expect(region).toBeVisible()
  await region.getByRole('button', { name: 'Conferir lote no servidor' }).click()
  await expect(page.getByText(/servidor ainda não encontrou/i)).toBeVisible()
  await selectFile(page, differentCSV)
  await expect(page.getByText(/Há uma importação anterior pendente/)).toBeVisible()
  await expect(page.getByRole('button', { name: /Confirmar estoque inicial de 1 produto/ })).toHaveCount(0)

  await selectFile(page, sampleCSV)
  await page.getByLabel('Conferi os códigos e as quantidades').check()
  await page.getByRole('button', { name: /Confirmar estoque inicial de 1 produto/ }).click()
  await expect(page.getByText(/Nenhum estoque foi lançado novamente/i)).toBeVisible()
  expect(submittedKeys).toHaveLength(2)
  expect(submittedKeys[0]).toBe(submittedKeys[1])
  await expect(region).toHaveCount(0)
  const remaining = await page.evaluate(() =>
    Object.keys(localStorage).filter((key) => key.startsWith('sistemaemgo:pendingOpeningStock:v1')),
  )
  expect(remaining).toEqual([])
})

test('consulta de lote confirmado encerra recuperação sem novo POST', async ({ page }) => {
  let writes = 0
  await page.route('**/api/v1/inventory/opening-stock', async (route) => {
    writes += 1
    await route.abort('failed')
  })
  await page.route('**/api/v1/inventory/opening-stock/batches/*', (route) =>
    route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ batch_id: '00000000-0000-4000-8000-000000000020', item_count: 1, replayed: true }),
    }))
  page.on('dialog', (dialog) => void dialog.accept())
  await login(page, 'gerente@sistema.local')
  await page.goto('/inventory')
  await selectFile(page, sampleCSV)
  await page.getByLabel('Conferi os códigos e as quantidades').check()
  await page.getByRole('button', { name: /Confirmar estoque inicial de 1 produto/ }).click()
  await expect(page.getByRole('region', { name: 'Importação pendente de confirmação' })).toBeVisible()
  await page.reload()
  await page.getByRole('button', { name: 'Conferir lote no servidor' }).click()
  await expect(page.getByText(/servidor confirmou o lote/i)).toBeVisible()
  expect(writes).toBe(1)
  await expect(page.getByRole('region', { name: 'Importação pendente de confirmação' })).toHaveCount(0)
})

test('caixa não tem permissão para consultar lotes de estoque inicial', async ({ page }) => {
  await login(page, 'caixa@sistema.local')
  const status = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/inventory/opening-stock/batches/00000000-0000-4000-8000-000000000010')
      return 200
    } catch (e) {
      if (e instanceof APIError) return e.status
      throw e
    }
  })
  expect(status).toBe(403)
})
