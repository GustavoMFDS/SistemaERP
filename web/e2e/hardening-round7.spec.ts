import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('shared admin switches CNPJ without leaking tenant catalog or local scope', async ({
  page,
}) => {
  await login(page)

  const tenantSelect = page.getByLabel('Loja ativa')
  await expect(tenantSelect).toBeVisible()
  await expect(tenantSelect.locator('option')).toHaveCount(2)

  await expect(page.getByText('Coca-Cola 2L')).toBeVisible()
  await expect(page.getByText('Arroz E2E Tenant B')).toHaveCount(0)

  await page.evaluate(async () => {
    const { setCashSessionId } = await import('/src/lib/auth.ts')
    setCashSessionId('local-cash-tenant-a')
  })
  page.once('dialog', (dialog) => void dialog.accept())
  await tenantSelect.selectOption({ label: 'Loja E2E B' })

  await expect(page.getByLabel('Loja ativa')).toBeVisible()
  await expect(page.getByLabel('Loja ativa').locator('option:checked')).toHaveText(
    'Loja E2E B',
  )
  await expect(page.getByText('Arroz E2E Tenant B')).toBeVisible()
  await expect(page.getByText('Coca-Cola 2L')).toHaveCount(0)

  const tenantBLocalCash = await page.evaluate(async () => {
    const { getCashSessionId, setCashSessionId } = await import('/src/lib/auth.ts')
    const before = getCashSessionId()
    setCashSessionId('local-cash-tenant-b')
    return before
  })
  expect(tenantBLocalCash).toBe('')

  const tenantB = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    const me = await apiJson<{ tenant_id: string }>('/api/v1/auth/me')
    const tenants = await apiJson<{
      current_tenant_id: string
      items: Array<{ id: string; trade_name?: string | null }>
    }>('/api/v1/auth/tenants')
    return {
      meTenant: me.tenant_id,
      currentTenant: tenants.current_tenant_id,
      selected: tenants.items.find((item) => item.trade_name === 'Loja E2E B')?.id,
    }
  })

  expect(tenantB.selected).toBeTruthy()
  expect(tenantB.meTenant).toBe(tenantB.selected)
  expect(tenantB.currentTenant).toBe(tenantB.selected)

  page.once('dialog', (dialog) => void dialog.accept())
  await page.getByLabel('Loja ativa').selectOption({ label: 'Loja Exemplo' })

  await expect(page.getByLabel('Loja ativa').locator('option:checked')).toHaveText(
    'Loja Exemplo',
  )
  await expect(page.getByText('Coca-Cola 2L')).toBeVisible()
  await expect(page.getByText('Arroz E2E Tenant B')).toHaveCount(0)

  const tenantALocalCash = await page.evaluate(async () => {
    const { getCashSessionId } = await import('/src/lib/auth.ts')
    return getCashSessionId()
  })
  expect(tenantALocalCash).toBe('local-cash-tenant-a')
})
