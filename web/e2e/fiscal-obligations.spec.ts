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
  const filterRequests: string[] = []
  const manualReviews = new Set<string>()
  let reviewWrites = 0
  const pending = [
    { sale_id: sale1, created_at: '2026-10-09T12:00:00Z',
      document_kind: 'nfce', sale_status: 'finalized', status: 'pending',
      authorized: false, legacy_review: false, dispatch_state: 'awaiting_fiscal_processing',
      preflight_result: 'issuer_incomplete' },
    { sale_id: sale2, created_at: '2026-10-09T11:00:00Z',
      document_kind: 'nfce', sale_status: 'finalized', status: 'rejected',
      authorized: false, legacy_review: false, dispatch_state: 'rejected_review',
      preflight_result: 'payment_review_required' },
  ]
  const authorized = { sale_id: sale3, created_at: '2026-10-09T10:00:00Z',
    document_kind: 'nfce', sale_status: 'finalized', status: 'authorized',
    authorized: true, legacy_review: false, invoice_id: 'dd11bc2d-415b-47b3-98d3-eec344d3be54',
    processed_xml_available: true, dispatch_state: 'authorized' }

  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const reply = (data: unknown, status = 200) => route.fulfill({ status, json: data })
    if (url.pathname === '/api/v1/setup/reviews') return reply({
      items: [...manualReviews].map((step) => ({ step, reviewed_at: '2026-10-12T12:00:00Z' })),
    })
    if (url.pathname.startsWith('/api/v1/setup/reviews/') && route.request().method() === 'PUT') {
      reviewWrites += 1
      const step = url.pathname.split('/').at(-1) || ''
      const { reviewed } = route.request().postDataJSON() as { reviewed: boolean }
      if (reviewed) manualReviews.add(step)
      else manualReviews.delete(step)
      return reply({ items: [...manualReviews].map((value) => ({
        step: value, reviewed_at: '2026-10-12T12:00:00Z',
      })) })
    }
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
    if (url.pathname === '/api/v1/fiscal/obligations/' + sale1 + '/preparation') {
      return reply({sale_id:sale1,status:'blocked',
        result_code:'issuer_incomplete',source_matches:false,
        prepared_at:'2026-10-11T20:00:00Z'})
    }
    if (url.pathname === '/api/v1/fiscal/obligations/' + sale1 + '/preflight-audit') {
      return reply({ sale_id: sale1, items: [
        { id: 2, worker_identity_id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
          event_kind: 'completed', reclaimed: false, result_code: 'issuer_incomplete',
          recorded_at: '2026-10-11T20:00:01Z' },
        { id: 1, worker_identity_id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
          event_kind: 'claimed', reclaimed: true,
          recorded_at: '2026-10-11T20:00:00Z' },
      ] })
    }
    if (url.pathname === '/api/v1/fiscal/obligations') {
      filterRequests.push(url.searchParams.get('unresolved') || '')
      const onlyPending = url.searchParams.get('unresolved') !== 'false'
      const items = onlyPending ? pending : [...pending, authorized]
      return reply({ items, total: items.length, limit: 20, offset: 0 })
    }
    if (url.pathname === '/api/v1/fiscal/nfce/invoices/' + authorized.invoice_id + '/danfe') {
      return route.fulfill({
        status: 200, contentType: 'text/html; charset=utf-8',
        body: '<!doctype html><html><head><title>DANFE TESTE</title></head>' +
          '<body><h1>DANFE SIMULADO DE TESTE</h1></body></html>',
      })
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
  const guide = page.getByRole('region', { name: 'Guia de configuração fiscal MG' })
  await expect(guide.getByText('Guia passo a passo: emissão fiscal em Minas Gerais')).toBeVisible()
  await expect(guide.getByText('Etapa 1 de 6')).toBeVisible()
  await expect(guide.getByText('Dados já cadastrados nesta loja (preenchimento automático)')).toBeVisible()
  await expect(guide.getByText('Loja Fictícia')).toBeVisible()
  await expect(guide.getByText('0 de 4 tarefas marcadas manualmente')).toBeVisible()
  page.once('dialog', (dialog) => void dialog.accept())
  await guide.getByRole('button', { name: 'Marcar tarefa organizada' }).click()
  await expect(guide.getByRole('button', { name: 'Reabrir tarefa' })).toBeVisible()
  await expect(guide.getByText('1 de 4 tarefas marcadas manualmente')).toBeVisible()
  expect(manualReviews.has('fiscal_docs_collected')).toBe(true)
  expect(reviewWrites).toBe(1)
  await guide.getByRole('button', { name: 'Próxima etapa' }).click()
  await expect(guide.getByText('Etapa 2 de 6')).toBeVisible()
  await guide.getByRole('button', { name: 'Voltar etapa' }).click()
  await expect(guide.getByText('Etapa 1 de 6')).toBeVisible()
  await guide.getByText('Confirmar os dados e o regime da empresa').click()
  await expect(guide.getByRole('link', { name: /Consultar CNPJ/ }))
    .toHaveAttribute('href', 'https://www.gov.br/pt-br/servicos/consultar-cadastro-nacional-de-pessoas-juridicas')
  await guide.getByRole('button', { name: 'Ir para dados da loja' }).click()
  await expect(page.locator('#fiscal-issuer')).toHaveAttribute('open', '')
  await guide.getByText('Conferir credenciamento da NFC-e na SEF/MG').click()
  await expect(guide.getByRole('link', { name: /Abrir SIARE/ }))
    .toHaveAttribute('href', 'https://siare.fazenda.mg.gov.br/sicaf/')
  await guide.getByText('Providenciar certificado digital e conferir CSC').click()
  await guide.getByRole('button', { name: 'Ir para certificado e configurações' }).click()
  await expect(page.locator('#fiscal-certificate')).toHaveAttribute('open', '')
  await guide.getByRole('button', { name: 'Próxima etapa' }).click()
  await expect(guide.getByText('Etapa 4 de 6')).toBeVisible()
  await expect(guide).toContainText('A transmissão permanece desativada')
  const section = page.getByRole('region', { name: 'Pendências fiscais das vendas' })
  await expect(section.getByText('Vendas que precisam de acompanhamento: 2')).toBeVisible()
  await expect(section.getByText('Aguardando emissão')).toBeVisible()
  await expect(section.getByText('Rejeitada — corrigir')).toBeVisible()
  await expect(section.getByRole('cell', { name: 'Autorizada', exact: true })).toHaveCount(0)
  await expect(section.getByText('Acompanhamento: Configuração registrada; emissão ainda não iniciada')).toBeVisible()
  await expect(section.getByText('Acompanhamento: Rejeição fiscal exige análise')).toBeVisible()
  await expect(section.getByText('Verificação: Conferir CNPJ, inscrição estadual, regime (CRT), endereço, UF e município do emitente')).toBeVisible()
  await expect(section.getByText('Verificação: Forma de pagamento ainda requer suporte fiscal')).toBeVisible()
  expect(filterRequests).toContain('true')

  await section.getByRole('checkbox', { name: 'Mostrar também vendas com nota autorizada' }).check()
  await expect(section.getByText('Registros fiscais da loja: 3')).toBeVisible()
  await expect(section.getByRole('cell', { name: 'Autorizada', exact: true })).toBeVisible()
  const authorizedRow = section.getByRole('row').filter({ hasText: sale3.slice(0, 8) })
  const pendingRow = section.getByRole('row').filter({ hasText: sale1.slice(0, 8) })
  await pendingRow.getByRole('button', { name: 'Histórico técnico' }).click()
  const history = section.getByRole('region', { name: 'Histórico técnico da venda' })
  await expect(history.getByText(/Plano desatualizado/)).toBeVisible()
  await expect(history.getByText(/Verificação concluída/)).toBeVisible()
  await expect(history.getByText(/Retomada após interrupção/)).toBeVisible()
  await expect(history).toContainText('não representam emissão nem autorização pela SEFAZ')
  await expect(pendingRow.getByRole('button', { name: 'Imprimir DANFE' })).toHaveCount(0)
  await expect(authorizedRow.getByRole('button', { name: 'Imprimir DANFE' })).toBeVisible()
  const printPopupPromise = page.waitForEvent('popup')
  await authorizedRow.getByRole('button', { name: 'Imprimir DANFE' }).click()
  const printPopup = await printPopupPromise
  await expect(printPopup.locator('body')).toContainText('DANFE SIMULADO DE TESTE')
  await printPopup.close()
  expect(filterRequests).toContain('false')
  await section.getByRole('checkbox', { name: 'Mostrar também vendas com nota autorizada' }).uncheck()
  await expect(section.getByText('Vendas que precisam de acompanhamento: 2')).toBeVisible()
})
