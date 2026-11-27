import { expect, test } from './fixtures'

test('real backend rotates sessions across tabs and confirms shared logout', async ({ page, context }) => {
  test.skip(process.env.E2E_DISPOSABLE_DATABASE !== '1', 'requires an explicitly disposable backend')
  await page.goto('/login')
  await page.getByLabel(/E-mail/).fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar', exact: true }).click()
  await expect(page).toHaveURL(/\/home$/)
  const other = await context.newPage()
  try {
    await other.goto('/setup')
    await expect(other.getByRole('button', { name: 'Sair', exact: true })).toBeVisible()
    const refresh = (tab: typeof page) => tab.evaluate(async () => {
      const api = await import('/src/lib/api.ts')
      const ok = await api.refreshAccessToken()
      if (!ok) return { ok }
      const me = await api.apiJson<{ id: string; tenant_id: string }>('/api/v1/auth/me')
      return { ok, id: me.id, tenant: me.tenant_id }
    })
    const [first, second] = await Promise.all([refresh(page), refresh(other)])
    expect(first.ok).toBe(true)
    expect(second).toEqual(first)
    expect(first).toMatchObject({ id: expect.any(String), tenant: expect.any(String) })
    const cookiesBefore = (await context.cookies()).filter((cookie) => cookie.httpOnly)
    expect(cookiesBefore.length).toBeGreaterThan(0)
    await other.getByRole('button', { name: 'Sair', exact: true }).click()
    await expect(other).toHaveURL(/\/login$/)
    await expect(page).toHaveURL(/\/login$/)
    expect((await context.cookies()).filter((cookie) => cookie.httpOnly)).toHaveLength(0)
    await page.goto('/setup')
    await expect(page).toHaveURL(/\/login$/)
    await other.getByLabel(/E-mail/).fill('admin-b@sistema.local')
    await other.getByLabel('Senha', { exact: true }).fill('admin123')
    await other.getByRole('button', { name: 'Entrar', exact: true }).click()
    await expect(other).toHaveURL(/\/home$/)
    await expect(page).toHaveURL(/\/login$/)
    const newIdentity = await other.evaluate(async () => {
      const { apiJson } = await import('/src/lib/api.ts')
      const me = await apiJson<{ id: string; tenant_id: string }>('/api/v1/auth/me')
      return { id: me.id, tenant: me.tenant_id }
    })
    expect(newIdentity.id).not.toBe(first.id)
    expect(newIdentity.tenant).not.toBe(first.tenant)
  } finally { await other.close() }
})
