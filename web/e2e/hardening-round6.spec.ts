import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email = 'admin@sistema.local') {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('cash movements and per-method reconciliation stay consistent and closed sales cannot be cancelled', async ({
  page,
}) => {
  await login(page)

  const result = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')

    const products = await apiJson<{
      items: Array<{ id: string; active: boolean; price_cash: number }>
    }>('/api/v1/products?limit=200&offset=0')
    const product = products.items.find((item) => item.active)
    if (!product) throw new Error('seeded product missing')

    const opening = 100
    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: opening, notes: 'round6 reconciliation' },
    })

    const supply = await apiJson<{ id: string }>(
      `/api/v1/cash/sessions/${cash.id}/movements`,
      {
        method: 'POST',
        body: { movement_type: 'supply', amount: 20, notes: 'round6 supply' },
      },
    )

    const total = product.price_cash
    const cashPart = Math.min(5, Math.max(0.01, Math.round((total / 2) * 100) / 100))
    const pixPart = Math.round((total - cashPart) * 100) / 100
    const sale = await apiJson<{ id: string }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': `round6-${crypto.randomUUID()}` },
      body: {
        cash_session_id: cash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
        payments: [
          { method: 'cash', amount: cashPart },
          { method: 'pix', amount: pixPart },
        ],
      },
    })

    const withdrawal = await apiJson<{ id: string }>(
      `/api/v1/cash/sessions/${cash.id}/movements`,
      {
        method: 'POST',
        body: { movement_type: 'withdrawal', amount: 10, notes: 'round6 withdrawal' },
      },
    )

    const expectedCash = Math.round((opening + 20 - 10 + cashPart) * 100) / 100
    const declaredCash = Math.round((expectedCash - 1.25) * 100) / 100
    const declaredPix = Math.round(Math.max(0, pixPart - 0.1) * 100) / 100

    const close = await apiJson<{
      status: string
      expected_cash: number
      closing_amount: number
      closing_difference: number
      expected_by_method: Record<string, number>
      declared_by_method: Record<string, number>
      difference_by_method: Record<string, number>
    }>(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: {
        closing_amount: declaredCash,
        closing_by_method: {
          pix: declaredPix,
          debit: 0,
          credit: 0,
          transfer: 0,
          voucher: 0,
        },
        notes: 'round6 close',
      },
    })

    let cancelStatus = 0
    try {
      await apiJson(`/api/v1/sales/${sale.id}/cancel`, {
        method: 'POST',
        body: { reason: 'must be blocked after cash close' },
      })
      cancelStatus = 200
    } catch (error) {
      if (error instanceof APIError) cancelStatus = error.status
      else throw error
    }

    const audit = await apiJson<{
      items: Array<{ action: string; resource_id?: string | null }>
    }>('/api/v1/audit/logs?limit=200&offset=0')
    const ledger = await apiJson<{
      items: Array<{
        entry_type: string
        cash_session_id?: string | null
        amount_net: number
      }>
    }>('/api/v1/finance/ledger?limit=200&offset=0')

    const count = (action: string, resourceId: string) =>
      audit.items.filter(
        (item) => item.action === action && item.resource_id === resourceId,
      ).length

    return {
      cashPart,
      pixPart,
      expectedCash,
      declaredCash,
      declaredPix,
      close,
      cancelStatus,
      auditCounts: {
        open: count('cash.open', cash.id),
        supply: count('cash.supply', supply.id),
        sale: count('sale.create', sale.id),
        withdrawal: count('cash.withdrawal', withdrawal.id),
        close: count('cash.close', cash.id),
      },
      ledgerMovements: {
        supply: ledger.items.find(
          (item) => item.entry_type === 'supply' && item.cash_session_id === cash.id,
        )?.amount_net,
        withdrawal: ledger.items.find(
          (item) => item.entry_type === 'withdrawal' && item.cash_session_id === cash.id,
        )?.amount_net,
      },
    }
  })

  expect(result.close.status).toBe('closed')
  expect(result.close.expected_cash).toBeCloseTo(result.expectedCash, 2)
  expect(result.close.closing_amount).toBeCloseTo(result.declaredCash, 2)
  expect(result.close.closing_difference).toBeCloseTo(-1.25, 2)
  expect(result.close.expected_by_method.cash).toBeCloseTo(result.expectedCash, 2)
  expect(result.close.expected_by_method.pix).toBeCloseTo(result.pixPart, 2)
  expect(result.close.declared_by_method.pix).toBeCloseTo(result.declaredPix, 2)
  expect(result.close.difference_by_method.pix).toBeCloseTo(-0.1, 2)
  expect(result.cancelStatus).toBe(409)
  expect(result.auditCounts).toEqual({
    open: 1,
    supply: 1,
    sale: 1,
    withdrawal: 1,
    close: 1,
  })
  expect(result.ledgerMovements.supply).toBeCloseTo(20, 2)
  expect(result.ledgerMovements.withdrawal).toBeCloseTo(-10, 2)
})

test('cashier cannot perform cash supply or withdrawal without cash:move permission', async ({
  page,
}) => {
  await login(page, 'caixa@sistema.local')

  const status = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'round6 cashier RBAC' },
    })

    let movementStatus = 0
    try {
      await apiJson(`/api/v1/cash/sessions/${cash.id}/movements`, {
        method: 'POST',
        body: { movement_type: 'supply', amount: 10, notes: 'must be forbidden' },
      })
      movementStatus = 201
    } catch (error) {
      if (error instanceof APIError) movementStatus = error.status
      else throw error
    }

    await apiJson(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: { closing_amount: 0, notes: 'round6 cashier cleanup' },
    })
    return movementStatus
  })

  expect(status).toBe(403)
})
