import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('reconciles digital payments and settles return refunds without double-counting cash', async ({ page }) => {
  await login(page)
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 16)

  const result = await page.evaluate(async (suffix) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')

    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-RECON-${suffix}`,
        barcode: null,
        name: `Produto Conciliacao ${suffix}`,
        description: null,
        unit: 'UN',
        cost_price: 7,
        price_cash: 20,
        promo_price: null,
        min_stock: 0,
        active: true,
      },
    })

    await apiJson('/api/v1/inventory/adjust', {
      method: 'POST',
      body: {
        product_id: product.id,
        delta: 3,
        reason: 'Carga E2E conciliacao',
        type: 'adjustment',
      },
    })

    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 100, notes: 'E2E reconciliation' },
    })

    const sale = await apiJson<{ id: string; total: number }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: cash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
        payments: [{
          method: 'pix',
          amount: 20,
          provider: 'e2e-provider',
          transaction_ref: `TX-${suffix}`,
          authorization_code: null,
          installments: 1,
        }],
      },
    })

    const paymentList = await apiJson<{
      items: Array<{
        id: string
        sale_id: string
        method: string
        amount: number
        provider?: string | null
        transaction_ref?: string | null
        reconciliation_status: string
      }>
      total: number
    }>('/api/v1/finance/payments?method=pix&status=pending&limit=200&offset=0')
    const payment = paymentList.items.find((item) => item.sale_id === sale.id)
    if (!payment) throw new Error('payment not found in finance list')

    const reconcileKey = crypto.randomUUID()
    const reconciliation = await apiJson<{ id: string; status: string; replayed: boolean }>(
      `/api/v1/finance/payments/${payment.id}/reconcile`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': reconcileKey },
        body: {
          received_amount: 19,
          fee_amount: 1,
          provider: 'e2e-provider',
          external_ref: `SETTLE-${suffix}`,
          notes: 'conciliacao E2E',
        },
      },
    )
    const reconciliationReplay = await apiJson<{ id: string; status: string; replayed: boolean }>(
      `/api/v1/finance/payments/${payment.id}/reconcile`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': reconcileKey },
        body: {
          received_amount: 19,
          fee_amount: 1,
          provider: 'e2e-provider',
          external_ref: `SETTLE-${suffix}`,
          notes: 'conciliacao E2E',
        },
      },
    )

    let invalidReferencePairStatus = 0
    try {
      await apiJson(`/api/v1/finance/payments/${payment.id}/reconcile`, {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          received_amount: 20,
          fee_amount: 1,
          provider: 'e2e-provider',
          external_ref: null,
          notes: 'par externo incompleto deve falhar',
        },
      })
      invalidReferencePairStatus = 200
    } catch (error) {
      if (error instanceof APIError) invalidReferencePairStatus = error.status
      else throw error
    }

    let duplicateReconcileStatus = 0
    try {
      await apiJson(`/api/v1/finance/payments/${payment.id}/reconcile`, {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          received_amount: 20,
          fee_amount: 1,
          provider: 'e2e-provider',
          external_ref: `SETTLE-SECOND-${suffix}`,
          notes: 'segunda conciliacao deve falhar',
        },
      })
      duplicateReconcileStatus = 200
    } catch (error) {
      if (error instanceof APIError) duplicateReconcileStatus = error.status
      else throw error
    }

    const secondSale = await apiJson<{ id: string }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: cash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
        payments: [{
          method: 'pix',
          amount: 20,
          provider: 'e2e-provider',
          transaction_ref: `TX-SECOND-${suffix}`,
          authorization_code: null,
          installments: 1,
        }],
      },
    })

    const secondPayments = await apiJson<{
      items: Array<{ id: string; sale_id: string }>
      total: number
    }>('/api/v1/finance/payments?method=pix&status=pending&limit=200&offset=0')
    const secondPayment = secondPayments.items.find((item) => item.sale_id === secondSale.id)
    if (!secondPayment) throw new Error('second payment not found')

    const collisionSale = await apiJson<{ id: string }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: cash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
        payments: [{
          method: 'pix',
          amount: 20,
          provider: 'e2e-provider',
          transaction_ref: `UNRECONCILED-${suffix}`,
          authorization_code: null,
          installments: 1,
        }],
      },
    })

    let paymentReferenceConflictStatus = 0
    try {
      await apiJson(`/api/v1/finance/payments/${secondPayment.id}/reconcile`, {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          received_amount: 20,
          fee_amount: 1,
          provider: 'e2e-provider',
          external_ref: `UNRECONCILED-${suffix}`,
          notes: 'referencia ja usada por pagamento pendente deve falhar',
        },
      })
      paymentReferenceConflictStatus = 200
    } catch (error) {
      if (error instanceof APIError) paymentReferenceConflictStatus = error.status
      else throw error
    }

    let duplicateExternalRefStatus = 0
    try {
      await apiJson(`/api/v1/finance/payments/${secondPayment.id}/reconcile`, {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          received_amount: 20,
          fee_amount: 1,
          provider: 'e2e-provider',
          external_ref: `SETTLE-${suffix}`,
          notes: 'referencia externa duplicada deve falhar',
        },
      })
      duplicateExternalRefStatus = 200
    } catch (error) {
      if (error instanceof APIError) duplicateExternalRefStatus = error.status
      else throw error
    }

    const adjustmentKey = crypto.randomUUID()
    const adjustment = await apiJson<{ id: string; status: string; replayed: boolean }>(
      `/api/v1/finance/payments/${payment.id}/reconciliation-adjustments`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': adjustmentKey },
        body: {
          received_amount: 20,
          fee_amount: 1,
          notes: 'corrigir divergencia E2E',
        },
      },
    )
    const adjustmentReplay = await apiJson<{ id: string; status: string; replayed: boolean }>(
      `/api/v1/finance/payments/${payment.id}/reconciliation-adjustments`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': adjustmentKey },
        body: {
          received_amount: 20,
          fee_amount: 1,
          notes: 'corrigir divergencia E2E',
        },
      },
    )
    const adjustmentAudit = await apiJson<{
      items: Array<{ action: string; resource_id: string }>
    }>('/api/v1/audit/logs?action=payment.reconcile.adjust&resource_type=payment&limit=200&offset=0')

    await apiJson(`/api/v1/sales/${secondSale.id}/cancel`, {
      method: 'POST',
      body: { reason: 'cleanup duplicate external ref E2E' },
    })
    await apiJson(`/api/v1/sales/${collisionSale.id}/cancel`, {
      method: 'POST',
      body: { reason: 'cleanup payment reference collision E2E' },
    })

    const saleDetail = await apiJson<{
      items: Array<{ id: string }>
    }>(`/api/v1/sales/${sale.id}`)
    const saleItemId = saleDetail.items[0].id

    const saleReturn = await apiJson<{ id: string; refund_due: number }>(
      `/api/v1/sales/${sale.id}/returns`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          kind: 'return',
          reason: 'Reembolso E2E',
          items: [{ sale_item_id: saleItemId, qty: 1, restock: true }],
        },
      },
    )

    const cashRefundKey = crypto.randomUUID()
    const cashRefund = await apiJson<{
      id: string
      status: string
      remaining_amount: number
      replayed: boolean
    }>(`/api/v1/finance/returns/${saleReturn.id}/refunds`, {
      method: 'POST',
      headers: { 'Idempotency-Key': cashRefundKey },
      body: {
        method: 'cash',
        amount: 5,
        provider: null,
        external_ref: null,
        cash_session_id: cash.id,
        notes: 'parcial em dinheiro',
      },
    })
    const cashRefundReplay = await apiJson<{
      id: string
      status: string
      remaining_amount: number
      replayed: boolean
    }>(`/api/v1/finance/returns/${saleReturn.id}/refunds`, {
      method: 'POST',
      headers: { 'Idempotency-Key': cashRefundKey },
      body: {
        method: 'cash',
        amount: 5,
        provider: null,
        external_ref: null,
        cash_session_id: cash.id,
        notes: 'parcial em dinheiro',
      },
    })

    const pixRefund = await apiJson<{
      id: string
      status: string
      remaining_amount: number
      replayed: boolean
    }>(`/api/v1/finance/returns/${saleReturn.id}/refunds`, {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        method: 'pix',
        amount: 15,
        provider: 'e2e-provider',
        external_ref: `REFUND-${suffix}`,
        cash_session_id: cash.id,
        notes: 'saldo por pix',
      },
    })

    const cashRefundLateReplay = await apiJson<{
      id: string
      status: string
      remaining_amount: number
      replayed: boolean
    }>(`/api/v1/finance/returns/${saleReturn.id}/refunds`, {
      method: 'POST',
      headers: { 'Idempotency-Key': cashRefundKey },
      body: {
        method: 'cash',
        amount: 5,
        provider: null,
        external_ref: null,
        cash_session_id: cash.id,
        notes: 'parcial em dinheiro',
      },
    })

    let overRefundStatus = 0
    try {
      await apiJson(`/api/v1/finance/returns/${saleReturn.id}/refunds`, {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          method: 'pix',
          amount: 1,
          provider: 'e2e-provider',
          external_ref: `EXTRA-${suffix}`,
          cash_session_id: cash.id,
          notes: 'deve falhar',
        },
      })
      overRefundStatus = 200
    } catch (error) {
      if (error instanceof APIError) overRefundStatus = error.status
      else throw error
    }

    const refunds = await apiJson<{
      items: Array<{
        return_id: string
        status: string
        refund_due: number
        settled_amount: number
        remaining_amount: number
      }>
      total: number
    }>('/api/v1/finance/refunds?limit=200&offset=0')
    const refundSummary = refunds.items.find((item) => item.return_id === saleReturn.id)
    if (!refundSummary) throw new Error('refund summary missing')

    const paymentsAfter = await apiJson<{
      items: Array<{
        id: string
        reconciliation_status: string
        reconciled_amount?: number | null
        reconciled_fee?: number | null
      }>
      total: number
    }>('/api/v1/finance/payments?method=pix&limit=200&offset=0')
    const paymentAfter = paymentsAfter.items.find((item) => item.id === payment.id)
    if (!paymentAfter) throw new Error('reconciled payment missing')

    const close = await apiJson<{
      expected_cash: number
      closing_amount: number
      closing_difference: number
      expected_by_method: Record<string, number>
      declared_by_method: Record<string, number>
      difference_by_method: Record<string, number>
    }>(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: {
        closing_amount: 95,
        closing_by_method: {
          pix: 5,
          debit: 0,
          credit: 0,
          transfer: 0,
          voucher: 0,
        },
        notes: 'close reconciliation E2E',
      },
    })

    const ledger = await apiJson<{
      items: Array<{ entry_type: string; amount_net: number; sale_id?: string | null }>
      total: number
    }>('/api/v1/finance/ledger?limit=200&offset=0')
    const refundLedgerTotal = ledger.items
      .filter((item) => item.entry_type === 'return_refund' && item.sale_id === sale.id)
      .reduce((sum, item) => sum + item.amount_net, 0)

    return {
      saleTotal: sale.total,
      paymentProvider: payment.provider,
      paymentReference: payment.transaction_ref,
      reconciliationStatus: reconciliation.status,
      reconciliationReplaySameId: reconciliationReplay.id === reconciliation.id,
      reconciliationReplayFlag: reconciliationReplay.replayed,
      invalidReferencePairStatus,
      duplicateReconcileStatus,
      paymentReferenceConflictStatus,
      duplicateExternalRefStatus,
      adjustmentStatus: adjustment.status,
      adjustmentReplaySameId: adjustmentReplay.id === adjustment.id,
      adjustmentReplayFlag: adjustmentReplay.replayed,
      adjustmentAuditCount: adjustmentAudit.items.filter((item) => item.resource_id === payment.id).length,
      paymentAfterStatus: paymentAfter.reconciliation_status,
      paymentAfterAmount: paymentAfter.reconciled_amount,
      paymentAfterFee: paymentAfter.reconciled_fee,
      returnDue: saleReturn.refund_due,
      cashRefundStatus: cashRefund.status,
      cashRefundRemaining: cashRefund.remaining_amount,
      cashRefundReplaySameId: cashRefundReplay.id === cashRefund.id,
      cashRefundReplayFlag: cashRefundReplay.replayed,
      cashRefundLateReplaySameId: cashRefundLateReplay.id === cashRefund.id,
      cashRefundLateReplayFlag: cashRefundLateReplay.replayed,
      cashRefundLateReplayRemaining: cashRefundLateReplay.remaining_amount,
      pixRefundStatus: pixRefund.status,
      pixRefundRemaining: pixRefund.remaining_amount,
      overRefundStatus,
      refundSummaryStatus: refundSummary.status,
      refundSummarySettled: refundSummary.settled_amount,
      refundSummaryRemaining: refundSummary.remaining_amount,
      closeExpectedCash: close.expected_cash,
      closeCashDifference: close.closing_difference,
      closeExpectedPix: close.expected_by_method.pix,
      closePixDifference: close.difference_by_method.pix,
      refundLedgerTotal,
    }
  }, suffix)

  expect(result.saleTotal).toBe(20)
  expect(result.paymentProvider).toBe('e2e-provider')
  expect(result.paymentReference).toContain('TX-')
  expect(result.reconciliationStatus).toBe('divergent')
  expect(result.reconciliationReplaySameId).toBe(true)
  expect(result.reconciliationReplayFlag).toBe(true)
  expect(result.invalidReferencePairStatus).toBe(422)
  expect(result.duplicateReconcileStatus).toBe(409)
  expect(result.paymentReferenceConflictStatus).toBe(409)
  expect(result.duplicateExternalRefStatus).toBe(409)
  expect(result.adjustmentStatus).toBe('reconciled')
  expect(result.adjustmentReplaySameId).toBe(true)
  expect(result.adjustmentReplayFlag).toBe(true)
  expect(result.adjustmentAuditCount).toBe(1)
  expect(result.paymentAfterStatus).toBe('reconciled')
  expect(result.paymentAfterAmount).toBe(20)
  expect(result.paymentAfterFee).toBe(1)
  expect(result.returnDue).toBe(20)
  expect(result.cashRefundStatus).toBe('partial')
  expect(result.cashRefundRemaining).toBe(15)
  expect(result.cashRefundReplaySameId).toBe(true)
  expect(result.cashRefundReplayFlag).toBe(true)
  expect(result.cashRefundLateReplaySameId).toBe(true)
  expect(result.cashRefundLateReplayFlag).toBe(true)
  expect(result.cashRefundLateReplayRemaining).toBe(15)
  expect(result.pixRefundStatus).toBe('settled')
  expect(result.pixRefundRemaining).toBe(0)
  expect(result.overRefundStatus).toBe(409)
  expect(result.refundSummaryStatus).toBe('settled')
  expect(result.refundSummarySettled).toBe(20)
  expect(result.refundSummaryRemaining).toBe(0)
  expect(result.closeExpectedCash).toBe(95)
  expect(result.closeCashDifference).toBe(0)
  expect(result.closeExpectedPix).toBe(5)
  expect(result.closePixDifference).toBe(0)
  expect(result.refundLedgerTotal).toBe(-20)

  await page.getByRole('link', { name: 'Financeiro' }).click()
  await expect(page).toHaveURL(/\/finance$/)
  await expect(page.getByText('Financeiro e conciliação')).toBeVisible()
})


test('supports a net-negative digital close after refunding an earlier sale', async ({ page }) => {
  await login(page)
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 16)

  const result = await page.evaluate(async (suffix) => {
    const { apiJson } = await import('/src/lib/api.ts')

    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-NEG-REFUND-${suffix}`,
        barcode: null,
        name: `Produto Refund Negativo ${suffix}`,
        description: null,
        unit: 'UN',
        cost_price: 3,
        price_cash: 10,
        promo_price: null,
        min_stock: 0,
        active: true,
      },
    })

    await apiJson('/api/v1/inventory/adjust', {
      method: 'POST',
      body: {
        product_id: product.id,
        delta: 1,
        reason: 'Carga E2E refund negativo',
        type: 'adjustment',
      },
    })

    const originalCash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'original sale for negative refund close' },
    })

    const sale = await apiJson<{ id: string }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: originalCash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
        payments: [{ method: 'pix', amount: 10 }],
      },
    })

    await apiJson(`/api/v1/cash/sessions/${originalCash.id}/close`, {
      method: 'POST',
      body: {
        closing_amount: 0,
        closing_by_method: {
          pix: 10,
          debit: 0,
          credit: 0,
          transfer: 0,
          voucher: 0,
        },
        notes: 'close original sale session',
      },
    })

    const saleDetail = await apiJson<{ items: Array<{ id: string }> }>(
      `/api/v1/sales/${sale.id}`,
    )
    const saleReturn = await apiJson<{ id: string; refund_due: number }>(
      `/api/v1/sales/${sale.id}/returns`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          kind: 'return',
          reason: 'Refund em sessao posterior',
          items: [{ sale_item_id: saleDetail.items[0].id, qty: 1, restock: true }],
        },
      },
    )

    const refundCash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'refund-only session' },
    })

    await apiJson(`/api/v1/finance/returns/${saleReturn.id}/refunds`, {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        method: 'pix',
        amount: 10,
        provider: 'e2e-provider',
        external_ref: `NEG-REFUND-${suffix}`,
        cash_session_id: refundCash.id,
        notes: 'refund without same-session pix sale',
      },
    })

    return apiJson<{
      expected_by_method: Record<string, number>
      declared_by_method: Record<string, number>
      difference_by_method: Record<string, number>
      expected_cash: number
      closing_difference: number
    }>(`/api/v1/cash/sessions/${refundCash.id}/close`, {
      method: 'POST',
      body: {
        closing_amount: 0,
        closing_by_method: {
          pix: -10,
          debit: 0,
          credit: 0,
          transfer: 0,
          voucher: 0,
        },
        notes: 'net-negative digital close E2E',
      },
    })
  }, suffix)

  expect(result.expected_cash).toBe(0)
  expect(result.closing_difference).toBe(0)
  expect(result.expected_by_method.pix).toBe(-10)
  expect(result.declared_by_method.pix).toBe(-10)
  expect(result.difference_by_method.pix).toBe(0)
})


test('rejects invalid finance date filters', async ({ page }) => {
  await login(page)

  const result = await page.evaluate(async () => {
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
      invalidDate: await statusFor('/api/v1/finance/payments?from=not-a-date&limit=1&offset=0'),
      invertedRange: await statusFor('/api/v1/finance/dashboard?from=2026-09-23&to=2026-09-22'),
    }
  })

  expect(result.invalidDate).toBe(422)
  expect(result.invertedRange).toBe(422)
})


test('finance UI retries lost responses with the original idempotency key', async ({ page }) => {
  await login(page)
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 16)

  const setup = await page.evaluate(async (suffix) => {
    const { apiJson } = await import('/src/lib/api.ts')

    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-FIN-UI-${suffix}`,
        barcode: null,
        name: `Produto Finance UI ${suffix}`,
        description: null,
        unit: 'UN',
        cost_price: 3,
        price_cash: 10,
        promo_price: null,
        min_stock: 0,
        active: true,
      },
    })
    await apiJson('/api/v1/inventory/adjust', {
      method: 'POST',
      body: {
        product_id: product.id,
        delta: 1,
        reason: 'Carga E2E finance UI',
        type: 'adjustment',
      },
    })
    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'finance UI retry E2E' },
    })
    const sale = await apiJson<{ id: string }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: cash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
        payments: [{ method: 'pix', amount: 10 }],
      },
    })
    const paymentList = await apiJson<{
      items: Array<{ id: string; sale_id: string }>
    }>('/api/v1/finance/payments?limit=200&offset=0')
    const payment = paymentList.items.find((item) => item.sale_id === sale.id)
    if (!payment) throw new Error('payment not found')

    const saleDetail = await apiJson<{ items: Array<{ id: string }> }>(
      `/api/v1/sales/${sale.id}`,
    )
    const saleReturn = await apiJson<{ id: string }>(
      `/api/v1/sales/${sale.id}/returns`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          kind: 'return',
          reason: 'Finance UI retry',
          items: [{ sale_item_id: saleDetail.items[0].id, qty: 1, restock: true }],
        },
      },
    )

    return { saleId: sale.id, paymentId: payment.id, returnId: saleReturn.id }
  }, suffix)

  await page.getByRole('link', { name: 'Financeiro' }).click()
  await expect(page).toHaveURL(/\/finance$/)

  const paymentKeys: string[] = []
  let losePaymentResponse = true
  await page.route(`**/api/v1/finance/payments/${setup.paymentId}/reconcile`, async (route) => {
    paymentKeys.push(route.request().headers()['idempotency-key'] ?? '')
    if (losePaymentResponse) {
      losePaymentResponse = false
      const response = await route.fetch()
      expect(response.ok()).toBeTruthy()
      await route.abort('failed')
      return
    }
    await route.continue()
  })

  const paymentRow = page.locator('tbody tr').filter({ hasText: setup.saleId }).first()
  await paymentRow.getByRole('button', { name: 'Conciliar' }).click()
  await page.getByLabel('Adquirente/provedor').fill('e2e-ui')
  await page.getByLabel('ID externo').fill(`UI-REC-${suffix}`)
  await page.getByRole('button', { name: 'Salvar conciliação' }).click()
  await expect(page.getByText(/erro|falha|network|fetch/i)).toBeVisible()
  await page.getByRole('button', { name: 'Salvar conciliação' }).click()

  expect(paymentKeys).toHaveLength(2)
  expect(paymentKeys[0]).not.toBe('')
  expect(paymentKeys[1]).toBe(paymentKeys[0])
  await expect(paymentRow.getByRole('button', { name: 'Ajustar' })).toBeVisible()

  const refundKeys: string[] = []
  let loseRefundResponse = true
  await page.route(`**/api/v1/finance/returns/${setup.returnId}/refunds`, async (route) => {
    refundKeys.push(route.request().headers()['idempotency-key'] ?? '')
    if (loseRefundResponse) {
      loseRefundResponse = false
      const response = await route.fetch()
      expect(response.ok()).toBeTruthy()
      await route.abort('failed')
      return
    }
    await route.continue()
  })

  const refundSection = page.getByRole('heading', { name: 'Reembolsos de devoluções' }).locator('..')
  const refundRow = refundSection.locator('tbody tr').filter({ hasText: setup.saleId }).first()
  await refundRow.getByRole('button', { name: 'Liquidar' }).click()
  await page.getByRole('button', { name: 'Registrar liquidação' }).click()
  await expect(page.getByText(/erro|falha|network|fetch/i)).toBeVisible()
  await page.getByRole('button', { name: 'Registrar liquidação' }).click()

  expect(refundKeys).toHaveLength(2)
  expect(refundKeys[0]).not.toBe('')
  expect(refundKeys[1]).toBe(refundKeys[0])
  await expect(refundRow.getByText('0.00')).toBeVisible()
})


test('serializes competing cash refunds against available drawer cash', async ({ page }) => {
  await login(page)
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 12)

  const result = await page.evaluate(async (suffix) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')

    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-CASH-RACE-${suffix}`,
        barcode: null,
        name: `Produto Cash Race ${suffix}`,
        description: null,
        unit: 'UN',
        cost_price: 20,
        price_cash: 60,
        promo_price: null,
        min_stock: 0,
        active: true,
      },
    })
    await apiJson('/api/v1/inventory/adjust', {
      method: 'POST',
      body: {
        product_id: product.id,
        delta: 2,
        reason: 'Carga E2E concorrencia reembolso',
        type: 'adjustment',
      },
    })

    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 100, notes: 'cash refund race E2E' },
    })

    const createReturnedSale = async (label: string) => {
      const sale = await apiJson<{ id: string }>('/api/v1/sales', {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          cash_session_id: cash.id,
          customer_id: null,
          discount_value: 0,
          items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
          payments: [{ method: 'pix', amount: 60 }],
        },
      })
      const detail = await apiJson<{ items: Array<{ id: string }> }>(
        `/api/v1/sales/${sale.id}`,
      )
      const saleReturn = await apiJson<{ id: string }>(
        `/api/v1/sales/${sale.id}/returns`,
        {
          method: 'POST',
          headers: { 'Idempotency-Key': crypto.randomUUID() },
          body: {
            kind: 'return',
            reason: `Cash race ${label}`,
            items: [{ sale_item_id: detail.items[0].id, qty: 1, restock: true }],
          },
        },
      )
      return saleReturn.id
    }

    const [returnA, returnB] = await Promise.all([
      createReturnedSale('A'),
      createReturnedSale('B'),
    ])

    const settle = async (returnId: string) => {
      try {
        await apiJson(`/api/v1/finance/returns/${returnId}/refunds`, {
          method: 'POST',
          headers: { 'Idempotency-Key': crypto.randomUUID() },
          body: {
            method: 'cash',
            amount: 60,
            provider: null,
            external_ref: null,
            cash_session_id: cash.id,
            notes: 'cash race E2E',
          },
        })
        return 201
      } catch (error) {
        if (error instanceof APIError) return error.status
        throw error
      }
    }

    const statuses = await Promise.all([settle(returnA), settle(returnB)])
    const refunds = await apiJson<{
      items: Array<{
        return_id: string
        settled_amount: number
        remaining_amount: number
        status: string
      }>
    }>('/api/v1/finance/refunds?limit=200&offset=0')
    const relevant = refunds.items.filter(
      (item) => item.return_id === returnA || item.return_id === returnB,
    )

    const close = await apiJson<{
      expected_cash: number
      closing_difference: number
      expected_by_method: Record<string, number>
    }>(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: {
        closing_amount: 40,
        closing_by_method: {
          pix: 120,
          debit: 0,
          credit: 0,
          transfer: 0,
          voucher: 0,
        },
        notes: 'cash refund race close',
      },
    })

    return {
      statuses: statuses.sort((a, b) => a - b),
      settledTotal: relevant.reduce((sum, item) => sum + item.settled_amount, 0),
      remainingTotal: relevant.reduce((sum, item) => sum + item.remaining_amount, 0),
      settledCount: relevant.filter((item) => item.status === 'settled').length,
      pendingCount: relevant.filter((item) => item.status === 'pending').length,
      expectedCash: close.expected_cash,
      cashDifference: close.closing_difference,
      expectedPix: close.expected_by_method.pix,
    }
  }, suffix)

  expect(result.statuses).toEqual([201, 409])
  expect(result.settledTotal).toBe(60)
  expect(result.remainingTotal).toBe(60)
  expect(result.settledCount).toBe(1)
  expect(result.pendingCount).toBe(1)
  expect(result.expectedCash).toBe(40)
  expect(result.cashDifference).toBe(0)
  expect(result.expectedPix).toBe(120)
})
