import { expect, test } from '@playwright/test'

async function openPDV(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
  await page.getByRole('link', { name: 'PDV', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'PDV', exact: true })).toBeVisible()
}

test('PDV desktop prioriza venda, pagamento e separa encerramento', async ({ page }) => {
  await page.setViewportSize({ width: 1500, height: 900 })
  await openPDV(page)
  const cash = page.getByRole('region', { name: 'Sessão de caixa' })
  const cart = page.getByRole('region', { name: 'Itens' })
  const payment = page.getByRole('region', { name: 'Conferir e receber' })
  await expect(cash).toBeVisible()
  await expect(cart).toBeVisible()
  await expect(payment).toBeVisible()
  await expect(page.getByRole('navigation', { name: 'Navegação principal' })).toBeVisible()

  const cashBox = await cash.boundingBox()
  const cartBox = await cart.boundingBox()
  const payBox = await payment.boundingBox()
  expect(cashBox && cartBox && payBox).toBeTruthy()
  expect(cartBox!.y).toBeGreaterThan(cashBox!.y)
  expect(payBox!.x).toBeGreaterThan(cartBox!.x + 40)
  expect(Math.abs(payBox!.y - cartBox!.y)).toBeLessThan(8)

  const close = page.getByRole('region', { name: 'Encerrar turno' })
  if (await close.count()) {
    const closeBox = await close.boundingBox()
    expect(closeBox!.y).toBeGreaterThan(cartBox!.y)
    await expect(page.getByRole('button', { name: 'Fechar caixa' })).toBeVisible()
  }
  await expect(page.getByLabel('Código de barras')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Finalizar' })).toBeVisible()
})

test('PDV no celular organiza as tarefas na vertical sem rolagem horizontal geral', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await openPDV(page)
  const cart = page.getByRole('region', { name: 'Itens' })
  const payment = page.getByRole('region', { name: 'Conferir e receber' })
  await expect(cart).toBeVisible()
  await expect(payment).toBeVisible()
  const cartBox = await cart.boundingBox()
  const payBox = await payment.boundingBox()
  expect(cartBox && payBox).toBeTruthy()
  expect(payBox!.y).toBeGreaterThan(cartBox!.y)
  const documentOverflow = await page.evaluate(() => ({
    pageWidth: document.documentElement.scrollWidth,
    viewportWidth: window.innerWidth,
  }))
  expect(documentOverflow.pageWidth).toBeLessThanOrEqual(documentOverflow.viewportWidth + 1)
  await expect(page.getByLabel('Código de barras')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Finalizar' })).toBeVisible()
})
