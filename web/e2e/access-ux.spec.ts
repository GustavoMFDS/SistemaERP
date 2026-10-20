import { expect, test } from '@playwright/test'

test('entrada permite conferir a senha e não altera a senha ao limpar o usuário', async ({ page }) => {
  let submitted: { email?: string; password?: string } | null = null

  await page.route('**/api/v1/auth/login', async (route) => {
    submitted = route.request().postDataJSON() as { email: string; password: string }
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ message: 'detalhe interno de autenticação' }),
    })
  })

  await page.goto('/login')
  const username = page.getByLabel('E-mail ou nome de usuário')
  const password = page.locator('#login-password')
  const enter = page.getByRole('button', { name: 'Entrar' })

  await expect(enter).toBeDisabled()
  await username.fill('  caixa  ')
  await password.fill(' senha com espaços ')
  await expect(password).toHaveAttribute('type', 'password')

  await page.getByRole('button', { name: 'Mostrar senha' }).click()
  await expect(password).toHaveAttribute('type', 'text')
  await page.getByRole('button', { name: 'Ocultar senha' }).click()
  await expect(password).toHaveAttribute('type', 'password')

  await enter.click()
  await expect(page.getByRole('alert')).toContainText('Confira seus dados de acesso')
  await expect(page.getByRole('alert')).not.toContainText('detalhe interno')
  expect(submitted).toEqual({ email: 'caixa', password: ' senha com espaços ' })
})

for (const scenario of [
  {
    name: 'ativação',
    path: '/accept-invite',
    endpoint: '/api/v1/staff/accept-invite',
    label: 'Criar senha',
    submit: 'Ativar minha conta',
    confirmation: 'Conta ativada.',
  },
  {
    name: 'recuperação',
    path: '/reset-password',
    endpoint: '/api/v1/staff/complete-password-reset',
    label: 'Nova senha',
    submit: 'Alterar minha senha',
    confirmation: 'Senha alterada.',
  },
]) {
  test(`${scenario.name} apresenta regras claras e conclui somente com senhas iguais`, async ({ page }) => {
    let submitted: { token?: string; password?: string } | null = null

    await page.route(`**${scenario.endpoint}`, async (route) => {
      submitted = route.request().postDataJSON() as { token: string; password: string }
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{}' })
    })

    await page.goto(`${scenario.path}#token=exemplo-descartavel`)
    await expect(page).toHaveURL(new RegExp(`${scenario.path}$`))
    const password = page.getByLabel(scenario.label)
    const confirmation = page.getByLabel('Confirmar nova senha')
    const submit = page.getByRole('button', { name: scenario.submit })

    await expect(submit).toBeDisabled()
    await password.fill('curta')
    await expect(page.getByText('Use pelo menos 12 caracteres para sua senha.')).toBeVisible()
    await confirmation.fill('diferente')
    await expect(submit).toBeDisabled()
    await expect(page.getByRole('alert')).toContainText('As senhas não são iguais')

    await password.fill('senhaMuitoForte123')
    await confirmation.fill('senhaMuitoForte123')
    await expect(submit).toBeEnabled()
    await page.getByRole('button', { name: 'Mostrar senha' }).first().click()
    await expect(password).toHaveAttribute('type', 'text')

    await submit.click()
    await expect(page.getByRole('status')).toContainText(scenario.confirmation)
    expect(submitted).toEqual({ token: 'exemplo-descartavel', password: 'senhaMuitoForte123' })
  })
}
