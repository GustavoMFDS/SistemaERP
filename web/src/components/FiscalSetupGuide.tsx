import { useEffect, useRef, useState } from 'react'
import { apiJson, errorMessage } from '../lib/api'
import type { ReactNode } from 'react'

type GuideReadiness = {
  tenant_id: string
  issuer_identity_configured: boolean
  issuer_address_configured: boolean
  municipality_code_configured: boolean
  config_exists: boolean
  environment?: string
  certificate_reference_configured: boolean
  active_products: number
  products_missing_ncm: number
  products_missing_fiscal_profile?: number
  blocking_reasons: string[]
}

type GuideIssuer = {
  legal_name: string
  cnpj: string
  ie: string
  crt: string
  address_street: string
  address_number: string
  address_city: string
  address_city_code: string
  address_state: string
  address_zip: string
}

type GuideConfig = {
  environment: string
  series: number
  certificate_reference_configured: boolean
}

type Props = {
  readiness: GuideReadiness | null
  issuer: GuideIssuer | null
  config: GuideConfig | null
  onOpenIssuer: () => void
  onOpenCertificate: () => void
  onOpenReadiness: () => void
  canReview: boolean
}

const officialLinks = {
  cnpj: 'https://www.gov.br/pt-br/servicos/consultar-cadastro-nacional-de-pessoas-juridicas',
  simples: 'https://www8.receita.fazenda.gov.br/SIMPLESNACIONAL/Servicos/Grupo.aspx?grp=9',
  contribuinteMG: 'https://portalsped.fazenda.mg.gov.br/spedmg/nfe/Consulta-Cadastro-de-Contribuintes/',
  municipios: 'https://www.ibge.gov.br/explica/codigos-dos-municipios.php',
  credenciamento: 'https://portalsped.fazenda.mg.gov.br/spedmg/nfce/credenciamento/index.html',
  siare: 'https://siare.fazenda.mg.gov.br/sicaf/',
  perguntas: 'https://portalsped.fazenda.mg.gov.br/spedmg/nfce/Perguntas-Frequentes/respostas_i/index.html',
  homologacao: 'https://portalsped.fazenda.mg.gov.br/spedmg/nfce/web-services/',
  consulta: 'https://portalsped.fazenda.mg.gov.br/spedmg/nfce/consultas/',
  nfe: 'https://portalsped.fazenda.mg.gov.br/spedmg/nfe/credenciamento/',
}

function OfficialLink({ href, children }: { href: string; children: string }) {
  return (
    <a href={href} target="_blank" rel="noopener noreferrer"
      className="font-medium text-blue-700 underline underline-offset-2 hover:text-blue-900">
      {children} ↗
    </a>
  )
}

type FiscalManualReview =
  | 'fiscal_docs_collected'
  | 'fiscal_siare_access_arranged'
  | 'fiscal_accountant_consulted'
  | 'fiscal_previous_series_checked'

type ReviewResponse = { items: Array<{ step: string; reviewed_at: string }> }
type StoredReviews = {
  tenantID: string
  steps: FiscalManualReview[]
}

const manualReviewSteps: FiscalManualReview[] = [
  'fiscal_docs_collected',
  'fiscal_siare_access_arranged',
  'fiscal_accountant_consulted',
  'fiscal_previous_series_checked',
]

function parseManualReviews(tenantID: string, response: ReviewResponse): StoredReviews {
  return {
    tenantID,
    steps: response.items
      .filter((item) => manualReviewSteps.includes(item.step as FiscalManualReview))
      .map((item) => item.step as FiscalManualReview),
  }
}

function ManualChecklistTask({
  title, step, reviewed, available, saving, onToggle,
}: {
  title: string
  step: FiscalManualReview
  reviewed: boolean
  available: boolean
  saving: boolean
  onToggle: (step: FiscalManualReview) => void
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-slate-200 bg-slate-50 p-3">
      <div className="flex-1">
        <p className="font-medium text-slate-900">{title}</p>
        <p className="text-xs text-slate-600">
          {reviewed
            ? 'Marcação manual registrada nesta empresa; não é comprovação perante a SEF/MG.'
            : 'Tarefa de organização ainda não marcada nesta empresa.'}
        </p>
      </div>
      <button type="button" disabled={!available || saving}
        aria-pressed={reviewed}
        onClick={() => onToggle(step)}
        className="rounded-md border border-slate-300 bg-white px-3 py-2 text-xs font-medium text-slate-800 disabled:opacity-50">
        {saving ? 'Salvando…' : reviewed ? 'Reabrir tarefa' : 'Marcar tarefa organizada'}
      </button>
    </div>
  )
}

function GuideStep({
  number, title, status, active, onSelect, children,
}: {
  number: number
  title: string
  status: string
  active: boolean
  onSelect: (step: number) => void
  children: ReactNode
}) {
  return (
    <div className={active
      ? 'rounded-lg border-2 border-slate-500 bg-white'
      : 'rounded-lg border border-slate-200 bg-white'}>
      <button type="button" onClick={() => onSelect(number)}
        aria-expanded={active}
        aria-controls={'fiscal-guide-panel-' + number}
        id={'fiscal-guide-tab-' + number}
        className="flex w-full flex-wrap items-center gap-2 rounded-lg px-3 py-3 text-left text-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-700">
        <span aria-hidden="true" className={active
          ? 'flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-slate-800 font-semibold text-white'
          : 'flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-slate-100 font-semibold text-slate-700'}>
          {number}
        </span>
        <span className="min-w-40 flex-1 font-semibold text-slate-900">{title}</span>
        <span className="text-xs text-slate-600">{status}</span>
        <span aria-hidden="true" className="text-slate-500">{active ? '⌃' : '⌄'}</span>
      </button>
      {active ? (
        <div id={'fiscal-guide-panel-' + number} role="region"
          aria-labelledby={'fiscal-guide-tab-' + number}
          className="space-y-3 border-t border-slate-100 px-4 py-3 text-sm leading-relaxed text-slate-700">
          {children}
        </div>
      ) : null}
    </div>
  )
}

function JumpButton({ onClick, children }: { onClick: () => void; children: string }) {
  return (
    <button type="button" onClick={onClick}
      className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-800 hover:bg-slate-50">
      {children}
    </button>
  )
}

// This guide never writes credentials, infers CRT or claims official
// approval. Manual checklist acknowledgements use the existing audited,
// tenant-scoped setup review API and cannot enable transmission.
export default function FiscalSetupGuide({
  readiness, issuer, config, onOpenIssuer, onOpenCertificate, onOpenReadiness, canReview,
}: Props) {
  const [activeStep, setActiveStep] = useState(1)
  const [storedReviews, setStoredReviews] = useState<StoredReviews | null>(null)
  const [reviewError, setReviewError] = useState('')
  const [savingReview, setSavingReview] = useState<FiscalManualReview | null>(null)
  const tenantID = readiness?.tenant_id ?? ''
  const tenantRef = useRef(tenantID)
  tenantRef.current = tenantID

  useEffect(() => {
    let active = true
    if (tenantID && canReview) {
      apiJson<ReviewResponse>('/api/v1/setup/reviews')
        .then((response) => {
          if (!active) return
          setStoredReviews(parseManualReviews(tenantID, response))
          setReviewError('')
        })
        .catch((error: unknown) => {
          if (active) setReviewError('Não foi possível recuperar as tarefas desta empresa: ' + errorMessage(error))
        })
    }
    return () => { active = false }
  }, [tenantID, canReview])

  const currentReviews = storedReviews?.tenantID === tenantID ? storedReviews.steps : []
  const canSaveReview = Boolean(canReview && tenantID && storedReviews?.tenantID === tenantID)
  async function toggleReview(step: FiscalManualReview) {
    if (!canSaveReview || savingReview || !manualReviewSteps.includes(step)) return
    const reviewed = !currentReviews.includes(step)
    if (reviewed && !window.confirm(
      'Registrar somente que a tarefa de organização foi realizada? ' +
      'Isso não confirma credenciamento, homologação, validade de dados fiscais ou autorização de notas.',
    )) return
    setSavingReview(step)
    setReviewError('')
    try {
      const response = await apiJson<ReviewResponse>('/api/v1/setup/reviews/' + step, {
        method: 'PUT', body: { reviewed },
      })
      if (tenantRef.current === tenantID) {
        setStoredReviews(parseManualReviews(tenantID, response))
      }
    } catch (error: unknown) {
      if (tenantRef.current === tenantID) {
        setReviewError('Não foi possível salvar a tarefa: ' + errorMessage(error))
      }
    } finally {
      setSavingReview(null)
    }
  }

  const manualTask = (step: FiscalManualReview, title: string) => (
    <ManualChecklistTask
      step={step} title={title}
      reviewed={currentReviews.includes(step)}
      available={canSaveReview}
      saving={savingReview === step}
      onToggle={(value) => { void toggleReview(value) }}
    />
  )
  const reasons = readiness?.blocking_reasons ?? []
  const issuerFilled = Boolean(
    readiness?.issuer_identity_configured &&
    readiness.issuer_address_configured &&
    readiness.municipality_code_configured &&
    !reasons.includes('issuer_uf_unsupported') &&
    !reasons.includes('issuer_municipality_uf_mismatch'),
  )
  const productsFilled = Boolean(readiness &&
    readiness.active_products > 0 &&
    readiness.products_missing_ncm === 0 &&
    readiness.products_missing_fiscal_profile === 0)
  const referenceFilled = Boolean(readiness?.certificate_reference_configured)
  const configFilled = Boolean(readiness?.config_exists &&
    readiness.environment === 'homologation')
  // Values are read-only reflections of the authenticated tenant's existing
  // issuer/config endpoints. Never fetch tax status, infer a CRT, or send a secret.
  const savedIssuerFields = [
    { label: 'Razão social', value: issuer?.legal_name },
    { label: 'CNPJ', value: issuer?.cnpj },
    { label: 'Inscrição estadual', value: issuer?.ie },
    { label: 'Regime (CRT)', value: issuer?.crt ? 'CRT ' + issuer.crt : '' },
    { label: 'Endereço', value: issuer
      ? [issuer.address_street, issuer.address_number, issuer.address_city, issuer.address_state]
        .filter(Boolean).join(', ')
      : '' },
    { label: 'CEP', value: issuer?.address_zip },
    { label: 'Código IBGE', value: issuer?.address_city_code },
  ]

  function goToStep(step: number) {
    if (step < 1 || step > 6) return
    setActiveStep(step)
    document.getElementById('fiscal-setup-guide')?.scrollIntoView({
      behavior: 'smooth', block: 'start',
    })
  }

  return (
    <section id="fiscal-setup-guide" aria-label="Guia de configuração fiscal MG"
      className="mt-5 rounded-xl border border-slate-200 bg-slate-50 p-4 sm:p-5">
      <details open>
        <summary className="cursor-pointer text-base font-bold text-slate-900">
          Guia passo a passo: emissão fiscal em Minas Gerais
        </summary>
        <p className="mt-2 text-sm text-slate-700">
          Siga as etapas na ordem. Os avisos indicam apenas se existem dados básicos no PDV,
          não se a SEF/MG aprovou o credenciamento, o certificado, os impostos ou uma nota.
          Não é necessário saber agora qual emissor fiscal a loja utilizava antes.
        </p>
        <p className="mt-2 text-sm font-medium text-amber-800">
          A transmissão permanece desativada. Não digite senha do SIARE, senha do
          certificado, token CSC ou chave privada nesta tela.
        </p>
        <div className="mt-3 rounded-lg border border-slate-200 bg-white p-3 text-sm">
          <p className="font-semibold">Tarefas organizacionais salvas por empresa</p>
          <p className="mt-1 text-xs text-slate-700">
            {canReview
              ? canSaveReview
                ? currentReviews.length + ' de 4 tarefas marcadas manualmente'
                : 'Consultando tarefas da empresa…'
              : 'Você pode ler o guia; só responsáveis com permissão fiscal podem marcar tarefas.'}
            {' '}Essas marcações não validam notas, credenciamento nem homologação.
          </p>
          {reviewError ? <p role="alert" className="mt-2 text-xs text-red-700">{reviewError}</p> : null}
        </div>
        <div className="mt-4 space-y-3">
          <div className="rounded-lg border border-slate-200 bg-white p-3" role="status"
            aria-live="polite" aria-atomic="true">
            <p className="text-sm font-semibold text-slate-900">Etapa {activeStep} de 6</p>
            <p className="text-xs text-slate-600">
              Indicador de navegação, não de aprovação fiscal. Pode voltar e avançar sem
              confirmar dados ou acessar portais externos.
            </p>
            <div aria-hidden="true" className="mt-2 h-2 overflow-hidden rounded-full bg-slate-100">
              <div className="h-full rounded-full bg-slate-700 transition-all"
                style={{ width: (activeStep / 6 * 100) + '%' }} />
            </div>
          </div>
          <GuideStep number={1} title="Confirmar os dados e o regime da empresa"
            active={activeStep === 1} onSelect={goToStep}
            status={issuerFilled ? 'Campos básicos registrados; conferir' : 'Dados da loja por preencher/conferir'}>
            <div className="rounded-md border border-slate-200 bg-slate-50 p-3">
              <p className="font-semibold text-slate-900">
                Dados já cadastrados nesta loja (preenchimento automático)
              </p>
              <p className="mb-2 text-xs text-slate-600">
                Os dados são carregados do cadastro do próprio PDV e já aparecem no formulário
                de edição. Confira no documento oficial; não são consultas à Receita ou à SEF/MG.
              </p>
              <dl className="grid gap-2 sm:grid-cols-2">
                {savedIssuerFields.map((field) => (
                  <div key={field.label} className="min-w-0">
                    <dt className="text-xs font-semibold text-slate-600">{field.label}</dt>
                    <dd className="break-words text-sm text-slate-900">
                      {field.value?.trim() || 'Não registrado no PDV'}
                    </dd>
                  </div>
                ))}
              </dl>
              {!issuer ? (
                <p className="mt-2 text-xs text-amber-800">
                  Não foi possível mostrar os dados da loja. Confira o carregamento da página.
                </p>
              ) : null}
            </div>
            <p><strong>Separe:</strong> CNPJ, razão social, inscrição estadual (IE),
              endereço completo, CEP, município e código IBGE de sete dígitos, além do
              regime tributário/CRT confirmado pelo contador.</p>
            <p><strong>Onde consultar:</strong> CNPJ na Receita Federal, opção pelo
              Simples Nacional no portal da Receita, IE no cadastro de contribuintes da SEF/MG e código municipal no IBGE.
              O comprovante de CNPJ ou o fato de não ser MEI não determina sozinho o CRT.</p>
            <div className="flex flex-wrap gap-x-4 gap-y-2">
              <OfficialLink href={officialLinks.cnpj}>Consultar CNPJ</OfficialLink>
              <OfficialLink href={officialLinks.simples}>Consultar Simples Nacional</OfficialLink>
              <OfficialLink href={officialLinks.contribuinteMG}>Consultar IE em MG</OfficialLink>
              <OfficialLink href={officialLinks.municipios}>Código do município (IBGE)</OfficialLink>
            </div>
            {manualTask('fiscal_docs_collected', 'Reuni os documentos e informações cadastrais da loja')}
            <JumpButton onClick={onOpenIssuer}>Ir para dados da loja</JumpButton>
          </GuideStep>

          <GuideStep number={2} title="Conferir credenciamento da NFC-e na SEF/MG"
            active={activeStep === 2} onSelect={goToStep}
            status="Confirmação externa obrigatória">
            <p><strong>Acesso necessário:</strong> SIARE da SEF/MG. Na opção de login,
              selecione o tipo de usuário adequado (por exemplo, Inscrição Estadual),
              informe os dados solicitados da IE, o CPF do responsável autorizado e a senha
              pessoal; também existe acesso por certificado digital. O contador ou
              representante autorizado pode orientar o procedimento.
              Esse login é feito no portal do governo, não no SistemaEmGo.</p>
            <p><strong>O que fazer:</strong> verificar se o CNPJ/IE está credenciado
              para emitir NFC-e modelo 65 e, se necessário, seguir as instruções
              oficiais de credenciamento. Confirme a situação antes de avançar.</p>
            <div className="flex flex-wrap gap-x-4 gap-y-2">
              <OfficialLink href={officialLinks.credenciamento}>Instruções da SEF/MG</OfficialLink>
              <OfficialLink href={officialLinks.siare}>Abrir SIARE (login)</OfficialLink>
            </div>
            {manualTask('fiscal_siare_access_arranged', 'Organizei o acesso do responsável ao portal SIARE')}
            <p className="text-xs text-slate-600">
              O sistema não consegue verificar sozinho se esse credenciamento foi realizado.
            </p>
          </GuideStep>

          <GuideStep number={3} title="Providenciar certificado digital e conferir CSC"
            active={activeStep === 3} onSelect={goToStep}
            status={referenceFilled ? 'Referência A1 cadastrada; não validada' : 'Certificado ainda não vinculado'}>
            <p className="rounded-md bg-slate-50 p-2 text-xs">
              No PDV: {referenceFilled
                ? 'há uma referência de certificado vinculada; validade e titular não verificados.'
                : 'a referência do certificado ainda não foi cadastrada.'}
            </p>
            <p><strong>Certificado:</strong> peça ao responsável da empresa ou contador
              um certificado ICP-Brasil válido que contenha o CNPJ do emitente.
              Para a integração automática atual, um técnico precisa instalar o A1
              com segurança no servidor e informar ao PDV somente sua referência segura.</p>
            <p><strong>CSC / ID Token:</strong> a SEF/MG informa esses dados no
              credenciamento. Para consultá-los, entre no SIARE e siga
              <strong> Home → Documentos Eletrônicos → Manutenção Cadastro Emissor</strong>;
              pesquise pela IE e selecione Atualizar. A necessidade de usar o CSC
              no emissor depende do leiaute do QR Code aplicável: o QR Code v3
              dispensa CSC, enquanto versões anteriores podem utilizá-lo.</p>
            <p className="text-xs text-amber-800">
              Nunca cole o arquivo PFX, a senha, o CSC ou uma chave privada no guia,
              em mensagens de suporte ou em campos de referência.
            </p>
            <div className="flex flex-wrap gap-x-4 gap-y-2">
              <OfficialLink href={officialLinks.perguntas}>Perguntas frequentes SEF/MG</OfficialLink>
              <OfficialLink href={officialLinks.siare}>Consultar no SIARE</OfficialLink>
            </div>
            <JumpButton onClick={onOpenCertificate}>Ir para certificado e configurações</JumpButton>
          </GuideStep>

          <GuideStep number={4} title="Revisar produtos, impostos e formas de pagamento"
            active={activeStep === 4} onSelect={goToStep}
            status={productsFilled ? 'Cadastro básico presente; revisar tributos' : 'Conferir produtos e tributação'}>
            <p className="rounded-md bg-slate-50 p-2 text-xs">
              Estoque no PDV: {readiness
                ? readiness.active_products + ' produtos ativos; ' +
                  readiness.products_missing_ncm + ' sem NCM; ' +
                  (readiness.products_missing_fiscal_profile ?? 'desconhecido') +
                  ' sem perfil fiscal. Isso não atesta classificação tributária.'
                : 'dados ainda não carregados.'}
            </p>
            <p>Peça ao contador a classificação das operações e produtos:
              NCM, CEST quando aplicável, CFOP, ICMS (CST ou CSOSN conforme CRT),
              PIS/Cofins e campos IBS/CBS vigentes. Verifique também cartão,
              dinheiro, Pix, descontos, devoluções e dados do consumidor quando exigidos.</p>
            <p><strong>Onde obter:</strong> escrituração, cadastro fiscal dos produtos,
              notas de fornecedores e orientação contábil. Não invente códigos ou
              aplique um mesmo imposto a todos os produtos por conveniência.</p>
            {manualTask('fiscal_accountant_consulted', 'Solicitei ao contador a revisão das informações tributárias')}
            <JumpButton onClick={onOpenReadiness}>Ver pendências detectadas no PDV</JumpButton>
          </GuideStep>

          <GuideStep number={5} title="Preparar homologação e numeração fiscal"
            active={activeStep === 5} onSelect={goToStep}
            status={configFilled ? 'Ambiente de testes configurado; validar' : 'Preparar ambiente de testes'}>
            <p className="rounded-md bg-slate-50 p-2 text-xs">
              Configuração salva no PDV: {config
                ? (config.environment === 'homologation' ? 'homologação' :
                  config.environment === 'production' ? 'produção' : 'ambiente desconhecido') +
                  '; série ' + config.series + '. Apenas registrada, não homologada.'
                : 'não cadastrada.'}
            </p>
            <p>Na seção de certificado, selecione <strong>Homologação</strong> e confirme
              série e numeração com o contador. Antes da primeira emissão real,
              investigue se a loja já emite NFC-e em outro sistema para não
              duplicar documentos. Não assuma que o emissor antigo era ECF,
              talonário ou NFC-e.</p>
            <p>O técnico deve validar certificado, endpoints e respostas reais da SEF/MG
              no ambiente correto. Salvar a configuração não liga transmissão.</p>
            <div className="flex flex-wrap gap-x-4 gap-y-2">
              <OfficialLink href={officialLinks.homologacao}>Serviços oficiais de MG</OfficialLink>
              <OfficialLink href={officialLinks.nfe}>NF-e modelo 55: credenciamento separado</OfficialLink>
            </div>
            {manualTask('fiscal_previous_series_checked', 'Conferi com o responsável a numeração e a série já utilizadas')}
            <JumpButton onClick={onOpenCertificate}>Abrir configuração de homologação</JumpButton>
          </GuideStep>

          <GuideStep number={6} title="Validar autorização e só então iniciar operação"
            active={activeStep === 6} onSelect={goToStep}
            status="Homologação real ainda pendente">
            <p>Com credenciamento e dados reais confirmados, a equipe técnica precisa
              testar assinatura, transmissão, consulta de resultado, rejeições, recuperação
              após falha e armazenamento do XML processado com protocolo. A verificação
              oficial de uma nota autorizada pode ser feita no portal de consultas MG.</p>
            <p>Na operação futura, cada venda deverá gerar sua obrigação fiscal.
              O cliente poderá escolher imprimir ou não o DANFE na térmica,
              mas essa escolha nunca cancela a emissão. A NF-e modelo 55
              segue um fluxo próprio, ainda não liberado neste sistema.</p>
            <OfficialLink href={officialLinks.consulta}>Consultas oficiais NFC-e MG</OfficialLink>
          </GuideStep>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-200 pt-3">
            <button type="button" onClick={() => goToStep(activeStep - 1)}
              disabled={activeStep === 1}
              className="rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-medium disabled:opacity-40">
              Voltar etapa
            </button>
            {activeStep < 6 ? (
              <button type="button" onClick={() => goToStep(activeStep + 1)}
                className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white">
                Próxima etapa
              </button>
            ) : (
              <button type="button" onClick={() => goToStep(1)}
                className="rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-medium">
                Revisar desde o início
              </button>
            )}
          </div>
        </div>
        <p className="mt-4 text-xs text-slate-600">
          Guia orientativo baseado nos canais oficiais da SEF/MG, Receita Federal e IBGE.
          Procedimentos e notas técnicas podem mudar; confirme com o contador antes de
          habilitar uma empresa. Nenhuma etapa desta página concede autorização fiscal.
        </p>
      </details>
    </section>
  )
}
