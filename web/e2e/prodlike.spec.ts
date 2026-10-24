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


// Runs against the compiled SPA, real HTTPS reverse proxy and staging API.
// No /src/ imports: Vite sources are deliberately absent from production builds.
test('dois caixas HTTPS isolados nao vendem a mesma ultima unidade de estoque', async ({
  page,
  context,
  browser,
}) => {
  const otherContext = await browser.newContext({
    baseURL: process.env.E2E_BASE_URL,
    ignoreHTTPSErrors: process.env.E2E_IGNORE_HTTPS_ERRORS === '1',
  })
  const otherPage = await otherContext.newPage()
  const login = async (terminal: import('@playwright/test').Page): Promise<string> => {
    await terminal.goto('/login')
    await terminal.getByLabel('E-mail').fill('admin@sistema.local')
    await terminal.getByLabel('Senha', { exact: true }).fill('admin123')
    const waiting = terminal.waitForResponse((response) =>
      response.url().includes('/api/v1/auth/login') && response.request().method() === 'POST')
    await terminal.getByRole('button', { name: 'Entrar' }).click()
    const response = await waiting
    expect(response.ok()).toBe(true)
    const data = (await response.json()) as { token: { access_token: string } }
    expect(data.token.access_token).toBeTruthy()
    await expect(terminal).toHaveURL(/[/]home$/)
    return data.token.access_token
  }

  try {
    const [tokenA, tokenB] = await Promise.all([login(page), login(otherPage)])
    const refreshA = (await context.cookies()).find((cookie) => cookie.name === '__Host-refresh_token')
    const refreshB = (await otherContext.cookies()).find((cookie) => cookie.name === '__Host-refresh_token')
    expect(refreshA?.value).toBeTruthy()
    expect(refreshB?.value).toBeTruthy()
    expect(refreshA?.value).not.toBe(refreshB?.value)
    for (const cookie of [refreshA, refreshB]) {
      expect(cookie?.secure).toBe(true)
      expect(cookie?.httpOnly).toBe(true)
    }

    const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 14)
    const created = await page.request.post('/api/v1/products', {
      headers: { Authorization: 'Bearer ' + tokenA },
      data: {
        category_id: null, sku: 'HTTPS-2CAIXAS-' + suffix,
        barcode: '2CAIXAS-' + suffix, name: 'Produto caixas ' + suffix,
        description: null, unit: 'UN', cost_price: 4,
        price_cash: 10, promo_price: null, min_stock: 0, active: true,
      },
    })
    expect(created.ok()).toBe(true)
    const product = (await created.json()) as { id: string }
    expect(product.id).toBeTruthy()

    const adjusted = await page.request.post('/api/v1/inventory/adjust', {
      headers: { Authorization: 'Bearer ' + tokenA },
      data: {
        product_id: product.id, delta: 1,
        reason: 'Duas vendas simultaneas HTTPS CI', type: 'adjustment',
      },
    })
    expect(adjusted.ok()).toBe(true)

    const opened = await page.request.post('/api/v1/cash/sessions/open', {
      headers: { Authorization: 'Bearer ' + tokenA },
      data: { opening_amount: 0, notes: 'dois caixas HTTPS CI' },
    })
    expect(opened.ok()).toBe(true)
    const cash = (await opened.json()) as { id: string }
    expect(cash.id).toBeTruthy()

    const view = await otherPage.request.get('/api/v1/cash/sessions/current', {
      headers: { Authorization: 'Bearer ' + tokenB },
    })
    expect(view.ok()).toBe(true)
    const current = (await view.json()) as { id: string; status: string }
    expect(current.id).toBe(cash.id)
    expect(current.status).toBe('open')

    const requestData = {
      cash_session_id: cash.id, customer_id: null, discount_value: 0,
      items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
      payments: [{ method: 'cash', amount: 10, cash_received: 20, installments: 1 }],
    }
    const keyA = crypto.randomUUID()
    const keyB = crypto.randomUUID()
    const [saleA, saleB] = await Promise.all([
      page.request.post('/api/v1/sales', {
        headers: { Authorization: 'Bearer ' + tokenA, 'Idempotency-Key': keyA },
        data: requestData,
      }),
      otherPage.request.post('/api/v1/sales', {
        headers: { Authorization: 'Bearer ' + tokenB, 'Idempotency-Key': keyB },
        data: requestData,
      }),
    ])
    const wins = [saleA, saleB].filter((result) => result.ok())
    const losses = [saleA, saleB].filter((result) => !result.ok())
    expect(wins).toHaveLength(1)
    expect(losses).toHaveLength(1)
    expect([409, 422]).toContain(losses[0].status())
    const winner = (await wins[0].json()) as { id: string; total: number }
    expect(winner.id).toBeTruthy()
    expect(winner.total).toBe(10)

    const stock = await page.request.get('/api/v1/products/' + product.id, {
      headers: { Authorization: 'Bearer ' + tokenA },
    })
    expect(stock.ok()).toBe(true)
    expect(((await stock.json()) as { qty_on_hand: number }).qty_on_hand).toBe(0)

    // Re-submit exactly the acknowledged request from the OTHER terminal.
    // A replay must return the same id without decrementing inventory again.
    const replay = await otherPage.request.post('/api/v1/sales', {
      headers: {
        Authorization: 'Bearer ' + tokenB,
        'Idempotency-Key': saleA.ok() ? keyA : keyB,
      },
      data: requestData,
    })
    expect(replay.ok()).toBe(true)
    expect(((await replay.json()) as { id: string }).id).toBe(winner.id)
    const finalStock = await otherPage.request.get('/api/v1/products/' + product.id, {
      headers: { Authorization: 'Bearer ' + tokenB },
    })
    expect(finalStock.ok()).toBe(true)
    expect(((await finalStock.json()) as { qty_on_hand: number }).qty_on_hand).toBe(0)

    const closed = await page.request.post('/api/v1/cash/sessions/' + cash.id + '/close', {
      headers: { Authorization: 'Bearer ' + tokenA },
      data: { closing_amount: 10, notes: 'fechamento dois caixas HTTPS CI' },
    })
    expect(closed.ok()).toBe(true)
  } finally {
    await otherContext.close()
  }
})
