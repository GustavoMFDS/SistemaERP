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
  await expect(page.getByRole('region', { name: 'Resumo da configuração' })).toContainText('dados básicos verificados')
  await expect(page.getByText('Etapa 1 de 5')).toBeVisible()
  await page.getByRole('button', { name: /Próxima etapa/ }).click()
  await expect(page.getByText('Etapa 2 de 5')).toBeVisible()
  await page.getByRole('button', { name: /Etapa anterior/ }).click()
  await expect(page.getByText('Etapa 1 de 5')).toBeVisible()
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
  let setupRequests = 0
  await page.route('**/api/v1/setup/**', async (route) => {
    setupRequests += 1
    await route.continue()
  })
  await page.route('**/api/v1/fiscal/**', async (route) => {
    fiscalRequests += 1
    await route.continue()
  })
  await page.goto('/setup')
  await expect(page.getByRole('heading', { name: 'Configurar minha loja' })).toBeVisible()
  await expect(page.getByText('Sem acesso').first()).toBeVisible()
  await expect(page.getByRole('button', { name: 'Salvar dados da minha loja' })).toHaveCount(0)
  expect(fiscalRequests).toBe(0)
  expect(setupRequests).toBe(0)
  await expect(page.getByRole('button', { name: 'Registrar revisão' })).toHaveCount(0)
})

test('revisão manual fica registrada entre recargas, sem virar aprovação fiscal', async ({ page }) => {
  let stockReviewedAt = ''
  await page.route('**/api/v1/setup/reviews**', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200, contentType: 'application/json',
        body: JSON.stringify({ items: stockReviewedAt ? [{ step: 'stock', reviewed_at: stockReviewedAt }] : [] }),
      })
      return
    }
    if (route.request().method() === 'PUT' && route.request().url().endsWith('/stock')) {
      const body = route.request().postDataJSON() as { reviewed: boolean }
      stockReviewedAt = body.reviewed ? '2026-10-08T20:00:00Z' : ''
      await route.fulfill({
        status: 200, contentType: 'application/json',
        body: JSON.stringify({ items: stockReviewedAt ? [{ step: 'stock', reviewed_at: stockReviewedAt }] : [] }),
      })
      return
    }
    await route.abort()
  })
  page.on('dialog', (dialog) => void dialog.accept())
  await login(page, 'admin@sistema.local')
  await page.goto('/setup')
  await page.getByRole('button', { name: /3\\. Estoque inicial/ }).click()
  const reviewSection = page.getByRole('region', { name: 'Revisão registrada da etapa' })
  await expect(reviewSection).toContainText('Ainda não há revisão registrada.')
  await reviewSection.getByRole('button', { name: 'Registrar revisão' }).click()
  await expect(reviewSection).toContainText('Revisão registrada em')
  await expect(page.getByRole('button', { name: /3\\. Estoque inicial/ })).toContainText('Conferir')
  await page.reload()
  await page.getByRole('button', { name: /3\\. Estoque inicial/ }).click()
  await expect(page.getByRole('region', { name: 'Revisão registrada da etapa' })).toContainText('Revisão registrada em')
  await reviewSection.getByRole('button', { name: 'Reabrir revisão' }).click()
  await expect(reviewSection).toContainText('Ainda não há revisão registrada.')
})
