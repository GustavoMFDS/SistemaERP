import { expect, test } from '@playwright/test'

// A slower previous request must not replace the result of the latest search.
test('catálogo mantém resultado da busca mais recente quando respostas chegam fora de ordem', async ({ page }) => {
  const fakeToken = 'e30.' + Buffer.from(JSON.stringify({
    sub: 'e5b97b0b-2f56-4204-942e-583f506e5b9f',
    tenant_id: 'c6abae40-7a8a-40c5-95be-a124bb5cb93a',
    exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url') + '.signature'
  const makeProduct = (sku: string, name: string) => ({
    id: sku === 'CAD-OLD' ? 'd8572bf0-a12c-426b-8e55-dcc437e48bad' : 'f5a22ee0-c239-47f0-9fe3-6540d2354d90',
    sku, name, unit: 'un', barcode: null, ncm: null, cest: null,
    price_cash: 10, cost_price: 0, min_stock: 0, qty_on_hand: 3, active: true,
  })

  let notifyOld!: () => void
  const oldRequested = new Promise<void>((resolve) => { notifyOld = resolve })
  let releaseOld: (() => Promise<void>) | undefined

  await page.route('**/api/v1/**', async (route) => {
    const { pathname, searchParams } = new URL(route.request().url())
    const respond = (data: unknown, status = 200) => route.fulfill({ status, json: data })
    if (pathname === '/api/v1/auth/login') {
      return respond({ token: { access_token: fakeToken }, user: {
        id: 'e5b97b0b-2f56-4204-942e-583f506e5b9f',
        email: 'gestor@teste.local', name: 'Gestor', roles: ['admin'],
      } })
    }
    if (pathname === '/api/v1/auth/me') {
      return respond({ roles: ['admin'], permissions: ['product:read', 'product:write'] })
    }
    if (pathname === '/api/v1/products' && route.request().method() === 'GET') {
      const q = (searchParams.get('query') ?? '').toLowerCase()
      if (q === 'caderno') {
        notifyOld()
        await new Promise<void>((resolve) => {
          releaseOld = async () => {
            await respond({ items: [makeProduct('CAD-OLD', 'Caderno antigo')], total: 1 })
            resolve()
          }
        })
        return
      }
      if (q === 'estojo') {
        return respond({ items: [makeProduct('EST-NEW', 'Estojo mais recente')], total: 1 })
      }
      return respond({ items: [], total: 0 })
    }
    if (pathname === '/api/v1/products/images/previews') return respond({ items: {} })
    return respond({ message: 'not mocked' }, 404)
  })

  await page.goto('/login')
  await page.getByLabel('E-mail').fill('gestor@teste.local')
  await page.getByLabel('Senha', { exact: true }).fill('teste1234')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
  await page.getByRole('navigation', { name: 'Navegação principal' })
    .getByRole('link', { name: 'Produtos', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Produtos da loja' })).toBeVisible()
  const search = page.getByRole('textbox', { name: 'Buscar produto' })
  await search.fill('Caderno')
  await oldRequested
  await search.fill('Estojo')
  await expect(page.getByRole('row', { name: /EST-NEW/ })).toBeVisible()

  if (!releaseOld) throw new Error('Old request was not held for replay')
  const oldResponse = page.waitForResponse((r) =>
    r.url().includes('/api/v1/products?query=Caderno'),
  )
  await releaseOld()
  await oldResponse
  await page.evaluate(() => new Promise<void>((resolve) => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  }))
  await expect(page.getByRole('row', { name: /EST-NEW/ })).toBeVisible()
  await expect(page.getByRole('row', { name: /CAD-OLD/ })).toHaveCount(0)
})
