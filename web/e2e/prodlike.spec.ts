import { expect, test } from '@playwright/test'

test.skip(process.env.E2E_PRODLIKE !== '1', 'runs only in the production-like CI job')

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

  const cashId = await page.evaluate(async () => {
    const { getCashSessionId } = await import('/src/lib/auth.ts')
    return getCashSessionId()
  })
  expect(cashId).not.toBe('')

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .toBe('')

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login$/)
})
