import { expect, test } from '@playwright/test'

// Browser-only contract test for the gallery and its API boundary. It does NOT
// replace a database-backed multi-tenant/integration test.
test('catálogo: foto opcional, miniatura e exclusão sem afetar estoque', async ({ page }) => {
  const productId = 'b26fa190-864e-41c8-920d-fd0e20d82c19'
  const imageId = 'd24edc98-0c12-4e2d-a036-75409af97b0b'
  const tenantID = 'c6abae40-7a8a-40c5-95be-a124bb5cb93a'
  const userID = 'e5b97b0b-2f56-4204-942e-583f506e5b9f'
  const tokenPayload = Buffer.from(JSON.stringify({
    sub: userID, tenant_id: tenantID, exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url')
  const token = 'e30.' + tokenPayload + '.signature'

  let photoBase64 = ''
  let attempts = 0
  const product = {
    id: productId, sku: 'FOTO-001', name: 'Caderno Brochurão 96 folhas',
    unit: 'un', price_cash: 12.5, cost_price: 0, min_stock: 5,
    qty_on_hand: 15, active: true, barcode: null,
  }
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const path = url.pathname
    const method = route.request().method()
    const reply = (payload: unknown, status = 200) => route.fulfill({ status, json: payload })
    if (path === '/api/v1/auth/login' && method === 'POST') {
      return reply({ token: { access_token: token }, user: { id: userID, email: 'teste@loja.local', name: 'Loja Teste', roles: ['admin'] } })
    }
    if (path === '/api/v1/auth/me') {
      return reply({ id: userID, email: 'teste@loja.local', name: 'Loja Teste',
        roles: ['admin'], permissions: ['product:read', 'product:write', 'inventory:read'] })
    }
    if (path === '/api/v1/products' && method === 'GET') {
      return reply({ items: [product], total: 1 })
    }
    if (path === '/api/v1/products/images/previews') {
      return reply({ items: photoBase64 ? { [productId]: 'data:image/jpeg;base64,' + photoBase64 } : {} })
    }
    if (path === '/api/v1/products/' + productId + '/images' && method === 'GET') {
      return reply({ items: photoBase64 ? [{
        id: imageId, data_url: 'data:image/jpeg;base64,' + photoBase64,
        thumbnail_url: 'data:image/jpeg;base64,' + photoBase64, principal: true,
      }] : [] })
    }
    if (path === '/api/v1/products/' + productId + '/images' && method === 'POST') {
      const key = route.request().headers()['idempotency-key']
      const body = route.request().postDataJSON() as { image_base64: string; thumbnail_base64: string }
      expect(key).toMatch(/^[0-9a-f-]{36}$/)
      expect(body.image_base64.length).toBeGreaterThan(50)
      expect(body.thumbnail_base64.length).toBeGreaterThan(50)
      attempts += 1
      photoBase64 = body.image_base64
      return reply({ id: imageId, replayed: false }, 201)
    }
    if (path === '/api/v1/products/' + productId + '/images/' + imageId && method === 'DELETE') {
      photoBase64 = ''
      return route.fulfill({ status: 204, body: '' })
    }
    return reply({ message: 'mock endpoint not implemented' }, 404)
  })

  await page.goto('/login')
  await page.getByLabel('E-mail').fill('teste@loja.local')
  await page.getByLabel('Senha').fill('teste1234')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page.getByRole('heading', { name: 'Produtos da loja' })).toBeVisible()
  await page.getByRole('button', { name: 'Fotos', exact: true }).click()
  const gallery = page.getByRole('region', { name: 'Fotos de Caderno Brochurão 96 folhas' })
  await expect(gallery.getByText('Este produto ainda não tem fotos.')).toBeVisible()

  const pngBase64 = await page.evaluate(() => {
    const canvas = document.createElement('canvas')
    canvas.width = 4
    canvas.height = 4
    const ctx = canvas.getContext('2d')!
    ctx.fillStyle = '#1f75cb'
    ctx.fillRect(0, 0, 4, 4)
    return canvas.toDataURL('image/png').split(',')[1]
  })
  await gallery.locator('input[type=file]').setInputFiles({
    name: 'caderno-azul.png', mimeType: 'image/png', buffer: Buffer.from(pngBase64, 'base64'),
  })
  await expect(gallery.getByText('Foto principal')).toBeVisible()
  expect(attempts).toBe(1)
  await expect(page.getByRole('row', { name: /FOTO-001/ }).locator('img')).toHaveCount(1)
  await expect(page.getByRole('row', { name: /FOTO-001/ })).toContainText('15.00')

  page.once('dialog', async (dialog) => { await dialog.accept() })
  await gallery.getByRole('button', { name: 'Excluir' }).click()
  await expect(gallery.getByText('Este produto ainda não tem fotos.')).toBeVisible()
  await expect(page.getByRole('row', { name: /FOTO-001/ })).toContainText('15.00')
})
