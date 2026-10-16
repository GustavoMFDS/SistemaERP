import { expect, test } from '@playwright/test'

// UI contract only: backend multi-company isolation is covered by integration.
test('PDV: escolher SKU e saldo da cor sem trocar o produto original', async ({ page }) => {
  const baseID = 'b26fa190-864e-41c8-920d-fd0e20d82c19'
  const blueID = 'ac186843-6f1d-4e77-86c3-7a21b4c9d4cb'
  const tenantID = 'c6abae40-7a8a-40c5-95be-a124bb5cb93a'
  const userID = 'e5b97b0b-2f56-4204-942e-583f506e5b9f'
  const token = 'e30.' + Buffer.from(JSON.stringify({
    sub: userID, tenant_id: tenantID, exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url') + '.signature'
  const base = {
    id: baseID, sku: 'CAD-BASE', name: 'Caderno Brochurão', unit: 'un',
    barcode: null, price_cash: 12.5, promo_price: null,
    qty_on_hand: 15, active: true, option_label: '', is_base: true,
  }
  const blue = {
    id: blueID, sku: 'CAD-AZUL', name: 'Caderno Brochurão — Azul', unit: 'un',
    barcode: null, price_cash: 12.5, promo_price: null,
    qty_on_hand: 4, active: true, option_label: 'Azul', is_base: false,
  }
  let catalogUnavailable = false
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const reply = (json: unknown, status = 200) => route.fulfill({ status, json })
    if (url.pathname === '/api/v1/auth/login') {
      return reply({ token: { access_token: token }, user: { id: userID,
        name: 'Gerente', email: 'loja@teste.local', roles: ['admin'] } })
    }
    if (url.pathname === '/api/v1/auth/refresh') return reply({ access_token: token })
    if (url.pathname === '/api/v1/auth/me') {
      return reply({ id: userID, name: 'Gerente', roles: ['admin'], email: 'loja@teste.local',
        permissions: ['product:read', 'sale:write', 'cash:open'] })
    }
    if (url.pathname === '/api/v1/products') {
      if (catalogUnavailable) return reply({ message: 'temporarily offline' }, 503)
      return reply({ items: [base], total: 1 })
    }
    if (url.pathname === '/api/v1/products/' + baseID + '/variations' ||
        url.pathname === '/api/v1/products/' + blueID + '/variations') {
      if (catalogUnavailable) return reply({ message: 'temporarily offline' }, 503)
      return reply({ parent_id: baseID, items: [base, blue] })
    }
    if (url.pathname === '/api/v1/products/images/previews') return reply({ items: {} })
    if (url.pathname === '/api/v1/cash/sessions/current') {
      return reply({ message: 'not found' }, 404)
    }
    return reply({ message: 'mock endpoint not implemented' }, 404)
  })
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('loja@teste.local')
  await page.getByLabel('Senha').fill('teste1234')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await page.getByRole('link', { name: 'PDV', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Produtos da venda' })).toBeVisible()
  await page.getByRole('combobox', { name: 'Produto', exact: true }).selectOption(baseID)
  const choice = page.getByRole('combobox', { name: 'Cor ou tamanho para vender' })
  await expect(choice).toBeVisible()
  await expect(choice.locator('option')).toHaveCount(2)
  await expect(choice).toContainText('Azul — CAD-AZUL • estoque 4')
  await choice.selectOption(blueID)
  await page.getByRole('button', { name: 'Adicionar', exact: true }).click()
  await expect(page.getByRole('row', { name: /CAD-AZUL — Caderno Brochurão/ })).toBeVisible()
  await expect(page.getByRole('row', { name: /CAD-BASE — Caderno Brochurão/ })).toHaveCount(0)
  // Variants discovered after the initial catalog fetch must join the scoped
  // 24-hour cache without changing its original freshness timestamp.
  const cached = await page.evaluate(() => {
    const key = Object.keys(localStorage).find((value) => value.includes('productsCache:v2'))
    if (!key) return null
    const value = JSON.parse(localStorage.getItem(key) || 'null') as {
      savedAt: number; items: Array<{ sku: string; id: string }>
    } | null
    return value && { savedAt: value.savedAt, skus: value.items.map((p) => p.sku) }
  })
  expect(cached).not.toBeNull()
  expect(cached?.skus).toContain('CAD-BASE')
  expect(cached?.skus).toContain('CAD-AZUL')

  // Simulate the catalog API becoming unavailable after a refresh. The
  // browser must still find the previously discovered variant in its cache.
  catalogUnavailable = true
  await page.reload()
  await expect(page.getByText(/Catálogo carregado do cache local/)).toBeVisible()
  await expect(page.getByRole('combobox', { name: 'Produto', exact: true })
    .locator('option[value="' + blueID + '"]')).toHaveCount(1)
})
