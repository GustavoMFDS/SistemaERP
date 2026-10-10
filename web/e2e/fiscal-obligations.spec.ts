import { expect, test } from '@playwright/test'

test('Fiscal: vendas sem autorização aparecem e filtro nunca confunde XML com emissão', async ({ page }) => {
  const userID = 'bd89e4a7-90f9-475e-ad37-7f672ae65cc7'
  const tenantID = 'ae1ad525-cb5a-4208-a6e3-d6bc1cc17885'
  const token = 'eyJhbGciOiJIUzI1NiJ9.' + Buffer.from(JSON.stringify({
    sub: userID, tenant_id: tenantID, exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url') + '.signature'
  const sale1 = 'f27e693b-c4b1-4db2-a257-01e64a937980'
  const sale2 = 'ab1014a5-e3bc-46dd-aa12-d7e7500e70a7'
  const sale3 = '4f439fed-d5fd-40be-bf19-3aa93419905c'
  let filterRequests: string[] = []
  const pending = [
    { sale_id: sale1, created_at: '2026-10-09T12:00:00Z',
      document_kind: 'nfce', sale_status: 'finalized', status: 'pending',
      authorized: false, legacy_review: false },
    { sale_id: sale2, created_at: '2026-10-09T11:00:00Z',
      document_kind: 'nfce', sale_status: 'finalized', status: 'rejected',
      authorized: false, legacy_review: false },
  ]
  const authorized = { sale_id: sale3, created_at: '2026-10-09T10:00:00Z',
    document_kind: 'nfce', sale_status: 'finalized', status: 'authorized',
    authorized: true, legacy_review: false, invoice_id: 'dd11bc2d-415b-47b3-98d3-eec344d3be54' }

  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const reply = (data: unknown, status = 200) => route.fulfill({ status, json: data })
    if (url.pathname === '/api/v1/auth/login') return reply({
      token: { access_token: token }, user: {
        id: userID, email: 'fiscal@teste.local', name: 'Responsável Fiscal', roles: ['admin'],
      },
    })
    if (url.pathname === '/api/v1/auth/refresh') return reply({ access_token: token })
    if (url.pathname === '/api/v1/auth/me') return reply({
      id: userID, name: 'Responsável Fiscal', email: 'fiscal@teste.local',
      permissions: ['invoice:read', 'invoice:generate', 'sale:read', 'product:read'],
    })
    if (url.pathname === '/api/v1/fiscal/obligations') {
      filterRequests.push(url.searchParams.get('unresolved') || '')
      const onlyPending = url.searchParams.get('unresolved') !== 'false'
      const items = onlyPending ? pending : [...pending, authorized]
      return reply({ items, total: items.length, limit: 20, offset: 0 })
    }
    if (url.pathname === '/api/v1/fiscal/nfce/readiness') return reply({
      tenant_id: tenantID, model: 65, issuer_identity_configured: false,
      issuer_address_configured: false, municipality_code_configured: false,
      config_exists: false, transmission_enabled: false, environment: 'homologation',
      certificate_reference_configured: false, csc_reference_configured: false,
      active_products: 0, products_missing_ncm: 0,
      ready_for_homologation_data: false, blocking_reasons: ['issuer_identity'],
    })
    if (url.pathname === '/api/v1/fiscal/nfce/issuer') return reply({
      tenant_id: tenantID, legal_name: 'Loja Fictícia', trade_name: 'Teste',
      cnpj: '00000000000000', ie: '', crt: '', address_street: '',
      address_number: '', address_neighborhood: '', address_city: '',
      address_city_code: '', address_state: '', address_zip: '',
    })
    if (url.pathname === '/api/v1/fiscal/nfce/config') return reply({
      message: 'Not configured',
    }, 404)
    if (url.pathname === '/api/v1/fiscal/nfe/xml') return reply({ items: [], total: 0 })
    return reply({ message: 'Not implemented in mock' }, 404)
  })

  await page.goto('/login')
  await page.getByLabel('E-mail').fill('fiscal@teste.local')
  await page.getByLabel('Senha').fill('senha-de-teste')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await page.getByRole('link', { name: /Fiscal/ }).click()
  await expect(page.getByRole('heading', { name: 'Vendas e emissão fiscal' })).toBeVisible()
  const section = page.getByRole('region', { name: 'Pendências fiscais das vendas' })
  await expect(section.getByText('Vendas que precisam de acompanhamento: 2')).toBeVisible()
  await expect(section.getByText('Aguardando emissão')).toBeVisible()
  await expect(section.getByText('Rejeitada — corrigir')).toBeVisible()
  await expect(section.getByText('Autorizada')).toHaveCount(0)
  expect(filterRequests).toContain('true')

  await section.getByRole('checkbox', { name: 'Mostrar também vendas com nota autorizada' }).check()
  await expect(section.getByText('Registros fiscais da loja: 3')).toBeVisible()
  await expect(section.getByText('Autorizada')).toBeVisible()
  expect(filterRequests).toContain('false')
  await section.getByRole('checkbox', { name: 'Mostrar também vendas com nota autorizada' }).uncheck()
  await expect(section.getByText('Vendas que precisam de acompanhamento: 2')).toBeVisible()
})
