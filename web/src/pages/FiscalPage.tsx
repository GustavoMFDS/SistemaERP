import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { APIError, apiDownload, apiJson, apiOpenPrintable, errorMessage } from '../lib/api'

type XMLFile = {
  id: string
  invoice_id: string
  file_name: string
  sha256: string
  created_at: string
}

type XMLListResponse = { items: XMLFile[]; total: number }

type FiscalObligation = {
  sale_id: string
  created_at: string
  document_kind: 'nfce' | 'nfe'
  sale_status: string
  status: string
  authorized: boolean
  legacy_review: boolean
  invoice_id?: string
  access_key?: string
  processed_xml_available?: boolean
}
type FiscalObligationsPage = {
  items: FiscalObligation[]
  total: number
  limit: number
  offset: number
}

const fiscalObligationStatusLabel: Record<string, string> = {
  pending: 'Aguardando emissão',
  legacy_review: 'Venda anterior: revisar',
  sale_cancelled_review: 'Venda cancelada: revisar fiscal',
  reserved: 'Número reservado — não autorizado',
  signed: 'Documento assinado — não autorizado',
  submitted: 'Enviado — consultar protocolo',
  rejected: 'Rejeitada — corrigir',
  authorized: 'Autorizada',
  cancelled: 'Nota cancelada — revisar',
  xml_generated: 'XML preparado — sem autorização',
}

type NFCeReadiness = {
  tenant_id: string
  model: number
  issuer_identity_configured: boolean
  issuer_address_configured: boolean
  municipality_code_configured: boolean
  config_exists: boolean
  transmission_enabled: boolean
  environment?: string
  series?: number
  csc_reference_configured: boolean
  certificate_reference_configured: boolean
  active_products: number
  products_missing_ncm: number
  ready_for_homologation_data: boolean
  blocking_reasons: string[]
}

type NFCeIssuerProfile = {
  tenant_id: string
  legal_name: string
  trade_name?: string | null
  cnpj: string
  ie: string
  crt: string
  address_street: string
  address_number: string
  address_complement?: string | null
  address_neighborhood: string
  address_city: string
  address_city_code: string
  address_state: string
  address_zip: string
}

type NFCeConfig = {
  tenant_id: string
  enabled: boolean
  environment: string
  series: number
  csc_id?: string | null
  csc_reference_configured: boolean
  certificate_reference_configured: boolean
}

const blockerLabels: Record<string, string> = {
  issuer_identity: 'Identidade fiscal do emitente incompleta',
  issuer_address: 'Endereço do emitente incompleto',
  issuer_municipality_code: 'Código IBGE do município ausente/inválido',
  nfce_config: 'Configuração NFC-e ainda não preparada',
  homologation_environment: 'Ambiente deve estar em homologação nesta etapa',
  certificate_secret_reference: 'Referência do certificado ausente',
  active_products: 'Nenhum produto ativo para validar',
  product_ncm: 'Há produtos ativos sem NCM',
}

export default function FiscalPage() {
  const [loading, setLoading] = useState(false)
  const [savingIssuer, setSavingIssuer] = useState(false)
  const [savingConfig, setSavingConfig] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [items, setItems] = useState<XMLFile[]>([])
  const [total, setTotal] = useState(0)
  const [fiscalObligations, setFiscalObligations] = useState<FiscalObligationsPage | null>(null)
  const [fiscalObligationsError, setFiscalObligationsError] = useState('')
  const [fiscalObligationsLoading, setFiscalObligationsLoading] = useState(false)
  const [obligationsOffset, setObligationsOffset] = useState(0)
  const [showAllObligations, setShowAllObligations] = useState(false)
  const [readiness, setReadiness] = useState<NFCeReadiness | null>(null)
  const [issuer, setIssuer] = useState<NFCeIssuerProfile | null>(null)
  const [config, setConfig] = useState<NFCeConfig | null>(null)
  const [canPrepare, setCanPrepare] = useState(false)

  const [ie, setIE] = useState('')
  const [crt, setCRT] = useState('')
  const [street, setStreet] = useState('')
  const [number, setNumber] = useState('')
  const [complement, setComplement] = useState('')
  const [neighborhood, setNeighborhood] = useState('')
  const [city, setCity] = useState('')
  const [cityCode, setCityCode] = useState('')
  const [state, setState] = useState('')
  const [zip, setZIP] = useState('')

  const [environment, setEnvironment] = useState('homologation')
  const [series, setSeries] = useState(1)
  const [certificateSecretRef, setCertificateSecretRef] = useState('')

  function applyIssuer(profile: NFCeIssuerProfile) {
    setIssuer(profile)
    setIE(profile.ie ?? '')
    setCRT(profile.crt ?? '')
    setStreet(profile.address_street ?? '')
    setNumber(profile.address_number ?? '')
    setComplement(profile.address_complement ?? '')
    setNeighborhood(profile.address_neighborhood ?? '')
    setCity(profile.address_city ?? '')
    setCityCode(profile.address_city_code ?? '')
    setState(profile.address_state ?? '')
    setZIP(profile.address_zip ?? '')
  }

  function applyConfig(next: NFCeConfig | null) {
    setConfig(next)
    setEnvironment(next?.environment || 'homologation')
    setSeries(next?.series ?? 1)
    setCertificateSecretRef('')
  }

  async function downloadAuthorizedProcessedXML(item: FiscalObligation) {
    if (!item.authorized || !item.invoice_id || item.document_kind !== 'nfce') return
    setFiscalObligationsError('')
    try {
      await apiDownload(
        '/api/v1/fiscal/nfce/invoices/' + encodeURIComponent(item.invoice_id) + '/processed-xml',
        (item.access_key ?? item.invoice_id) + '-procNFe.xml',
        'application/xml',
      )
    } catch (error: unknown) {
      setFiscalObligationsError(
        'XML autorizado indisponível: ' + errorMessage(error) +
        '. Verifique se o protocolo SEFAZ foi preservado para esta nota.',
      )
    }
  }

  async function printAuthorizedFiscalObligation(item: FiscalObligation) {
    if (!item.authorized || !item.invoice_id || item.document_kind !== 'nfce') return
    setFiscalObligationsError('')
    try {
      await apiOpenPrintable(
        '/api/v1/fiscal/nfce/invoices/' + encodeURIComponent(item.invoice_id) + '/danfe',
        true,
      )
    } catch (error: unknown) {
      setFiscalObligationsError('Impressão indisponível: ' + errorMessage(error))
    }
  }

  async function loadObligations(offset: number, includeAuthorized: boolean) {
    // A controlled checkbox must update synchronously; waiting for the
    // network before updating "checked" makes the browser undo the click.
    setShowAllObligations(includeAuthorized)
    setFiscalObligations(null)
    setFiscalObligationsLoading(true)
    setFiscalObligationsError('')
    try {
      const page = await apiJson<FiscalObligationsPage>(
        '/api/v1/fiscal/obligations?limit=20&offset=' + offset +
        '&unresolved=' + String(!includeAuthorized),
      )
      setFiscalObligations(page)
      setObligationsOffset(page.offset)
    } catch (error: unknown) {
      setFiscalObligationsError(errorMessage(error))
    } finally {
      setFiscalObligationsLoading(false)
    }
  }

  async function load() {
    setError('')
    setLoading(true)
    try {
      const [readinessData, issuerData, xmlData, me] = await Promise.all([
        apiJson<NFCeReadiness>('/api/v1/fiscal/nfce/readiness'),
        apiJson<NFCeIssuerProfile>('/api/v1/fiscal/nfce/issuer'),
        apiJson<XMLListResponse>('/api/v1/fiscal/nfe/xml?limit=50&offset=0'),
        apiJson<{ permissions: string[] }>('/api/v1/auth/me'),
      ])
      setReadiness({ ...readinessData, blocking_reasons: Array.isArray(readinessData.blocking_reasons) ? readinessData.blocking_reasons : [] })
      applyIssuer(issuerData)
      setItems(Array.isArray(xmlData.items) ? xmlData.items : [])
      setTotal(xmlData.total ?? 0)
      setCanPrepare(me.permissions.includes('invoice:generate'))

      try {
        const configData = await apiJson<NFCeConfig>('/api/v1/fiscal/nfce/config')
        applyConfig(configData)
      } catch (configError: unknown) {
        if (configError instanceof APIError && configError.status === 404) {
          applyConfig(null)
        } else {
          throw configError
        }
      }
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    void loadObligations(0, false)
  }, [])

  async function saveIssuer(e: FormEvent) {
    e.preventDefault()
    if (!canPrepare) return
    setError('')
    setMessage('')
    setSavingIssuer(true)
    try {
      const profile = await apiJson<NFCeIssuerProfile>('/api/v1/fiscal/nfce/issuer', {
        method: 'PUT',
        body: {
          ie: ie.trim(),
          crt: crt.trim(),
          address_street: street.trim(),
          address_number: number.trim(),
          address_complement: complement.trim() || null,
          address_neighborhood: neighborhood.trim(),
          address_city: city.trim(),
          address_city_code: cityCode.trim(),
          address_state: state.trim().toUpperCase(),
          address_zip: zip.trim(),
        },
      })
      applyIssuer(profile)
      setMessage('Dados da loja salvos. Próximo passo: configurar o certificado da NFC-e.')
      document.getElementById('fiscal-certificate')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      const next = await apiJson<NFCeReadiness>('/api/v1/fiscal/nfce/readiness')
      setReadiness({ ...next, blocking_reasons: Array.isArray(next.blocking_reasons) ? next.blocking_reasons : [] })
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setSavingIssuer(false)
    }
  }

  async function saveConfig(e: FormEvent) {
    e.preventDefault()
    if (!canPrepare) return
    setError('')
    setMessage('')
    setSavingConfig(true)
    try {
      const next = await apiJson<NFCeConfig>('/api/v1/fiscal/nfce/config', {
        method: 'PUT',
        body: {
          environment,
          series: Number(series),
          certificate_secret_ref: certificateSecretRef.trim(),
        },
      })
      applyConfig(next)
      setMessage('Preparação salva. Confira as pendências abaixo antes de pedir a homologação. O sistema não ativou a transmissão.')
      document.getElementById('fiscal-readiness')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      const readinessData = await apiJson<NFCeReadiness>('/api/v1/fiscal/nfce/readiness')
      setReadiness({ ...readinessData, blocking_reasons: Array.isArray(readinessData.blocking_reasons) ? readinessData.blocking_reasons : [] })
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setSavingConfig(false)
    }
  }

  async function onDownload(x: XMLFile) {
    setError('')
    try {
      await apiDownload(
        `/api/v1/fiscal/nfe/xml/${x.id}/download`,
        x.file_name || `nfce-preview-${x.id}.xml`,
        'application/xml',
      )
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  async function onDANFE(x: XMLFile) {
    setError('')
    try {
      await apiOpenPrintable(`/api/v1/fiscal/nfce/invoices/${x.invoice_id}/danfe`)
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-slate-900">Nota fiscal da loja</h2>
          <p className="text-sm text-gray-600">
            Veja o que falta para preparar a emissão de notas. Esta tela não transmite notas para a SEFAZ.
          </p>
        </div>
        <button
          onClick={() => void load()}
          className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
          disabled={loading}
        >
          {loading ? 'Atualizando…' : 'Atualizar'}
        </button>
      </div>

      <div className="mt-5 rounded-xl border border-blue-100 bg-blue-50 p-4 text-sm text-blue-950">
        <p className="font-semibold">Comece pelos dados da sua loja</p>
        <p className="mt-1 text-blue-900">
          Confira a situação abaixo. Se faltar algo, abra a seção correspondente e preencha os campos.
          Um contador ou suporte técnico deve conferir os dados fiscais antes de qualquer emissão real.
        </p>
      </div>

      {message ? (
        <div className="mt-3 rounded-md border border-green-200 bg-green-50 p-2 text-sm text-green-700">
          {message}
        </div>
      ) : null}

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      {readiness ? (
        <section id="fiscal-readiness" className="mt-4 rounded-md border p-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <h3 className="text-sm font-semibold">Prontidão para homologação</h3>
              <p className="text-xs text-gray-600">
                Ambiente: {readiness.environment || 'não configurado'} · Série:{' '}
                {readiness.series ?? '—'} · Produtos ativos: {readiness.active_products}
              </p>
            </div>
            <span
              className={
                readiness.ready_for_homologation_data
                  ? 'rounded-full bg-green-100 px-3 py-1 text-xs font-medium text-green-800'
                  : 'rounded-full bg-amber-100 px-3 py-1 text-xs font-medium text-amber-800'
              }
            >
              {readiness.ready_for_homologation_data ? 'Dados prontos' : 'Pendências'}
            </span>
          </div>

          <details className="mt-4 rounded-lg bg-slate-50 p-3 text-sm">
            <summary className="cursor-pointer font-medium text-slate-700">Ver detalhes técnicos da preparação</summary>
            <div className="mt-3 grid gap-3 text-xs sm:grid-cols-3">
            <div>Emitente: {readiness.issuer_identity_configured ? 'OK' : 'Pendente'}</div>
            <div>Endereço: {readiness.issuer_address_configured ? 'OK' : 'Pendente'}</div>
            <div>Município IBGE: {readiness.municipality_code_configured ? 'OK' : 'Pendente'}</div>
            <div>
              CSC legado (QR v2): {readiness.csc_reference_configured ? 'Configurado' : 'Opcional'}
            </div>
            <div>Certificado ref.: {readiness.certificate_reference_configured ? 'OK' : 'Pendente'}</div>
            <div>NCM ausente: {readiness.products_missing_ncm}</div>
            </div>
          </details>

          {(readiness.blocking_reasons ?? []).length > 0 ? (
            <ul className="mt-3 list-disc pl-5 text-xs text-amber-800">
              {(readiness.blocking_reasons ?? []).map((reason) => (
                <li key={reason}>{blockerLabels[reason] ?? reason}</li>
              ))}
            </ul>
          ) : null}

          <div className="mt-3 rounded-md bg-gray-50 p-2 text-xs text-gray-700">
            Configuração de transmissão: <strong>{readiness.transmission_enabled ? 'habilitada para esta loja' : 'desativada para esta loja'}</strong>.
            Os dados prontos são apenas uma das exigências: emissão fiscal exige certificado válido,
            regras tributárias revisadas, provedor SEFAZ e homologação. Esta tela não emite notas.
          </div>
        </section>
      ) : null}

      <section aria-label="Pendências fiscais das vendas" className="mt-4 rounded-2xl border border-amber-200 bg-white p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-bold text-slate-900">Vendas e emissão fiscal</h2>
            <p className="mt-1 text-xs text-slate-600">
              Cada venda gera uma obrigação fiscal independente da impressão.
              Uma pendência nesta lista não é uma NFC-e/NF-e autorizada.
            </p>
          </div>
          <button type="button" disabled={fiscalObligationsLoading}
            onClick={() => void loadObligations(obligationsOffset, showAllObligations)}
            className="rounded-md border border-slate-300 px-3 py-2 text-xs disabled:opacity-50">
            {fiscalObligationsLoading ? 'Consultando…' : 'Atualizar pendências'}
          </button>
        </div>
        <label className="mt-3 flex items-center gap-2 text-xs">
          <input type="checkbox" checked={showAllObligations} disabled={fiscalObligationsLoading}
            onChange={(event) => void loadObligations(0, event.target.checked)} />
          Mostrar também vendas com nota autorizada
        </label>
        {fiscalObligationsError ? (
          <p role="alert" className="mt-3 text-sm text-red-700">
            Não foi possível consultar as obrigações fiscais: {fiscalObligationsError}
          </p>
        ) : null}
        {fiscalObligations ? (
          <>
            <p className="mt-3 text-xs text-slate-600">
              {showAllObligations ? 'Registros fiscais da loja' : 'Vendas que precisam de acompanhamento'}:
              {' '}{fiscalObligations.total}
            </p>
            <div className="mt-2 overflow-x-auto rounded-md border">
              <table className="min-w-full text-left text-xs">
                <thead className="bg-slate-50 text-slate-600">
                  <tr>
                    <th className="px-3 py-2">Venda</th>
                    <th className="px-3 py-2">Data</th>
                    <th className="px-3 py-2">Modelo</th>
                    <th className="px-3 py-2">Situação fiscal</th>
                    <th className="px-3 py-2">Ações</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {fiscalObligations.items.map((item) => (
                    <tr key={item.sale_id}>
                      <td className="px-3 py-2 font-mono" title={item.sale_id}>
                        {item.sale_id.slice(0, 8)}…
                      </td>
                      <td className="px-3 py-2">
                        {new Date(item.created_at).toLocaleString('pt-BR')}
                      </td>
                      <td className="px-3 py-2">{item.document_kind.toUpperCase()}</td>
                      <td className={item.authorized
                        ? 'px-3 py-2 text-green-800'
                        : 'px-3 py-2 font-semibold text-amber-800'}>
                        {fiscalObligationStatusLabel[item.status] ?? item.status}
                        {item.legacy_review ? ' • histórico' : ''}
                      </td>
                      <td className="px-3 py-2">
                        {item.authorized && item.document_kind === 'nfce' && item.invoice_id ? (
                          <div className="flex flex-wrap gap-2">
                            <button type="button"
                              onClick={() => void printAuthorizedFiscalObligation(item)}
                              className="rounded border border-slate-300 px-2 py-1 text-xs">
                              Imprimir DANFE
                            </button>
                            {item.processed_xml_available ? (
                              <button type="button"
                                onClick={() => void downloadAuthorizedProcessedXML(item)}
                                className="rounded border border-slate-300 px-2 py-1 text-xs">
                                Baixar XML autorizado
                              </button>
                            ) : (
                              <span className="text-xs text-amber-800" title="XML processado com protocolo SEFAZ ainda não arquivado">
                                XML final pendente de arquivamento
                              </span>
                            )}
                          </div>
                        ) : (
                          <span className="text-slate-500">Acompanhar</span>
                        )}
                      </td>
                    </tr>
                  ))}
                  {fiscalObligations.items.length === 0 ? (
                    <tr><td colSpan={5} className="px-3 py-5 text-center text-slate-600">
                      Nenhum registro nesta consulta.
                    </td></tr>
                  ) : null}
                </tbody>
              </table>
            </div>
            <div className="mt-3 flex items-center gap-3 text-xs">
              <button type="button" disabled={fiscalObligationsLoading || obligationsOffset === 0}
                onClick={() => void loadObligations(Math.max(0, obligationsOffset - 20), showAllObligations)}
                className="rounded border border-slate-300 px-3 py-2 disabled:opacity-40">
                Anteriores
              </button>
              <span>Mostrando {fiscalObligations.items.length === 0 ? 0 : obligationsOffset + 1}
                {' '}a {obligationsOffset + fiscalObligations.items.length}
              </span>
              <button type="button" disabled={fiscalObligationsLoading ||
                obligationsOffset + fiscalObligations.items.length >= fiscalObligations.total}
                onClick={() => void loadObligations(obligationsOffset + 20, showAllObligations)}
                className="rounded border border-slate-300 px-3 py-2 disabled:opacity-40">
                Próximas
              </button>
            </div>
            <p className="mt-3 text-xs text-slate-600">
              A emissão real, correções de rejeição e transmissão à SEFAZ
              continuarão desabilitadas até completar homologação e segurança fiscal.
            </p>
          </>
        ) : null}
      </section>

      {issuer ? (
        <details id="fiscal-issuer" className="mt-4 rounded-2xl border border-slate-200 p-5">
          <summary className="cursor-pointer text-base font-bold text-slate-900">Preencher dados da loja</summary>
          <p className="mt-1 text-xs text-gray-600">Preencha conforme o cadastro oficial da empresa e confirme com o contador.</p>
          <p className="mt-1 text-xs text-gray-600">
            {issuer.legal_name} · CNPJ {issuer.cnpj}
          </p>
          <form onSubmit={saveIssuer} className="mt-3 grid grid-cols-1 gap-3 md:grid-cols-4">
            <label>
              <span className="text-xs text-gray-600">Inscrição estadual</span>
              <input value={ie} onChange={(e) => setIE(e.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 text-sm" disabled={!canPrepare} />
            </label>
            <label>
              <span className="text-xs text-gray-600">Regime tributário</span>
              <select value={crt} onChange={(e) => setCRT(e.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 text-sm" disabled={!canPrepare}>
                <option value="">Selecione</option>
                <option value="1">1 — Simples Nacional</option>
                <option value="2">2 — Simples Nacional, excesso</option>
                <option value="3">3 — Regime Normal</option>
                <option value="4">4 — MEI</option>
              </select>
            </label>
            <label className="md:col-span-2">
              <span className="text-xs text-gray-600">Rua ou avenida</span>
              <input value={street} onChange={(e) => setStreet(e.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 text-sm" disabled={!canPrepare} />
            </label>
            <label>
              <span className="text-xs text-gray-600">Número</span>
              <input value={number} onChange={(e) => setNumber(e.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 text-sm" disabled={!canPrepare} />
            </label>
            <label>
              <span className="text-xs text-gray-600">Complemento</span>
              <input value={complement} onChange={(e) => setComplement(e.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 text-sm" disabled={!canPrepare} />
            </label>
            <label>
              <span className="text-xs text-gray-600">Bairro</span>
              <input value={neighborhood} onChange={(e) => setNeighborhood(e.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 text-sm" disabled={!canPrepare} />
            </label>
            <label>
              <span className="text-xs text-gray-600">Cidade</span>
              <input value={city} onChange={(e) => setCity(e.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 text-sm" disabled={!canPrepare} />
            </label>
            <label>
              <span className="text-xs text-gray-600">Código da cidade (IBGE)</span>
              <input value={cityCode} onChange={(e) => setCityCode(e.target.value)} inputMode="numeric" maxLength={7} className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm" disabled={!canPrepare} />
            </label>
            <label>
              <span className="text-xs text-gray-600">Estado (UF)</span>
              <input value={state} onChange={(e) => setState(e.target.value.toUpperCase())} maxLength={2} className="mt-1 w-full rounded-md border px-3 py-2 text-sm uppercase" disabled={!canPrepare} />
            </label>
            <label>
              <span className="text-xs text-gray-600">CEP (8 dígitos)</span>
              <input value={zip} onChange={(e) => setZIP(e.target.value)} inputMode="numeric" maxLength={8} className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm" disabled={!canPrepare} />
            </label>
            {canPrepare ? (
              <div className="md:col-span-4">
                <button disabled={savingIssuer} className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60">
                  {savingIssuer ? 'Salvando…' : 'Salvar emitente'}
                </button>
              </div>
            ) : null}
          </form>
        </details>
      ) : null}

      <details id="fiscal-certificate" className="mt-4 rounded-md border p-4">
        <summary className="cursor-pointer text-base font-bold text-slate-900">Certificado digital e configurações avançadas</summary>
        <p className="mt-1 text-xs text-gray-600">
          Use primeiro o ambiente de testes (homologação). O certificado A1 deve ficar
          no gerenciador seguro do servidor; nunca envie o arquivo PFX, a senha ou a chave privada
          por e-mail ou mensagem. O responsável pela instalação fornece a referência segura.
          Caso ela já esteja configurada, deixe o campo em branco para manter a atual.
        </p>
        <form onSubmit={saveConfig} className="mt-3 grid grid-cols-1 gap-3 md:grid-cols-3">
          <label>
            <span className="text-xs text-gray-600">Ambiente</span>
            <select value={environment} onChange={(e) => setEnvironment(e.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 text-sm" disabled={!canPrepare}>
              <option value="homologation">Homologação</option>
              <option value="production">Produção (somente preparar)</option>
            </select>
          </label>
          <label>
            <span className="text-xs text-gray-600">Série</span>
            <input value={series} onChange={(e) => setSeries(Number(e.target.value))} type="number" min={0} max={889} className="mt-1 w-full rounded-md border px-3 py-2 text-sm" disabled={!canPrepare} />
          </label>
          <label className="md:col-span-3">
            <span className="text-xs text-gray-600">Certificado A1 (código seguro fornecido pelo suporte)</span>
            <input value={certificateSecretRef} onChange={(e) => setCertificateSecretRef(e.target.value)} placeholder={config?.certificate_reference_configured ? 'Certificado já vinculado: deixe em branco para manter' : 'Peça ao responsável técnico a referência segura do certificado'} className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm" disabled={!canPrepare} />
          </label>
          {canPrepare ? (
            <div className="md:col-span-3">
              {!config?.certificate_reference_configured && !certificateSecretRef.trim() ? (
                <p className="mb-2 text-xs text-amber-800">Solicite o vínculo seguro do certificado A1 antes de concluir esta etapa.</p>
              ) : null}
              <button disabled={savingConfig || (!config?.certificate_reference_configured && !certificateSecretRef.trim())} className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60">
                {savingConfig ? 'Salvando…' : 'Salvar preparação (transmissão permanece desativada)'}
              </button>
            </div>
          ) : null}
        </form>
      </details>

      <details className="mt-4 rounded-2xl border border-slate-200 p-5">
        <div className="flex items-baseline justify-between gap-3">
          <div>
            <summary className="cursor-pointer text-base font-bold text-slate-900">Notas preparadas e arquivos anteriores</summary>
            <p className="text-xs text-gray-600">Total: {total}</p>
            <p className="mt-1 max-w-lg text-xs text-amber-800">
              Os XMLs técnicos desta lista não substituem o XML processado (nfeProc)
              com protocolo genuíno da SEFAZ. Consulte a situação fiscal antes de
              entregar qualquer arquivo como nota autorizada.
            </p>
          </div>
        </div>
        <div className="mt-2 overflow-auto rounded-md border">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-gray-50 text-xs text-gray-600">
              <tr>
                <th className="px-3 py-2">Arquivo</th>

                <th className="px-3 py-2">Criado</th>
                <th className="px-3 py-2"></th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {items.map((x) => (
                <tr key={x.id}>
                  <td className="px-3 py-2 font-mono text-xs">{x.file_name}</td>

                  <td className="px-3 py-2 font-mono text-xs">{x.created_at}</td>
                  <td className="px-3 py-2">
                    <div className="flex gap-3">
                      <button type="button" onClick={() => void onDownload(x)} className="text-xs text-blue-700 hover:underline">
                        Baixar XML técnico
                      </button>
                      {x.file_name.startsWith('NFCe-') ? (
                        <button type="button" onClick={() => void onDANFE(x)} className="text-xs text-blue-700 hover:underline">
                          Imprimir DANFE
                        </button>
                      ) : null}
                    </div>
                  </td>
                </tr>
              ))}
              {items.length === 0 ? (
                <tr>
                  <td className="px-3 py-6 text-center text-sm text-gray-500" colSpan={3}>
                    Nenhum XML gerado.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </details>
    </div>
  )
}
