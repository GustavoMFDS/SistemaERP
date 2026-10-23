import { expect, test } from '@playwright/test'

test.skip(process.env.E2E_PRODLIKE !== '1', 'runs only in the production-like CI job')

function cashSessionFromStorage(): string {
  const key = Object.keys(localStorage).find((candidate) =>
    candidate.startsWith('sistemaemgo:cashSession:v2:'),
  )
  return key ? (localStorage.getItem(key) ?? '') : ''
}

test('staging-like HTTPS proxy keeps refresh cookie secure and core POS flow works', async ({
  page,
  context,
}) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  const cookies = await context.cookies()
  const refresh = cookies.find((cookie) => cookie.name === '__Host-refresh_token')
  expect(refresh).toBeTruthy()
  expect(refresh?.secure).toBe(true)
  expect(refresh?.httpOnly).toBe(true)
  expect(refresh?.sameSite).toBe('Strict')

  const reminder = page.getByRole('dialog', { name: 'Sua configuração fiscal ainda não está concluída' })
  await expect(reminder).toBeVisible()
  await reminder.getByRole('button', { name: 'Agora não' }).click()

  await page.getByRole('navigation', { name: 'Navegação principal' }).getByRole('link', { name: 'PDV', exact: true }).click()
  await page.getByRole('button', { name: 'Abrir' }).click()

  await expect
    .poll(async () => page.evaluate(cashSessionFromStorage))
    .not.toBe('')

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await expect
    .poll(async () => page.evaluate(cashSessionFromStorage))
    .toBe('')

  await page.route('https://staging.example.test:8443/api/v1/auth/logout', async (route) => {
    await route.abort('failed')
  })
  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).not.toHaveURL(/\/login$/)
  await expect(page.getByText(/Não foi possível confirmar a saída com segurança/)).toBeVisible()

  const cookieAfterFailedLogout = (await context.cookies()).find(
    (cookie) => cookie.name === '__Host-refresh_token',
  )
  expect(cookieAfterFailedLogout).toBeTruthy()

  await page.unroute('https://staging.example.test:8443/api/v1/auth/logout')
  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login$/)

  const cookieAfterSuccessfulLogout = (await context.cookies()).find(
    (cookie) => cookie.name === '__Host-refresh_token',
  )
  expect(cookieAfterSuccessfulLogout).toBeUndefined()

  await context.addCookies([
    {
      name: '__Host-refresh_token',
      value: 'invalid-refresh-token',
      url: 'https://staging.example.test:8443',
      secure: true,
      httpOnly: true,
      sameSite: 'Strict',
    },
  ])
  await page.goto('/products')
  await expect(page).toHaveURL(/\/login$/)

  const invalidCookieAfterRefreshFailure = (await context.cookies()).find(
    (cookie) => cookie.name === '__Host-refresh_token',
  )
  expect(invalidCookieAfterRefreshFailure).toBeUndefined()
})


test('dois caixas HTTPS isolados nao vendem a mesma ultima unidade de estoque', async ({
  page,
  context,
  browser,
}) => {
  // Two genuinely independent browser contexts: no shared cookies, session
  // storage or localStorage. The HTTPS proxy, Go API, PostgreSQL and Redis
  // are real disposable CI services, not mocked API responses.
  const secondContext = await browser.newContext({
    baseURL: process.env.E2E_BASE_URL,
    ignoreHTTPSErrors: process.env.E2E_IGNORE_HTTPS_ERRORS === '1',
  })
  const secondPage = await secondContext.newPage()
  const login = async (terminal: import('@playwright/test').Page) => {
    await terminal.goto('/login')
    await terminal.getByLabel('E-mail').fill('admin@sistema.local')
    await terminal.getByLabel('Senha', { exact: true }).fill('admin123')
    await terminal.getByRole('button', { name: 'Entrar' }).click()
    await expect(terminal).toHaveURL(/\\/home$/)
  }

  try {
    await Promise.all([login(page), login(secondPage)])

    const firstCookies = await context.cookies()
    const secondCookies = await secondContext.cookies()
    const refreshA = firstCookies.find((cookie) => cookie.name === '__Host-refresh_token')
    const refreshB = secondCookies.find((cookie) => cookie.name === '__Host-refresh_token')
    expect(refreshA?.value).toBeTruthy()
    expect(refreshB?.value).toBeTruthy()
    expect(refreshA?.value).not.toBe(refreshB?.value)
    expect(refreshA?.secure).toBe(true)
    expect(refreshB?.secure).toBe(true)
    expect(refreshA?.httpOnly).toBe(true)
    expect(refreshB?.httpOnly).toBe(true)

    const setup = await page.evaluate(async () => {
      const { apiJson } = await import('/src/lib/api.ts')
      const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 14)
      const product = await apiJson<{ id: string }>('/api/v1/products', {
        method: 'POST',
        body: {
          category_id: null, sku: `HTTPS-2CAIXAS-${suffix}`,
          barcode: `2CAIXAS-${suffix}`, name: `Produto caixas ${suffix}`,
          description: null, unit: 'UN', cost_price: 4,
          price_cash: 10, promo_price: null, min_stock: 0, active: true,
        },
      })
      await apiJson('/api/v1/inventory/adjust', {
        method: 'POST',
        body: {
          product_id: product.id, delta: 1,
          reason: 'Duas vendas simultaneas HTTPS CI', type: 'adjustment',
        },
      })
      const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
        method: 'POST',
        body: { opening_amount: 0, notes: 'dois caixas HTTPS CI' },
      })
      return { productId: product.id, cashId: cash.id }
    })

    const secondView = await secondPage.evaluate(async () => {
      const { apiJson } = await import('/src/lib/api.ts')
      return apiJson<{ id: string; status: string }>('/api/v1/cash/sessions/current')
    })
    expect(secondView.id).toBe(setup.cashId)
    expect(secondView.status).toBe('open')

    const sell = (terminal: import('@playwright/test').Page, key: string) =>
      terminal.evaluate(async ({ id, cashId, idempotencyKey }) => {
        const { APIError, apiJson } = await import('/src/lib/api.ts')
        try {
          const response = await apiJson<{ id: string; total: number }>('/api/v1/sales', {
            method: 'POST',
            headers: { 'Idempotency-Key': idempotencyKey },
            body: {
              cash_session_id: cashId, customer_id: null, discount_value: 0,
              items: [{ product_id: id, qty: 1, discount_value: 0 }],
              payments: [{
                method: 'cash', amount: 10, cash_received: 20, installments: 1,
              }],
            },
          })
          return { ok: true, id: response.id, total: response.total, key: idempotencyKey }
        } catch (error) {
          if (error instanceof APIError) return { ok: false, status: error.status }
          throw error
        }
      }, { id: setup.productId, cashId: setup.cashId, idempotencyKey: key })

    // Concurrent independent browser requests must serialize stock deduction.
    // The only available unit may be sold once, never oversold or booked twice.
    const [a, b] = await Promise.all([
      sell(page, crypto.randomUUID()),
      sell(secondPage, crypto.randomUUID()),
    ])
    const successful = [a, b].filter((result) => result.ok)
    const rejected = [a, b].filter((result) => !result.ok)
    expect(successful).toHaveLength(1)
    expect(rejected).toHaveLength(1)
    expect([409, 422]).toContain(rejected[0].status)
    expect(successful[0].total).toBe(10)
    expect(successful[0].id).toBeTruthy()

    const product = await page.evaluate(async (id) => {
      const { apiJson } = await import('/src/lib/api.ts')
      return apiJson<{ qty_on_hand: number }>(`/api/v1/products/${id}`)
    }, setup.productId)
    expect(product.qty_on_hand).toBe(0)

    // Retrying an acknowledged sale in the other browser is idempotent:
    // same sale id, still zero inventory and no duplicate booking.
    const retry = await sell(secondPage, successful[0].key!)
    expect(retry.ok).toBe(true)
    expect(retry.id).toBe(successful[0].id)
    const remaining = await secondPage.evaluate(async (id) => {
      const { apiJson } = await import('/src/lib/api.ts')
      return apiJson<{ qty_on_hand: number }>(`/api/v1/products/${id}`)
    }, setup.productId)
    expect(remaining.qty_on_hand).toBe(0)

    await page.evaluate(async (cashId) => {
      const { apiJson } = await import('/src/lib/api.ts')
      await apiJson(`/api/v1/cash/sessions/${cashId}/close`, {
        method: 'POST',
        body: { closing_amount: 10, notes: 'fechamento dois caixas HTTPS CI' },
      })
    }, setup.cashId)
  } finally {
    await secondContext.close()
  }
})
