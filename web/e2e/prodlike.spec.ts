import { expect, test } from '@playwright/test'

test.skip(process.env.E2E_PRODLIKE !== '1', 'runs only in the production-like CI job')

const prodlikeBaseURL = process.env.E2E_BASE_URL ?? 'https://staging.example.test:8443'

function cashSessionFromStorage(): string {
  const key = Object.keys(localStorage).find((candidate) =>
    candidate.startsWith('sistemaemgo:cashSession:v2:'),
  )
  return key ? (localStorage.getItem(key) ?? '') : ''
}

test('staging-like HTTPS proxy keeps refresh cookie secure and core POS flow works', async ({
  page,
  context,
}) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)

  const cookies = await context.cookies()
  const refresh = cookies.find((cookie) => cookie.name === '__Host-refresh_token')
  expect(refresh).toBeTruthy()
  expect(refresh?.secure).toBe(true)
  expect(refresh?.httpOnly).toBe(true)
  expect(refresh?.sameSite).toBe('Strict')

  await page.getByRole('link', { name: 'PDV' }).click()
  await page.getByRole('button', { name: 'Abrir' }).click()

  await expect
    .poll(async () => page.evaluate(cashSessionFromStorage))
    .not.toBe('')

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await expect
    .poll(async () => page.evaluate(cashSessionFromStorage))
    .toBe('')

  await page.route(`${prodlikeBaseURL}/api/v1/auth/logout`, async (route) => {
    await route.abort('failed')
  })
  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).not.toHaveURL(/\/login$/)
  await expect(page.getByText(/Não foi possível encerrar a sessão no servidor/)).toBeVisible()

  const cookieAfterFailedLogout = (await context.cookies()).find(
    (cookie) => cookie.name === '__Host-refresh_token',
  )
  expect(cookieAfterFailedLogout).toBeTruthy()

  await page.unroute(`${prodlikeBaseURL}/api/v1/auth/logout`)
  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login$/)

  const cookieAfterSuccessfulLogout = (await context.cookies()).find(
    (cookie) => cookie.name === '__Host-refresh_token',
  )
  expect(cookieAfterSuccessfulLogout).toBeUndefined()

  await context.addCookies([
    {
      name: '__Host-refresh_token',
      value: 'invalid-refresh-token',
      url: prodlikeBaseURL,
      secure: true,
      httpOnly: true,
      sameSite: 'Strict',
    },
  ])
  await page.goto('/products')
  await expect(page).toHaveURL(/\/login$/)

  const invalidCookieAfterRefreshFailure = (await context.cookies()).find(
    (cookie) => cookie.name === '__Host-refresh_token',
  )
  expect(invalidCookieAfterRefreshFailure).toBeUndefined()
})
