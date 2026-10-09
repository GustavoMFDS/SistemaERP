import { expect, test } from '@playwright/test'

async function login(
  page: import('@playwright/test').Page,
  email = 'admin@sistema.local',
) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('purchase receiving is partial, tenant-scoped and credits stock only on receipt', async ({ page }, testInfo) => {
  await login(page)
  const suffix = `${testInfo.project.name}-${crypto.randomUUID()}`.replace(/[^a-zA-Z0-9-]/g, '').slice(0, 24)

  const result = await page.evaluate(async (suffix) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')

    const productCreated = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-PURCHASE-${suffix}`,
        barcode: null,
        name: `Produto Compra E2E ${suffix}`,
        description: null,
        unit: 'UN',
        cost_price: 5,
        price_cash: 10,
        promo_price: null,
        min_stock: 0,
        active: true,
      },
    })

    const inactiveProduct = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-INACTIVE-PURCHASE-${suffix}`,
        barcode: null,
        name: `Produto Inativo Compra E2E ${suffix}`,
        description: null,
        unit: 'UN',
        cost_price: 5,
        price_cash: 10,
        promo_price: null,
        min_stock: 0,
        active: false,
      },
    })

    const supplierKey = crypto.randomUUID()
    const supplier = await apiJson<{ id: string; replayed: boolean }>('/api/v1/suppliers', {
      method: 'POST',
      headers: { 'Idempotency-Key': supplierKey },
      body: {
        name: `Fornecedor E2E ${suffix}`,
        document: `DOC-${suffix}`,
        email: null,
        phone: '34999999999',
        contact_name: null,
        notes: null,
        active: true,
      },
    })

    const supplierReplay = await apiJson<{ id: string; replayed: boolean }>('/api/v1/suppliers', {
      method: 'POST',
      headers: { 'Idempotency-Key': supplierKey },
      body: {
        name: `Fornecedor E2E ${suffix}`,
        document: `DOC-${suffix}`,
        email: null,
        phone: '34999999999',
        contact_name: null,
        notes: null,
        active: true,
      },
    })

    let inactiveProductPurchaseStatus = 0
    try {
      await apiJson('/api/v1/purchases', {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          supplier_id: supplier.id,
          invoice_number: null,
          payment_due_date: null,
          notes: 'Produto inativo deve falhar',
          items: [{ product_id: inactiveProduct.id, qty: 1, unit_cost: 5 }],
        },
      })
      inactiveProductPurchaseStatus = 200
    } catch (error) {
      if (error instanceof APIError) inactiveProductPurchaseStatus = error.status
      else throw error
    }

    const purchaseKey = crypto.randomUUID()
    const purchase = await apiJson<{ id: string; status: string; replayed: boolean }>('/api/v1/purchases', {
      method: 'POST',
      headers: { 'Idempotency-Key': purchaseKey },
      body: {
        supplier_id: supplier.id,
        invoice_number: `NF-${suffix}`,
        payment_due_date: '2026-12-31',
        notes: 'Compra criada pelo E2E',
        items: [{ product_id: productCreated.id, qty: 4, unit_cost: 6.25 }],
      },
    })

    const purchaseReplay = await apiJson<{ id: string; status: string; replayed: boolean }>(
      '/api/v1/purchases',
      {
        method: 'POST',
        headers: { 'Idempotency-Key': purchaseKey },
        body: {
          supplier_id: supplier.id,
          invoice_number: `NF-${suffix}`,
          payment_due_date: '2026-12-31',
          notes: 'Compra criada pelo E2E',
          items: [{ product_id: productCreated.id, qty: 4, unit_cost: 6.25 }],
        },
      },
    )

    const before = await apiJson<{
      items: Array<{ id: string; qty_on_hand: number; cost_price: number }>
      total: number
    }>(`/api/v1/products?query=E2E-PURCHASE-${suffix}`)

    const detail = await apiJson<{
      purchase: { status: string }
      items: Array<{ id: string; qty_ordered: number; qty_received: number }>
      receipts: Array<{ id: string }>
    }>(`/api/v1/purchases/${purchase.id}`)

    const purchaseItemId = detail.items[0].id

    const partialKey = crypto.randomUUID()
    const partial = await apiJson<{ receipt_id: string; status: string; replayed: boolean }>(
      `/api/v1/purchases/${purchase.id}/receive`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': partialKey },
        body: {
          items: [{ purchase_item_id: purchaseItemId, qty: 1.5 }],
          notes: 'Recebimento parcial',
        },
      },
    )

    const partialReplay = await apiJson<{
      receipt_id: string
      status: string
      replayed: boolean
    }>(`/api/v1/purchases/${purchase.id}/receive`, {
      method: 'POST',
      headers: { 'Idempotency-Key': partialKey },
      body: {
        items: [{ purchase_item_id: purchaseItemId, qty: 1.5 }],
        notes: 'Recebimento parcial',
      },
    })

    const middle = await apiJson<{
      items: Array<{ id: string; qty_on_hand: number; cost_price: number }>
      total: number
    }>(`/api/v1/products?query=E2E-PURCHASE-${suffix}`)

    const completed = await apiJson<{ receipt_id: string; status: string; replayed: boolean }>(
      `/api/v1/purchases/${purchase.id}/receive`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
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

    const cancellable = await apiJson<{ id: string; status: string }>('/api/v1/purchases', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        supplier_id: supplier.id,
        invoice_number: `NF-CANCEL-${suffix}`,
        payment_due_date: '2026-12-31',
        notes: 'Compra a cancelar',
        items: [{ product_id: productCreated.id, qty: 1, unit_cost: 6.25 }],
      },
    })
    await apiJson(`/api/v1/purchases/${cancellable.id}/cancel`, { method: 'POST' })
    const cancelledDetail = await apiJson<{
      purchase: { status: string }
      items: Array<{ id: string }>
      receipts: Array<{ id: string }>
    }>(`/api/v1/purchases/${cancellable.id}`)

    let receiveCancelledStatus = 0
    try {
      await apiJson(`/api/v1/purchases/${cancellable.id}/receive`, {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          items: [{ purchase_item_id: cancelledDetail.items[0].id, qty: 1 }],
          notes: 'Recebimento de compra cancelada deve falhar',
        },
      })
      receiveCancelledStatus = 200
    } catch (error) {
      if (error instanceof APIError) receiveCancelledStatus = error.status
      else throw error
    }

    const after = await apiJson<{
      items: Array<{ id: string; qty_on_hand: number; cost_price: number }>
      total: number
    }>(`/api/v1/products?query=E2E-PURCHASE-${suffix}`)

    const movements = await apiJson<{
      items: Array<{ movement_type: string; delta: number; reference_type?: string | null }>
      total: number
    }>(`/api/v1/inventory/movements?product_id=${productCreated.id}&limit=20&offset=0`)

    const createAudit = await apiJson<{
      items: Array<{ action: string; resource_id: string }>
    }>('/api/v1/audit/logs?action=purchase.create&resource_type=purchase&limit=200&offset=0')
    const receiveAudit = await apiJson<{
      items: Array<{ action: string; resource_id: string }>
    }>('/api/v1/audit/logs?action=purchase.receive&resource_type=purchase&limit=200&offset=0')
    const cancelAudit = await apiJson<{
      items: Array<{ action: string; resource_id: string }>
    }>('/api/v1/audit/logs?action=purchase.cancel&resource_type=purchase&limit=200&offset=0')

    return {
      purchaseId: purchase.id,
      supplierId: supplier.id,
      supplierReplaySameId: supplierReplay.id === supplier.id,
      supplierReplayFlag: supplierReplay.replayed,
      purchaseReplaySameId: purchaseReplay.id === purchase.id,
      purchaseReplayFlag: purchaseReplay.replayed,
      inactiveProductPurchaseStatus,
      beforeQty: before.items[0].qty_on_hand,
      partialStatus: partial.status,
      partialReplaySameReceipt: partialReplay.receipt_id === partial.receipt_id,
      partialReplayFlag: partialReplay.replayed,
      middleQty: middle.items[0].qty_on_hand,
      completedStatus: completed.status,
      finalStatus: finalDetail.purchase.status,
      finalReceived: finalDetail.items[0].qty_received,
      cancelledStatus: cancelledDetail.purchase.status,
      receiveCancelledStatus,
      receiptCount: finalDetail.receipts.length,
      afterQty: after.items[0].qty_on_hand,
      afterCost: after.items[0].cost_price,
      purchaseMovements: movements.items.filter(
        (movement) =>
          movement.movement_type === 'purchase' &&
          movement.reference_type === 'purchase_receipt',
      ).length,
      purchaseCreateAuditCount: createAudit.items.filter((item) => item.resource_id === purchase.id).length,
      purchaseReceiveAuditCount: receiveAudit.items.filter((item) => item.resource_id === purchase.id).length,
      purchaseCancelAuditCount: cancelAudit.items.filter((item) => item.resource_id === cancellable.id).length,
    }
  }, suffix)

  expect(result.supplierReplaySameId).toBe(true)
  expect(result.supplierReplayFlag).toBe(true)
  expect(result.purchaseReplaySameId).toBe(true)
  expect(result.purchaseReplayFlag).toBe(true)
  expect(result.inactiveProductPurchaseStatus).toBe(422)
  expect(result.beforeQty).toBe(0)
  expect(result.partialStatus).toBe('partially_received')
  expect(result.partialReplaySameReceipt).toBe(true)
  expect(result.partialReplayFlag).toBe(true)
  expect(result.middleQty).toBe(1.5)
  expect(result.completedStatus).toBe('received')
  expect(result.finalStatus).toBe('received')
  expect(result.finalReceived).toBe(4)
  expect(result.cancelledStatus).toBe('cancelled')
  expect(result.receiveCancelledStatus).toBe(409)
  expect(result.receiptCount).toBe(2)
  expect(result.afterQty).toBe(4)
  expect(result.afterCost).toBe(6.25)
  expect(result.purchaseMovements).toBe(2)
  expect(result.purchaseCreateAuditCount).toBe(1)
  expect(result.purchaseReceiveAuditCount).toBe(2)
  expect(result.purchaseCancelAuditCount).toBe(1)

  await page.getByRole('link', { name: 'Compras' }).click()
  await expect(page).toHaveURL(/\/purchases$/)
  await expect(page.getByText(`Fornecedor E2E ${suffix}`).first()).toBeVisible()
  await expect(page.getByText('Recebida', { exact: true }).first()).toBeVisible()
})


test('cashier cannot read procurement data', async ({ page }) => {
  await login(page, 'caixa@sistema.local')
  await expect(page.getByText('caixa@sistema.local')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Compras' })).toHaveCount(0)

  const statuses = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    const statusFor = async (path: string) => {
      try {
        await apiJson(path)
        return 200
      } catch (error) {
        if (error instanceof APIError) return error.status
        throw error
      }
    }
    return {
      suppliers: await statusFor('/api/v1/suppliers?limit=1&offset=0'),
      purchases: await statusFor('/api/v1/purchases?limit=1&offset=0'),
    }
  })

  expect(statuses.suppliers).toBe(403)
  expect(statuses.purchases).toBe(403)
})


test('supplier maintenance edits status and blocks inactive purchasing', async ({ page }) => {
  await login(page)
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 12)

  const setup = await page.evaluate(async (suffix) => {
    const { apiJson } = await import('/src/lib/api.ts')
    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-SUP-MAINT-${suffix}`,
        barcode: null,
        name: `Produto Supplier Maint ${suffix}`,
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
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        name: `Fornecedor Manutencao ${suffix}`,
        document: `DOC-MAINT-${suffix}`,
        email: null,
        phone: '34999990000',
        contact_name: null,
        notes: 'manutencao E2E',
        active: true,
      },
    })
    return { productId: product.id, supplierId: supplier.id }
  }, suffix)

  await page.getByRole('link', { name: 'Compras' }).click()
  await expect(page).toHaveURL(/\/purchases$/)

  const originalName = `Fornecedor Manutencao ${suffix}`
  const editedName = `Fornecedor Atualizado ${suffix}`
  let row = page.locator('table tbody tr').filter({ hasText: originalName }).first()
  await expect(row).toBeVisible()
  await row.getByRole('button', { name: 'Editar' }).click()
  await page.getByLabel(`Editar nome de ${originalName}`).fill(editedName)
  await page.getByRole('button', { name: 'Salvar edição' }).click()

  row = page.locator('table tbody tr').filter({ hasText: editedName }).first()
  await expect(row).toBeVisible()
  await row.getByRole('button', { name: 'Desativar' }).click()
  await expect(row.getByText('Inativo', { exact: true })).toBeVisible()

  await expect(
    page.getByLabel('Fornecedor da compra').locator(`option[value="${setup.supplierId}"]`),
  ).toHaveCount(0)

  const inactiveStatus = await page.evaluate(async (setup) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/purchases', {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          supplier_id: setup.supplierId,
          invoice_number: null,
          payment_due_date: null,
          notes: 'fornecedor inativo deve falhar',
          items: [{ product_id: setup.productId, qty: 1, unit_cost: 5 }],
        },
      })
      return 200
    } catch (error) {
      if (error instanceof APIError) return error.status
      throw error
    }
  }, setup)
  expect(inactiveStatus).toBe(422)

  await row.getByRole('button', { name: 'Ativar' }).click()
  await expect(row.getByText('Ativo', { exact: true })).toBeVisible()
  await expect(
    page.getByLabel('Fornecedor da compra').locator(`option[value="${setup.supplierId}"]`),
  ).toHaveCount(1)
})
