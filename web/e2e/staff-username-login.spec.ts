import { expect, test } from './fixtures'

test('funcionário sem e-mail ativa seu acesso e entra por nome de usuário', async ({ page }) => {
  const username = 'caixa_' + crypto.randomUUID().replace(/-/g, '').slice(0, 12)
  const password = 'Senha-Individual-2026!'

  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  const invite = await page.evaluate(async (username) => {
    const { apiJson } = await import('/src/lib/api.ts')
    const me = await apiJson<{ tenant_id: string }>('/api/v1/auth/me')
    const result = await apiJson<{
      invitation: { username: string; email: string; role: string }
      token: string
    }>('/api/v1/staff/invitations', { method: 'POST', body: {
      name: 'Operador sem email', email: '', username, role: 'cashier',
    } })
    return { ...result, tenantId: me.tenant_id }
  }, username)
  expect(invite.invitation.email).toBe('')
  expect(invite.invitation.username).toBe(username)
  expect(invite.invitation.role).toBe('cashier')
  expect(invite.token).toMatch(/^[a-f0-9]{64}$/)

  await page.goto('/accept-invite#token=' + invite.token)
  await page.getByLabel('Criar senha (12 a 72 caracteres)').fill(password)
  await page.getByLabel('Confirmar nova senha').fill(password)
  await page.getByRole('button', { name: 'Ativar minha conta' }).click()
  await expect(page.getByText(/Conta ativada/)).toBeVisible()

  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill(username)
  await page.getByLabel('Senha').fill(password)
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  const me = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ email: string; tenant_id: string; roles: string[]; permissions: string[] }>('/api/v1/auth/me')
  })
  expect(me.email).toBe('')
  expect(me.tenant_id).toBe(invite.tenantId)
  expect(me.roles).toContain('cashier')
  expect(me.permissions).toContain('sale:write')
  expect(me.permissions).not.toContain('team:manage')

  // Another company's administrator cannot take or link that identity.
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin-b@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
  const status = await page.evaluate(async (username) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/staff/invitations', { method: 'POST', body: {
        name: 'Funcionário duplicado', username, email: '', role: 'cashier',
      } })
      return 201
    } catch (e) {
      if (e instanceof APIError) return e.status
      throw e
    }
  }, username)
  expect(status).toBe(409)

  // Suspension in the original company immediately prevents another login.
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
  const disabled = await page.evaluate(async (username) => {
    const { apiJson } = await import('/src/lib/api.ts')
    const team = await apiJson<{
      members: Array<{ id: string; username?: string }>
    }>('/api/v1/staff')
    const target = team.members.find((person) => person.username === username)
    if (!target) throw new Error('Funcionário ativado não encontrado na própria loja')
    await apiJson('/api/v1/staff/' + target.id + '/status', {
      method: 'PUT', body: { active: false },
    })
    return target.id
  }, username)
  expect(disabled).toBeTruthy()

  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill(username)
  await page.getByLabel('Senha').fill(password)
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByRole('alert')).toContainText('Não foi possível entrar')
})
