import { test as base, expect } from '@playwright/test'

/**
 * Most E2E scenarios exercise normal store tasks, not the optional fiscal
 * onboarding prompt. Dismiss that real prompt as an operator would.
 * The dedicated fiscal-setup-reminder.spec.ts imports the raw Playwright
 * runner to assert the dialog is shown, hidden, and repeated correctly.
 */

function disposableE2EApiBaseUrl(): string {
  if (process.env.E2E_DISPOSABLE_DATABASE !== '1') {
    throw new Error('E2E cash cleanup requires E2E_DISPOSABLE_DATABASE=1.')
  }

  const configuredUrl = process.env.E2E_API_BASE_URL ?? 'http://127.0.0.1:8080'
  let parsed: URL
  try {
    parsed = new URL(configuredUrl)
  } catch {
    throw new Error('E2E cash cleanup requires a valid loopback API URL.')
  }

  const loopbackHosts = new Set(['127.0.0.1', 'localhost', '[::1]'])
  if (
    parsed.protocol !== 'http:' ||
    !loopbackHosts.has(parsed.hostname) ||
    parsed.pathname !== '/' ||
    parsed.search !== '' ||
    parsed.hash !== '' ||
    parsed.username !== '' ||
    parsed.password !== ''
  ) {
    throw new Error('E2E cash cleanup is limited to a loopback API on an explicitly disposable database.')
  }

  return parsed.origin
}

async function cleanupOpenCashSessions(
  request: import('@playwright/test').APIRequestContext,
): Promise<void> {
  const apiBaseUrl = disposableE2EApiBaseUrl()
  const loginResponse = await request.post(apiBaseUrl + '/api/v1/auth/login', {
    data: { email: 'admin@sistema.local', password: 'admin123' },
  })
  if (!loginResponse.ok()) {
    throw new Error('E2E cash cleanup login failed (' + loginResponse.status() + ') at ' + loginResponse.url() + '.')
  }

  const login = (await loginResponse.json()) as {
    token?: { access_token?: string }
  }
  const accessToken = login.token?.access_token
  if (!accessToken) throw new Error('E2E cash cleanup received no access token.')
  const headers = { Authorization: 'Bearer ' + accessToken }

  for (let attempt = 0; attempt < 10; attempt += 1) {
    const currentResponse = await request.get(apiBaseUrl + '/api/v1/cash/sessions/current', { headers })
    if (currentResponse.status() === 404) return
    if (!currentResponse.ok()) {
      throw new Error(
        'E2E cash cleanup current-session query failed (' + currentResponse.status() + ').',
      )
    }

    const current = (await currentResponse.json()) as {
      id?: string
      status?: string
      expected_cash?: number
    }
    if (current.status !== 'open' || !current.id) {
      throw new Error('E2E cash cleanup received an invalid open-session response.')
    }
    const expectedCash = current.expected_cash
    if (typeof expectedCash !== 'number' || !Number.isFinite(expectedCash) || expectedCash < 0) {
      throw new Error('E2E cash cleanup received no valid expected cash balance.')
    }

    const closeResponse = await request.post(
      apiBaseUrl + '/api/v1/cash/sessions/' + encodeURIComponent(current.id) + '/close',
      {
        headers,
        data: { closing_amount: expectedCash, notes: 'Playwright E2E fixture cleanup' },
      },
    )
    if (!closeResponse.ok()) {
      throw new Error('E2E cash cleanup close failed (' + closeResponse.status() + ').')
    }
    const closeResult = (await closeResponse.json()) as { closing_difference?: number }
    if (typeof closeResult.closing_difference !== 'number' || closeResult.closing_difference !== 0) {
      throw new Error('E2E cash cleanup closed a session with a non-zero cash difference.')
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

export const cashTest = test.extend<{ cashSessionCleanup: void }>({
  cashSessionCleanup: [
    async ({ request }, runCleanupScenario) => {
      await cleanupOpenCashSessions(request)
      try {
        await runCleanupScenario()
      } finally {
        await cleanupOpenCashSessions(request)
      }
    },
    { auto: true },
  ],
})

export { expect }
