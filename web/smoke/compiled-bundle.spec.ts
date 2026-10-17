import { expect, test } from '@playwright/test'

test('compiled ERP interface communicates with the bundled Go API', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))

  await page.goto('/login')
  await expect(page.getByRole('heading', { name: 'Bem-vindo de volta' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Entrar' })).toBeEnabled()

  const ready = await page.evaluate(async () => {
    const result = await fetch('/health/ready')
    return { status: result.status, sameOrigin: new URL(result.url).origin === location.origin }
  })
  expect(ready.status).toBe(200)
  expect(ready.sameOrigin).toBe(true)

  await page.getByLabel('E-mail ou nome de usuário').fill('ci-nonexistent-user')
  await page.getByLabel('Senha').fill('ci-incorrect-password')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page.getByRole('alert')).toContainText('Não foi possível entrar')

  expect(errors).toEqual([])
})
