import { expect, test } from './fixtures'

async function enterAsAdmin(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
}

test('falha na consulta de clientes mostra uma opção de tentar novamente, sem fingir lista vazia', async ({ page }) => {
  let requests = 0
  await page.route('**/api/v1/customers?*', async (route) => {
    requests += 1
    if (requests === 1) {
      await route.fulfill({ status: 503, contentType: 'application/json', body: '{"message":"detalhe interno indisponível"}' })
      return
    }
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ items: [], total: 0, limit: 20, offset: 0 }),
    })
  })
  await enterAsAdmin(page)
  await page.goto('/customers')

  const clients = page.getByRole('region', { name: 'Lista de clientes' })
  const error = page.getByRole('alert')
  await expect(error).toContainText('Não foi possível consultar os clientes')
  await expect(error).not.toContainText('detalhe interno')
  await expect(clients.getByText('Ainda não há clientes cadastrados')).toHaveCount(0)

  await page.getByRole('button', { name: 'Tentar novamente' }).click()
  await expect(error).toHaveCount(0)
  await expect(clients.getByText(/Ainda não há clientes cadastrados/)).toBeVisible()
  expect(requests).toBe(2)
})

test('busca sem resultados é distinguida de loja sem clientes e pode ser limpa', async ({ page }) => {
  await page.route('**/api/v1/customers?*', async (route) => {
    const query = new URL(route.request().url()).searchParams.get('q')
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        items: query ? [] : [{ id: 'cliente-ficticio', name: 'Maria Silva', email: null, phone: null }],
        total: query ? 0 : 1,
        limit: 20, offset: 0,
      }),
    })
  })
  await enterAsAdmin(page)
  await page.goto('/customers')

  const clients = page.getByRole('region', { name: 'Lista de clientes' })
  await expect(clients.getByText('Maria Silva')).toBeVisible()
  await page.getByLabel('Buscar pelo nome').fill('nome que não existe')
  await page.getByRole('button', { name: 'Buscar', exact: true }).click()
  await expect(clients.getByText(/Não encontramos clientes com esse nome/)).toBeVisible()
  await expect(clients.getByText(/Ainda não há clientes cadastrados/)).toHaveCount(0)
  await page.getByRole('button', { name: 'Mostrar todos' }).click()
  await expect(clients.getByText('Maria Silva')).toBeVisible()
})
