import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
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

test('double finalize intent produces only one sale request', async ({ page }) => {
  await login(page)

  const before = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ total: number }>('/api/v1/sales?limit=200&offset=0')
  })

  await page.getByRole('link', { name: 'PDV' }).click()
  await page.getByRole('button', { name: 'Abrir' }).click()
  await page.getByLabel('Produto').selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Adicionar' }).click()

  let salePosts = 0
  const keys: string[] = []
  await page.route('http://127.0.0.1:8080/api/v1/sales', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.continue()
      return
    }
    salePosts += 1
    keys.push(route.request().headers()['idempotency-key'] ?? '')
    await new Promise((resolve) => setTimeout(resolve, 400))
    await route.continue()
  })

  await page.getByRole('button', { name: 'Finalizar' }).evaluate((element) => {
    const button = element as HTMLButtonElement
    button.click()
    button.click()
  })

  await expect(page.getByText(/Venda finalizada:/)).toBeVisible()
  expect(salePosts).toBe(1)
  expect(keys).toHaveLength(1)
  expect(keys[0]).not.toBe('')

  const after = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ total: number }>('/api/v1/sales?limit=200&offset=0')
  })
  expect(after.total).toBe(before.total + 1)

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await waitForCashClosed(page)
})

test('rebinding an already committed legacy sale preserves key and cannot duplicate it', async ({
  page,
}) => {
  await login(page)

  const result = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    const auth = await import('/src/lib/auth.ts')
    const queue = await import('/src/lib/offlineQueue.ts')

    const before = await apiJson<{ total: number }>('/api/v1/sales?limit=200&offset=0')
    const products = await apiJson<{
      items: Array<{ id: string; active: boolean; price_cash: number }>
    }>('/api/v1/products?limit=200&offset=0')
    const product = products.items.find((item) => item.active)
    if (!product) throw new Error('seeded product missing')

    const firstCash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'round4 original cash' },
    })

    const originalKey = `legacy-committed-${crypto.randomUUID()}`
    const originalBody = {
      cash_session_id: firstCash.id,
      customer_id: null,
      discount_value: 0,
      items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
      payments: [{ method: 'cash', amount: product.price_cash }],
    }
    const sale = await apiJson<{ id: string }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': originalKey },
      body: originalBody,
    })
    await apiJson(`/api/v1/sales/${sale.id}/cancel`, {
      method: 'POST',
      body: { reason: 'round4 pre-close cleanup' },
    })
    await apiJson(`/api/v1/cash/sessions/${firstCash.id}/close`, {
      method: 'POST',
      body: { closing_amount: 0, notes: 'round4 original close' },
    })

    const secondCash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'round4 rebind cash' },
    })

    const itemID = queue.enqueueRequest({
      method: 'POST',
      path: '/api/v1/sales',
      body: originalBody,
      headers: { 'Idempotency-Key': originalKey },
    })

    const storageKey = auth.scopedStorageKey('sistemaemgo:offlineQueue:v2')
    if (!storageKey) throw new Error('queue storage key missing')
    const stored = JSON.parse(localStorage.getItem(storageKey) ?? '[]') as Array<{
      id: string
      state?: string
      attentionReason?: string
      headers?: Record<string, string>
      body?: { cash_session_id?: string }
    }>
    const item = stored.find((candidate) => candidate.id === itemID)
    if (!item) throw new Error('queued item missing')
    item.state = 'attention'
    item.attentionReason = 'legacy_migration'
    localStorage.setItem(storageKey, JSON.stringify(stored))

    if (!queue.rebindQueueItemToCashSession(itemID, secondCash.id)) {
      throw new Error('rebind failed')
    }

    const rebound = queue.getQueueItems().find((candidate) => candidate.id === itemID)
    const preservedKey = rebound?.headers?.['Idempotency-Key'] ?? ''
    const reboundCash =
      rebound?.body && typeof rebound.body === 'object'
        ? (rebound.body as { cash_session_id?: string }).cash_session_id
        : undefined

    const flushed = await queue.flushQueue()
    const after = await apiJson<{ total: number }>('/api/v1/sales?limit=200&offset=0')

    queue.discardQueueItem(itemID)
    await apiJson(`/api/v1/cash/sessions/${secondCash.id}/close`, {
      method: 'POST',
      body: { closing_amount: 0, notes: 'round4 cleanup' },
    })

    return {
      beforeTotal: before.total,
      afterTotal: after.total,
      originalKey,
      preservedKey,
      reboundCash,
      secondCashID: secondCash.id,
      flushAttention: flushed.attention,
    }
  })

  expect(result.preservedKey).toBe(result.originalKey)
  expect(result.reboundCash).toBe(result.secondCashID)
  expect(result.afterTotal).toBe(result.beforeTotal + 1)
  expect(result.flushAttention).toBeGreaterThanOrEqual(1)
})

test('network failure during logout does not pretend the HttpOnly session was revoked', async ({
  page,
}) => {
  await login(page)

  await page.getByRole('link', { name: 'PDV' }).click()
  await page.getByLabel('Produto').selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Adicionar' }).click()
  await page.getByRole('button', { name: 'Suspender' }).click()
  const suspendedBefore = await page.evaluate(async () => {
    const { getSuspendedCarts } = await import('/src/lib/suspendedCart.ts')
    return getSuspendedCarts().length
  })
  expect(suspendedBefore).toBe(1)

  await page.route('http://127.0.0.1:8080/api/v1/auth/logout', async (route) => {
    await route.abort('failed')
  })

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/pdv$/)
  await expect(page.getByText(/Não foi possível encerrar a sessão no servidor/)).toBeVisible()

  const suspendedAfterFailure = await page.evaluate(async () => {
    const { getSuspendedCarts } = await import('/src/lib/suspendedCart.ts')
    return getSuspendedCarts().length
  })
  expect(suspendedAfterFailure).toBe(1)

  await page.unroute('http://127.0.0.1:8080/api/v1/auth/logout')
  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login$/)

  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
  const suspendedAfterSuccess = await page.evaluate(async () => {
    const { getSuspendedCarts } = await import('/src/lib/suspendedCart.ts')
    return getSuspendedCarts().length
  })
  expect(suspendedAfterSuccess).toBe(1)

})


test('logout is blocked while offline finalized sales are preserved', async ({ page }) => {
  await login(page)

  const queuedId = await page.evaluate(async () => {
    const queue = await import('/src/lib/offlineQueue.ts')
    return queue.enqueueRequest({
      method: 'POST',
      path: '/api/v1/sales',
      body: {
        cash_session_id: 'e2e-offline-cash-placeholder',
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: 'e2e-product-placeholder', qty: 1, discount_value: 0 }],
        payments: [{ method: 'pix', amount: 1 }],
      },
      headers: { 'Idempotency-Key': crypto.randomUUID() },
    })
  })

  let logoutCalls = 0
  await page.route('http://127.0.0.1:8080/api/v1/auth/logout', async (route) => {
    logoutCalls += 1
    await route.continue()
  })

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/products$/)
  await expect(
    page.getByText(/Não é possível sair enquanto existirem 1 venda\(s\) offline preservada\(s\)/),
  ).toBeVisible()
  expect(logoutCalls).toBe(0)

  await page.evaluate(async (id) => {
    const queue = await import('/src/lib/offlineQueue.ts')
    if (!queue.discardQueueItem(id)) throw new Error('queued item was not discarded')
  }, queuedId)

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login$/)
  expect(logoutCalls).toBe(1)
})


test('legacy offline queue blocks cash close until explicitly reviewed', async ({ page }) => {
  await login(page)

  await page.getByRole('link', { name: 'PDV' }).click()
  await page.getByRole('button', { name: 'Abrir' }).click()

  const cashSessionId = await page.evaluate(async () => {
    const { getCashSessionId } = await import('/src/lib/auth.ts')
    return getCashSessionId()
  })
  expect(cashSessionId).not.toBe('')

  await page.evaluate((cashId) => {
    localStorage.setItem(
      'sistemaemgo:offlineQueue:v1',
      JSON.stringify([
        {
          id: 'legacy-close-guard',
          createdAt: Date.now(),
          method: 'POST',
          path: '/api/v1/sales',
          body: {
            cash_session_id: cashId,
            customer_id: null,
            discount_value: 0,
            items: [],
            payments: [],
          },
          headers: { 'Idempotency-Key': 'legacy-close-guard' },
        },
      ]),
    )
  }, cashSessionId)

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await expect(page.getByText(/fila offline legada ainda não revisada/)).toBeVisible()

  const stillOpen = await page.evaluate(async () => {
    const { getCashSessionId } = await import('/src/lib/auth.ts')
    return getCashSessionId()
  })
  expect(stillOpen).toBe(cashSessionId)

  await page.evaluate(async () => {
    const { discardLegacyQueue } = await import('/src/lib/offlineQueue.ts')
    discardLegacyQueue()
  })

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await waitForCashClosed(page)
})


test('logout is blocked while legacy offline work is unresolved', async ({ page }) => {
  await login(page)

  await page.evaluate(() => {
    localStorage.setItem(
      'sistemaemgo:offlineQueue:v1',
      JSON.stringify([
        {
          id: 'legacy-logout-guard',
          createdAt: Date.now(),
          method: 'POST',
          path: '/api/v1/sales',
          body: {
            cash_session_id: 'legacy-unscoped-cash',
            customer_id: null,
            discount_value: 0,
            items: [],
            payments: [],
          },
          headers: { 'Idempotency-Key': 'legacy-logout-guard' },
        },
      ]),
    )
  })

  let logoutCalls = 0
  await page.route('http://127.0.0.1:8080/api/v1/auth/logout', async (route) => {
    logoutCalls += 1
    await route.continue()
  })

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/products$/)
  await expect(page.getByText(/fila offline legada ainda não revisada/)).toBeVisible()
  expect(logoutCalls).toBe(0)

  await page.evaluate(async () => {
    const { discardLegacyQueue } = await import('/src/lib/offlineQueue.ts')
    discardLegacyQueue()
  })

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login$/)
  expect(logoutCalls).toBe(1)
})
