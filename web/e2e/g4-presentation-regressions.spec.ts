import { expect, test } from '@playwright/test'

const token = 'fixture.' + Buffer.from(JSON.stringify({
  sub: 'lab-user', tenant_id: 'lab-tenant', exp: Math.floor(Date.now() / 1000) + 900,
})).toString('base64url') + '.fixture'
const me = { id: 'lab-user', tenant_id: 'lab-tenant', name: 'Lab user', email: 'lab@example.invalid',
  roles: ['admin'], permissions: ['invoice:read', 'finance:read', 'finance:reconcile'] }
const reasons = ['issuer_identity', 'issuer_address', 'issuer_ie_missing', 'issuer_street_missing',
  'issuer_zip_invalid', 'product_fiscal_profile', 'future_unknown_one', 'future_unknown_two', 'issuer_ie_missing']

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let body: unknown = { items: [], total: 0 }
    if (path.endsWith('/auth/refresh')) body = { access_token: token }
    if (path.endsWith('/auth/me')) body = me
    if (path.endsWith('/nfce/readiness')) body = {
      tenant_id: me.tenant_id, issuer_identity_configured: false, issuer_address_configured: false,
      municipality_code_configured: false, config_exists: false, certificate_reference_configured: false,
      ready_for_homologation_data: false, transmission_enabled: false, active_products: 1,
      products_missing_ncm: 0, products_missing_fiscal_profile: 1, blocking_reasons: reasons,
    }
    if (path.endsWith('/nfce/issuer')) body = { tenant_id: me.tenant_id, legal_name: 'Lab', cnpj: '00000000000000' }
    if (path.endsWith('/finance/accounts')) body = {
      items: [{ id: 'a', kind: 'payable', description: 'Conta ficticia encerrada', amount: 6, due_date: '2026-12-10', status: 'cancelled', settled_at: null, settlement_method: null },
        { id: 'b', kind: 'payable', description: 'Synthetic open account', amount: 6, due_date: '2026-12-10', status: 'open', settled_at: null, settlement_method: null }],
      total: 2, limit: 80, offset: 0, truncated: false, summary: { payable_open: 6, receivable_open: 0, overdue_open: 0 },
    }
    await route.fulfill({ json: body })
  })
})

test('setup renders specific fiscal blockers and a single safe unknown summary', async ({ page }) => {
  await page.goto('/setup')
  await page.getByRole('button', { name: /5\. Preparar/ }).click()
  const list = page.locator('ul').filter({ hasText: 'Inscrição estadual não informada' })
  await expect(list).toBeVisible()
  await expect(list.getByRole('listitem')).toHaveCount(5)
  await expect(list).toContainText('Rua ou avenida não informada')
  await expect(list).toContainText('CEP deve ter exatamente 8 dígitos numéricos')
  await expect(list).toContainText('Classificação tributária dos produtos')
  await expect(list).toContainText('2 outras pendências fiscais')
  await expect(list).not.toContainText('future_unknown')
  await expect(page.locator('p').filter({ hasText: /Dados básicos de homologação:/ })).toContainText('incompletos')
})

test('cancelled financial account is localized and cannot be settled', async ({ page }) => {
  await page.goto('/finance')
  const cancelled = page.getByRole('article').filter({ hasText: 'Conta ficticia encerrada' })
  const open = page.getByRole('article').filter({ hasText: 'Synthetic open account' })
  await expect(cancelled).toContainText('Cancelada')
  await expect(cancelled).not.toContainText('cancelled', { ignoreCase: false })
  await expect(cancelled.getByRole('button', { name: 'Dar baixa' })).toHaveCount(0)
  await expect(open).toContainText('Aberta')
  await expect(open.getByRole('button', { name: 'Dar baixa' })).toBeVisible()
})
