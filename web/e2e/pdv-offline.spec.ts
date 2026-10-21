import { expect, test } from './fixtures'

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
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  await page.getByRole('link', { name: 'PDV' }).click()
  await expect(page.getByRole('heading', { name: 'PDV' })).toBeVisible()
  await page.getByRole('button', { name: 'Abrir' }).click()
  await expect.poll(async () => page.evaluate(async () => {
    const { getCashSessionId } = await import('/src/lib/auth.ts')
    return getCashSessionId()
  })).not.toBe('')
  await expect(page.getByRole('region', { name: 'Caixa aberto' })).toBeVisible()

  await page.getByRole('region', { name: 'Produtos da venda' }).getByRole('combobox').first().selectOption({ index: 1 })
  await page.getByRole('button', { name: 'Adicionar', exact: true }).click()

  await context.setOffline(true)
  await page.getByRole('checkbox', { name: 'Confirmei que o Pix foi recebido.' }).check()
  await page.getByRole('button', { name: 'Finalizar venda' }).click()
  await expect(page.getByText(/Venda salva neste dispositivo \(não confirmada pelo servidor\)/)).toBeVisible()
  await expect(page.getByRole('dialog', { name: 'Venda concluída' })).toHaveCount(0)
  const offlineSaleStatus = page.getByRole('status').filter({ hasText: 'Aguardando confirmação no servidor' })
  await expect(offlineSaleStatus).toHaveClass(/bg-amber-50/)
  await expect(offlineSaleStatus).toContainText('Aguardando confirmação no servidor')
  await expect(page.getByText(/1 venda\(s\) aguardando sincronização/)).toBeVisible()

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await expect(
    page.getByText(/Não é possível fechar o caixa: há 1 venda\(s\) sem confirmação/),
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
  await expect.poll(async () => page.evaluate(async () => {
    const { getQueueSummary } = await import('/src/lib/offlineQueue.ts')
    return getQueueSummary().total
  }), { timeout: 15000 }).toBe(0)
  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await waitForCashClosed(page)
})
