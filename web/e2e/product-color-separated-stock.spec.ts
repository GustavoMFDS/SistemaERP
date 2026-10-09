import { expect, test } from '@playwright/test'

// Contract test: colors with separate stock are independent product/SKU records.
// Sales, returns and adjustments reuse the existing product ID ledger.
test('catálogo: criar cor com estoque próprio sem dividir saldo existente', async ({ page }) => {
  const parentID = 'b26fa190-864e-41c8-920d-fd0e20d82c19'
  const variantID = 'ac186843-6f1d-4e77-86c3-7a21b4c9d4cb'
  const tenantID = 'c6abae40-7a8a-40c5-95be-a124bb5cb93a'
  const userID = 'e5b97b0b-2f56-4204-942e-583f506e5b9f'
  const token = 'e30.' + Buffer.from(JSON.stringify({
    sub: userID, tenant_id: tenantID, exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url') + '.signature'
  const baseProduct = {
    id: parentID, sku: 'CAD-BASE', name: 'Caderno Brochurão', unit: 'un',
    barcode: '1234567890123', ncm: '48201000', cest: null,
    price_cash: 12.5, cost_price: 0, min_stock: 3, qty_on_hand: 15, active: true,
  }
  let variant: typeof baseProduct | null = null
  let createdPayload: Record<string, unknown> | null = null
  await page.route('**/api/v1/**', (route) => {
    const request = route.request()
    const { pathname, searchParams } = new URL(request.url())
    const reply = (body: unknown, status = 200) => route.fulfill({ status, json: body })
    if (pathname === '/api/v1/auth/login') {
      return reply({ token: { access_token: token }, user: {
        id: userID, email: 'loja@teste.local', name: 'Gerente', roles: ['admin'],
      } })
    }
    if (pathname === '/api/v1/auth/me') {
      return reply({ id: userID, email: 'loja@teste.local', name: 'Gerente',
        roles: ['admin'], permissions: ['product:read', 'product:write'] })
    }
    if (pathname === '/api/v1/products' && request.method() === 'GET') {
      const q = (searchParams.get('query') ?? '').toLocaleLowerCase()
      const result = [baseProduct, ...(variant ? [variant] : [])].filter(
        (product) => product.name.toLocaleLowerCase().includes(q) || product.sku.toLowerCase().includes(q),
      )
      return reply({ items: result, total: result.length })
    }
    if (pathname === '/api/v1/products/' + parentID + '/variations' && request.method() === 'POST') {
      const requestBody = request.postDataJSON() as { option_label: string; product: Record<string, unknown> }
      expect(requestBody.option_label).toBe('Azul')
      createdPayload = requestBody.product
      variant = {
        ...baseProduct, ...createdPayload, id: variantID,
        qty_on_hand: 0, barcode: (createdPayload.barcode as string | null) ?? null,
      } as typeof baseProduct
      return reply({ id: variantID, parent_id: parentID }, 201)
    }
    if ((pathname === '/api/v1/products/' + parentID + '/variations' ||
        pathname === '/api/v1/products/' + variantID + '/variations') && request.method() === 'GET') {
      return reply({ parent_id: parentID, items: [
        { ...baseProduct, option_label: '', is_base: true },
        ...(variant ? [{ ...variant, option_label: 'Azul', is_base: false }] : []),
      ] })
    }
    if (pathname === '/api/v1/products/images/previews') return reply({ items: {} })
    if (pathname === '/api/v1/products/' + variantID + '/images') return reply({ items: [] })
    return reply({ message: 'mock endpoint not implemented' }, 404)
  })
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('loja@teste.local')
  await page.getByLabel('Senha').fill('teste1234')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page.getByRole('heading', { name: 'Produtos da loja' })).toBeVisible()
  const parentRow = page.getByRole('row', { name: /CAD-BASE/ })
  await expect(parentRow).toContainText('15.00')
  page.once('dialog', async (dialog) => { await dialog.accept('Azul') })
  await parentRow.getByRole('button', { name: 'Nova cor/tamanho com estoque próprio' }).click()

  await expect(page.getByText(/O saldo começa zerado/)).toBeVisible()
  await expect(page.getByLabel('Nome do produto')).toHaveValue('Caderno Brochurão — Azul')
  await expect(page.getByLabel('Código do produto (SKU)')).toHaveValue('')
  await expect(page.getByLabel('Código de barras', { exact: true })).toHaveValue('')
  await expect(page.getByLabel('Classificação fiscal (NCM)')).toHaveValue('')
  await page.getByLabel('Código do produto (SKU)').fill('CAD-AZUL')
  await page.getByRole('button', { name: 'Cadastrar', exact: true }).click()

  await expect.poll(() => createdPayload).not.toBeNull()
  expect(createdPayload).toMatchObject({
    sku: 'CAD-AZUL', name: 'Caderno Brochurão — Azul',
    barcode: null, ncm: null, cost_price: 0, price_cash: 12.5,
  })
  await page.getByRole('textbox', { name: 'Buscar produto' }).fill('Caderno')
  await expect(page.getByRole('row', { name: /CAD-AZUL/ })).toContainText('0.00')
  await expect(page.getByRole('row', { name: /CAD-BASE/ })).toContainText('15.00')
  await page.getByRole('row', { name: /CAD-AZUL/ }).getByRole('button', { name: 'Ver cores/tamanhos' }).click()
  const family = page.getByRole('region', { name: 'Cores e tamanhos do produto' })
  await expect(family).toContainText('CAD-BASE')
  await expect(family).toContainText('CAD-AZUL')
  await expect(family).toContainText('15.00')
  await expect(family).toContainText('0.00')
  // Only one POST must atomically commit both new SKU and family relationship.
  expect(createdPayload).not.toBeNull()
})
