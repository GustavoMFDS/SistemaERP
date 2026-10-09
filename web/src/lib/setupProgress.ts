export type SetupStatus = 'ready' | 'pending' | 'review' | 'restricted' | 'unknown'

export type SetupStepKey = 'company' | 'products' | 'stock' | 'team' | 'fiscal'
export type SetupStep = {
  key: SetupStepKey
  title: string
  status: SetupStatus
  detail: string
}

export type SetupPermissions = {
  permissions: string[]
  roles: string[]
}

export type FiscalReadinessData = {
  tenant_id: string
  issuer_identity_configured: boolean
  issuer_address_configured: boolean
  municipality_code_configured: boolean
  config_exists: boolean
  certificate_reference_configured: boolean
  ready_for_homologation_data: boolean
  transmission_enabled: boolean
  active_products: number
  products_missing_ncm: number
  blocking_reasons: string[]
}

export type SetupSnapshot = {
  me: SetupPermissions
  productsTotal: number | null
  stockMovementsTotal: number | null
  fiscalReadiness: FiscalReadinessData | null
}

export function buildSetupSteps(data: SetupSnapshot): SetupStep[] {
  const permits = new Set(data.me.permissions)
  const fiscalAccess = permits.has('invoice:read')
  const productsAccess = permits.has('product:read')
  const inventoryAccess = permits.has('inventory:read')
  const ready = data.fiscalReadiness

  let company: SetupStatus = 'unknown'
  if (!fiscalAccess) company = 'restricted'
  else if (ready) {
    company = ready.issuer_identity_configured &&
      ready.issuer_address_configured &&
      ready.municipality_code_configured ? 'ready' : 'pending'
  }

  let products: SetupStatus = 'unknown'
  if (!productsAccess) products = 'restricted'
  else if (data.productsTotal !== null) products = data.productsTotal > 0 ? 'ready' : 'pending'

  // Movement history proves some activity, NOT an audited completed opening count.
  let stock: SetupStatus = 'unknown'
  if (!inventoryAccess) stock = 'restricted'
  else if (data.stockMovementsTotal !== null) {
    stock = data.stockMovementsTotal > 0 ? 'review' : 'pending'
  }

  let fiscal: SetupStatus = 'unknown'
  if (!fiscalAccess) fiscal = 'restricted'
  else if (ready) {
    // Data readiness is never equivalent to production SEFAZ authorization.
    fiscal = ready.ready_for_homologation_data ? 'review' : 'pending'
  }

  return [
    {
      key: 'company', title: 'Dados da minha loja', status: company,
      detail: company === 'ready' ? 'Identificação e endereço fiscal preenchidos.' :
        'Confirme os dados oficiais do CNPJ e o endereço com o contador.',
    },
    {
      key: 'products', title: 'Produtos e preços', status: products,
      detail: products === 'ready' ? `${data.productsTotal} produto(s) cadastrado(s).` :
        'Cadastre os produtos individualmente ou importe uma planilha.',
    },
    {
      key: 'stock', title: 'Estoque inicial', status: stock,
      detail: stock === 'review' ? 'Há movimentações de estoque; confira os saldos com a contagem física.' :
        'Confira quantidades físicas e use a importação inicial apenas em produtos nunca movimentados.',
    },
    {
      key: 'team', title: 'Funcionários e permissões', status: 'review',
      detail: permits.has('team:manage')
        ? 'Use Funcionários para convidar pessoas e conferir seus acessos. Revise a equipe antes do piloto.'
        : 'Peça ao administrador da loja para revisar a equipe e suas permissões.',
    },
    {
      key: 'fiscal', title: 'Preparar a NFC-e', status: fiscal,
      detail: fiscal === 'review' ? 'Dados básicos preparados para avaliação de homologação; emissão real não está liberada por este assistente.' :
        'Verifique NCM, regras fiscais, certificado A1 e as pendências de homologação.',
    },
  ]
}

/**
 * Estes números descrevem apenas evidências obtidas do servidor.
 * Etapas "review" não viram "ready" sem uma comprovação verificável;
 * especialmente a NFC-e nunca é tratada como homologada pelo assistente.
 */
export function summarizeSetupSteps(steps: SetupStep[]) {
  return {
    verified: steps.filter((step) => step.status === 'ready').length,
    attention: steps.filter((step) => step.status !== 'ready' && step.status !== 'restricted').length,
    restricted: steps.filter((step) => step.status === 'restricted').length,
  }
}

/** Próxima etapa acessível que merece conferência, após a etapa atual. */
export function nextSetupAttention(steps: SetupStep[], current: SetupStepKey): SetupStepKey | null {
  const index = steps.findIndex((step) => step.key === current)
  const ordered = index < 0 ? steps : [...steps.slice(index + 1), ...steps.slice(0, index + 1)]
  return ordered.find((step) => step.status !== 'ready' && step.status !== 'restricted')?.key ?? null
}
