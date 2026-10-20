import { expect, test } from './fixtures'
import type { Page } from '@playwright/test'

const oldPassword = 'Senha-Original-2026!'
const newPassword = 'Senha-Renovada-2026!'
const apiBase = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8080'

async function login(page: Page, identity: string, password: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill(identity)
  await page.getByLabel('Senha', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
}

test('recuperação presencial consome link único e revoga sessões anteriores', async ({ page, browser }) => {
  test.setTimeout(90_000)

  const username = 'reset_' + crypto.randomUUID().replace(/-/g, '').slice(0, 12)
  await login(page, 'admin@sistema.local', 'admin123')

  // Create a real, tenant-scoped staff account through the application API.
  const invitation = await page.evaluate(async (identity) => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ token: string; invitation: { username: string; email: string } }>(
      '/api/v1/staff/invitations',
      { method: 'POST', body: { name: 'Funcionário Recuperação', username: identity, email: '', role: 'cashier' } },
    )
  }, username)
  expect(invitation.invitation.username).toBe(username)
  expect(invitation.invitation.email).toBe('')

  const employeeContext = await browser.newContext()
  const resetContext = await browser.newContext()
  try {
    const employee = await employeeContext.newPage()
    await employee.goto('/accept-invite#token=' + invitation.token)
    await expect(employee).toHaveURL(/\/accept-invite$/)
    await employee.getByLabel('Criar senha (12 a 72 caracteres)').fill(oldPassword)
    await employee.getByLabel('Confirmar nova senha').fill(oldPassword)
    await employee.getByRole('button', { name: 'Ativar minha conta' }).click()
    await expect(employee.getByText(/Conta ativada/)).toBeVisible()
    await login(employee, username, oldPassword)

    const previousToken = await employee.evaluate(async () => {
      const { getToken } = await import('/src/lib/auth.ts')
      return getToken()
    })
    expect(previousToken).toBeTruthy()

    // Seed one durable, tenant/user-scoped offline intent without dispatching it.
    // A full PDV offline sale plus subsequent reconciliation remains a separate
    // acceptance scenario; this only verifies reset never erases the local queue.
    const pendingOffline = await employee.evaluate(async () => {
      const { enqueueRequest, getQueueItems } = await import('/src/lib/offlineQueue.ts')
      const id = enqueueRequest({
        method: 'POST', path: '/api/v1/sales',
        body: {
          cash_session_id: crypto.randomUUID(),
          items: [{ product_id: crypto.randomUUID(), qty: 1, discount_value: 0 }],
          discount_value: 0, payments: [{ method: 'cash', amount: 10 }],
        },
      })
      const item = getQueueItems().find((candidate) => candidate.id === id)
      return { id, key: item?.headers?.['Idempotency-Key'] }
    })
    expect(pendingOffline.key).toBe(pendingOffline.id)

    await page.goto('/staff')
    const row = page.getByRole('region', { name: 'Equipe da loja' })
      .getByRole('row').filter({ hasText: username })
    await expect(row.getByRole('button', { name: 'Recuperar senha' })).toBeEnabled()
    page.once('dialog', (dialog) => void dialog.accept())
    await row.getByRole('button', { name: 'Recuperar senha' }).click()
    const linkField = page.getByLabel('Link para recuperar senha')
    await expect(linkField).toHaveValue(/\/reset-password#token=[a-f0-9]{64}$/)
    let link = await linkField.inputValue()
    let secret = new URL(link).hash.slice('#token='.length)
    expect(secret).toMatch(/^[a-f0-9]{64}$/)

    // Re-loading the manager screen must not expose a copy of the secret.
    await page.reload()
    await expect(page.getByLabel('Link para recuperar senha')).toHaveCount(0)
    const pending = page.locator('details[aria-label="Recuperações pendentes"]')
    await expect(pending).toBeVisible()
    await pending.locator('summary').click()
    await expect(pending).toContainText(username)

    // An administrator can cancel a link after reopening the team screen.
    await pending.getByRole('button', { name: 'Cancelar link' }).click()
    await expect(pending.getByText(username)).toHaveCount(0)
    const cancelledStatus = await page.evaluate(async (token) => {
      const { APIError, apiJson } = await import('/src/lib/api.ts')
      try {
        await apiJson('/api/v1/staff/complete-password-reset', {
          method: 'POST', body: { token, password: 'Senha-Revogada-2026!' },
        })
        return 200
      } catch (error) {
        if (error instanceof APIError) return error.status
        throw error
      }
    }, secret)
    expect(cancelledStatus).toBe(422)

    // A fresh link replaces the revoked one without changing the old password.
    const priorLink = link
    page.once('dialog', (dialog) => void dialog.accept())
    await row.getByRole('button', { name: 'Recuperar senha' }).click()
    await expect(linkField).toHaveValue(/\/reset-password#token=[a-f0-9]{64}$/)
    link = await linkField.inputValue()
    secret = new URL(link).hash.slice('#token='.length)
    expect(link).not.toBe(priorLink)

    const reset = await resetContext.newPage()
    const requestedUrls: string[] = []
    reset.on('request', (request) => requestedUrls.push(request.url()))
    await reset.goto(link)
    await expect(reset).toHaveURL(/\/reset-password$/)
    await expect(reset.getByRole('heading', { name: 'Recuperar meu acesso' })).toBeVisible()
    expect(requestedUrls.every((url) => !url.includes(secret))).toBe(true)
    const secretAtRest = await reset.evaluate((token) =>
      [...Object.values(localStorage), ...Object.values(sessionStorage)]
        .some((value) => value.includes(token)), secret)
    expect(secretAtRest).toBe(false)

    await reset.getByLabel('Nova senha (12 a 72 caracteres)').fill(newPassword)
    await reset.getByLabel('Confirmar nova senha').fill(newPassword)
    await reset.getByRole('button', { name: 'Alterar minha senha' }).click()
    await expect(reset.getByRole('status')).toContainText('Senha alterada')

    // An already issued JWT and refresh cookie must not survive the reset.
    const stale = await employee.evaluate(async ({ token, base }) => {
      const response = await fetch(base + '/api/v1/auth/me', {
        headers: { Authorization: 'Bearer ' + token }, credentials: 'omit',
      })
      const refresh = await fetch(base + '/api/v1/auth/refresh', {
        method: 'POST', credentials: 'include',
      })
      return { access: response.status, refresh: refresh.status }
    }, { token: previousToken, base: apiBase })
    expect(stale).toEqual({ access: 401, refresh: 401 })

    // Replay cannot overwrite the new password, even with a different payload.
    const replayStatus = await reset.evaluate(async ({ token, base }) => {
      const response = await fetch(base + '/api/v1/staff/complete-password-reset', {
        method: 'POST', credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, password: 'Senha-Nova-Invalida-2026!' }),
      })
      return response.status
    }, { token: secret, base: apiBase })
    expect(replayStatus).toBe(422)

    await login(employee, username, newPassword)
    const me = await employee.evaluate(async () => {
      const { apiJson } = await import('/src/lib/api.ts')
      return apiJson<{ email: string; roles: string[]; permissions: string[] }>('/api/v1/auth/me')
    })
    expect(me.email).toBe('')
    expect(me.roles).toContain('cashier')
    expect(me.permissions).not.toContain('team:manage')

    const restoredQueue = await employee.evaluate(async () => {
      const { getQueueItems } = await import('/src/lib/offlineQueue.ts')
      return getQueueItems().map((item) => ({
        id: item.id, key: item.headers?.['Idempotency-Key'], state: item.state,
      }))
    })
    expect(restoredQueue).toContainEqual({
      id: pendingOffline.id, key: pendingOffline.key, state: 'pending',
    })
  } finally {
    await resetContext.close()
    await employeeContext.close()
  }
})
