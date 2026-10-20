import { expect, test } from '@playwright/test'

async function submit(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('operador@loja.test')
  await page.getByLabel('Senha', { exact: true }).fill('senha-de-teste')
  await page.getByRole('button', { name: 'Entrar', exact: true }).click()
}

test('login incorreto não expõe detalhes técnicos nem revela existência do usuário', async ({ page }) => {
  await page.route('**/api/v1/auth/login', (route) =>
    route.fulfill({ status: 401, json: { message: 'database: no rows in result set; email unknown' } }))
  await submit(page)
  await expect(page).toHaveURL(/\/login$/)
  const alert = page.getByRole('alert')
  await expect(alert).toHaveText('Não foi possível entrar. Confira seus dados de acesso e tente novamente.')
  await expect(alert).not.toContainText('database')
  await expect(alert).not.toContainText('unknown')
})

test('limite de tentativas de login mostra orientação sem códigos HTTP', async ({ page }) => {
  await page.route('**/api/v1/auth/login', (route) =>
    route.fulfill({ status: 429, json: { message: 'HTTP 429: rate_limit_redis' } }))
  await submit(page)
  await expect(page.getByRole('alert')).toHaveText('Muitas tentativas de acesso. Aguarde um pouco antes de tentar novamente.')
})

test('falha de conexão não informa sucesso de autenticação', async ({ page }) => {
  await page.route('**/api/v1/auth/login', (route) => route.abort('failed'))
  await submit(page)
  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByRole('alert')).toHaveText('Não foi possível acessar o sistema agora. Confira sua conexão e tente novamente.')
})
