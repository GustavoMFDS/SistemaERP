import { expect, test } from '@playwright/test'

async function login(
  page: import('@playwright/test').Page,
  email = 'admin@sistema.local',
) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('PDV supports shortcuts, quick search, suspended carts, quantity editing and authorized discount', async ({ page }) => {
  await login(page)
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 14)

  const setup = await page.evaluate(async (suffix) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    const me = await apiJson<{ permissions: string[] }>('/api/v1/auth/me')
    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-PDV-${suffix}`,
        barcode: `BAR-${suffix}`,
        name: `Produto Operacional ${suffix}`,
        description: null,
        unit: 'UN',
        cost_price: 4,
        price_cash: 10,
        promo_price: 8,
        min_stock: 0,
        active: true,
      },
    })
    let reservedMovementStatus = 0
    try {
      await apiJson('/api/v1/inventory/adjust', {
        method: 'POST',
        body: {
          product_id: product.id,
          delta: 1,
          reason: 'Tentativa de forjar compra',
          type: 'purchase',
        },
      })
      reservedMovementStatus = 200
    } catch (error) {
      if (error instanceof APIError) reservedMovementStatus = error.status
      else throw error
    }

    await apiJson('/api/v1/inventory/adjust', {
      method: 'POST',
      body: {
        product_id: product.id,
        delta: 5,
        reason: 'Carga E2E operacional',
        type: 'adjustment',
      },
    })
    const productDetail = await apiJson<{ cost_price: number }>(`/api/v1/products/${product.id}`)
    const adjustmentAudit = await apiJson<{
      items: Array<{ action: string; resource_id: string }>
    }>('/api/v1/audit/logs?action=inventory.adjust&resource_type=product&limit=200&offset=0')
    return {
      productId: product.id,
      barcode: `BAR-${suffix}`,
      adminCostPrice: productDetail.cost_price,
      inventoryAdjustAuditCount: adjustmentAudit.items.filter((item) => item.resource_id === product.id).length,
      reservedMovementStatus,
      canDiscount: me.permissions.includes('sale:discount'),
    }
  }, suffix)

  expect(setup.canDiscount).toBe(true)
  expect(setup.adminCostPrice).toBe(4)
  expect(setup.inventoryAdjustAuditCount).toBe(1)
  expect(setup.reservedMovementStatus).toBe(422)

  await page.getByRole('link', { name: 'PDV' }).click()
  await expect(page).toHaveURL(/\/pdv$/)

  const openResponsePromise = page.waitForResponse(
    (response) =>
      response.url().includes('/api/v1/cash/sessions/open') &&
      response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: 'Abrir' }).click()
  const openResponse = await openResponsePromise
  expect(openResponse.ok()).toBe(true)
  const opened = (await openResponse.json()) as { id: string }

  const recoveredCash = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ id: string; status: string }>('/api/v1/cash/sessions/current')
  })
  expect(recoveredCash.id).toBe(opened.id)
  expect(recoveredCash.status).toBe('open')

  const movementIdempotency = await page.evaluate(async (cashSessionId) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    const key = crypto.randomUUID()
    const path = `/api/v1/cash/sessions/${cashSessionId}/movements`
    const body = { movement_type: 'supply', amount: 1, notes: null }

    const first = await apiJson<{ id: string; replayed: boolean }>(path, {
      method: 'POST',
      headers: { 'Idempotency-Key': key },
      body,
    })
    const replay = await apiJson<{ id: string; replayed: boolean }>(path, {
      method: 'POST',
      headers: { 'Idempotency-Key': key },
      body,
    })

    let changedPayloadStatus = 0
    try {
      await apiJson(path, {
        method: 'POST',
        headers: { 'Idempotency-Key': key },
        body: { ...body, amount: 2 },
      })
      changedPayloadStatus = 200
    } catch (error) {
      if (error instanceof APIError) changedPayloadStatus = error.status
      else throw error
    }

    return {
      firstId: first.id,
      firstReplayed: first.replayed,
      replayId: replay.id,
      replayReplayed: replay.replayed,
      changedPayloadStatus,
    }
  }, opened.id)

  expect(movementIdempotency.firstReplayed).toBe(false)
  expect(movementIdempotency.replayReplayed).toBe(true)
  expect(movementIdempotency.replayId).toBe(movementIdempotency.firstId)
  expect(movementIdempotency.changedPayloadStatus).toBe(409)

  const invalidPaymentSemanticsStatus = await page.evaluate(
    async ({ cashSessionId, productId }) => {
      const { APIError, apiJson } = await import('/src/lib/api.ts')
      try {
        await apiJson('/api/v1/sales', {
          method: 'POST',
          headers: { 'Idempotency-Key': crypto.randomUUID() },
          body: {
            cash_session_id: cashSessionId,
            customer_id: null,
            discount_value: 0,
            items: [{ product_id: productId, qty: 1, discount_value: 0 }],
            payments: [{ method: 'pix', amount: 8, installments: 2 }],
          },
        })
        return 200
      } catch (error) {
        if (error instanceof APIError) return error.status
        throw error
      }
    },
    { cashSessionId: opened.id, productId: setup.productId },
  )
  expect(invalidPaymentSemanticsStatus).toBe(422)

  await page.keyboard.press('F4')
  const quickSearch = page.getByLabel('Busca rápida por nome, SKU ou código')
  await expect(quickSearch).toBeFocused()
  await quickSearch.fill(`E2E-PDV-${suffix}`)

  await page.getByLabel('Produto').selectOption(setup.productId)
  await page.getByLabel('Qtd').fill('1')
  await page.getByRole('button', { name: 'Adicionar' }).click()

  const qtyInput = page.getByLabel(/Quantidade de E2E-PDV-/)
  await expect(qtyInput).toHaveValue('1')
  await page.getByRole('button', { name: /Aumentar quantidade de E2E-PDV-/ }).click()
  await expect(qtyInput).toHaveValue('2')

  const discount = page.getByLabel('Desconto da venda (R$)')
  await expect(discount).toBeVisible()
  await discount.fill('2')
  await expect(page.getByText('R$ 14.00', { exact: true })).toBeVisible()

  await page.getByRole('button', { name: 'Suspender' }).click()
  await expect(page.getByText('Vendas suspensas')).toBeVisible()
  await expect(page.getByText('Nenhum item.')).toBeVisible()

  await page.evaluate(
    async ({ suffix, productId }) => {
      const { apiJson } = await import('/src/lib/api.ts')
      await apiJson(`/api/v1/products/${productId}`, {
        method: 'PUT',
        body: {
          category_id: null,
          sku: `E2E-PDV-${suffix}`,
          barcode: `BAR-${suffix}`,
          name: `Produto Operacional ${suffix}`,
          description: null,
          unit: 'UN',
          cost_price: 4,
          price_cash: 10,
          promo_price: 7,
          min_stock: 0,
          active: true,
        },
      })
    },
    { suffix, productId: setup.productId },
  )

  await page.getByRole('button', { name: 'Retomar' }).click()
  await expect(page.getByLabel(/Quantidade de E2E-PDV-/)).toHaveValue('2')
  await expect(page.getByLabel('Desconto da venda (R$)')).toHaveValue('2')
  await expect(page.getByText('R$ 12.00', { exact: true })).toBeVisible()
  await expect(page.getByText('A venda suspensa foi retomada com os preços atuais do catálogo.')).toBeVisible()

  await page.keyboard.press('F2')
  await expect(page.getByLabel('Código de barras')).toBeFocused()

  const saleResponsePromise = page.waitForResponse(
    (response) =>
      response.url().endsWith('/api/v1/sales') &&
      response.request().method() === 'POST',
  )
  await page.keyboard.press('F8')
  const saleResponse = await saleResponsePromise
  expect(saleResponse.ok()).toBe(true)
  const sale = (await saleResponse.json()) as { id: string; total: number }
  expect(sale.total).toBe(12)

  const adminSaleDetail = await page.evaluate(async (saleId) => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{
      sale: { profit_estimated: number }
      items: Array<{ cost_unit: number }>
    }>(`/api/v1/sales/${saleId}`)
  }, sale.id)
  expect(adminSaleDetail.sale.profit_estimated).toBeGreaterThan(0)
  expect(adminSaleDetail.items[0].cost_unit).toBe(4)

  await expect(page.getByText(/Venda finalizada:/)).toBeVisible()
  const printButton = page.getByRole('button', { name: 'Imprimir comprovante não fiscal' })
  await expect(printButton).toBeVisible()

  const popupPromise = page.waitForEvent('popup')
  await printButton.click()
  const receiptPage = await popupPromise
  await expect(receiptPage.getByText('COMPROVANTE NÃO FISCAL', { exact: true })).toBeVisible()
  await expect(receiptPage.getByText(`Venda: ${sale.id}`, { exact: true })).toBeVisible()
  await expect(receiptPage.getByText('TOTAL R$ 12.00', { exact: true })).toBeVisible()
  await receiptPage.close()

  await page.evaluate(
    async ({ cashId }) => {
      const { apiJson } = await import('/src/lib/api.ts')
      await apiJson(`/api/v1/cash/sessions/${cashId}/close`, {
        method: 'POST',
        body: {
          closing_amount: 0,
          closing_by_method: {
            pix: 12,
            debit: 0,
            credit: 0,
            transfer: 0,
            voucher: 0,
          },
          notes: 'cleanup PDV operations E2E',
        },
      })
    },
    { cashId: opened.id },
  )

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login$/)

  await login(page, 'caixa@sistema.local')
  await expect(page.getByRole('heading', { name: 'Cadastrar produto' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Salvar' })).toHaveCount(0)

  await page.getByRole('link', { name: 'Estoque' }).click()
  await expect(page).toHaveURL(/\/inventory$/)
  await expect(page.getByRole('heading', { name: 'Ajuste de estoque' })).toHaveCount(0)

  const cashierResult = await page.evaluate(async (input) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    const me = await apiJson<{ permissions: string[] }>('/api/v1/auth/me')
    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'cashier discount guard E2E' },
    })

    let status = 0
    try {
      await apiJson('/api/v1/sales', {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          cash_session_id: cash.id,
          customer_id: null,
          discount_value: 1,
          items: [{ product_id: input.productId, qty: 1, discount_value: 0 }],
          payments: [{ method: 'pix', amount: 9 }],
        },
      })
      status = 200
    } catch (error) {
      if (error instanceof APIError) status = error.status
      else throw error
    }

    await apiJson(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: {
        closing_amount: 0,
        closing_by_method: {
          pix: 0,
          debit: 0,
          credit: 0,
          transfer: 0,
          voucher: 0,
        },
        notes: 'cleanup cashier guard E2E',
      },
    })

    const productDetail = await apiJson<{ cost_price: number }>(`/api/v1/products/${input.productId}`)
    const barcodeDetail = await apiJson<{ cost_price: number }>(`/api/v1/products/barcode/${encodeURIComponent(input.barcode)}`)
    const saleDetail = await apiJson<{
      sale: { profit_estimated: number }
      items: Array<{ cost_unit: number }>
    }>(`/api/v1/sales/${input.saleId}`)

    const forbiddenStatus = async (path: string) => {
      try {
        await apiJson(path)
        return 200
      } catch (error) {
        if (error instanceof APIError) return error.status
        throw error
      }
    }

    return {
      hasDiscountPermission: me.permissions.includes('sale:discount'),
      hasFinancePermission: me.permissions.includes('finance:read'),
      discountedSaleStatus: status,
      productCost: productDetail.cost_price,
      barcodeCost: barcodeDetail.cost_price,
      saleProfit: saleDetail.sale.profit_estimated,
      saleCostUnit: saleDetail.items[0].cost_unit,
      financePaymentsStatus: await forbiddenStatus('/api/v1/finance/payments?limit=1&offset=0'),
      financeRefundsStatus: await forbiddenStatus('/api/v1/finance/refunds?limit=1&offset=0'),
      returnsStatus: await forbiddenStatus('/api/v1/returns?limit=1&offset=0'),
    }
  }, { productId: setup.productId, barcode: setup.barcode, saleId: sale.id })

  expect(cashierResult.hasDiscountPermission).toBe(false)
  expect(cashierResult.hasFinancePermission).toBe(false)
  expect(cashierResult.discountedSaleStatus).toBe(403)
  expect(cashierResult.productCost).toBe(0)
  expect(cashierResult.barcodeCost).toBe(0)
  expect(cashierResult.saleProfit).toBe(0)
  expect(cashierResult.saleCostUnit).toBe(0)
  expect(cashierResult.financePaymentsStatus).toBe(403)
  expect(cashierResult.financeRefundsStatus).toBe(403)
  expect(cashierResult.returnsStatus).toBe(403)
})
