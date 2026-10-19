import { expect, test } from '@playwright/test'

test('PDV valida dinheiro, troco e revalida aprovações após mudanças no carrinho', async ({ page }) => {
  // Isolated browser contract: no payment gateway, real receipt or fiscal authorization.
  const userID = 'cdb8df15-a848-44da-9b7b-605dcd814a3b'
  const tenantID = '06dce31b-7bde-4c10-aab3-2a3864d013f3'
  const cashID = 'e4b7de48-e7a4-4cac-9386-8c53c2cac5b4'
  const productID = '56465036-7aa8-4e3c-bcbf-4c0825c8eace'
  const saleID = '3a527be8-aa74-4d66-987e-bf9598c7710b'
  const token = 'eyJ0eXAiOiJKV1QifQ.' + Buffer.from(JSON.stringify({
    sub: userID, tenant_id: tenantID, exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url') + '.signature'
  let salePosts = 0
  let lastSale: { items: Array<{ qty: number }>; payments: Array<{ method: string; amount: number }> } | null = null

  await page.route('**/api/v1/**', (route) => {
    const url = new URL(route.request().url())
    const reply = (data: unknown, status = 200) => route.fulfill({ status, json: data })
    if (url.pathname === '/api/v1/auth/login') return reply({
      token: { access_token: token },
      user: { id: userID, name: 'Operador', email: 'caixa@teste.local', roles: ['cashier'] },
    })
    if (url.pathname === '/api/v1/auth/refresh') return reply({ access_token: token })
    if (url.pathname === '/api/v1/auth/me') return reply({
      id: userID, name: 'Operador', email: 'caixa@teste.local',
      permissions: ['sale:write', 'product:read', 'cash:open'],
    })
    if (url.pathname === '/api/v1/products') return reply({ items: [{
      id: productID, sku: 'TEST-TROCO', name: 'Caderno', unit: 'un',
      barcode: null, price_cash: 12.50, promo_price: null, qty_on_hand: 10, active: true,
    }], total: 1 })
    if (url.pathname === '/api/v1/products/images/previews') return reply({ items: {} })
    if (url.pathname === `/api/v1/products/${productID}/variations`) return reply({
      parent_id: productID, items: [{
        id: productID, sku: 'TEST-TROCO', name: 'Caderno', unit: 'un',
        barcode: null, price_cash: 12.50, promo_price: null,
        qty_on_hand: 10, active: true, option_label: '', is_base: true,
      }],
    })
    if (url.pathname === '/api/v1/cash/sessions/current') return reply({
      id: cashID, cash_register_id: cashID, opened_by_user_id: userID,
      opening_amount: 0, status: 'open',
    })
    if (url.pathname === `/api/v1/cash/sessions/${cashID}/printer`) return reply({
      cash_register_id: cashID, paper_width_mm: 80,
    })
    if (url.pathname === '/api/v1/sales' && route.request().method() === 'POST') {
      salePosts += 1
      expect(route.request().headers()['idempotency-key']).toBeTruthy()
      lastSale = route.request().postDataJSON() as typeof lastSale
      return reply({ id: saleID, status: 'finalized', total: 25 }, 201)
    }
    if (url.pathname === `/api/v1/sales/${saleID}/fiscal-status`) return reply({
      sale_id: saleID, required: true, document_kind: 'nfce',
      status: 'pending', authorized: false, printable: false, legacy_review: false,
    })
    return reply({ message: 'test endpoint unavailable' }, 404)
  })

  await page.goto('/login')
  await page.getByLabel('E-mail').fill('caixa@teste.local')
  await page.getByLabel('Senha').fill('senha-ficticia')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await page.getByRole('link', { name: 'PDV', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Produtos da venda' })).toBeVisible()

  const finalize = page.getByRole('button', { name: 'Finalizar venda', exact: true })
  await expect(finalize).toHaveCount(1)
  await expect(finalize).toBeDisabled()
  await page.getByRole('combobox', { name: 'Produto', exact: true }).selectOption(productID)
  await page.getByRole('button', { name: 'Adicionar', exact: true }).click()

  // Money is the one method which needs no separate confirmation.
  await page.getByLabel('Forma de pagamento').selectOption('cash')
  const cashReceived = page.getByLabel('Valor recebido em dinheiro (R$)')
  await cashReceived.fill('12.49')
  await expect(finalize).toBeDisabled()
  await cashReceived.fill('12.345')
  await expect(finalize).toBeDisabled()
  await cashReceived.fill('12,345')
  await expect(finalize).toBeDisabled()
  await cashReceived.fill('12.500,00')
  await expect(finalize).toBeDisabled()
  await cashReceived.fill('1e3')
  await expect(finalize).toBeDisabled()
  await cashReceived.fill('20,00')
  await expect(page.getByRole('status').filter({ hasText: 'Troco: R$ 7.50' })).toBeVisible()
  await expect(finalize).toBeEnabled()
  await expect(page.getByRole('checkbox')).toHaveCount(0)

  // Editing quantity invalidates even an already sufficient cash tender.
  const qty = page.getByLabel(/Quantidade de .*Caderno/)
  await qty.fill('2')
  await expect(cashReceived).toHaveValue('')
  await expect(finalize).toBeDisabled()
  await cashReceived.fill('30')
  await expect(page.getByRole('status').filter({ hasText: 'Troco: R$ 5.00' })).toBeVisible()
  await expect(finalize).toBeEnabled()

  // Each electronic payment needs explicit operator confirmation. A previous
  // approval cannot be reused after a method or cart change.
  await page.getByLabel('Forma de pagamento').selectOption('pix')
  await expect(finalize).toBeDisabled()
  const pix = page.getByRole('checkbox', { name: 'Confirmei que o Pix foi recebido.' })
  await pix.check()
  await expect(finalize).toBeEnabled()

  await page.getByLabel('Forma de pagamento').selectOption('credit')
  await expect(finalize).toBeDisabled()
  const card = page.getByRole('checkbox', { name: 'Confirmei que o pagamento foi aprovado na máquina.' })
  await expect(card).not.toBeChecked()
  await card.check()
  await expect(finalize).toBeEnabled()

  await qty.fill('3')
  await expect(card).not.toBeChecked()
  await expect(finalize).toBeDisabled()
  await qty.fill('2')
  await expect(card).not.toBeChecked()
  await expect(finalize).toBeDisabled()
  await page.getByLabel('Forma de pagamento').selectOption('pix')
  await expect(pix).not.toBeChecked()
  await expect(finalize).toBeDisabled()
  expect(salePosts).toBe(0)

  // Re-selecting money requires a new amount, then sends only the sale total
  // (not tendered cash) as payment; the difference is change, not revenue.
  await page.getByLabel('Forma de pagamento').selectOption('cash')
  await expect(cashReceived).toHaveValue('')
  await expect(finalize).toBeDisabled()
  await cashReceived.fill('30,00')
  await expect(page.getByRole('status').filter({ hasText: 'Troco: R$ 5.00' })).toBeVisible()
  await finalize.click()
  await expect(page.getByText(/Venda finalizada:/)).toBeVisible()
  expect(salePosts).toBe(1)
  expect(lastSale).toMatchObject({
    items: [{ qty: 2 }],
    payments: [{ method: 'cash', amount: 25 }],
  })
  await expect(page.getByRole('button', { name: 'Sim, abrir para imprimir' })).toBeDisabled()
})
