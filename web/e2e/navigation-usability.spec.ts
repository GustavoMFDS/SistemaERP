import { expect, test } from '@playwright/test'

test('navegação clara em Início, Produtos, Estoque e Configurar loja', async ({ page }) => {
  const errors:string[] = []
  page.on('pageerror', e => errors.push(e.message))
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
  await page.getByRole('link', { name: 'Início' }).click()
  await expect(page.getByRole('heading', { name: /Sua loja está em movimento/ })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Acessos principais' })).toBeVisible()
  await page.getByRole('link', { name: 'Produtos', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Produtos da loja' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Editar códigos e dados fiscais' })).toBeVisible()
  await expect(page.getByRole('cell', { name: 'NCM', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Editar códigos e dados fiscais' }).click()
  await expect(page.getByRole('cell', { name: 'NCM', exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Estoque', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Estoque da loja' })).toBeVisible()
  await expect(page.getByText('Cadastrar estoque inicial por planilha', { exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Configurar loja' }).click()
  await expect(page.getByRole('heading', { name: 'Prepare sua loja em poucos passos' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Resumo da configuração' })).toBeVisible()
  expect(errors).toEqual([])
})

test('busca de produto no PDV consulta catálogo completo online', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await page.getByRole('link', { name: 'PDV', exact: true }).click()
  const result = page.waitForResponse(r => r.url().includes('/api/v1/products?query=') && r.url().includes('limit=200'))
  await page.getByLabel('Busca rápida por nome, SKU ou código').fill('arroz')
  const response = await result
  expect(response.status()).toBe(200)
})
