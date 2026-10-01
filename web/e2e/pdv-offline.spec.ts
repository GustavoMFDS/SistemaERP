import { expect, test } from '@playwright/test'

async function waitForCashClosed(page: import('@playwright/test').Page) {
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .toBe('')
}

test('login, sale offline queue and reconnect sync', async ({ page, context }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)

  await page.getByRole('link', { name: 'PDV' }).click()
  await expect(page.getByRole('heading', { name: 'PDV' })).toBeVisible()
  await page.getByRole('button', { name: 'Abrir' }).click()
  await expect(page.getByText(/cash_session_id/)).toBeVisible()

  await page.getByLabel('Produto').selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Adicionar' }).click()

  await context.setOffline(true)
  await page.getByRole('button', { name: 'Finalizar' }).click()
  await expect(page.getByText(/Venda registrada offline/)).toBeVisible()
  await expect(page.getByText(/Pendências: 1/)).toBeVisible()

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await expect(
    page.getByText(/Não é possível fechar o caixa: existem 1 venda\(s\) offline deste caixa/),
  ).toBeVisible()
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .not.toBe('')

  await context.setOffline(false)
  await expect(page.getByText(/Pendências: 0/)).toBeVisible({ timeout: 15000 })
  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await waitForCashClosed(page)
})
