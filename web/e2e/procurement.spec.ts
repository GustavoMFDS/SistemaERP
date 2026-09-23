import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('purchase receiving is partial, tenant-scoped and credits stock only on receipt', async ({ page }) => {
  await login(page)

  const result = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')

    const productCreated = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: 'E2E-PURCHASE-ITEM',
        barcode: null,
        name: 'Produto Compra E2E',
        description: null,
        unit: 'UN',
        cost_price: 5,
        price_cash: 10,
        promo_price: null,
        min_stock: 0,
        active: true,
      },
    })

    const supplier = await apiJson<{ id: string }>('/api/v1/suppliers', {
      method: 'POST',
      body: {
        name: 'Fornecedor E2E',
        document: '11222333000199',
        email: null,
        phone: '34999999999',
        contact_name: null,
        notes: null,
        active: true,
      },
    })

    const purchase = await apiJson<{ id: string; status: string }>('/api/v1/purchases', {
      method: 'POST',
      body: {
        supplier_id: supplier.id,
        invoice_number: 'NF-E2E-001',
        payment_due_date: '2026-12-31',
        notes: 'Compra criada pelo E2E',
        items: [{ product_id: productCreated.id, qty: 4, unit_cost: 6.25 }],
      },
    })

    const before = await apiJson<{
      items: Array<{ id: string; qty_on_hand: number; cost_price: number }>
      total: number
    }>('/api/v1/products?query=E2E-PURCHASE-ITEM')

    const detail = await apiJson<{
      purchase: { status: string }
      items: Array<{ id: string; qty_ordered: number; qty_received: number }>
      receipts: Array<{ id: string }>
    }>(`/api/v1/purchases/${purchase.id}`)

    const purchaseItemId = detail.items[0].id

    const partial = await apiJson<{ receipt_id: string; status: string }>(
      `/api/v1/purchases/${purchase.id}/receive`,
      {
        method: 'POST',
        body: {
          items: [{ purchase_item_id: purchaseItemId, qty: 1.5 }],
          notes: 'Recebimento parcial',
        },
      },
    )

    const middle = await apiJson<{
      items: Array<{ id: string; qty_on_hand: number; cost_price: number }>
      total: number
    }>('/api/v1/products?query=E2E-PURCHASE-ITEM')

    const completed = await apiJson<{ receipt_id: string; status: string }>(
      `/api/v1/purchases/${purchase.id}/receive`,
      {
        method: 'POST',
        body: {
          items: [{ purchase_item_id: purchaseItemId, qty: 2.5 }],
          notes: 'Recebimento final',
        },
      },
    )

    const finalDetail = await apiJson<{
      purchase: { status: string }
      items: Array<{ id: string; qty_ordered: number; qty_received: number }>
      receipts: Array<{ id: string }>
    }>(`/api/v1/purchases/${purchase.id}`)

    const after = await apiJson<{
      items: Array<{ id: string; qty_on_hand: number; cost_price: number }>
      total: number
    }>('/api/v1/products?query=E2E-PURCHASE-ITEM')

    const movements = await apiJson<{
      items: Array<{ movement_type: string; delta: number; reference_type?: string | null }>
      total: number
    }>(`/api/v1/inventory/movements?product_id=${productCreated.id}&limit=20&offset=0`)

    return {
      purchaseId: purchase.id,
      supplierId: supplier.id,
      beforeQty: before.items[0].qty_on_hand,
      partialStatus: partial.status,
      middleQty: middle.items[0].qty_on_hand,
      completedStatus: completed.status,
      finalStatus: finalDetail.purchase.status,
      finalReceived: finalDetail.items[0].qty_received,
      receiptCount: finalDetail.receipts.length,
      afterQty: after.items[0].qty_on_hand,
      afterCost: after.items[0].cost_price,
      purchaseMovements: movements.items.filter(
        (movement) =>
          movement.movement_type === 'purchase' &&
          movement.reference_type === 'purchase_receipt',
      ).length,
    }
  })

  expect(result.beforeQty).toBe(0)
  expect(result.partialStatus).toBe('partially_received')
  expect(result.middleQty).toBe(1.5)
  expect(result.completedStatus).toBe('received')
  expect(result.finalStatus).toBe('received')
  expect(result.finalReceived).toBe(4)
  expect(result.receiptCount).toBe(2)
  expect(result.afterQty).toBe(4)
  expect(result.afterCost).toBe(6.25)
  expect(result.purchaseMovements).toBe(2)

  await page.getByRole('link', { name: 'Compras' }).click()
  await expect(page).toHaveURL(/\/purchases$/)
  await expect(page.getByText('Fornecedor E2E').first()).toBeVisible()
  await expect(page.getByText('Recebida', { exact: true }).first()).toBeVisible()
})
