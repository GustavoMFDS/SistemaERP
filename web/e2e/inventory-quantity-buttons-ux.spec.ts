import { expect, test } from './fixtures'

test('ajuste de estoque aceita mais e menos sem teclado e mantém casas decimais', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  await page.goto('/inventory')
  await page.getByText('Ajustar quantidade no estoque').click()
  const input = page.getByLabel('Variação do estoque (positiva para adicionar, negativa para retirar)')
  const add = page.getByRole('button', { name: 'Adicionar uma unidade ao ajuste' })
  const remove = page.getByRole('button', { name: 'Retirar uma unidade do ajuste' })

  await expect(input).toHaveValue('0')
  await add.click()
  await add.click()
  await expect(input).toHaveValue('2')
  await remove.click()
  await remove.click()
  await remove.click()
  await expect(input).toHaveValue('-1')

  await input.fill('2.50')
  await add.click()
  await expect(input).toHaveValue('3.5')
  await remove.click()
  await expect(input).toHaveValue('2.5')
  await expect(page.getByRole('button', { name: 'Aplicar ajuste' })).toBeDisabled()
})
