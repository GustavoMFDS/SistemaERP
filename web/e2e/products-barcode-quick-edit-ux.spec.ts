import { expect, test } from '@playwright/test'

test('código de barras pode ser salvo sem abrir os campos fiscais', async ({ page }) => {
  const productID = 'b26fa190-864e-41c8-920d-fd0e20d82c19'
  const userID = 'e5b97b0b-2f56-4204-942e-583f506e5b9f'
  const tenantID = 'c6abae40-7a8a-40c5-95be-a124bb5cb93a'
  const token = 'e30.' + Buffer.from(JSON.stringify({
    sub: userID, tenant_id: tenantID, exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url') + '.signature'
  const product = {
    id: productID, sku: 'BAR-001', name: 'Produto com leitor',
    unit: 'un', price_cash: 12.5, cost_price: 8,
    min_stock: 2, qty_on_hand: 7, active: true,
    barcode: null, ncm: '12345678', cest: '1234567', return_policy: 'allowed',
  }
  let savedBarcode = ''
  let savedPrice = 0
  let savedPolicy = ''
  let savedNCM = ''
  let savedCEST = ''

  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const method = route.request().method()
    const reply = (data: unknown, status = 200) => route.fulfill({ status, json: data })
    if (path === '/api/v1/auth/login' && method === 'POST') {
      return reply({
        token: { access_token: token },
        user: { id: userID, name: 'Gestor de teste', email: 'teste@loja.local', roles: ['admin'] },
      })
    }
    if (path === '/api/v1/auth/refresh') return reply({ access_token: token })
    if (path === '/api/v1/auth/me') {
      return reply({
        id: userID, name: 'Gestor de teste', email: 'teste@loja.local',
        roles: ['admin'], permissions: ['product:read', 'product:write', 'inventory:read'],
      })
    }
    if (path === '/api/v1/products' && method === 'GET') {
      return reply({ items: [{ ...product, barcode: savedBarcode || null }], total: 1 })
    }
    if (path === '/api/v1/products/images/previews') return reply({ items: {} })
    if (path === '/api/v1/products/' + productID && method === 'PUT') {
      const body = route.request().postDataJSON() as { barcode: string; price_cash: number; return_policy: string; ncm: string; cest: string }
      savedBarcode = body.barcode
      savedPrice = body.price_cash
      savedPolicy = body.return_policy
      savedNCM = body.ncm
      savedCEST = body.cest
      return reply({ id: productID })
    }
    return reply({ message: 'endpoint de teste não disponível' }, 404)
  })

  await page.goto('/login')
  await page.getByLabel('E-mail').fill('teste@loja.local')
  await page.getByLabel('Senha', { exact: true }).fill('senha-ficticia')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
  await page.getByRole('navigation', { name: 'Navegação principal' }).getByRole('link', { name: 'Produtos', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Produtos da loja' })).toBeVisible()
  try {
    await expect(page.getByRole('columnheader', { name: 'Código de barras' })).toBeVisible()
  } catch (error) {
    console.error('ProductsPage debug:', (await page.locator('body').innerText()).slice(0, 2800))
    throw error
  }
  await expect(page.getByRole('columnheader', { name: 'Classificação (NCM)' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Editar códigos fiscais' }).click()
  await page.getByPlaceholder('8 dígitos').fill('87654321')
  await page.getByLabel('Código de barras de Produto com leitor').fill('7891234567895')
  await page.getByRole('button', { name: 'Salvar código de barras de Produto com leitor' }).click()
  await expect.poll(() => savedBarcode).toBe('7891234567895')
  expect(savedPrice).toBe(12.5)
  expect(savedPolicy).toBe('allowed')
  expect(savedNCM).toBe('12345678')
  expect(savedCEST).toBe('1234567')
  await expect(page.getByPlaceholder('8 dígitos')).toHaveValue('87654321')
  await expect(page.getByLabel('Código de barras de Produto com leitor')).toHaveValue('7891234567895')
})
