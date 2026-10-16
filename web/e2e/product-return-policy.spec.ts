import { expect, test } from './fixtures'

test('devolução aplica a regra registrada na venda, mesmo depois de editar o produto', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 12)
  const result = await page.evaluate(async (suffix) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    let cashId = ''
    try {
      const p = await apiJson<{ id: string }>('/api/v1/products', {
        method: 'POST',
        body: {
          sku: 'E2E-POL-' + suffix, name: 'Produto Regra ' + suffix,
          unit: 'UN', cost_price: 3, price_cash: 12,
          min_stock: 0, active: true, return_policy: 'denied', return_days: null,
        },
      })
      await apiJson('/api/v1/inventory/adjust', { method: 'POST', body: {
        product_id: p.id, delta: 3, reason: 'Ajuste de teste', type: 'adjustment',
      } })
      const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
        method: 'POST', body: { opening_amount: 0, notes: 'E2E policy ' + suffix },
      })
      cashId = cash.id
      const sale = await apiJson<{ id: string }>('/api/v1/sales', {
        method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: { cash_session_id: cash.id, discount_value: 0,
          items: [{ product_id: p.id, qty: 1, discount_value: 0 }],
          payments: [{ method: 'cash', amount: 12 }],
        },
      })
      const saleInfo = await apiJson<{
        sale: { finalized_at: string },
        items: Array<{ id: string; return_policy_snapshot: string | null }>,
      }>('/api/v1/sales/' + sale.id)
      const product = await apiJson<{ name: string; sku: string; unit: string; price_cash: number; cost_price: number; min_stock: number; active: boolean }>('/api/v1/products/' + p.id)
      await apiJson('/api/v1/products/' + p.id, { method: 'PUT', body: {
        name: product.name, sku: product.sku, unit: product.unit,
        price_cash: product.price_cash, cost_price: product.cost_price,
        min_stock: product.min_stock, active: product.active,
        return_policy: 'allowed', return_days: null,
      } })
      const item = saleInfo.items[0]
      let voluntaryStatus = 0
      try {
        await apiJson('/api/v1/sales/' + sale.id + '/returns', {
          method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() },
          body: { kind: 'return', basis: 'voluntary', reason: 'Mudou de ideia',
            items: [{ sale_item_id: item.id, qty: 1, restock: true }] },
        })
        voluntaryStatus = 200
      } catch (e) {
        if (e instanceof APIError) voluntaryStatus = e.status
        else throw e
      }
      const protectedReturn = await apiJson<{ id: string; refund_due: number }>(
        '/api/v1/sales/' + sale.id + '/returns',
        { method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() },
          body: { kind: 'return', basis: 'consumer_rights',
            reason: 'Produto com defeito confirmado pelo responsavel',
            items: [{ sale_item_id: item.id, qty: 1, restock: false }] },
        },
      )
      const history = await apiJson<{ return: { return_basis: string } }>(
        '/api/v1/returns/' + protectedReturn.id)
      return {
        snapshot: item.return_policy_snapshot,
        voluntaryStatus, protectedBasis: history.return.return_basis,
        refunded: protectedReturn.refund_due, saleHasTimestamp: !!saleInfo.sale.finalized_at,
      }
    } finally {
      if (cashId) {
        try {
          await apiJson('/api/v1/cash/sessions/' + cashId + '/close', {
            method: 'POST', body: { closing_amount: 12, notes: 'E2E policy cleanup' },
          })
        } catch { /* Preserve the original test error for investigation. */ }
      }
    }
  }, suffix)

  expect(result.snapshot).toBe('denied')
  expect(result.saleHasTimestamp).toBe(true)
  expect(result.voluntaryStatus).toBe(409)
  expect(result.protectedBasis).toBe('consumer_rights')
  expect(result.refunded).toBe(12)
})
