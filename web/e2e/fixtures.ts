import { test as base, expect } from '@playwright/test'

/**
 * Most E2E scenarios exercise normal store tasks, not the optional fiscal
 * onboarding prompt. Dismiss that real prompt as an operator would.
 * The dedicated fiscal-setup-reminder.spec.ts imports the raw Playwright
 * runner to assert the dialog is shown, hidden, and repeated correctly.
 */

async function cleanupOpenCashSessions(
  request: import('@playwright/test').APIRequestContext,
): Promise<void> {
  const loginResponse = await request.post('/api/v1/auth/login', {
    data: { email: 'admin@sistema.local', password: 'admin123' },
  })
  if (!loginResponse.ok()) {
    throw new Error('E2E cash cleanup login failed (' + loginResponse.status() + ').')
  }

  const login = (await loginResponse.json()) as {
    token?: { access_token?: string }
  }
  const accessToken = login.token?.access_token
  if (!accessToken) throw new Error('E2E cash cleanup received no access token.')
  const headers = { Authorization: 'Bearer ' + accessToken }

  for (let attempt = 0; attempt < 10; attempt += 1) {
    const currentResponse = await request.get('/api/v1/cash/sessions/current', { headers })
    if (currentResponse.status() === 404) return
    if (!currentResponse.ok()) {
      throw new Error(
        'E2E cash cleanup current-session query failed (' + currentResponse.status() + ').',
      )
    }

    const current = (await currentResponse.json()) as { id?: string; status?: string }
    if (current.status !== 'open' || !current.id) {
      throw new Error('E2E cash cleanup received an invalid open-session response.')
    }

    const closeResponse = await request.post(
      '/api/v1/cash/sessions/' + encodeURIComponent(current.id) + '/close',
      {
        headers,
        data: { closing_amount: 0, notes: 'Playwright E2E fixture cleanup' },
      },
    )
    if (!closeResponse.ok()) {
      throw new Error('E2E cash cleanup close failed (' + closeResponse.status() + ').')
    }
  }

  throw new Error('E2E cash cleanup reached its open-session safety limit.')
}

export const test = base.extend({
  page: async ({ page }, runScenario) => {
    await page.addLocatorHandler(
      page.getByRole('dialog', { name: 'Sua configuração fiscal ainda não está concluída' }),
      async (dialog) => {
        await dialog.getByRole('button', { name: 'Agora não' }).click()
      },
    )
    await runScenario(page)
  },
})

export const cashTest = test.extend({
  page: async ({ page, request }, runScenario) => {
    await cleanupOpenCashSessions(request)
    try {
      await runScenario(page)
    } finally {
      await cleanupOpenCashSessions(request)
    }
  },
})

export { expect }
