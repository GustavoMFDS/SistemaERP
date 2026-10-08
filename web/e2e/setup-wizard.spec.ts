import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('gestor acessa assistente e encontra caminhos para cada configuração', async ({ page }) => {
  await login(page, 'admin@sistema.local')
  await page.getByRole('link', { name: 'Configurar loja' }).click()
  await expect(page).toHaveURL(/\/setup$/)
  await expect(page.getByRole('heading', { name: 'Configurar minha loja' })).toBeVisible()
  await expect(page.getByRole('button', { name: /1\. Dados da minha loja/ })).toBeVisible()
  await expect(page.getByText('Razão social:', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: /2\. Produtos e preços/ }).click()
  await expect(page.getByRole('link', { name: /Ir para Produtos/ })).toBeVisible()
  await page.getByRole('button', { name: /3\. Estoque inicial/ }).click()
  await expect(page.getByRole('link', { name: /Ir para Estoque/ })).toBeVisible()
  await page.getByRole('button', { name: /4\. Funcionários e permissões/ }).click()
  await expect(page.getByText(/ainda não cria funcionários nem altera permissões/)).toBeVisible()
  await page.getByRole('button', { name: /5\. Preparar a NFC-e/ }).click()
  await expect(page.getByText(/emissão exige homologação/)).toBeVisible()
})

test('caixa não recebe formulário de empresa ou acesso a dados fiscais', async ({ page }) => {
  await login(page, 'caixa@sistema.local')
  let fiscalRequests = 0
  await page.route('**/api/v1/fiscal/**', async (route) => {
    fiscalRequests += 1
    await route.continue()
  })
  await page.goto('/setup')
  await expect(page.getByRole('heading', { name: 'Configurar minha loja' })).toBeVisible()
  await expect(page.getByText('Sem acesso').first()).toBeVisible()
  await expect(page.getByRole('button', { name: 'Salvar dados da minha loja' })).toHaveCount(0)
  expect(fiscalRequests).toBe(0)
})
