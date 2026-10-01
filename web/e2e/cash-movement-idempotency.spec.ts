import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('cash movement retries reuse the committed result', async ({ page }) => {
  await login(page)

  const result = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'cash movement idempotency E2E' },
    })

    const key = crypto.randomUUID()
    const path = `/api/v1/cash/sessions/${cash.id}/movements`
    const body = { movement_type: 'supply', amount: 1, notes: null }

    const first = await apiJson<{ id: string; replayed: boolean }>(path, {
      method: 'POST',
      headers: { 'Idempotency-Key': key },
      body,
    })
    const replay = await apiJson<{ id: string; replayed: boolean }>(path, {
      method: 'POST',
      headers: { 'Idempotency-Key': key },
      body,
    })

    let changedPayloadStatus = 0
    try {
      await apiJson(path, {
        method: 'POST',
        headers: { 'Idempotency-Key': key },
        body: { ...body, amount: 2 },
      })
      changedPayloadStatus = 200
    } catch (error) {
      if (error instanceof APIError) changedPayloadStatus = error.status
      else throw error
    }

    await apiJson(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: { closing_amount: 1, notes: 'cash movement idempotency E2E cleanup' },
    })

    return { first, replay, changedPayloadStatus }
  })

  expect(result.first.replayed).toBe(false)
  expect(result.replay.replayed).toBe(true)
  expect(result.replay.id).toBe(result.first.id)
  expect(result.changedPayloadStatus).toBe(409)
})
