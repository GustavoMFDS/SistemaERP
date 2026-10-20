import { expect, test } from '@playwright/test'
import { expandDisclosure } from './expandDisclosure'
import { captureBrowserFailures } from './pageErrors'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
  await page.goto('/products')
  await expect(page.getByRole('heading', { name: 'Produtos da loja' })).toBeVisible()
}

const sampleCSV = 'sku;quantidade\nSTOCK-RESUME-TEST;5,500\n'
const differentCSV = 'sku;quantidade\nSTOCK-DIFFERENT;8\n'

// Match only an individual UUID receipt; never mock the /batches/history listing.
const batchReceiptRoute = /\/api\/v1\/inventory\/opening-stock\/batches\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(?:\?.*)?$/

async function selectFile(page: import('@playwright/test').Page, csv: string) {
  await expandDisclosure(page, 'Cadastrar estoque inicial por planilha')
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
  await page.route(batchReceiptRoute, (route) =>
    route.fulfill({ status: 404, contentType: 'application/json', body: '{"code":"not_found"}' }))
  page.on('dialog', (dialog) => void dialog.accept())
  await login(page, 'gerente@sistema.local')
  await page.getByRole('link', { name: 'Estoque', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Estoque da loja' })).toBeVisible()
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
  const supportReference = region.locator('code')
  await expect(supportReference).toBeHidden()
  await region.getByText('Ver código da tentativa', { exact: true }).click()
  await expect(supportReference).toHaveText(submittedKeys[0])
  await expect(supportReference).toBeVisible()
  await region.getByText('Ver código da tentativa', { exact: true }).click()
  await expect(supportReference).toBeHidden()
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
  const failures = captureBrowserFailures(page)
  let writes = 0
  await page.route('**/api/v1/inventory/opening-stock', async (route) => {
    writes += 1
    await route.abort('failed')
  })
  await page.route(batchReceiptRoute, (route) =>
    route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ batch_id: '00000000-0000-4000-8000-000000000020', item_count: 1, replayed: true }),
    }))
  page.on('dialog', (dialog) => void dialog.accept())
  await login(page, 'gerente@sistema.local')
  await page.getByRole('link', { name: 'Estoque', exact: true }).click()
  try {
    await expect(page.getByRole('heading', { name: 'Estoque da loja' })).toBeVisible()
  } catch (error) {
    const visible = (await page.locator('body').innerText().catch(() => '')).slice(0, 700)
    throw new Error('Estoque não carregou: ' + page.url() + '; tela=' + JSON.stringify(visible) +
      '; navegador=' + failures() + '; causa=' + String(error))
  }
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

test('resposta incompleta do histórico não derruba a tela Estoque', async ({ page }) => {
  await page.route('**/api/v1/inventory/opening-stock/batches/history?*', (route) =>
    route.fulfill({ status: 200, json: { batch_id: 'resposta-incompleta' } }))
  await login(page, 'gerente@sistema.local')
  await page.getByRole('link', { name: 'Estoque', exact: true }).click()
  await expandDisclosure(page, 'Cadastrar estoque inicial por planilha')
  await expect(page.getByRole('alert').filter({ hasText: 'O servidor devolveu dados incompletos' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Estoque da loja' })).toBeVisible()
})
