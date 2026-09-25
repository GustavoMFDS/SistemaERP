import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('fractional PDV quantities use backend-equivalent cent rounding', async ({ page }) => {
  await login(page)

  const fixture = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    const sku = `R9-FRAC-${crypto.randomUUID().slice(0, 8)}`
    const created = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        sku,
        name: `Round9 Fractional ${sku}`,
        unit: 'KG',
        cost_price: 0.5,
        price_cash: 1,
        min_stock: 0,
        active: true,
      },
    })
    await apiJson('/api/v1/inventory/adjust', {
      method: 'POST',
      body: {
        product_id: created.id,
        delta: 1,
        reason: 'round9 fractional sale stock',
        type: 'adjustment',
      },
    })

    const current = await apiJson<{ session: { id: string } | null }>(
      '/api/v1/cash/sessions/current',
    )
    if (current.session) {
      await apiJson(`/api/v1/cash/sessions/${current.session.id}/close`, {
        method: 'POST',
        body: { closing_amount: 0, notes: 'round9 fractional pre-cleanup' },
      })
    }

    return { id: created.id, sku }
  })

  await page.getByRole('link', { name: 'PDV' }).click()
  await page.getByRole('button', { name: 'Abrir' }).click()
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .not.toBe('')

  await page.getByLabel('Produto').selectOption(fixture.id)

  const qty = page.getByLabel('Qtd')
  await qty.fill('0.0005')
  await page.getByRole('button', { name: 'Adicionar' }).click()
  await expect(page.getByText(/Quantidade inválida/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Remover' })).toHaveCount(0)

  await qty.fill('0.005')
  await page.getByRole('button', { name: 'Adicionar' }).click()
  await page.getByRole('button', { name: 'Adicionar' }).click()

  await expect(page.getByRole('button', { name: 'Remover' })).toHaveCount(2)
  await expect(page.getByText('0.005')).toHaveCount(2)
  await expect(page.getByText('R$ 0.02')).toBeVisible()

  await page.getByRole('button', { name: 'Finalizar' }).click()
  await expect(page.getByText(/Venda finalizada:/)).toBeVisible()
  await expect(page.getByText(/Total R\$ 0\.02/)).toBeVisible()

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .toBe('')
})

test('offline price drift is quarantined with the original price snapshot preserved', async ({
  page,
}) => {
  await login(page)

  const fixture = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    const sku = `R9-DRIFT-${crypto.randomUUID().slice(0, 8)}`
    const created = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        sku,
        name: `Round9 Price Drift ${sku}`,
        unit: 'UN',
        cost_price: 4,
        price_cash: 10,
        min_stock: 0,
        active: true,
      },
    })
    await apiJson('/api/v1/inventory/adjust', {
      method: 'POST',
      body: {
        product_id: created.id,
        delta: 5,
        reason: 'round9 price drift stock',
        type: 'adjustment',
      },
    })

    const current = await apiJson<{ session: { id: string } | null }>(
      '/api/v1/cash/sessions/current',
    )
    if (current.session) {
      await apiJson(`/api/v1/cash/sessions/${current.session.id}/close`, {
        method: 'POST',
        body: { closing_amount: 0, notes: 'round9 price drift pre-cleanup' },
      })
    }

    return { id: created.id, sku }
  })

  await page.getByRole('link', { name: 'PDV' }).click()
  await page.getByRole('button', { name: 'Abrir' }).click()

  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .not.toBe('')

  const openCashID = await page.evaluate(async () => {
    const { getCashSessionId } = await import('/src/lib/auth.ts')
    return getCashSessionId()
  })
  expect(openCashID).not.toBe('')

  const queuedID = await page.evaluate(
    async ({ productID, cashID }) => {
      const queue = await import('/src/lib/offlineQueue.ts')
      return queue.enqueueRequest({
        method: 'POST',
        path: '/api/v1/sales',
        body: {
          cash_session_id: cashID,
          customer_id: null,
          discount_value: 0,
          items: [
            {
              product_id: productID,
              qty: 1,
              unit_price: 10,
              discount_value: 0,
            },
          ],
          payments: [{ method: 'cash', amount: 10 }],
        },
        headers: { 'Idempotency-Key': crypto.randomUUID() },
      })
    },
    { productID: fixture.id, cashID: openCashID },
  )

  await page.evaluate(async (productID) => {
    const { apiJson } = await import('/src/lib/api.ts')
    const product = await apiJson<{
      category_id?: string | null
      sku: string
      barcode?: string | null
      name: string
      description?: string | null
      unit: string
      cost_price: number
      price_cash: number
      promo_price?: number | null
      min_stock: number
      active: boolean
    }>(`/api/v1/products/${productID}`)
    await apiJson(`/api/v1/products/${productID}`, {
      method: 'PUT',
      body: {
        category_id: product.category_id ?? null,
        sku: product.sku,
        barcode: product.barcode ?? null,
        name: product.name,
        description: product.description ?? null,
        unit: product.unit,
        cost_price: product.cost_price,
        price_cash: 12,
        promo_price: product.promo_price ?? null,
        min_stock: product.min_stock,
        active: product.active,
      },
    })
  }, fixture.id)

  const result = await page.evaluate(async (id) => {
    const queue = await import('/src/lib/offlineQueue.ts')
    const flushed = await queue.flushQueue()
    const item = queue.getQueueItems().find((candidate) => candidate.id === id)
    const body = item?.body as
      | {
          items?: Array<{ unit_price?: number }>
        }
      | undefined
    return {
      flushed,
      state: item?.state,
      reason: item?.attentionReason,
      error: item?.lastError,
      unitPrice: body?.items?.[0]?.unit_price,
    }
  }, queuedID)

  expect(result.flushed.processed).toBe(0)
  expect(result.state).toBe('attention')
  expect(result.reason).toBe('request_rejected')
  expect(result.error).toMatch(/preco do produto mudou/i)
  expect(result.unitPrice).toBe(10)

  const sales = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ items: Array<{ id: string }> }>('/api/v1/sales?limit=200&offset=0')
  })
  expect(sales.items).not.toContainEqual(expect.objectContaining({ id: queuedID }))

  await page.evaluate(async (id) => {
    const queue = await import('/src/lib/offlineQueue.ts')
    queue.discardQueueItem(id)
  }, queuedID)

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .toBe('')
})

