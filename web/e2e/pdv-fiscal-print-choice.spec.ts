import { expect, test } from '@playwright/test'

test('PDV: escolher não imprimir preserva obrigação fiscal; DANFE não autorizado fica bloqueado', async ({ page }) => {
  const userID = 'cdb8df15-a848-44da-9b7b-605dcd814a3b'
  const tenantID = '06dce31b-7bde-4c10-aab3-2a3864d013f3'
  const cashID = 'e4b7de48-e7a4-4cac-9386-8c53c2cac5b4'
  const productID = '56465036-7aa8-4e3c-bcbf-4c0825c8eace'
  const saleID = '3a527be8-aa74-4d66-987e-bf9598c7710b'
  const token = 'eyJ0eXAiOiJKV1QifQ.' + Buffer.from(JSON.stringify({
    sub: userID, tenant_id: tenantID, exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url') + '.signature'
  let salePosts = 0
  let fiscalReads = 0
  let danfeRequests = 0

  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const reply = (body: unknown, status = 200) => route.fulfill({ status, json: body })
    if (url.pathname === '/api/v1/auth/login') return reply({
      token: { access_token: token }, user: {
        id: userID, name: 'Caixa', email: 'caixa@teste.local', roles: ['cashier'],
      },
    })
    if (url.pathname === '/api/v1/auth/refresh') return reply({ access_token: token })
    if (url.pathname === '/api/v1/auth/me') return reply({
      id: userID, name: 'Caixa', email: 'caixa@teste.local',
      permissions: ['sale:write', 'product:read', 'cash:open'],
    })
    if (url.pathname === '/api/v1/products') return reply({ items: [{
      id: productID, sku: 'TEST-PRINT', name: 'Caderno', unit: 'un',
      barcode: null, price_cash: 12.5, promo_price: null,
      qty_on_hand: 10, active: true,
    }], total: 1 })
    if (url.pathname === '/api/v1/products/images/previews') return reply({ items: {} })
    if (url.pathname === `/api/v1/products/${productID}/variations`) return reply({
      parent_id: productID, items: [{
        id: productID, sku: 'TEST-PRINT', name: 'Caderno', unit: 'un',
        barcode: null, price_cash: 12.5, promo_price: null,
        qty_on_hand: 10, active: true, option_label: '', is_base: true,
      }],
    })
    if (url.pathname === '/api/v1/cash/sessions/current') return reply({
      id: cashID, cash_register_id: cashID, opened_by_user_id: userID,
      opening_amount: 0, status: 'open',
    })
    if (url.pathname === '/api/v1/sales' && route.request().method() === 'POST') {
      salePosts += 1
      expect(route.request().headers()['idempotency-key']).toBeTruthy()
      return reply({ id: saleID, status: 'finalized', total: 12.5 }, 201)
    }
    if (url.pathname === `/api/v1/sales/${saleID}/fiscal-status`) {
      fiscalReads += 1
      return reply({
        sale_id: saleID, required: true, document_kind: 'nfce',
        status: 'pending', authorized: false, printable: false,
        legacy_review: false,
      })
    }
    if (url.pathname === `/api/v1/sales/${saleID}/fiscal-danfe`) {
      danfeRequests += 1
      return reply({ message: 'DANFE indisponível' }, 409)
    }
    return reply({ message: 'test endpoint unavailable' }, 404)
  })
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('caixa@teste.local')
  await page.getByLabel('Senha').fill('senha-ficticia')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await page.getByRole('link', { name: 'PDV', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Produtos da venda' })).toBeVisible()
  await page.getByRole('combobox', { name: 'Produto', exact: true }).selectOption(productID)
  await page.getByRole('button', { name: 'Adicionar', exact: true }).click()
  await page.getByRole('button', { name: 'Finalizar', exact: true }).click()
  await expect(page.getByText(/Venda finalizada:/)).toBeVisible()
  await expect(page.getByText('O cliente deseja imprimir o DANFE NFC-e?')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Sim, abrir para imprimir' })).toBeDisabled()
  await expect(page.getByText(/NFC-e ainda não autorizada/)).toBeVisible()
  expect(salePosts).toBe(1)
  expect(fiscalReads).toBeGreaterThan(0)

  await page.getByRole('button', { name: 'Não imprimir' }).click()
  await expect(page.getByRole('button', { name: 'Não imprimir' })).toHaveCount(0)
  await expect(page.getByText(/Pendência fiscal permanece registrada/)).toBeVisible()
  expect(danfeRequests).toBe(0)
  expect(salePosts).toBe(1)
})
