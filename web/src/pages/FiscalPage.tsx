import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { APIError, apiDownload, apiJson, errorMessage } from '../lib/api'

type XMLFile = {
  id: string
  invoice_id: string
  file_name: string
  sha256: string
  created_at: string
}

type XMLListResponse = { items: XMLFile[]; total: number }

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
  csc_secret_reference: 'Referência do CSC ausente',
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
  const [cscId, setCSCId] = useState('')
  const [cscSecretRef, setCSCSecretRef] = useState('')
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
    setCSCId(next?.csc_id ?? '')
    setCSCSecretRef('')
    setCertificateSecretRef('')
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
      setReadiness(readinessData)
      applyIssuer(issuerData)
      setItems(xmlData.items)
      setTotal(xmlData.total)
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
      setMessage('Dados fiscais do emitente atualizados.')
      const next = await apiJson<NFCeReadiness>('/api/v1/fiscal/nfce/readiness')
      setReadiness(next)
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
          csc_id: cscId.trim(),
          csc_secret_ref: cscSecretRef.trim(),
          certificate_secret_ref: certificateSecretRef.trim(),
        },
      })
      applyConfig(next)
      setMessage('Preparação NFC-e salva. A transmissão continua desativada.')
      const readinessData = await apiJson<NFCeReadiness>('/api/v1/fiscal/nfce/readiness')
      setReadiness(readinessData)
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

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Fiscal — preparação NFC-e</h2>
          <p className="text-sm text-gray-600">
            Modelo 65. Esta tela não transmite nem autoriza documentos na SEFAZ.
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
        <section className="mt-4 rounded-md border p-4">
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

          <div className="mt-3 grid gap-2 text-xs md:grid-cols-3">
            <div>Emitente: {readiness.issuer_identity_configured ? 'OK' : 'Pendente'}</div>
            <div>Endereço: {readiness.issuer_address_configured ? 'OK' : 'Pendente'}</div>
            <div>Município IBGE: {readiness.municipality_code_configured ? 'OK' : 'Pendente'}</div>
            <div>CSC ref.: {readiness.csc_reference_configured ? 'OK' : 'Pendente'}</div>
            <div>Certificado ref.: {readiness.certificate_reference_configured ? 'OK' : 'Pendente'}</div>
            <div>NCM ausente: {readiness.products_missing_ncm}</div>
          </div>

          {readiness.blocking_reasons.length > 0 ? (
            <ul className="mt-3 list-disc pl-5 text-xs text-amber-800">
              {readiness.blocking_reasons.map((reason) => (
                <li key={reason}>{blockerLabels[reason] ?? reason}</li>
              ))}
            </ul>
          ) : null}

          <div className="mt-3 rounded-md bg-gray-50 p-2 text-xs text-gray-700">
            Transmissão: <strong>desativada</strong>. Mesmo com todos os dados prontos, a emissão real
            só será liberada após provider SEFAZ, assinatura, schemas e homologação.
          </div>
        </section>
      ) : null}

      {issuer ? (
        <section className="mt-4 rounded-md border p-4">
          <h3 className="text-sm font-semibold">Emitente</h3>
          <p className="mt-1 text-xs text-gray-600">
            {issuer.legal_name} · CNPJ {issuer.cnpj}
          </p>
          <form onSubmit={saveIssuer} className="mt-3 grid grid-cols-1 gap-3 md:grid-cols-4">
            <label>
              <span className="text-xs text-gray-600">Inscrição estadual</span>
              <input value={ie} onChange={(e) => setIE(e.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 text-sm" disabled={!canPrepare} />
            </label>
            <label>
              <span className="text-xs text-gray-600">CRT</span>
              <select value={crt} onChange={(e) => setCRT(e.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 text-sm" disabled={!canPrepare}>
                <option value="">Selecione</option>
                <option value="1">1 — Simples Nacional</option>
                <option value="2">2 — Simples Nacional, excesso</option>
                <option value="3">3 — Regime Normal</option>
                <option value="4">4 — MEI</option>
              </select>
            </label>
            <label className="md:col-span-2">
              <span className="text-xs text-gray-600">Logradouro</span>
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
              <span className="text-xs text-gray-600">Código IBGE</span>
              <input value={cityCode} onChange={(e) => setCityCode(e.target.value)} inputMode="numeric" maxLength={7} className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm" disabled={!canPrepare} />
            </label>
            <label>
              <span className="text-xs text-gray-600">UF</span>
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
        </section>
      ) : null}

      <section className="mt-4 rounded-md border p-4">
        <h3 className="text-sm font-semibold">Configuração de homologação</h3>
        <p className="mt-1 text-xs text-gray-600">
          Informe referências do seu secret manager. Não cole o certificado PFX nem o CSC secreto.
          As referências são write-only nesta tela.
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
          <label>
            <span className="text-xs text-gray-600">ID do CSC</span>
            <input value={cscId} onChange={(e) => setCSCId(e.target.value)} className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm" disabled={!canPrepare} />
          </label>
          <label className="md:col-span-3">
            <span className="text-xs text-gray-600">Referência do secret do CSC</span>
            <input value={cscSecretRef} onChange={(e) => setCSCSecretRef(e.target.value)} placeholder={config?.csc_reference_configured ? 'Já configurado — informe novamente para salvar alterações' : 'Ex.: secret://nfce/csc'} className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm" disabled={!canPrepare} />
          </label>
          <label className="md:col-span-3">
            <span className="text-xs text-gray-600">Referência do certificado A1</span>
            <input value={certificateSecretRef} onChange={(e) => setCertificateSecretRef(e.target.value)} placeholder={config?.certificate_reference_configured ? 'Já configurado — informe novamente para salvar alterações' : 'Ex.: secret://nfce/certificate'} className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm" disabled={!canPrepare} />
          </label>
          {canPrepare ? (
            <div className="md:col-span-3">
              <button disabled={savingConfig} className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60">
                {savingConfig ? 'Salvando…' : 'Salvar preparação (transmissão permanece desativada)'}
              </button>
            </div>
          ) : null}
        </form>
      </section>

      <section className="mt-4">
        <div className="flex items-baseline justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold">XMLs históricos / previews</h3>
            <p className="text-xs text-gray-600">Total: {total}</p>
          </div>
        </div>
        <div className="mt-2 overflow-auto rounded-md border">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-gray-50 text-xs text-gray-600">
              <tr>
                <th className="px-3 py-2">Arquivo</th>
                <th className="px-3 py-2">SHA256</th>
                <th className="px-3 py-2">Criado</th>
                <th className="px-3 py-2"></th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {items.map((x) => (
                <tr key={x.id}>
                  <td className="px-3 py-2 font-mono text-xs">{x.file_name}</td>
                  <td className="px-3 py-2 font-mono text-xs">{x.sha256}</td>
                  <td className="px-3 py-2 font-mono text-xs">{x.created_at}</td>
                  <td className="px-3 py-2">
                    <button type="button" onClick={() => void onDownload(x)} className="text-xs text-blue-700 hover:underline">
                      Download
                    </button>
                  </td>
                </tr>
              ))}
              {items.length === 0 ? (
                <tr>
                  <td className="px-3 py-6 text-center text-sm text-gray-500" colSpan={4}>
                    Nenhum XML gerado.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  )
}
