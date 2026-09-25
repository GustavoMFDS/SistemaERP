import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

async function waitForCashOpen(page: import('@playwright/test').Page) {
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .not.toBe('')
}

async function waitForCashClosed(page: import('@playwright/test').Page) {
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .toBe('')
}

test('write-ahead storage failure prevents any sale request from leaving the browser', async ({
  page,
}) => {
  await login(page)
  await page.getByRole('link', { name: 'PDV' }).click()
  await page.getByRole('button', { name: 'Abrir' }).click()
  await waitForCashOpen(page)
  await page.getByLabel('Produto').selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Adicionar' }).click()

  let salePosts = 0
  await page.route('http://127.0.0.1:8080/api/v1/sales', async (route) => {
    if (route.request().method() === 'POST') salePosts += 1
    await route.continue()
  })

  await page.evaluate(() => {
    const w = window as typeof window & { __originalSetItem?: Storage['setItem'] }
    w.__originalSetItem = Storage.prototype.setItem
    Storage.prototype.setItem = function (key: string, value: string) {
      if (key.startsWith('sistemaemgo:offlineQueue:v2:')) {
        throw new DOMException('quota unavailable', 'QuotaExceededError')
      }
      return w.__originalSetItem!.call(this, key, value)
    }
  })

  await page.getByRole('button', { name: 'Finalizar' }).click()
  await expect(page.getByText(/Nenhuma venda foi enviada/)).toBeVisible()
  expect(salePosts).toBe(0)

  await page.evaluate(() => {
    const w = window as typeof window & { __originalSetItem?: Storage['setItem'] }
    if (w.__originalSetItem) Storage.prototype.setItem = w.__originalSetItem
    delete w.__originalSetItem
  })
  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await waitForCashClosed(page)
})

test('sales API rejects a request without Idempotency-Key', async ({ page }) => {
  await login(page)

  const result = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    const products = await apiJson<{
      items: Array<{ id: string; active: boolean; price_cash: number }>
    }>('/api/v1/products?limit=200&offset=0')
    const product = products.items.find((item) => item.active)
    if (!product) throw new Error('seeded product missing')

    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'round5 missing-idempotency' },
    })

    let status = 0
    try {
      await apiJson('/api/v1/sales', {
        method: 'POST',
        body: {
          cash_session_id: cash.id,
          customer_id: null,
          discount_value: 0,
          items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
          payments: [{ method: 'pix', amount: product.price_cash }],
        },
      })
      status = 201
    } catch (error) {
      if (error instanceof APIError) status = error.status
      else throw error
    }

    await apiJson(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: { closing_amount: 0, notes: 'round5 cleanup' },
    })
    return status
  })

  expect(result).toBe(422)
})

test('cash close persists expected cash, difference and audit evidence', async ({ page }) => {
  await login(page)

  const result = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')

    const products = await apiJson<{
      items: Array<{ id: string; active: boolean; price_cash: number }>
    }>('/api/v1/products?limit=200&offset=0')
    const product = products.items.find((item) => item.active)
    if (!product) throw new Error('seeded product missing')

    const opening = 100
    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: opening, notes: 'round5 reconciliation' },
    })

    await apiJson<{ id: string }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': `round5-cash-${crypto.randomUUID()}` },
      body: {
        cash_session_id: cash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
        payments: [{ method: 'cash', amount: product.price_cash }],
      },
    })

    const expected = Math.round((opening + product.price_cash) * 100) / 100
    const declared = Math.round((expected - 1.25) * 100) / 100
    const close = await apiJson<{
      status: string
      expected_cash: number
      closing_amount: number
      closing_difference: number
    }>(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: { closing_amount: declared, notes: 'round5 reconciliation close' },
    })

    const audit = await apiJson<{
      items: Array<{
        action: string
        resource_id?: string | null
        metadata?: Record<string, unknown>
      }>
    }>('/api/v1/audit/logs?limit=200&offset=0')

    return {
      expected,
      declared,
      close,
      openAudit: audit.items.some(
        (item) => item.action === 'cash.open' && item.resource_id === cash.id,
      ),
      closeAudit: audit.items.some(
        (item) =>
          item.action === 'cash.close' &&
          item.resource_id === cash.id &&
          item.metadata?.closing_difference === '-1.25',
      ),
    }
  })

  expect(result.close.status).toBe('closed')
  expect(result.close.expected_cash).toBeCloseTo(result.expected, 2)
  expect(result.close.closing_amount).toBeCloseTo(result.declared, 2)
  expect(result.close.closing_difference).toBeCloseTo(-1.25, 2)
  expect(result.openAudit).toBe(true)
  expect(result.closeAudit).toBe(true)
})
