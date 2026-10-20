import { expect, test } from '@playwright/test'

test('PDV: escolher não imprimir preserva obrigação fiscal; DANFE não autorizado fica bloqueado', async ({ page }) => {
  const userID = 'cdb8df15-a848-44da-9b7b-605dcd814a3b'
  const tenantID = '06dce31b-7bde-4c10-aab3-2a3864d013f3'
  const cashID = 'e4b7de48-e7a4-4cac-9386-8c53c2cac5b4'
  const productID = '56465036-7aa8-4e3c-bcbf-4c0825c8eace'
  const saleID = '3a527be8-aa74-4d66-987e-bf9598c7710b'
  const token = 'eyJ0eXAiOiJKV1QifQ.' + Buffer.from(JSON.stringify({
    sub: userID, tenant_id: tenantID, exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url') + '.signature'
  let salePosts = 0
  let fiscalReads = 0
  let danfeRequests = 0
  let processedXMLDownloads = 0
  let sefazAuthorizedLater = false
  let awaitingConsultation = false
  let manualReview = false
  let corruptedArchive = false
  let printerWidthMM = 80
  let printerUpdates = 0
  let calibrationRequests = 0

  await page.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url())
    const reply = (body: unknown, status = 200) => route.fulfill({ status, json: body })
    if (url.pathname === '/api/v1/auth/login') return reply({
      token: { access_token: token }, user: {
        id: userID, name: 'Caixa', email: 'caixa@teste.local', roles: ['cashier'],
      },
    })
    if (url.pathname === '/api/v1/auth/refresh') return reply({ access_token: token })
    if (url.pathname === '/api/v1/auth/me') return reply({
      id: userID, name: 'Caixa', email: 'caixa@teste.local',
      permissions: ['sale:write', 'product:read', 'cash:open'],
    })
    if (url.pathname === '/api/v1/products') return reply({ items: [{
      id: productID, sku: 'TEST-PRINT', name: 'Caderno', unit: 'un',
      barcode: null, price_cash: 12.5, promo_price: null,
      qty_on_hand: 10, active: true,
    }], total: 1 })
    if (url.pathname === '/api/v1/products/images/previews') return reply({ items: {} })
    if (url.pathname === `/api/v1/products/${productID}/variations`) return reply({
      parent_id: productID, items: [{
        id: productID, sku: 'TEST-PRINT', name: 'Caderno', unit: 'un',
        barcode: null, price_cash: 12.5, promo_price: null,
        qty_on_hand: 10, active: true, option_label: '', is_base: true,
      }],
    })
    if (url.pathname === '/api/v1/cash/sessions/' + cashID + '/printer/test') {
      calibrationRequests += 1
      return route.fulfill({
        status: 200,
        contentType: 'text/html; charset=utf-8',
        body: '<!doctype html><html lang="pt-BR"><head><meta charset="utf-8"></head><body><strong>TESTE DE IMPRESSÃO — NÃO É DOCUMENTO FISCAL</strong></body></html>',
      })
    }
    if (url.pathname === '/api/v1/cash/sessions/' + cashID + '/printer') {
      if (route.request().method() === 'PUT') {
        const body = route.request().postDataJSON() as { paper_width_mm: number }
        expect([58, 80]).toContain(body.paper_width_mm)
        printerWidthMM = body.paper_width_mm
        printerUpdates += 1
      }
      return reply({ cash_register_id: cashID, paper_width_mm: printerWidthMM })
    }
    if (url.pathname === '/api/v1/cash/sessions/current') return reply({
      id: cashID, cash_register_id: cashID, opened_by_user_id: userID,
      opening_amount: 0, status: 'open',
    })
    if (url.pathname === '/api/v1/sales' && route.request().method() === 'POST') {
      salePosts += 1
      expect(route.request().headers()['idempotency-key']).toBeTruthy()
      return reply({ id: saleID, status: 'finalized', total: 12.5 }, 201)
    }
    if (url.pathname === `/api/v1/sales/${saleID}/fiscal-status`) {
      fiscalReads += 1
      return reply({
        sale_id: saleID, required: true, document_kind: 'nfce',
        status: sefazAuthorizedLater ? 'authorized' : awaitingConsultation ? 'submitted' : 'pending',
        authorized: sefazAuthorizedLater && !corruptedArchive,
        printable: sefazAuthorizedLater && !corruptedArchive,
        automatic_state: manualReview ? 'manual_review' : awaitingConsultation ? 'awaiting_sefaz' : undefined,
        consultation_state: manualReview ? 'manual_review' : awaitingConsultation ? 'awaiting_consultation' : undefined,
        consultation_attempts: manualReview ? 10 : awaitingConsultation ? 2 : undefined,
        access_key: sefazAuthorizedLater ? '31261011222333000181650010000000011123456789' : undefined,
        legacy_review: false,
      })
    }
    if (url.pathname === `/api/v1/sales/${saleID}/fiscal-xml`) {
      processedXMLDownloads += 1
      if (!sefazAuthorizedLater) return reply({ message: 'NFC-e nao autorizada' }, 409)
      return route.fulfill({
        status: 200,
        contentType: 'application/xml; charset=utf-8',
        headers: { 'Content-Disposition': 'attachment; filename="fixture-procNFe.xml"' },
        body: '<?xml version="1.0"?><nfeProc><protNFe>fake authorization</protNFe></nfeProc>',
      })
    }
    if (url.pathname === `/api/v1/sales/${saleID}/fiscal-danfe`) {
      danfeRequests += 1
      if (!sefazAuthorizedLater) return reply({ message: 'DANFE indisponível' }, 409)
      return route.fulfill({
        status: 200,
        contentType: 'text/html; charset=utf-8',
        body: '<!doctype html><html lang="pt-BR"><head><meta charset="utf-8"></head><body><main>DANFE NFC-e AUTORIZADA - visualização de teste</main></body></html>',
      })
    }
    return reply({ message: 'test endpoint unavailable' }, 404)
  })
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('caixa@teste.local')
  await page.getByLabel('Senha', { exact: true }).fill('senha-ficticia')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await page.getByRole('link', { name: 'PDV', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Produtos da venda' })).toBeVisible()
  // This works before any transaction or real issuer configuration.
  await expect(page.getByRole('button', { name: 'Pré-visualizar teste' })).toBeEnabled()
  const nonFiscalPopup = page.waitForEvent('popup')
  await page.getByRole('button', { name: 'Pré-visualizar teste' }).click()
  const testPopup = await nonFiscalPopup
  await testPopup.waitForURL(/^blob:/, { waitUntil: 'domcontentloaded' })
  await expect(testPopup.getByText('TESTE DE IMPRESSÃO — NÃO É DOCUMENTO FISCAL')).toBeVisible()
  expect(calibrationRequests).toBe(1)
  expect(salePosts).toBe(0)
  expect(danfeRequests).toBe(0)
  await testPopup.close()
  await page.getByRole('combobox', { name: 'Produto', exact: true }).selectOption(productID)
  await page.getByRole('button', { name: 'Adicionar', exact: true }).click()
  await page.getByRole('checkbox', { name: 'Confirmei que o Pix foi recebido.' }).check()
  await page.getByRole('button', { name: 'Finalizar venda', exact: true }).click()
  await expect(page.getByText(/Venda finalizada:/)).toBeVisible()
  await expect(page.getByText('O cliente deseja imprimir o DANFE NFC-e?')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Sim, abrir para imprimir' })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Pré-visualizar DANFE' })).toBeDisabled()
  await expect(page.getByRole('combobox', { name: 'Largura da bobina do DANFE' })).toHaveValue('80')
  await page.getByRole('combobox', { name: 'Largura da bobina do DANFE' }).selectOption('58')
  await expect.poll(() => printerUpdates).toBe(1)
  expect(printerWidthMM).toBe(58)
  expect(calibrationRequests).toBe(1)
  await expect(page.getByText(/NFC-e ainda não autorizada/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Baixar XML autorizado' })).toHaveCount(0)
  expect(processedXMLDownloads).toBe(0)
  expect(salePosts).toBe(1)
  expect(fiscalReads).toBeGreaterThan(0)

  await page.getByRole('button', { name: 'Não imprimir' }).click()
  await expect(page.getByRole('button', { name: 'Não imprimir' })).toHaveCount(0)
  await expect(page.getByText(/Pendência fiscal permanece registrada/)).toBeVisible()
  expect(danfeRequests).toBe(0)
  expect(salePosts).toBe(1)

  // Authorization can arrive asynchronously after the customer decides not
  // to print. The same finalized sale is refreshed automatically; issuing a
  // new sale or triggering silent printing would be wrong.
  // Recovering a SOAP timeout never triggers a new sale or a blind resend.
  awaitingConsultation = true
  await expect(page.getByText(/NFC-e enviada. Aguardando consulta à SEFAZ/))
    .toBeVisible({ timeout: 12000 })
  manualReview = true
  await expect(page.getByRole('alert').filter({ hasText: 'Não tente enviar outra NFC-e' }))
    .toBeVisible({ timeout: 12000 })
  await expect(page.getByText('Consultas realizadas: 10 de 10.')).toBeVisible()
  expect(salePosts).toBe(1)
  // After a simulated external reconciliation, real authorization status is
  // restored from the server (never inferred from the browser or printing).
  const readsBeforeAuthorization = fiscalReads
  sefazAuthorizedLater = true
  await expect(page.getByRole('button', { name: 'Imprimir depois' }))
    .toBeVisible({ timeout: 12000 })
  expect(fiscalReads).toBeGreaterThan(readsBeforeAuthorization)
  await expect(page.getByRole('status').filter({ hasText: 'NFC-e autorizada.' })).toBeVisible()
  const xmlDownload = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Baixar XML autorizado' }).click()
  const xmlFile = await xmlDownload
  expect(xmlFile.suggestedFilename()).toMatch(/-procNFe\.xml$/)
  expect(processedXMLDownloads).toBe(1)
  expect(danfeRequests).toBe(0)
  expect(salePosts).toBe(1)

  // Reopening the print decision after authorization permits preview, but
  // never creates another sale or silently prints the document.
  await page.getByRole('button', { name: 'Imprimir depois' }).click()
  await expect(page.getByRole('combobox', { name: 'Largura da bobina do DANFE' })).toHaveValue('58')
  const popupPromise = page.waitForEvent('popup')
  await page.getByRole('button', { name: 'Pré-visualizar DANFE' }).click()
  const popup = await popupPromise
  await popup.waitForURL(/^blob:/, { waitUntil: 'domcontentloaded' })
  await expect(popup.getByText('DANFE NFC-e AUTORIZADA - visualização de teste')).toBeVisible()
  expect(danfeRequests).toBe(1)
  expect(salePosts).toBe(1)
  await popup.close()
  // Even an authorized note can become unavailable after a storage
  // integrity incident. A manual refresh must revoke the print/download
  // affordances and warn the cashier without creating another fiscal sale.
  corruptedArchive = true
  await page.getByRole('button', { name: 'Atualizar situação fiscal' }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Não tente enviar outra NFC-e' })).toBeVisible()
  await expect(page.getByText(/XML processado não está íntegro/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Baixar XML autorizado' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Sim, abrir para imprimir' })).toBeDisabled()
  expect(salePosts).toBe(1)
})
