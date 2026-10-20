import { expect, test } from '@playwright/test'

test('navegação avisa quando acessos e saída não podem ser confirmados', async ({ page }) => {
  const payload = Buffer.from(JSON.stringify({
    sub: 'operador-teste',
    tenant_id: 'loja-teste',
    exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url')
  const accessToken = `eyJhbGciOiJub25lIn0.${payload}.assinatura-ficticia`

  await page.route('**/api/v1/auth/login', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        token: { access_token: accessToken, token_type: 'Bearer', expires_in: 3600 },
        user: { id: 'operador-teste', email: '', name: 'Operador', roles: [] },
      }),
    })
  })
  await page.route('**/api/v1/auth/me', (route) =>
    route.fulfill({ status: 503, contentType: 'application/json', body: '{"message":"detalhe interno"}' }),
  )
  await page.route('**/api/v1/auth/logout', (route) =>
    route.fulfill({ status: 503, contentType: 'application/json', body: '{"message":"detalhe interno"}' }),
  )

  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('operador')
  await page.getByLabel('Senha', { exact: true }).fill('senha-teste')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  const accessWarning = page.getByRole('alert').filter({ hasText: 'Não foi possível carregar seus acessos.' })
  await expect(accessWarning).toBeVisible()
  await expect(accessWarning).not.toContainText('detalhe interno')

  const skipLink = page.getByRole('link', { name: 'Pular para o conteúdo' })
  await skipLink.focus()
  await page.keyboard.press('Enter')
  await expect(page.locator('#conteudo-principal')).toBeFocused()

  await page.getByRole('button', { name: 'Sair' }).click()
  const exitWarning = page.getByRole('alert').filter({ hasText: 'Não foi possível encerrar a sessão no servidor' })
  await expect(exitWarning).toBeVisible()
  await expect(exitWarning).not.toContainText('detalhe interno')
  await expect(page).toHaveURL(/\/home$/)
})
