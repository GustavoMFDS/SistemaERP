import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('cash register allows only one open session and supports close/reopen', async ({ page }) => {
  await login(page, 'admin@sistema.local')
  await page.getByRole('link', { name: 'PDV' }).click()
  await page.getByRole('button', { name: 'Abrir' }).click()

  const firstCash = await page.evaluate(async () => {
    const { getCashSessionId } = await import('/src/lib/auth.ts')
    return getCashSessionId()
  })
  expect(firstCash).not.toBe('')

  const duplicateStatus = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/cash/sessions/open', {
        method: 'POST',
        body: { opening_amount: 0, notes: 'duplicate e2e' },
      })
      return 201
    } catch (error) {
      if (error instanceof APIError) return error.status
      throw error
    }
  })
  expect(duplicateStatus).toBe(409)

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .toBe('')

  await page.getByRole('button', { name: 'Abrir' }).click()
  const secondCash = await page.evaluate(async () => {
    const { getCashSessionId } = await import('/src/lib/auth.ts')
    return getCashSessionId()
  })
  expect(secondCash).not.toBe('')
  expect(secondCash).not.toBe(firstCash)

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
})

test('legacy offline queue is visible and requires explicit operator reconciliation', async ({ page }) => {
  await login(page, 'admin@sistema.local')

  await page.evaluate(() => {
    localStorage.setItem(
      'sistemaemgo:offlineQueue:v1',
      JSON.stringify([
        {
          id: 'legacy-sale-1',
          createdAt: Date.now(),
          method: 'POST',
          path: '/api/v1/sales',
          body: { cash_session_id: 'legacy-cash', items: [], payments: [] },
          headers: { 'Idempotency-Key': 'legacy-idem-1' },
        },
      ]),
    )
  })

  await page.getByRole('link', { name: 'PDV' }).click()
  await expect(page.getByText('Fila offline legada detectada')).toBeVisible()
  await page.getByRole('button', { name: 'Importar para revisão' }).click()

  await expect(page.getByText('Reconciliação offline')).toBeVisible()
  await expect(page.getByText(/legacy_migration/)).toBeVisible()

  page.once('dialog', (dialog) => void dialog.accept())
  await page.getByRole('button', { name: 'Descartar' }).click()
  await expect(page.getByText('Reconciliação offline')).not.toBeVisible()

  const remaining = await page.evaluate(() => localStorage.getItem('sistemaemgo:offlineQueue:v1'))
  expect(remaining).toBeNull()
})

test('failed refresh immediately returns the UI to login', async ({ page }) => {
  await login(page, 'admin@sistema.local')

  await page.route(/http:\/\/127\.0\.0\.1:8080\/api\/v1\/products\?probe=session-expired/, async (route) => {
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ message: 'expired' }),
    })
  })
  await page.route('http://127.0.0.1:8080/api/v1/auth/refresh', async (route) => {
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ message: 'refresh expired' }),
    })
  })

  const status = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/products?probe=session-expired')
      return 200
    } catch (error) {
      if (error instanceof APIError) return error.status
      throw error
    }
  })

  expect(status).toBe(401)
  await expect(page).toHaveURL(/\/login$/)
})

test('real tenant B cannot access tenant A sale, fiscal, finance, privacy or audit data', async ({ page }) => {
  await login(page, 'admin@sistema.local')

  const tenantA = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')

    const products = await apiJson<{
      items: Array<{ id: string; active: boolean; price_cash: number }>
    }>('/api/v1/products?limit=200&offset=0')
    const product = products.items.find((item) => item.active)
    if (!product) throw new Error('tenant A product missing')

    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'tenant isolation e2e' },
    })

    const sale = await apiJson<{ id: string }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: cash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
        payments: [{ method: 'pix', amount: product.price_cash }],
      },
    })

    const fiscal = await apiJson<{ invoice_id: string; xml_file_id: string }>(
      '/api/v1/fiscal/nfe/xml',
      { method: 'POST', body: { sale_id: sale.id } },
    )

    const privacy = await apiJson<{ id: string }>('/api/v1/privacy/requests', {
      method: 'POST',
      body: {
        subject_type: 'customer',
        subject_id: null,
        requester_email: 'tenant-a-subject@example.com',
        request_type: 'export',
        notes: 'tenant isolation e2e',
      },
    })

    await apiJson(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: { closing_amount: 0, notes: 'tenant isolation cleanup' },
    })

    return {
      saleId: sale.id,
      xmlId: fiscal.xml_file_id,
      privacyId: privacy.id,
    }
  })

  await login(page, 'admin-b@sistema.local')

  const tenantB = await page.evaluate(async (ids) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')

    async function statusOf(path: string): Promise<number> {
      try {
        await apiJson(path)
        return 200
      } catch (error) {
        if (error instanceof APIError) return error.status
        throw error
      }
    }

    const saleStatus = await statusOf(`/api/v1/sales/${ids.saleId}`)
    const xmlStatus = await statusOf(`/api/v1/fiscal/nfe/xml/${ids.xmlId}/download`)
    const privacyStatus = await statusOf(`/api/v1/privacy/requests/${ids.privacyId}`)

    const ledger = await apiJson<{
      items: Array<{ sale_id?: string | null }>
    }>('/api/v1/finance/ledger?limit=200&offset=0')
    const fiscal = await apiJson<{
      items: Array<{ id: string }>
    }>('/api/v1/fiscal/nfe/xml?limit=200&offset=0')
    const privacy = await apiJson<{
      items: Array<{ id: string }>
    }>('/api/v1/privacy/requests?limit=200&offset=0')
    const audit = await apiJson<{
      items: Array<{ resource_id?: string | null }>
    }>('/api/v1/audit/logs?limit=200&offset=0')

    return {
      saleStatus,
      xmlStatus,
      privacyStatus,
      ledgerLeak: ledger.items.some((item) => item.sale_id === ids.saleId),
      fiscalLeak: fiscal.items.some((item) => item.id === ids.xmlId),
      privacyLeak: privacy.items.some((item) => item.id === ids.privacyId),
      auditLeak: audit.items.some((item) =>
        [ids.saleId, ids.xmlId, ids.privacyId].includes(item.resource_id ?? ''),
      ),
    }
  }, tenantA)

  expect(tenantB.saleStatus).toBe(404)
  expect(tenantB.xmlStatus).toBe(404)
  expect(tenantB.privacyStatus).toBe(404)
  expect(tenantB.ledgerLeak).toBe(false)
  expect(tenantB.fiscalLeak).toBe(false)
  expect(tenantB.privacyLeak).toBe(false)
  expect(tenantB.auditLeak).toBe(false)
})
