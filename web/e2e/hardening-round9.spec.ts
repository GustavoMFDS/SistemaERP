import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('fractional PDV quantities use backend-equivalent cent rounding', async ({ page }) => {
  await login(page)

  const fixture = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    const sku = `R9-FRAC-${crypto.randomUUID().slice(0, 8)}`
    const created = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        sku,
        name: `Round9 Fractional ${sku}`,
        unit: 'KG',
        cost_price: 0.5,
        price_cash: 1,
        min_stock: 0,
        active: true,
      },
    })
    await apiJson('/api/v1/inventory/adjust', {
      method: 'POST',
      body: {
        product_id: created.id,
        delta: 1,
        reason: 'round9 fractional sale stock',
        type: 'adjustment',
      },
    })

    const current = await apiJson<{ session: { id: string } | null }>(
      '/api/v1/cash/sessions/current',
    )
    if (current.session) {
      await apiJson(`/api/v1/cash/sessions/${current.session.id}/close`, {
        method: 'POST',
        body: { closing_amount: 0, notes: 'round9 fractional pre-cleanup' },
      })
    }

    return { id: created.id, sku }
  })

  await page.getByRole('link', { name: 'PDV' }).click()
  await page.getByRole('button', { name: 'Abrir' }).click()
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .not.toBe('')

  await page.getByLabel('Produto').selectOption(fixture.id)

  const qty = page.getByLabel('Qtd')
  await qty.fill('0.0005')
  await page.getByRole('button', { name: 'Adicionar' }).click()
  await expect(page.getByText(/Quantidade inválida/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Remover' })).toHaveCount(0)

  await qty.fill('0.005')
  await page.getByRole('button', { name: 'Adicionar' }).click()
  await page.getByRole('button', { name: 'Adicionar' }).click()

  await expect(page.getByRole('button', { name: 'Remover' })).toHaveCount(2)
  await expect(page.getByText('0.005')).toHaveCount(2)
  await expect(page.getByText('R$ 0.02')).toBeVisible()

  await page.getByRole('button', { name: 'Finalizar' }).click()
  await expect(page.getByText(/Venda finalizada:/)).toBeVisible()
  await expect(page.getByText(/Total R\$ 0\.02/)).toBeVisible()

  await page.getByRole('button', { name: 'Fechar caixa' }).click()
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .toBe('')
})
