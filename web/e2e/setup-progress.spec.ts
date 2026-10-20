import { expect, test } from '@playwright/test'
import { buildSetupSteps, nextSetupAttention, summarizeSetupSteps, type SetupSnapshot } from '../src/lib/setupProgress'

const readyFiscal = {
  tenant_id: 'store-a',
  issuer_identity_configured: true,
  issuer_address_configured: true,
  municipality_code_configured: true,
  config_exists: true,
  certificate_reference_configured: true,
  ready_for_homologation_data: true,
  transmission_enabled: false,
  active_products: 2,
  products_missing_ncm: 0,
  blocking_reasons: [],
}

test('a preparação fiscal nunca é tratada como emissão homologada', () => {
  const snapshot: SetupSnapshot = {
    me: { permissions: ['invoice:read', 'product:read', 'inventory:read'], roles: ['admin'] },
    productsTotal: 2,
    stockMovementsTotal: 3,
    fiscalReadiness: readyFiscal,
  }
  const steps = buildSetupSteps(snapshot)
  expect(steps.find((item) => item.key === 'company')?.status).toBe('ready')
  expect(steps.find((item) => item.key === 'products')?.status).toBe('ready')
  expect(steps.find((item) => item.key === 'stock')?.status).toBe('review')
  expect(steps.find((item) => item.key === 'team')?.status).toBe('review')
  expect(steps.find((item) => item.key === 'fiscal')?.status).toBe('review')
})

test('dados desconhecidos não são aprovados automaticamente', () => {
  const steps = buildSetupSteps({
    me: { permissions: ['product:read', 'inventory:read', 'invoice:read'], roles: [] },
    productsTotal: null,
    stockMovementsTotal: null,
    fiscalReadiness: null,
  })
  for (const step of steps) expect(step.status).not.toBe('ready')
  expect(steps.find((item) => item.key === 'products')?.status).toBe('unknown')
  expect(steps.find((item) => item.key === 'fiscal')?.status).toBe('unknown')
})

test('caixa sem permissões administrativas não recebe estado ou dados fiscais', () => {
  const steps = buildSetupSteps({
    me: { permissions: ['sale:write'], roles: ['cashier'] },
    productsTotal: null,
    stockMovementsTotal: null,
    fiscalReadiness: readyFiscal,
  })
  for (const key of ['company', 'products', 'stock', 'fiscal']) {
    expect(steps.find((item) => item.key === key)?.status).toBe('restricted')
  }
})

test('uma loja sem produtos ou perfil fiscal fica pendente', () => {
  const steps = buildSetupSteps({
    me: { permissions: ['invoice:read', 'product:read', 'inventory:read'], roles: ['manager'] },
    productsTotal: 0,
    stockMovementsTotal: 0,
    fiscalReadiness: { ...readyFiscal, issuer_address_configured: false, ready_for_homologation_data: false },
  })
  expect(steps.find((item) => item.key === 'company')?.status).toBe('pending')
  expect(steps.find((item) => item.key === 'products')?.status).toBe('pending')
  expect(steps.find((item) => item.key === 'stock')?.status).toBe('pending')
  expect(steps.find((item) => item.key === 'fiscal')?.status).toBe('pending')
})

test('resumo do assistente não trata revisão ou restrição como concluída', () => {
  const steps = buildSetupSteps({
    me: { permissions: ['invoice:read', 'product:read', 'inventory:read'], roles: ['manager'] },
    productsTotal: 3,
    stockMovementsTotal: 1,
    fiscalReadiness: readyFiscal,
  })
  expect(summarizeSetupSteps(steps)).toEqual({ verified: 2, attention: 3, restricted: 0 })
  const cashierSteps = buildSetupSteps({
    me: { permissions: ['sale:write'], roles: ['cashier'] },
    productsTotal: null,
    stockMovementsTotal: null,
    fiscalReadiness: null,
  })
  expect(summarizeSetupSteps(cashierSteps)).toEqual({ verified: 0, attention: 1, restricted: 4 })
})

test('avançar para atenção ignora prontas e inacessíveis e retorna após a última etapa', () => {
  const steps = buildSetupSteps({
    me: { permissions: ['invoice:read', 'product:read', 'inventory:read'], roles: ['manager'] },
    productsTotal: 0,
    stockMovementsTotal: 0,
    fiscalReadiness: readyFiscal,
  })
  expect(nextSetupAttention(steps, 'company')).toBe('products')
  expect(nextSetupAttention(steps, 'products')).toBe('stock')
  expect(nextSetupAttention(steps, 'fiscal')).toBe('products')
  expect(nextSetupAttention(steps.filter((step) => step.status === 'ready'), 'company')).toBeNull()
  const cashierSteps = buildSetupSteps({
    me: { permissions: ['sale:write'], roles: ['cashier'] },
    productsTotal: null,
    stockMovementsTotal: null,
    fiscalReadiness: null,
  })
  expect(nextSetupAttention(cashierSteps, 'company')).toBe('team')
})
