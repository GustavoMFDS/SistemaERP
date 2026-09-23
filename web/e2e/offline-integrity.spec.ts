import { expect, test } from '@playwright/test'

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

test('lost sale response reuses the original idempotency key', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)

  const before = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ total: number }>('/api/v1/sales?limit=200&offset=0')
  })

  await page.getByRole('link', { name: 'PDV' }).click()
  await page.getByRole('button', { name: 'Abrir' }).click()
  await waitForCashOpen(page)
  await page.getByLabel('Produto').selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Adicionar' }).click()

  const idempotencyKeys: string[] = []
  let loseFirstResponse = true
  await page.route('http://127.0.0.1:8080/api/v1/sales', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.continue()
      return
    }

    idempotencyKeys.push(route.request().headers()['idempotency-key'] ?? '')
    if (loseFirstResponse) {
      loseFirstResponse = false
      const response = await route.fetch()
      expect(response.ok()).toBeTruthy()
      await route.abort('failed')
      return
    }
    await route.continue()
  })

  await page.getByRole('button', { name: 'Finalizar' }).click()
  await expect(page.getByText(/Venda registrada offline/)).toBeVisible()
  await expect(page.getByText(/Pendências: 1/)).toBeVisible()

  const flush = await page.evaluate(async () => {
    const { flushQueue } = await import('/src/lib/offlineQueue.ts')
    return flushQueue()
  })

  expect(flush.ok).toBe(true)
  expect(idempotencyKeys).toHaveLength(2)
  expect(idempotencyKeys[0]).not.toBe('')
  expect(idempotencyKeys[1]).toBe(idempotencyKeys[0])

  const after = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ total: number }>('/api/v1/sales?limit=200&offset=0')
  })
  expect(after.total).toBe(before.total + 1)

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await waitForCashClosed(page)
})

test('offline browser state is isolated by tenant and user', async ({ page }) => {
  await page.goto('/login')

  const result = await page.evaluate(async () => {
    const auth = await import('/src/lib/auth.ts')

    function token(sub: string, tenant: string): string {
      const payload = btoa(JSON.stringify({ sub, tenant_id: tenant }))
        .replace(/\+/g, '-')
        .replace(/\//g, '_')
        .replace(/=+$/, '')
      return `x.${payload}.x`
    }

    auth.setToken(token('user-a', 'tenant-a'))
    const productKeyA = auth.scopedStorageKey('sistemaemgo:productsCache:v2')
    if (!productKeyA) throw new Error('missing tenant A scope')
    localStorage.setItem(productKeyA, JSON.stringify([{ id: 'product-a' }]))
    auth.setCashSessionId('cash-a')

    auth.setToken(token('user-b', 'tenant-b'))
    const productKeyB = auth.scopedStorageKey('sistemaemgo:productsCache:v2')
    if (!productKeyB) throw new Error('missing tenant B scope')

    return {
      productKeyA,
      productKeyB,
      tenantBCache: localStorage.getItem(productKeyB),
      tenantBCash: auth.getCashSessionId(),
      tenantACacheStillStored: localStorage.getItem(productKeyA),
    }
  })

  expect(result.productKeyB).not.toBe(result.productKeyA)
  expect(result.tenantBCache).toBeNull()
  expect(result.tenantBCash).toBe('')
  expect(result.tenantACacheStillStored).toContain('product-a')
})

test('permanent queue conflict does not block later sales and expired items are preserved', async ({ page }) => {
  await page.goto('/login')

  await page.route('http://127.0.0.1:8080/api/v1/sales', async (route) => {
    const body = route.request().postDataJSON() as { cash_session_id?: string }
    if (body.cash_session_id === 'cash-conflict') {
      await route.fulfill({
        status: 409,
        contentType: 'application/json',
        body: JSON.stringify({ message: 'cash session closed' }),
      })
      return
    }
    await route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify({ id: 'sale-ok', status: 'finalized', total: 10 }),
    })
  })

  const result = await page.evaluate(async () => {
    const auth = await import('/src/lib/auth.ts')
    const queue = await import('/src/lib/offlineQueue.ts')

    const payload = btoa(JSON.stringify({ sub: 'user-1', tenant_id: 'tenant-1' }))
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/, '')
    auth.setToken(`x.${payload}.x`)

    queue.enqueueRequest({
      method: 'POST',
      path: '/api/v1/sales',
      body: { cash_session_id: 'cash-conflict', items: [], payments: [] },
      headers: { 'Idempotency-Key': 'conflict-key' },
    })
    queue.enqueueRequest({
      method: 'POST',
      path: '/api/v1/sales',
      body: { cash_session_id: 'cash-ok', items: [], payments: [] },
      headers: { 'Idempotency-Key': 'ok-key' },
    })

    const flushed = await queue.flushQueue()

    queue.enqueueRequest({
      method: 'POST',
      path: '/api/v1/sales',
      body: { cash_session_id: 'cash-expired', items: [], payments: [] },
      headers: { 'Idempotency-Key': 'expired-key' },
    })

    const storageKey = auth.scopedStorageKey('sistemaemgo:offlineQueue:v2')
    if (!storageKey) throw new Error('missing queue scope')
    const stored = JSON.parse(localStorage.getItem(storageKey) ?? '[]') as Array<{
      headers?: Record<string, string>
      createdAt: number
    }>
    const expired = stored.find((item) => item.headers?.['Idempotency-Key'] === 'expired-key')
    if (!expired) throw new Error('expired test item missing')
    expired.createdAt = Date.now() - 25 * 60 * 60 * 1000
    localStorage.setItem(storageKey, JSON.stringify(stored))

    const summary = queue.getQueueSummary()
    const preserved = JSON.parse(localStorage.getItem(storageKey) ?? '[]') as unknown[]
    return { flushed, summary, preservedCount: preserved.length }
  })

  expect(result.flushed.ok).toBe(true)
  expect(result.flushed.processed).toBe(1)
  expect(result.flushed.attention).toBe(1)
  expect(result.summary.pending).toBe(0)
  expect(result.summary.attention).toBe(2)
  expect(result.summary.total).toBe(2)
  expect(result.preservedCount).toBe(2)
})


test('offline sale older than safe replay window cannot retry or rebind', async ({ page }) => {
  await page.goto('/login')

  let salePosts = 0
  await page.route('http://127.0.0.1:8080/api/v1/sales', async (route) => {
    if (route.request().method() === 'POST') salePosts += 1
    await route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify({ id: 'must-not-be-created', status: 'finalized', total: 10 }),
    })
  })

  const result = await page.evaluate(async () => {
    const auth = await import('/src/lib/auth.ts')
    const queue = await import('/src/lib/offlineQueue.ts')

    const payload = btoa(JSON.stringify({ sub: 'retention-user', tenant_id: 'retention-tenant' }))
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/, '')
    auth.setToken(`x.${payload}.x`)

    const id = queue.enqueueRequest({
      method: 'POST',
      path: '/api/v1/sales',
      body: { cash_session_id: 'old-cash', items: [], payments: [] },
      headers: { 'Idempotency-Key': 'too-old-idempotency-key' },
    })

    const storageKey = auth.scopedStorageKey('sistemaemgo:offlineQueue:v2')
    if (!storageKey) throw new Error('missing queue scope')

    const stored = JSON.parse(localStorage.getItem(storageKey) ?? '[]') as Array<{
      id: string
      createdAt: number
      state?: string
    }>
    const item = stored.find((candidate) => candidate.id === id)
    if (!item) throw new Error('retention test item missing')
    item.createdAt = Date.now() - 29 * 24 * 60 * 60 * 1000
    item.state = 'attention'
    localStorage.setItem(storageKey, JSON.stringify(stored))

    const retry = queue.retryQueueItem(id)
    const rebind = queue.rebindQueueItemToCashSession(id, 'new-cash')
    const flushed = await queue.flushQueue()
    const finalItem = queue.getQueueItems().find((candidate) => candidate.id === id)

    return {
      retry,
      rebind,
      flushed,
      state: finalItem?.state,
      reason: finalItem?.attentionReason,
      cashSessionID:
        finalItem?.body && typeof finalItem.body === 'object'
          ? (finalItem.body as { cash_session_id?: string }).cash_session_id
          : undefined,
    }
  })

  expect(result.retry).toBe(false)
  expect(result.rebind).toBe(false)
  expect(result.state).toBe('attention')
  expect(result.reason).toBe('retention_expired')
  expect(result.cashSessionID).toBe('old-cash')
  expect(result.flushed.processed).toBe(0)
  expect(salePosts).toBe(0)
})
