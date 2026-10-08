import { useCallback, useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'
import { buildSetupSteps, nextSetupAttention, summarizeSetupSteps, type FiscalReadinessData, type SetupStepKey, type SetupSnapshot, type SetupStatus } from '../lib/setupProgress'

type Me = { id: string; tenant_id: string; name: string; email: string; roles: string[]; permissions: string[] }
type Issuer = {
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
type ReviewStep = 'stock' | 'team'
type ReviewResponse = { items: Array<{ step: ReviewStep; reviewed_at: string }> }
type ReviewDates = Partial<Record<ReviewStep, string>>

function reviewDates(response: ReviewResponse): ReviewDates {
  const result: ReviewDates = {}
  for (const item of response.items) {
    if (item.step === 'stock' || item.step === 'team') result[item.step] = item.reviewed_at
  }
  return result
}

type IssuerFields = {
  ie: string
  crt: string
  address_street: string
  address_number: string
  address_complement: string
  address_neighborhood: string
  address_city: string
  address_city_code: string
  address_state: string
  address_zip: string
}

const emptyFields: IssuerFields = {
  ie: '', crt: '', address_street: '', address_number: '', address_complement: '',
  address_neighborhood: '', address_city: '', address_city_code: '', address_state: '',
  address_zip: '',
}

const statusLabels: Record<SetupStatus, string> = {
  ready: 'Dados preenchidos',
  pending: 'Precisa de atenção',
  review: 'Conferir',
  restricted: 'Sem acesso',
  unknown: 'Não foi possível verificar',
}

const blockers: Record<string, string> = {
  issuer_identity: 'Dados fiscais oficiais da loja',
  issuer_address: 'Endereço da loja',
  issuer_municipality_code: 'Código IBGE do município',
  nfce_config: 'Preparação da NFC-e',
  homologation_environment: 'Ambiente de homologação',
  certificate_secret_reference: 'Certificado digital A1 vinculado com segurança',
  active_products: 'Ao menos um produto ativo',
  product_ncm: 'NCM dos produtos',
  product_fiscal_profile: 'Classificação tributária dos produtos',
}

function issuerFields(issuer: Issuer): IssuerFields {
  return {
    ie: issuer.ie ?? '', crt: issuer.crt ?? '',
    address_street: issuer.address_street ?? '',
    address_number: issuer.address_number ?? '',
    address_complement: issuer.address_complement ?? '',
    address_neighborhood: issuer.address_neighborhood ?? '',
    address_city: issuer.address_city ?? '',
    address_city_code: issuer.address_city_code ?? '',
    address_state: issuer.address_state ?? '',
    address_zip: issuer.address_zip ?? '',
  }
}

export default function SetupPage() {
  const [me, setMe] = useState<Me | null>(null)
  const [productsTotal, setProductsTotal] = useState<number | null>(null)
  const [stockMovementsTotal, setStockMovementsTotal] = useState<number | null>(null)
  const [readiness, setReadiness] = useState<FiscalReadinessData | null>(null)
  const [issuer, setIssuer] = useState<Issuer | null>(null)
  const [reviews, setReviews] = useState<ReviewDates | null>(null)
  const [fields, setFields] = useState<IssuerFields>(emptyFields)
  const [active, setActive] = useState<SetupStepKey>('company')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [warnings, setWarnings] = useState<string[]>([])

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    setWarnings([])
    // Never show another session's previously loaded setup data.
    setMe(null)
    setProductsTotal(null)
    setStockMovementsTotal(null)
    setReadiness(null)
    setIssuer(null)
    setReviews(null)
    setFields(emptyFields)
    try {
      const user = await apiJson<Me>('/api/v1/auth/me')
      setMe(user)
      const permissions = new Set(user.permissions)
      const jobs: Promise<void>[] = []
      const problems: string[] = []
      if (permissions.has('product:read')) {
        jobs.push(apiJson<{ total: number }>('/api/v1/products?limit=1&offset=0')
          .then((response) => setProductsTotal(response.total))
          .catch((err: unknown) => { problems.push('Produtos: ' + errorMessage(err)) }))
      }
      if (permissions.has('inventory:read')) {
        jobs.push(apiJson<{ total: number }>('/api/v1/inventory/movements?limit=1&offset=0')
          .then((response) => setStockMovementsTotal(response.total))
          .catch((err: unknown) => { problems.push('Estoque: ' + errorMessage(err)) }))
      }
      if (permissions.has('invoice:generate')) {
        jobs.push(apiJson<ReviewResponse>('/api/v1/setup/reviews')
          .then((response) => setReviews(reviewDates(response)))
          .catch((err: unknown) => { problems.push('Revisões do assistente: ' + errorMessage(err)) }))
      }
      if (permissions.has('invoice:read')) {
        jobs.push(apiJson<FiscalReadinessData>('/api/v1/fiscal/nfce/readiness')
          .then(setReadiness)
          .catch((err: unknown) => { problems.push('NFC-e: ' + errorMessage(err)) }))
        jobs.push(apiJson<Issuer>('/api/v1/fiscal/nfce/issuer')
          .then((response) => { setIssuer(response); setFields(issuerFields(response)) })
          .catch((err: unknown) => { problems.push('Dados da empresa: ' + errorMessage(err)) }))
      }
      await Promise.all(jobs)
      setWarnings(problems)
    } catch (err: unknown) {
      setError('Não foi possível identificar a loja e suas permissões: ' + errorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  async function saveIssuer(event: FormEvent) {
    event.preventDefault()
    if (!me?.permissions.includes('invoice:generate') || !issuer || saving) return
    if (!window.confirm('Conferiu os dados fiscais da empresa com o contador?')) return
    setSaving(true)
    setError('')
    setMessage('')
    try {
      await apiJson<Issuer>('/api/v1/fiscal/nfce/issuer', {
        method: 'PUT',
        body: {
          ...fields,
          ie: fields.ie.trim(),
          crt: fields.crt,
          address_street: fields.address_street.trim(),
          address_number: fields.address_number.trim(),
          address_complement: fields.address_complement.trim() || null,
          address_neighborhood: fields.address_neighborhood.trim(),
          address_city: fields.address_city.trim(),
          address_city_code: fields.address_city_code.trim(),
          address_state: fields.address_state.trim().toUpperCase(),
          address_zip: fields.address_zip.replace(/\D/g, ''),
        },
      })
      await refresh()
      setMessage('Dados fiscais salvos para a loja atual. Confira as próximas etapas.')
    } catch (err: unknown) {
      setError('Não foi possível salvar os dados da loja: ' + errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  async function saveReview(step: ReviewStep, reviewed: boolean) {
    if (saving || !me?.permissions.includes('invoice:generate') || reviews === null) return
    const description = step === 'stock' ? 'orientações de estoque' : 'permissões e acesso da equipe'
    if (reviewed && !window.confirm(
      `Registrar que revisou as ${description}? Isso não confirma contagem física, cadastro de funcionários nem prontidão fiscal.`,
    )) return
    setSaving(true)
    setError('')
    setMessage('')
    try {
      const response = await apiJson<ReviewResponse>(`/api/v1/setup/reviews/${step}`, {
        method: 'PUT', body: { reviewed },
      })
      setReviews(reviewDates(response))
      setMessage(reviewed ? 'Revisão registrada no servidor para esta empresa.' : 'Revisão reaberta para esta empresa.')
    } catch (err: unknown) {
      setError('Não foi possível atualizar a revisão: ' + errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  const snapshot: SetupSnapshot | null = me ? {
    me, productsTotal, stockMovementsTotal, fiscalReadiness: readiness,
  } : null
  const steps = snapshot ? buildSetupSteps(snapshot) : []
  const overview = summarizeSetupSteps(steps)
  const activeIndex = steps.findIndex((step) => step.key === active)
  const nextAttention = nextSetupAttention(steps, active)
  const canSaveCompany = Boolean(me?.permissions.includes('invoice:generate') && issuer)
  const updateField = (key: keyof IssuerFields, value: string) => {
    setFields((previous) => ({ ...previous, [key]: value }))
  }

  return (
    <div>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Configurar minha loja</h2>
          <p className="mt-1 text-sm text-gray-600">
            Confira as etapas uma por uma. O sistema consulta os cadastros da empresa em que você está conectado.
          </p>
        </div>
        <button type="button" disabled={loading || saving} onClick={() => void refresh()}
          className="rounded-md border px-3 py-2 text-sm disabled:opacity-50">
          {loading ? 'Verificando…' : 'Conferir novamente'}
        </button>
      </div>
      {issuer ? (
        <p className="mt-3 text-sm font-medium">{issuer.trade_name || issuer.legal_name} · CNPJ {issuer.cnpj}</p>
      ) : null}
      <p className="mt-2 rounded-md border border-amber-200 p-3 text-xs text-amber-900">
        A preparação da loja não libera notas fiscais automaticamente. A NFC-e exige certificado, regras tributárias
        conferidas, homologação e autorização da SEFAZ. Cada CNPJ mantém dados e permissões separados.
      </p>
      {error ? <p role="alert" className="mt-3 rounded-md border border-red-200 p-3 text-sm text-red-700">{error}</p> : null}
      {message ? <p role="status" className="mt-3 rounded-md border border-green-200 p-3 text-sm text-green-700">{message}</p> : null}
      {warnings.length > 0 ? (
        <div role="alert" className="mt-3 rounded-md border border-amber-200 p-3 text-sm">
          <p className="font-medium">Algumas verificações não puderam ser concluídas:</p>
          <ul className="mt-1 list-disc pl-5">{warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>
        </div>
      ) : null}

      {me ? (
        <>
          <section aria-label="Resumo da configuração" className="mt-5 rounded-lg border bg-slate-50 p-3 text-sm">
            <p><strong>{overview.verified}</strong> etapa(s) com dados básicos verificados;
              {' '}<strong>{overview.attention}</strong> para preencher, verificar ou revisar;
              {' '}<strong>{overview.restricted}</strong> sem permissão de acesso.</p>
            <p className="mt-1 text-xs text-gray-600">
              Esta contagem não confirma estoque conferido, equipe revisada nem emissão fiscal liberada.
            </p>
            {nextAttention && nextAttention !== active ? (
              <button type="button" disabled={loading || saving}
                onClick={() => setActive(nextAttention)}
                className="mt-2 rounded-md border border-blue-300 bg-white px-3 py-2 text-sm font-medium text-blue-800 disabled:opacity-50">
                Ver outra etapa que precisa de atenção →
              </button>
            ) : null}
          </section>
          <div className="mt-3 grid gap-2 sm:grid-cols-2">
            {steps.map((step, index) => (
              <button type="button" key={step.key} disabled={loading || saving} onClick={() => setActive(step.key)}
                aria-current={active === step.key ? 'step' : undefined}
                className={`rounded-lg border p-3 text-left hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-500 ${active === step.key ? 'border-blue-500 bg-blue-50' : ''}`}>
                <span className="block text-sm font-semibold">{index + 1}. {step.title}</span>
                <span className="mt-1 block text-xs text-gray-600">{statusLabels[step.status]}</span>
              </button>
            ))}
          </div>
          <section className="mt-4 rounded-lg border p-4">
            <p className="text-sm text-gray-600">{steps.find((step) => step.key === active)?.detail}</p>

            {active === 'company' ? (
              issuer ? (
                <form className="mt-4 space-y-3" onSubmit={(event) => void saveIssuer(event)}>
                  <div className="rounded-md bg-gray-50 p-3 text-sm">
                    <p><strong>Razão social:</strong> {issuer.legal_name}</p>
                    <p><strong>CNPJ:</strong> {issuer.cnpj}</p>
                    <p className="mt-1 text-xs text-gray-600">
                      A razão social e o CNPJ são identificadores oficiais desta empresa; não podem ser alterados aqui.
                    </p>
                  </div>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <label className="text-sm">Inscrição estadual
                      <input className="mt-1 block w-full rounded-md border px-3 py-2" value={fields.ie}
                        disabled={!canSaveCompany} onChange={(e) => updateField('ie', e.target.value)} required />
                    </label>
                    <label className="text-sm">Regime tributário (confirme com o contador)
                      <select className="mt-1 block w-full rounded-md border px-3 py-2" value={fields.crt}
                        disabled={!canSaveCompany} onChange={(e) => updateField('crt', e.target.value)} required>
                        <option value="">Selecione</option>
                        <option value="1">Simples Nacional</option>
                        <option value="2">Simples Nacional (excesso)</option>
                        <option value="3">Regime normal</option>
                        <option value="4">MEI</option>
                      </select>
                    </label>
                    {([
                      ['address_street', 'Rua / logradouro'],
                      ['address_number', 'Número'],
                      ['address_complement', 'Complemento (opcional)'],
                      ['address_neighborhood', 'Bairro'],
                      ['address_city', 'Município'],
                      ['address_city_code', 'Código IBGE do município (7 números)'],
                      ['address_state', 'UF (2 letras)'],
                      ['address_zip', 'CEP (8 números)'],
                    ] as Array<[keyof IssuerFields, string]>).map(([key, label]) => (
                      <label key={key} className="text-sm">{label}
                        <input className="mt-1 block w-full rounded-md border px-3 py-2" value={fields[key]}
                          disabled={!canSaveCompany} required={key !== 'address_complement'}
                          inputMode={key === 'address_city_code' || key === 'address_zip' ? 'numeric' : undefined}
                          maxLength={key === 'address_city_code' ? 7 : key === 'address_state' ? 2 : key === 'address_zip' ? 9 : undefined}
                          pattern={key === 'address_city_code' ? '[0-9]{7}' : key === 'address_state' ? '[A-Z]{2}' : key === 'address_zip' ? '[0-9]{5}-?[0-9]{3}' : undefined}
                          title={key === 'address_city_code' ? 'Informe os 7 dígitos do código IBGE' : key === 'address_state' ? 'Informe duas letras da UF' : key === 'address_zip' ? 'CEP com 8 dígitos' : undefined}
                          onChange={(e) => updateField(key, key === 'address_state' ? e.target.value.toUpperCase() : e.target.value)} />
                      </label>
                    ))}
                  </div>
                  {canSaveCompany ? (
                    <button disabled={saving || loading} className="rounded-md bg-gray-900 px-4 py-2 text-sm text-white disabled:opacity-50">
                      {saving ? 'Salvando…' : 'Salvar dados da minha loja'}
                    </button>
                  ) : <p className="text-xs text-amber-800">A alteração exige permissão fiscal. Peça ajuda ao responsável da loja.</p>}
                </form>
              ) : (
                <p className="mt-4 text-sm">Os dados fiscais não estão disponíveis para sua conta ou a consulta falhou.
                  {me.permissions.includes('invoice:read') ? ' Tente conferir novamente.' : ' Solicite autorização ao responsável.'}
                </p>
              )
            ) : null}

            {active === 'products' ? (
              <div className="mt-4 space-y-3 text-sm">
                {productsTotal !== null ? <p><strong>{productsTotal}</strong> produto(s) cadastrados nesta empresa.</p> : null}
                <p>Você pode incluir um produto por vez ou preencher uma planilha com nome, código e preço.</p>
                {me.permissions.includes('product:read') ? (
                  <Link to="/products" className="inline-block rounded-md border px-3 py-2 font-medium text-blue-700">
                    Ir para Produtos →
                  </Link>
                ) : <p>Seu acesso não permite consultar o cadastro de produtos.</p>}
              </div>
            ) : null}

            {active === 'stock' ? (
              <div className="mt-4 space-y-3 text-sm">
                {stockMovementsTotal !== null ? <p>Movimentações registradas no histórico: <strong>{stockMovementsTotal}</strong>.</p> : null}
                <p>Conferir o estoque é diferente de cadastrar produtos. Conte fisicamente o que está na loja;
                  use a carga inicial apenas para produtos que nunca tiveram movimentações.</p>
                <p className="text-xs text-amber-800">
                  Ter movimentações no histórico não significa que toda a contagem da loja foi concluída.
                </p>
                {me.permissions.includes('inventory:read') ? (
                  <Link to="/inventory" className="inline-block rounded-md border px-3 py-2 font-medium text-blue-700">
                    Ir para Estoque →
                  </Link>
                ) : <p>Seu acesso não permite consultar o estoque.</p>}
              </div>
            ) : null}

            {active === 'team' ? (
              <div className="mt-4 space-y-3 text-sm">
                <p><strong>Seu acesso:</strong> {me.name} ({me.email})</p>
                <p><strong>Perfil:</strong> {me.roles.length ? me.roles.join(', ') : 'Sem perfil atribuído'}</p>
                <p><strong>Permissões atuais:</strong> {me.permissions.length}.</p>
                <p className="rounded-md bg-amber-50 p-3 text-amber-900">
                  O assistente ainda não cria funcionários nem altera permissões. Para evitar acesso indevido,
                  o cadastro de contas de outras pessoas precisa de uma área administrativa própria,
                  com confirmação e auditoria. Nunca compartilhe uma única senha entre operadores.
                </p>
              </div>
            ) : null}

            {active === 'fiscal' ? (
              <div className="mt-4 space-y-3 text-sm">
                {readiness ? (
                  <>
                    <p><strong>Dados básicos de homologação:</strong> {readiness.ready_for_homologation_data ? 'preenchidos para avaliação' : 'incompletos'}.</p>
                    <p><strong>Certificado A1:</strong> {readiness.certificate_reference_configured ? 'referência cadastrada (validade não verificada aqui)' : 'ainda não vinculado'}.</p>
                    {readiness.blocking_reasons.length > 0 ? (
                      <div>
                        <p className="font-medium">Pendências informadas pelo servidor:</p>
                        <ul className="mt-1 list-disc pl-5">
                          {readiness.blocking_reasons.map((code) => <li key={code}>{blockers[code] ?? 'Verificação fiscal: ' + code}</li>)}
                        </ul>
                      </div>
                    ) : <p className="text-amber-800">Não há bloqueios de dados relatados. Isso não confirma homologação nem autorização de emissão.</p>}
                  </>
                ) : <p>Não foi possível consultar a preparação fiscal ou você não tem essa permissão.</p>}
                {me.permissions.includes('invoice:read') ? (
                  <Link to="/fiscal" className="inline-block rounded-md border px-3 py-2 font-medium text-blue-700">
                    Abrir preparação da NFC-e →
                  </Link>
                ) : null}
                <p className="text-xs text-amber-800">
                  Não envie senha, PFX nem chave privada por esta tela. A emissão exige homologação e confirmação da SEFAZ.
                </p>
              </div>
            ) : null}
          </section>
          {(active === 'stock' || active === 'team') && me.permissions.includes('invoice:generate') ? (
            <section aria-label="Revisão registrada da etapa" className="mt-3 rounded-lg border p-3 text-sm">
              <h3 className="font-semibold">Registro de revisão desta empresa</h3>
              <p className="mt-1 text-xs text-gray-600">
                Esta anotação acompanha o processo, mas não certifica estoque contado, equipe cadastrada
                ou emissão fiscal autorizada. A etapa continuará exigindo conferência real.
              </p>
              {reviews === null ? (
                <p className="mt-2 text-amber-800">Não foi possível consultar o registro; não é permitido confirmar sem conexão.</p>
              ) : (
                <>
                  <p className="mt-2">
                    {reviews[active] ? `Revisão registrada em ${new Date(reviews[active]).toLocaleString('pt-BR')}.` : 'Ainda não há revisão registrada.'}
                  </p>
                  <button type="button" disabled={loading || saving}
                    onClick={() => void saveReview(active, !reviews[active])}
                    className="mt-2 rounded-md border px-3 py-2 font-medium disabled:opacity-50">
                    {saving ? 'Salvando…' : reviews[active] ? 'Reabrir revisão' : 'Registrar revisão'}
                  </button>
                </>
              )}
            </section>
          ) : null}
          <nav aria-label="Navegação do assistente" className="mt-4 flex flex-wrap items-center justify-between gap-2">
            <button type="button" disabled={loading || saving || activeIndex <= 0}
              onClick={() => setActive(steps[activeIndex - 1].key)}
              className="rounded-md border px-3 py-2 text-sm disabled:opacity-50">← Etapa anterior</button>
            <span className="text-xs text-gray-600">Etapa {activeIndex + 1} de {steps.length}</span>
            <button type="button" disabled={loading || saving || activeIndex < 0 || activeIndex >= steps.length - 1}
              onClick={() => setActive(steps[activeIndex + 1].key)}
              className="rounded-md border px-3 py-2 text-sm disabled:opacity-50">Próxima etapa →</button>
          </nav>
          <p className="mt-3 text-xs text-gray-500">
            As etapas são recalculadas a partir de dados do servidor quando você clica em “Conferir novamente”.
            Este assistente não armazena certificado, senha, documento fiscal nem dados de outra empresa.
          </p>
        </>
      ) : loading ? <p className="mt-4 text-sm">Consultando sua loja…</p> : null}
    </div>
  )
}
