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

test('PDV recovers an open server cash session after local state is lost', async ({
  page,
}) => {
  await login(page)

  await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    const current = await apiJson<{
      session: { id: string } | null
    }>('/api/v1/cash/sessions/current')
    if (current.session) {
      await apiJson(`/api/v1/cash/sessions/${current.session.id}/close`, {
        method: 'POST',
        body: { closing_amount: 0, notes: 'round7 recovery pre-cleanup' },
      })
    }
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

  const originalID = await page.evaluate(async () => {
    const { getCashSessionId } = await import('/src/lib/auth.ts')
    return getCashSessionId()
  })
  expect(originalID).not.toBe('')

  await page.evaluate(async () => {
    const auth = await import('/src/lib/auth.ts')
    const key = auth.scopedStorageKey('sistemaemgo:cashSession:v2')
    if (!key) throw new Error('cash scope missing')
    localStorage.removeItem(key)
  })

  expect(
    await page.evaluate(async () => {
      const { getCashSessionId } = await import('/src/lib/auth.ts')
      return getCashSessionId()
    }),
  ).toBe('')

  await page.reload()

  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .toBe(originalID)

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(
    page.getByText(/caixa\(s\) local\(is\) ainda aberto\(s\)/),
  ).toBeVisible()
  await expect(page).toHaveURL(/\/pdv$/)

  await page.getByRole('button', { name: 'Fechar caixa' }).click()

  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .toBe('')

  const currentAfterClose = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ session: { id: string } | null }>(
      '/api/v1/cash/sessions/current',
    )
  })
  expect(currentAfterClose.session).toBeNull()
})

test('logout detects server cash even after local cash id is lost outside the PDV', async ({
  page,
}) => {
  await login(page)

  await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    const current = await apiJson<{ session: { id: string } | null }>(
      '/api/v1/cash/sessions/current',
    )
    if (current.session) {
      await apiJson(`/api/v1/cash/sessions/${current.session.id}/close`, {
        method: 'POST',
        body: { closing_amount: 0, notes: 'round9 server logout pre-cleanup' },
      })
    }
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

  await page.evaluate(async () => {
    const auth = await import('/src/lib/auth.ts')
    const key = auth.scopedStorageKey('sistemaemgo:cashSession:v2')
    if (!key) throw new Error('cash scope missing')
    localStorage.removeItem(key)
  })

  await page.getByRole('link', { name: 'Produtos' }).click()
  await expect(page).toHaveURL(/\/products$/)

  await page.getByRole('button', { name: 'Sair' }).click()

  await expect(
    page.getByText(/caixa\(s\) aberto\(s\) no servidor por este usuário/),
  ).toBeVisible()
  await expect(page).toHaveURL(/\/products$/)

  await page.getByRole('link', { name: 'PDV' }).click()
  await expect
    .poll(async () =>
      page.evaluate(async () => {
        const { getCashSessionId } = await import('/src/lib/auth.ts')
        return getCashSessionId()
      }),
    )
    .not.toBe('')

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

