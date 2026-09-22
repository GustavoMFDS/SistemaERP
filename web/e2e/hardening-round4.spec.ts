import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
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
      payments: [{ method: 'pix', amount: product.price_cash }],
    }
    const sale = await apiJson<{ id: string }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': originalKey },
      body: originalBody,
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
    await apiJson(`/api/v1/sales/${sale.id}/cancel`, {
      method: 'POST',
      body: { reason: 'round4 cleanup' },
    })
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
  context: _context,
}) => {
  await login(page)

  await page.route('http://127.0.0.1:8080/api/v1/auth/logout', async (route) => {
    await route.abort('failed')
  })

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/products$/)
  await expect(page.getByText(/Não foi possível encerrar a sessão no servidor/)).toBeVisible()

  await page.unroute('http://127.0.0.1:8080/api/v1/auth/logout')
  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login$/)

})
