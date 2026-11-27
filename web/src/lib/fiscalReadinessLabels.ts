// Presentation only: these labels never change the server's readiness decision.
const labels: Record<string, string> = {
  issuer_identity: 'Identidade fiscal do emitente incompleta',
  issuer_legal_name_missing: 'Razão social ausente no cadastro da empresa',
  issuer_cnpj_invalid: 'CNPJ não passou na validação local dos dígitos verificadores',
  issuer_ie_missing: 'Inscrição estadual não informada',
  issuer_crt_invalid: 'CRT ainda não informado ou código não reconhecido: confirme com o contador',
  issuer_address: 'Endereço do emitente incompleto',
  issuer_street_missing: 'Rua ou avenida não informada',
  issuer_number_missing: 'Número do endereço não informado',
  issuer_neighborhood_missing: 'Bairro não informado',
  issuer_city_missing: 'Nome do município não informado',
  issuer_zip_invalid: 'CEP deve ter exatamente 8 dígitos numéricos',
  issuer_municipality_code: 'Código IBGE do município ausente/inválido',
  issuer_uf_unsupported: 'A integração atual da NFC-e só aceita emitente de MG',
  issuer_municipality_uf_mismatch: 'O município do emitente precisa usar código IBGE de MG (31xxxxx)',
  nfce_config: 'Configuração NFC-e ainda não preparada',
  homologation_environment: 'Ambiente deve estar em homologação nesta etapa',
  certificate_secret_reference: 'Referência do certificado ausente',
  active_products: 'Nenhum produto ativo para validar',
  product_ncm: 'Há produtos ativos sem NCM',
  product_fiscal_profile: 'Classificação tributária dos produtos',
}

const identityDetails = ['issuer_legal_name_missing', 'issuer_cnpj_invalid', 'issuer_ie_missing', 'issuer_crt_invalid']
const addressDetails = ['issuer_street_missing', 'issuer_number_missing', 'issuer_neighborhood_missing', 'issuer_city_missing', 'issuer_zip_invalid']

export function fiscalReadinessLabels(reasons: readonly string[]): string[] {
  const codes = new Set(reasons)
  const result: string[] = []
  let unknown = 0
  for (const code of codes) {
    if (code === 'issuer_identity' && identityDetails.some((detail) => codes.has(detail))) continue
    if (code === 'issuer_address' && addressDetails.some((detail) => codes.has(detail))) continue
    if (Object.prototype.hasOwnProperty.call(labels, code)) result.push(labels[code])
    else unknown++
  }
  if (unknown === 1) result.push('Existe uma verificação fiscal que precisa de orientação do responsável.')
  if (unknown > 1) result.push(unknown + ' outras pendências fiscais precisam de orientação do responsável.')
  return result
}
