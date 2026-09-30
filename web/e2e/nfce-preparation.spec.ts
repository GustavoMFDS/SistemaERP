import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('NFC-e preparation stays disabled and never exposes secret references', async ({ page }) => {
  await login(page)

  const result = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')

    const issuer = await apiJson<Record<string, unknown>>('/api/v1/fiscal/nfce/issuer', {
      method: 'PUT',
      body: {
        ie: '110042490114',
        crt: '4',
        address_street: 'Avenida Fiscal',
        address_number: '100',
        address_complement: null,
        address_neighborhood: 'Centro',
        address_city: 'Uberlandia',
        address_city_code: '3170206',
        address_state: 'MG',
        address_zip: '38400000',
      },
    })

    const config = await apiJson<Record<string, unknown>>('/api/v1/fiscal/nfce/config', {
      method: 'PUT',
      body: {
        environment: 'homologation',
        series: 1,
        certificate_secret_ref: 'secret://nfce/e2e/certificate',
      },
    })

    const fetchedConfig = await apiJson<Record<string, unknown>>('/api/v1/fiscal/nfce/config')
    const readiness = await apiJson<Record<string, unknown>>('/api/v1/fiscal/nfce/readiness')

    return { issuer, config, fetchedConfig, readiness }
  })

  expect(result.issuer.crt).toBe('4')
  expect(result.issuer.address_city_code).toBe('3170206')

  expect(result.config.enabled).toBe(false)
  expect(result.config.environment).toBe('homologation')
  expect(result.config.csc_reference_configured).toBe(false)
  expect(result.config.certificate_reference_configured).toBe(true)

  expect(result.fetchedConfig.enabled).toBe(false)
  expect(result.fetchedConfig).not.toHaveProperty('csc_secret_ref')
  expect(result.fetchedConfig).not.toHaveProperty('certificate_secret_ref')

  expect(result.readiness.model).toBe(65)
  expect(result.readiness.transmission_enabled).toBe(false)
  expect(result.readiness.issuer_identity_configured).toBe(true)
  expect(result.readiness.issuer_address_configured).toBe(true)
  expect(result.readiness.municipality_code_configured).toBe(true)
  expect(result.readiness.csc_reference_configured).toBe(false)
  expect(result.readiness.certificate_reference_configured).toBe(true)
  expect(result.readiness.products_missing_ncm).toBe(0)
  expect(result.readiness.ready_for_homologation_data).toBe(true)

  await page.goto('/fiscal')
  await expect(page.getByRole('heading', { name: 'Fiscal — preparação NFC-e' })).toBeVisible()
  await expect(page.getByText('Esta tela não transmite nem autoriza documentos na SEFAZ.')).toBeVisible()
  await expect(page.getByRole('button', { name: /Gerar XML|Emitir|Transmitir/i })).toHaveCount(0)
})

test('NFC-e preparation rejects invalid municipality and series', async ({ page }) => {
  await login(page)

  const statuses = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')

    const statusFor = async (path: string, body: unknown) => {
      try {
        await apiJson(path, { method: 'PUT', body })
        return 200
      } catch (error) {
        if (error instanceof APIError) return error.status
        throw error
      }
    }

    return {
      issuer: await statusFor('/api/v1/fiscal/nfce/issuer', {
        ie: '123',
        crt: '4',
        address_street: 'Rua A',
        address_number: '1',
        address_complement: null,
        address_neighborhood: 'Centro',
        address_city: 'Cidade',
        address_city_code: '123',
        address_state: 'MG',
        address_zip: '38400000',
      }),
      issuerUF: await statusFor('/api/v1/fiscal/nfce/issuer', {
        ie: '123',
        crt: '4',
        address_street: 'Rua A',
        address_number: '1',
        address_complement: null,
        address_neighborhood: 'Centro',
        address_city: 'Cidade',
        address_city_code: '3170206',
        address_state: 'XX',
        address_zip: '38400000',
      }),
      config: await statusFor('/api/v1/fiscal/nfce/config', {
        environment: 'homologation',
        series: 890,
        certificate_secret_ref: 'secret://nfce/e2e/certificate',
      }),
    }
  })

  expect(statuses.issuer).toBe(422)
  expect(statuses.issuerUF).toBe(422)
  expect(statuses.config).toBe(422)
})
