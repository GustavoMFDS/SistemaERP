import { expect, test } from '@playwright/test'

test('refino final: alertas, resumo fiscal e relatórios com permissões', async ({ page }) => {
  const userID = 'ac9a9182-5cdd-4ccf-a633-f0dd7aca9969'
  const tenantID = '229f5e22-c019-41ef-8bb8-bb9b8b6c291b'
  const token = 'eyJhbGciOiJIUzI1NiJ9.' + Buffer.from(JSON.stringify({
    sub: userID, tenant_id: tenantID, exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url') + '.synthetic-signature'
  const queried: string[] = []
  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const path = url.pathname
    const reply = (data: unknown, status = 200) => route.fulfill({ status, json: data })
    if (path === '/api/v1/auth/login') return reply({
      token: { access_token: token }, user: { id: userID, email: 'teste@exemplo.local', name: 'Admin', roles: ['admin'] },
    })
    if (path === '/api/v1/auth/me') return reply({
      id: userID, tenant_id: tenantID, name: 'Admin', email: 'teste@exemplo.local',
      roles: ['admin'], permissions: ['product:read', 'inventory:read', 'invoice:read', 'finance:read', 'audit:read'],
    })
    if (path === '/api/v1/fiscal/obligations') return reply({ items: [], total: 3, limit: 1, offset: 0 })
    if (path === '/api/v1/audit/backup-status') return reply({ configured: true, state: 'failed', archive_verified: false })
    if (path === '/api/v1/inventory/low-stock') return reply({ items: [], total: 2 })
    if (path === '/api/v1/finance/accounts') return reply({
      items: [], total: 0, limit: 1, offset: 0,
      summary: { payable_open: 0, receivable_open: 0, overdue_open: 120 },
    })
    if (path === '/api/v1/finance/overview') return reply({
      overview: { sales_after_cancellations: 0, estimated_gross_profit: 0,
        refunds_recorded: 0, sales_count: 0, cancelled_count: 0 },
    })
    if (path === '/api/v1/fiscal/daily-close') {
      queried.push('fiscal-daily')
      return reply({ date: url.searchParams.get('date'), total: 3, nfce: 2, nfe: 1,
        authorized_nfce: 1, unresolved: 2, manual_review: 1,
        cancelled_sales: 0, integrity_alert: 0 })
    }
    if (path === '/api/v1/finance/operational-reports') {
      queried.push('finance-reports')
      return reply({
        from: url.searchParams.get('from'), to: url.searchParams.get('to'),
        hourly: [{ hour: 10, count: 3, total: 120 }],
        margins: [{ sku: 'CADERNO', name: 'Caderno', quantity: 3,
          revenue_items: 120, estimated_margin: 35 }],
        unsold: [{ sku: 'BORRACHA', name: 'Borracha' }], unsold_total: 1,
        cash_discrepancies: 2, overdue_accounts: 1,
      })
    }
    if (path === '/api/v1/fiscal/nfce/readiness') return reply({ ready: false, blocking_reasons: [] })
    if (path === '/api/v1/fiscal/nfce/config') return reply({ environment: 'homologation', series: 1 })
    if (path === '/api/v1/fiscal/nfce/issuer') return reply({})
    if (path === '/api/v1/fiscal/nfe/xml') return reply({ items: [], total: 0 })
    if (path === '/api/v1/setup/reviews') return reply({ items: [] })
    return reply({ items: [], total: 0 })
  })

  await page.goto('/login')
  await page.getByLabel('E-mail').fill('teste@exemplo.local')
  await page.getByLabel('Senha', { exact: true }).fill('somente-teste')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
  await page.getByRole('link', { name: 'Início' }).click()
  await expect(page.getByRole('heading', { name: 'Atenção da loja' })).toBeVisible()
  await expect(page.getByText('3 venda(s) com obrigação fiscal pendente')).toBeVisible()
  await expect(page.getByText('2 produto(s) em reposição')).toBeVisible()
  await expect(page.getByText(/Cópia de segurança precisa de atenção/)).toBeVisible()

  await page.getByText('Mais áreas do sistema').click()
  await page.getByRole('link', { name: 'Fiscal', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Conferência diária das notas' })).toBeVisible()
  await page.getByRole('button', { name: 'Conferir dia' }).click()
  await expect(page.getByText('NFC-e comprovadas')).toBeVisible()
  await expect(page.getByText('Sem comprovação')).toBeVisible()

  await page.getByRole('link', { name: 'Relatórios', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Indicadores para administrar a loja' })).toBeVisible()
  await expect(page.getByText('Margem bruta estimada por produto')).toBeVisible()
  await expect(page.getByText('CADERNO')).toBeVisible()
  expect(queried).toEqual(expect.arrayContaining(['fiscal-daily', 'finance-reports']))
})
