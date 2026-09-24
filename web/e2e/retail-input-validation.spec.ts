import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('retail modules reject malformed UUIDs before database access', async ({ page }) => {
  await login(page)

  const statuses = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')

    const statusFor = async (
      path: string,
      init?: { method?: string; headers?: Record<string, string>; body?: unknown },
    ) => {
      try {
        await apiJson(path, init)
        return 200
      } catch (error) {
        if (error instanceof APIError) return error.status
        throw error
      }
    }

    return {
      purchaseGet: await statusFor('/api/v1/purchases/not-a-uuid'),
      returnsFilter: await statusFor('/api/v1/returns?sale_id=not-a-uuid&limit=1&offset=0'),
      reconcilePayment: await statusFor('/api/v1/finance/payments/not-a-uuid/reconcile', {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          received_amount: 10,
          fee_amount: 0,
          provider: 'e2e',
          external_ref: crypto.randomUUID(),
          notes: null,
        },
      }),
      settleRefund: await statusFor('/api/v1/finance/returns/not-a-uuid/refunds', {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          method: 'pix',
          amount: 10,
          provider: 'e2e',
          external_ref: crypto.randomUUID(),
          cash_session_id: null,
          notes: null,
        },
      }),
    }
  })

  expect(statuses.purchaseGet).toBe(422)
  expect(statuses.returnsFilter).toBe(422)
  expect(statuses.reconcilePayment).toBe(422)
  expect(statuses.settleRefund).toBe(422)
})
