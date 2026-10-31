import { expect, test, type Page } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'

const screens = [
  ['/login', 'Bem-vindo de volta'],
  ['/accept-invite', 'Ativar acesso à loja'],
  ['/reset-password', 'Recuperar meu acesso'],
  ['/home', 'O ritmo da loja'],
  ['/setup', 'Vamos preparar sua loja'],
  ['/staff', 'Funcionários e permissões'],
  ['/products', 'Produtos da loja'],
  ['/inventory', 'Estoque da loja'],
  ['/stock-movements', 'Movimentações de estoque'],
  ['/imports', 'Histórico de importações'],
  ['/purchases', 'Compras e fornecedores'],
  ['/customers', 'Clientes da loja'],
  ['/returns', 'Devoluções e trocas'],
  ['/pdv', 'PDV'],
  ['/finance', 'Finanças da loja'],
  ['/reports', 'Relatórios da loja'],
  ['/fiscal', 'Nota fiscal da loja'],
] as const

type JsonRecord = Record<string, unknown>
type ApiResult = { status: number; body: unknown; text: string }

async function browserApi(
  page: Page,
  path: string,
  token: string,
  options: { method?: string; body?: unknown; idempotencyKey?: string } = {},
): Promise<ApiResult> {
  return page.evaluate(async ({ path, token, options }) => {
    const headers: Record<string, string> = {}
    if (token) headers.Authorization = `Bearer ${token}`
    if (options.body !== undefined) headers['Content-Type'] = 'application/json'
    if (options.idempotencyKey) headers['Idempotency-Key'] = options.idempotencyKey
    const response = await fetch(path, {
      method: options.method ?? 'GET',
      credentials: 'include',
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    })
    const text = await response.text()
    let body: unknown = null
    if (text) {
      try { body = JSON.parse(text) as unknown } catch { body = text }
    }
    return { status: response.status, body, text }
  }, { path, token, options })
}

function expectBody<T>(result: ApiResult, label: string): T {
  expect(result.status, `${label}: ${result.text}`).toBeGreaterThanOrEqual(200)
  expect(result.status, `${label}: ${result.text}`).toBeLessThan(300)
  return result.body as T
}

async function loginApi(page: Page, email: string, password: string): Promise<string> {
  const result = await browserApi(page, '/api/v1/auth/login', '', {
    method: 'POST', body: { email, password },
  })
  const payload = expectBody<{ token?: { access_token?: string } }>(result, 'login API')
  const token = payload.token?.access_token ?? ''
  expect(token).not.toBe('')
  return token
}

async function loginUi(page: Page): Promise<void> {
  const email = process.env.ESTERIUM_LAB_ADMIN_EMAIL ?? ''
  const password = process.env.ESTERIUM_LAB_ADMIN_PASSWORD ?? ''
  expect(process.env.ESTERIUM_INSTALLED_LAB).toBe('1')
  expect(process.env.E2E_IGNORE_HTTPS_ERRORS).toBe('0')
  expect(email).toBe('ci-admin@esterium.invalid')
  expect(password).toMatch(/^CIonly-[0-9a-f]{32}$/)
  await page.goto('/login')
  await page.getByLabel(/E-mail/).fill(email)
  await page.getByLabel('Senha', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Entrar', exact: true }).click()
  await expect(page).toHaveURL(/\/home$/)
  await expect(page.getByRole('heading', { name: 'O ritmo da loja' })).toBeVisible()
}

test('installed HTTPS login opens all 17 screens and both route redirects', async ({ page }) => {
  const evidenceDir = process.env.ESTERIUM_LAB_EVIDENCE_DIR ?? ''
  expect(evidenceDir).not.toBe('')
  mkdirSync(evidenceDir, { recursive: true })
  const pageErrors: string[] = []
  page.on('pageerror', (error) => pageErrors.push(error.message))

  await page.goto('/login')
  await expect(page.getByRole('heading', { name: 'Bem-vindo de volta' })).toBeVisible()
  await page.screenshot({ path: join(evidenceDir, `${test.info().project.name}-login.png`), fullPage: true })
  await loginUi(page)
  await expect(page.getByRole('navigation', { name: 'Navegação principal' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Resumo da semana' })).toBeVisible()
  await page.screenshot({ path: join(evidenceDir, `${test.info().project.name}-home.png`), fullPage: true })

  for (const [path, heading] of screens.slice(1)) {
    await page.goto(path)
    await expect(page.getByRole('heading', { name: heading, exact: true }).first()).toBeVisible()
  }
  await page.goto('/')
  await expect(page).toHaveURL(/\/home$/)
  await expect(page.getByRole('heading', { name: 'O ritmo da loja' })).toBeVisible()
  await page.goto('/esterium-unknown-route')
  await expect(page).toHaveURL(/\/home$/)
  await expect(page.getByRole('heading', { name: 'O ritmo da loja' })).toBeVisible()
  expect(pageErrors).toEqual([])
})

test('installed mobile routes remain usable without horizontal overflow', async ({ page }) => {
  test.skip(test.info().project.name !== 'chromium-mobile', 'only runs on the Chromium mobile viewport')
  await loginUi(page)
  for (const [path, heading] of screens) {
    await page.goto(path)
    await expect(page.getByRole('heading', { name: heading, exact: true }).first()).toBeVisible()
    const widths = await page.evaluate(() => ({
      viewport: document.documentElement.clientWidth,
      content: document.documentElement.scrollWidth,
    }))
    expect(widths.content, `${path} overflows ${widths.viewport}px viewport`).toBeLessThanOrEqual(widths.viewport + 1)
  }
})

test('installed navigation, attention alerts, and sale confirmation messages work with real services', async ({ page }) => {
  test.skip(test.info().project.name !== 'chromium', 'screen 18-19 confirmation smoke runs once on Chromium')
  await loginUi(page)
  const evidenceDir = process.env.ESTERIUM_LAB_EVIDENCE_DIR ?? ''
  expect(evidenceDir).not.toBe('')
  mkdirSync(evidenceDir, { recursive: true })

  const navigation = page.getByRole('navigation', { name: 'Navegação principal' })
  await expect(navigation).toBeVisible()
  await navigation.getByText('Mais áreas do sistema', { exact: true }).click()
  await expect(navigation.getByRole('link', { name: 'Financeiro' })).toBeVisible()
  await expect(navigation.getByRole('link', { name: 'Relatórios' })).toBeVisible()
  await expect(navigation.getByRole('link', { name: 'Fiscal' })).toBeVisible()

  const attention = page.getByRole('region', { name: 'Central de atenção' })
  await expect(attention.getByText('Avisos reais das áreas que você tem permissão para consultar.')).toBeVisible()
  await attention.getByRole('button', { name: 'Atualizar avisos' }).click()
  await expect(attention.getByRole('button', { name: 'Atualizar avisos' })).toBeEnabled()
  await expect(attention.getByText(
    /Nenhuma pendência encontrada|venda\(s\) com obrigação fiscal pendente|Não foi possível verificar todas as áreas/,
  )).toBeVisible()
  await page.screenshot({
    path: join(evidenceDir, 'installed-screen-18-navigation-alerts.png'),
    fullPage: true,
  })

  const adminToken = await loginApi(
    page,
    process.env.ESTERIUM_LAB_ADMIN_EMAIL ?? '',
    process.env.ESTERIUM_LAB_ADMIN_PASSWORD ?? '',
  )
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 14)
  const productName = `Synthetic confirmation ${suffix}`
  const product = expectBody<{ id: string }>(await browserApi(page, '/api/v1/products', adminToken, {
    method: 'POST', body: {
      category_id: null, sku: `CI-SCREEN-${suffix}`, barcode: null, name: productName,
      description: 'Synthetic installed-lab confirmation product', unit: 'UN', ncm: '00000000', cost_price: 3,
      price_cash: 10, promo_price: null, min_stock: 0, active: true,
    },
  }), 'screen 19 synthetic product create')
  expectBody<JsonRecord>(await browserApi(page, '/api/v1/inventory/adjust', adminToken, {
    method: 'POST',
    body: { product_id: product.id, delta: 1, reason: 'Synthetic confirmation screen stock', type: 'adjustment' },
  }), 'screen 19 opening stock')
  const cash = expectBody<{ id: string }>(await browserApi(page, '/api/v1/cash/sessions/open', adminToken, {
    method: 'POST', body: { opening_amount: 0, notes: 'Synthetic screen 19 confirmation' },
  }), 'screen 19 cash session')
  let openSessionId = cash.id
  try {
    await page.getByRole('link', { name: 'PDV', exact: true }).click()
    await expect(page.getByText('Caixa pronto para atender.')).toBeVisible()
    await page.getByLabel('Busca rápida por nome ou código').fill(productName)
    await page.getByRole('button', { name: `Adicionar ${productName} à venda` }).click()
    await page.getByLabel('Valor recebido em dinheiro (R$)').fill('10')
    await expect(page.getByRole('button', { name: 'Finalizar venda' })).toBeEnabled()
    await page.getByRole('button', { name: 'Finalizar venda' }).click()

    const completed = page.getByRole('dialog', { name: 'Venda concluída' })
    await expect(completed).toBeVisible()
    await expect(completed.getByText(/Pagamento registrado\. Total: R\$\s*10,00/)).toBeVisible()
    await expect(completed.getByText('A autorização da nota fiscal, quando necessária, é conferida separadamente.')).toBeVisible()
    await completed.getByRole('button', { name: 'Continuar no caixa' }).click()
    await expect(page.getByRole('status').filter({
      hasText: 'Obrigação fiscal registrada. A emissão só é concluída após autorização fiscal.',
    })).toBeVisible()
    await page.screenshot({
      path: join(evidenceDir, 'installed-screen-19-sale-confirmation.png'),
      fullPage: true,
    })

    expectBody<JsonRecord>(await browserApi(page, `/api/v1/cash/sessions/${openSessionId}/close`, adminToken, {
      method: 'POST', body: { closing_amount: 10, notes: 'Synthetic screen 19 confirmation close' },
    }), 'screen 19 cash close')
    openSessionId = ''
  } finally {
    if (openSessionId) {
      try {
        await browserApi(page, `/api/v1/cash/sessions/${openSessionId}/close`, adminToken, {
          method: 'POST', body: { closing_amount: 10, notes: 'Synthetic screen 19 cleanup' },
        })
      } catch {
        // Preserve the primary assertion if the test fails before cleanup.
      }
    }
  }
})

test('installed ERP completes real synthetic retail operations and a two-terminal stock race', async ({ page, browser, browserName }) => {
  test.skip(test.info().project.name !== 'chromium', 'business write smoke runs once on Chromium')
  expect(browserName).toBe('chromium')
  const email = process.env.ESTERIUM_LAB_ADMIN_EMAIL ?? ''
  const password = process.env.ESTERIUM_LAB_ADMIN_PASSWORD ?? ''
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 14)
  const evidenceDir = process.env.ESTERIUM_LAB_EVIDENCE_DIR ?? ''
  expect(evidenceDir).not.toBe('')
  mkdirSync(evidenceDir, { recursive: true })

  const cashierContext = await browser.newContext({ ignoreHTTPSErrors: false })
  const cashierPage = await cashierContext.newPage()
  try {
    await page.goto('/login')
    const adminToken = await loginApi(page, email, password)
    const me = expectBody<{ roles?: string[]; permissions?: string[] }>(
      await browserApi(page, '/api/v1/auth/me', adminToken), 'admin profile',
    )
    expect(me.roles).toContain('admin')
    expect(me.permissions).toContain('team:manage')
    expect(me.permissions).toContain('product:write')

    const staffBefore = expectBody<{ members: Array<{ email: string }> }>(
      await browserApi(page, '/api/v1/staff', adminToken), 'staff list',
    )
    expect(staffBefore.members.some((member) => member.email === email)).toBe(true)
    const cashierEmail = `ci-cashier-${suffix}@esterium.invalid`
    const cashierPassword = `LabCashier-${suffix}!`
    const invitation = expectBody<{ token: string; invitation: { email: string } }>(
      await browserApi(page, '/api/v1/staff/invitations', adminToken, {
        method: 'POST', body: { name: 'Synthetic CI Cashier', email: cashierEmail, username: '', role: 'cashier' },
      }), 'staff invitation',
    )
    expect(invitation.invitation.email).toBe(cashierEmail)
    expect(invitation.token).toMatch(/^[0-9a-f]{64}$/)
    expectBody<JsonRecord>(await browserApi(page, '/api/v1/staff/accept-invite', '', {
      method: 'POST', body: { token: invitation.token, password: cashierPassword },
    }), 'staff invitation acceptance')
    const staffAfter = expectBody<{ members: Array<{ email: string; role: string }> }>(
      await browserApi(page, '/api/v1/staff', adminToken), 'staff list after invite',
    )
    expect(staffAfter.members.some((member) => member.email === cashierEmail && member.role === 'cashier')).toBe(true)

    await cashierPage.goto('/login')
    const cashierToken = await loginApi(cashierPage, cashierEmail, cashierPassword)
    const cashierMe = expectBody<{ roles?: string[] }>(
      await browserApi(cashierPage, '/api/v1/auth/me', cashierToken), 'cashier profile',
    )
    expect(cashierMe.roles).toContain('cashier')
    expect((await browserApi(cashierPage, '/api/v1/finance/accounts?limit=1', cashierToken)).status).toBe(403)

    const productName = `Synthetic product ${suffix}`
    const product = expectBody<{ id: string }>(await browserApi(page, '/api/v1/products', adminToken, {
      method: 'POST', body: {
        category_id: null, sku: `CI-${suffix}`, barcode: null, name: productName,
        description: 'Synthetic installed-lab product', unit: 'UN', ncm: '00000000', cost_price: 4,
        price_cash: 10, promo_price: null, min_stock: 0, active: true,
      },
    }), 'product create')
    const variant = expectBody<{ id: string; parent_id: string }>(
      await browserApi(page, `/api/v1/products/${product.id}/variations`, adminToken, {
        method: 'POST', body: {
          option_label: 'Azul',
          product: {
            category_id: null, sku: `CI-V-${suffix}`, barcode: null, name: `${productName} Azul`,
            description: null, unit: 'UN', cost_price: 4, price_cash: 10,
            promo_price: null, min_stock: 0, active: true,
          },
        },
      }), 'product variation create',
    )
    expect(variant.parent_id).toBe(product.id)
    await expectBody<JsonRecord>(await browserApi(page, '/api/v1/inventory/adjust', adminToken, {
      method: 'POST', body: { product_id: product.id, delta: 2, reason: 'Synthetic lab opening stock', type: 'adjustment' },
    }), 'opening inventory adjustment')

    const imageBase64 = await page.evaluate(() => {
      const canvas = document.createElement('canvas')
      canvas.width = 2; canvas.height = 2
      const context = canvas.getContext('2d')
      if (!context) throw new Error('Canvas unavailable for synthetic JPEG fixture')
      context.fillStyle = '#2463eb'; context.fillRect(0, 0, 2, 2)
      return canvas.toDataURL('image/jpeg').split(',')[1]
    })
    const photo = expectBody<{ id: string }>(await browserApi(page, `/api/v1/products/${product.id}/images`, adminToken, {
      method: 'POST', idempotencyKey: crypto.randomUUID(), body: { image_base64: imageBase64 },
    }), 'product photo upload')
    await browserApi(page, `/api/v1/products/${product.id}/images/${photo.id}/caption`, adminToken, {
      method: 'PATCH', body: { caption: 'Synthetic lab photo' },
    }).then((result) => expectBody<JsonRecord>(result, 'photo caption'))

    const customer = expectBody<{ id: string }>(await browserApi(page, '/api/v1/customers', adminToken, {
      method: 'POST', body: {
        name: `Synthetic Customer ${suffix}`, email: `ci-customer-${suffix}@esterium.invalid`, phone: '31999990000',
      },
    }), 'customer create')
    const supplier = expectBody<{ id: string }>(await browserApi(page, '/api/v1/suppliers', adminToken, {
      method: 'POST', idempotencyKey: crypto.randomUUID(), body: {
        name: `Synthetic Supplier ${suffix}`, document: `CI-${suffix}`, email: null,
        phone: '31999990001', contact_name: null, notes: 'Synthetic installed-lab supplier', active: true,
      },
    }), 'supplier create')
    const dueDate = new Date(Date.now() + 7 * 86_400_000).toISOString().slice(0, 10)
    const purchase = expectBody<{ id: string }>(await browserApi(page, '/api/v1/purchases', adminToken, {
      method: 'POST', idempotencyKey: crypto.randomUUID(), body: {
        supplier_id: supplier.id, invoice_number: `CI-${suffix}`, payment_due_date: dueDate,
        notes: 'Synthetic purchase receipt', items: [{ product_id: product.id, qty: 4, unit_cost: 4 }],
      },
    }), 'purchase create')
    const purchaseDetail = expectBody<{ items: Array<{ id: string }>; purchase: { status: string } }>(
      await browserApi(page, `/api/v1/purchases/${purchase.id}`, adminToken), 'purchase detail',
    )
    const receipt = expectBody<{ status: string }>(await browserApi(page, `/api/v1/purchases/${purchase.id}/receive`, adminToken, {
      method: 'POST', idempotencyKey: crypto.randomUUID(), body: {
        items: [{ purchase_item_id: purchaseDetail.items[0].id, qty: 4 }], notes: 'Synthetic full receipt',
      },
    }), 'purchase receipt')
    expect(receipt.status).toBe('received')

    const cash = expectBody<{ id: string }>(await browserApi(page, '/api/v1/cash/sessions/open', adminToken, {
      method: 'POST', body: { opening_amount: 100, notes: 'Synthetic installed-lab drawer' },
    }), 'cash session open')
    for (const [movement_type, amount] of [['supply', 10], ['withdrawal', 2]] as const) {
      expectBody<JsonRecord>(await browserApi(page, `/api/v1/cash/sessions/${cash.id}/movements`, adminToken, {
        method: 'POST', idempotencyKey: crypto.randomUUID(),
        body: { movement_type, amount, notes: `Synthetic ${movement_type}` },
      }), `cash ${movement_type}`)
    }

    const saleBody = {
      cash_session_id: cash.id, customer_id: customer.id, discount_value: 0,
      items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
      payments: [{ method: 'cash', amount: 10, cash_received: 20, installments: 1 }],
    }
    const saleKey = crypto.randomUUID()
    const sale = expectBody<{ id: string; total: number }>(await browserApi(page, '/api/v1/sales', adminToken, {
      method: 'POST', idempotencyKey: saleKey, body: saleBody,
    }), 'cash sale')
    expect(sale.total).toBe(10)
    const saleReplay = expectBody<{ id: string; replayed: boolean }>(await browserApi(page, '/api/v1/sales', adminToken, {
      method: 'POST', idempotencyKey: saleKey, body: saleBody,
    }), 'idempotent sale replay')
    expect(saleReplay.id).toBe(sale.id)
    expect(saleReplay.replayed).toBe(true)

    const invoice = expectBody<{ invoice_id: string; xml_file_id: string }>(
      await browserApi(page, '/api/v1/fiscal/nfe/xml', adminToken, {
        method: 'POST', body: { sale_id: sale.id },
      }), 'local synthetic invoice XML generation',
    )
    expect(invoice.invoice_id).not.toBe('')
    const xmlList = expectBody<{ items: Array<{ id: string; invoice_id: string }> }>(
      await browserApi(page, '/api/v1/fiscal/nfe/xml?limit=200&offset=0', adminToken), 'local XML list',
    )
    expect(xmlList.items.some((item) => item.id === invoice.xml_file_id && item.invoice_id === invoice.invoice_id)).toBe(true)
    const xmlDownload = await browserApi(page, `/api/v1/fiscal/nfe/xml/${invoice.xml_file_id}/download`, adminToken)
    expect(xmlDownload.status, `preview XML download: ${xmlDownload.text}`).toBe(200)
    expect(String(xmlDownload.body)).toContain('<NFe')
    expect(String(xmlDownload.body)).toContain('PREVIEW NFC-e 65 NAO FISCAL / NAO TRANSMITIR')
    const parsedXML = await page.evaluate((xml) => {
      const document = new DOMParser().parseFromString(xml, 'application/xml')
      return { root: document.documentElement.localName, parserError: document.querySelector('parsererror')?.textContent ?? '' }
    }, String(xmlDownload.body))
    expect(parsedXML.root).toBe('NFe')
    expect(parsedXML.parserError).toBe('')

    const saleDetail = expectBody<{
      items: Array<{ id: string }>
      payments?: Array<{ cash_received?: number; cash_change?: number }>
    }>(
      await browserApi(page, `/api/v1/sales/${sale.id}`, adminToken), 'sale detail',
    )
    expect(saleDetail.payments?.[0]?.cash_received).toBe(20)
    expect(saleDetail.payments?.[0]?.cash_change).toBe(10)
    const returned = expectBody<{ id: string; refund_due: number }>(
      await browserApi(page, `/api/v1/sales/${sale.id}/returns`, adminToken, {
        method: 'POST', idempotencyKey: crypto.randomUUID(), body: {
          kind: 'return', reason: 'Synthetic lab return',
          items: [{ sale_item_id: saleDetail.items[0].id, qty: 1, restock: true }],
        },
      }), 'product return',
    )
    expect(returned.refund_due).toBe(10)
    const refund = expectBody<{ status: string }>(await browserApi(
      page, `/api/v1/finance/returns/${returned.id}/refunds`, adminToken,
      { method: 'POST', idempotencyKey: crypto.randomUUID(), body: {
        method: 'cash', amount: 10, provider: null, external_ref: null,
        cash_session_id: cash.id, notes: 'Synthetic cash refund',
      } },
    ), 'cash refund')
    expect(refund.status).toBe('settled')

    const account = expectBody<{ id: string }>(await browserApi(page, '/api/v1/finance/accounts', adminToken, {
      method: 'POST', idempotencyKey: crypto.randomUUID(), body: {
        kind: 'payable', description: `Synthetic supplier payable ${suffix}`, amount: 25.5, due_date: dueDate,
      },
    }), 'manual payable create')
    expectBody<JsonRecord>(await browserApi(page, `/api/v1/finance/accounts/payable/${account.id}/settle`, adminToken, {
      method: 'POST', idempotencyKey: crypto.randomUUID(), body: { method: 'transfer', note: 'Synthetic lab settlement' },
    }), 'manual payable settle')
    const from = new Date(Date.now() - 3 * 86_400_000).toISOString().slice(0, 10)
    const to = new Date(Date.now() + 86_400_000).toISOString().slice(0, 10)
    const report = expectBody<{ from: string; to: string; hourly: unknown[] }>(
      await browserApi(page, `/api/v1/finance/operational-reports?from=${from}&to=${to}`, adminToken), 'operational report',
    )
    expect(report.hourly.length).toBeGreaterThan(0)

    const close = expectBody<{ status: string; expected_cash: number; closing_difference: number }>(
      await browserApi(page, `/api/v1/cash/sessions/${cash.id}/close`, adminToken, {
        method: 'POST', body: { closing_amount: 107, notes: 'Synthetic cash count with difference' },
      }), 'cash close with declared difference',
    )
    expect(close.status).toBe('closed')
    expect(Number.isFinite(close.expected_cash)).toBe(true)
    expect(Number.isFinite(close.closing_difference)).toBe(true)
    expect(close.closing_difference).toBeCloseTo(107 - close.expected_cash, 2)

    const scarce = expectBody<{ id: string }>(await browserApi(page, '/api/v1/products', adminToken, {
      method: 'POST', body: {
        category_id: null, sku: `CI-RACE-${suffix}`, barcode: null, name: `Synthetic last unit ${suffix}`,
        description: null, unit: 'UN', cost_price: 4, price_cash: 10,
        promo_price: null, min_stock: 0, active: true,
      },
    }), 'concurrency product create')
    expectBody<JsonRecord>(await browserApi(page, '/api/v1/inventory/adjust', adminToken, {
      method: 'POST', body: { product_id: scarce.id, delta: 1, reason: 'Synthetic last-unit race', type: 'adjustment' },
    }), 'concurrency stock adjustment')
    const raceCash = expectBody<{ id: string }>(await browserApi(page, '/api/v1/cash/sessions/open', adminToken, {
      method: 'POST', body: { opening_amount: 0, notes: 'Synthetic two-terminal race' },
    }), 'concurrency cash session open')
    const raceBody = {
      cash_session_id: raceCash.id, customer_id: null, discount_value: 0,
      items: [{ product_id: scarce.id, qty: 1, discount_value: 0 }],
      payments: [{ method: 'cash', amount: 10, cash_received: 10, installments: 1 }],
    }
    const keyA = crypto.randomUUID()
    const keyB = crypto.randomUUID()
    const [raceA, raceB] = await Promise.all([
      browserApi(page, '/api/v1/sales', adminToken, { method: 'POST', idempotencyKey: keyA, body: raceBody }),
      browserApi(cashierPage, '/api/v1/sales', cashierToken, { method: 'POST', idempotencyKey: keyB, body: raceBody }),
    ])
    const winnerIndex = raceA.status >= 200 && raceA.status < 300 ? 0 : raceB.status >= 200 && raceB.status < 300 ? 1 : -1
    const loser = winnerIndex === 0 ? raceB : raceA
    expect(winnerIndex).not.toBe(-1)
    expect([409, 422]).toContain(loser.status)
    const winnerResult = expectBody<{ id: string }>(winnerIndex === 0 ? raceA : raceB, 'two-terminal winning sale')
    const winnerPage = winnerIndex === 0 ? page : cashierPage
    const winnerToken = winnerIndex === 0 ? adminToken : cashierToken
    const winnerKey = winnerIndex === 0 ? keyA : keyB
    const replay = expectBody<{ id: string; replayed: boolean }>(await browserApi(
      winnerPage, '/api/v1/sales', winnerToken,
      { method: 'POST', idempotencyKey: winnerKey, body: raceBody },
    ), 'two-terminal sale replay')
    expect(replay.id).toBe(winnerResult.id)
    expect(replay.replayed).toBe(true)
    const finalStock = expectBody<{ qty_on_hand: number }>(
      await browserApi(page, `/api/v1/products/${scarce.id}`, adminToken), 'concurrency final stock',
    )
    expect(finalStock.qty_on_hand).toBe(0)
    expectBody<JsonRecord>(await browserApi(page, `/api/v1/cash/sessions/${raceCash.id}/close`, adminToken, {
      method: 'POST', body: { closing_amount: 10, notes: 'Synthetic two-terminal close' },
    }), 'two-terminal cash close')

    await page.screenshot({ path: join(evidenceDir, 'installed-real-operations.png'), fullPage: true })
  } finally {
    await cashierContext.close()
  }
})

