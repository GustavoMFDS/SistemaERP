import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('admin online sale updates stock and finance, generates fiscal XML, then cancellation restores stock', async ({ page }) => {
  await login(page, 'admin@sistema.local')

  const result = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')

    type Product = {
      id: string
      active: boolean
      price_cash: number
      qty_on_hand: number
    }
    type Products = { items: Product[]; total: number }
    type CashOpen = { id: string }
    type SaleCreate = { id: string; total: number; replayed: boolean }
    type LedgerItem = { entry_type: string; sale_id?: string | null }
    type Ledger = { items: LedgerItem[]; total: number }
    type FiscalGenerate = { invoice_id: string; xml_file_id: string }
    type FiscalList = { items: Array<{ id: string; invoice_id: string }>; total: number }

    const productsBefore = await apiJson<Products>('/api/v1/products?limit=200&offset=0')
    const product = productsBefore.items.find((item) => item.active)
    if (!product) throw new Error('no active product available for business E2E')

    const cash = await apiJson<CashOpen>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: null },
    })

    const sale = await apiJson<SaleCreate>('/api/v1/sales', {
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

    const productAfterSale = await apiJson<Product>(`/api/v1/products/${product.id}`)
    const ledgerAfterSale = await apiJson<Ledger>('/api/v1/finance/ledger?limit=200&offset=0')

    const fiscal = await apiJson<FiscalGenerate>('/api/v1/fiscal/nfe/xml', {
      method: 'POST',
      body: { sale_id: sale.id },
    })
    const fiscalList = await apiJson<FiscalList>('/api/v1/fiscal/nfe/xml?limit=200&offset=0')

    const privacy = await apiJson<{ items: unknown[] }>('/api/v1/privacy/requests?limit=10&offset=0')

    await apiJson(`/api/v1/sales/${sale.id}/cancel`, {
      method: 'POST',
      body: { reason: 'E2E integrity cancellation' },
    })

    const productAfterCancel = await apiJson<Product>(`/api/v1/products/${product.id}`)
    const ledgerAfterCancel = await apiJson<Ledger>('/api/v1/finance/ledger?limit=200&offset=0')
    const saleAfterCancel = await apiJson<{ sale: { status: string } }>(`/api/v1/sales/${sale.id}`)

    return {
      beforeQty: product.qty_on_hand,
      afterSaleQty: productAfterSale.qty_on_hand,
      afterCancelQty: productAfterCancel.qty_on_hand,
      saleId: sale.id,
      replayed: sale.replayed,
      saleStatus: saleAfterCancel.sale.status,
      saleLedger: ledgerAfterSale.items.some(
        (item) => item.entry_type === 'sale' && item.sale_id === sale.id,
      ),
      cancelLedger: ledgerAfterCancel.items.some(
        (item) => item.entry_type === 'sale_cancel' && item.sale_id === sale.id,
      ),
      fiscalId: fiscal.xml_file_id,
      fiscalListed: fiscalList.items.some((item) => item.id === fiscal.xml_file_id),
      privacyListReadable: Object.prototype.hasOwnProperty.call(privacy, 'items'),
    }
  })

  expect(result.replayed).toBe(false)
  expect(result.afterSaleQty).toBe(result.beforeQty - 1)
  expect(result.afterCancelQty).toBe(result.beforeQty)
  expect(result.saleStatus).toBe('cancelled')
  expect(result.saleLedger).toBe(true)
  expect(result.cancelLedger).toBe(true)
  expect(result.fiscalId).not.toBe('')
  expect(result.fiscalListed).toBe(true)
  expect(result.privacyListReadable).toBe(true)
})

test('cashier is authenticated but RBAC denies privileged domains', async ({ page }) => {
  await login(page, 'caixa@sistema.local')

  const statuses = await page.evaluate(async () => {
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

    const products = await statusOf('/api/v1/products?limit=1&offset=0')
    const finance = await statusOf('/api/v1/finance/dashboard')
    const fiscal = await statusOf('/api/v1/fiscal/nfe/xml?limit=1&offset=0')
    const audit = await statusOf('/api/v1/audit/logs?limit=1&offset=0')
    const privacy = await statusOf('/api/v1/privacy/requests?limit=1&offset=0')

    return { products, finance, fiscal, audit, privacy }
  })

  expect(statuses.products).toBe(200)
  expect(statuses.finance).toBe(403)
  expect(statuses.fiscal).toBe(403)
  expect(statuses.audit).toBe(403)
  expect(statuses.privacy).toBe(403)
})
