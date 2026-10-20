import { expect, test } from '@playwright/test'

test('sessão em verificação informa progresso sem mostrar dados protegidos', async ({ page }) => {
  let releaseRequest!: () => void
  const released = new Promise<void>((resolve) => { releaseRequest = resolve })

  await page.route('**/api/v1/auth/refresh', async (route) => {
    await released
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ message: 'Sessão encerrada' }),
    })
  })

  await page.goto('/home')
  await expect(page.getByRole('status')).toContainText('Verificando acesso à loja')
  await expect(page.getByRole('heading', { name: /Sua loja está em movimento/ })).toHaveCount(0)

  releaseRequest()
  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByRole('heading', { name: 'Bem-vindo de volta' })).toBeVisible()
})
