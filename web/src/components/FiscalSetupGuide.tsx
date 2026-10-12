type GuideReadiness = {
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

type Props = {
  readiness: GuideReadiness | null
  onOpenIssuer: () => void
  onOpenCertificate: () => void
  onOpenReadiness: () => void
}

const officialLinks = {
  cnpj: 'https://www.gov.br/pt-br/servicos/consultar-cadastro-nacional-de-pessoas-juridicas',
  simples: 'https://www8.receita.fazenda.gov.br/SIMPLESNACIONAL/Servicos/Grupo.aspx?grp=9',
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

function GuideStep({
  number, title, status, children,
}: {
  number: number
  title: string
  status: string
  children: React.ReactNode
}) {
  return (
    <details className="group rounded-lg border border-slate-200 bg-white">
      <summary className="flex cursor-pointer list-none flex-wrap items-center gap-2 px-3 py-3 text-sm [&::-webkit-details-marker]:hidden">
        <span aria-hidden="true" className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-slate-100 font-semibold text-slate-700">
          {number}
        </span>
        <span className="min-w-40 flex-1 font-semibold text-slate-900">{title}</span>
        <span className="text-xs text-slate-600">{status}</span>
        <span aria-hidden="true" className="text-slate-500 group-open:rotate-180">⌄</span>
      </summary>
      <div className="space-y-3 border-t border-slate-100 px-4 py-3 text-sm leading-relaxed text-slate-700">
        {children}
      </div>
    </details>
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

// This guide is informational: it does not write credentials, infer fiscal
// regimes, persist manual checkmarks or claim government authorization.
export default function FiscalSetupGuide({
  readiness, onOpenIssuer, onOpenCertificate, onOpenReadiness,
}: Props) {
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

  return (
    <section aria-label="Guia de configuração fiscal MG"
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
        <div className="mt-4 space-y-2">
          <GuideStep number={1} title="Confirmar os dados e o regime da empresa"
            status={issuerFilled ? 'Campos básicos registrados; conferir' : 'Dados da loja por preencher/conferir'}>
            <p><strong>Separe:</strong> CNPJ, razão social, inscrição estadual (IE),
              endereço completo, CEP, município e código IBGE de sete dígitos, além do
              regime tributário/CRT confirmado pelo contador.</p>
            <p><strong>Onde consultar:</strong> CNPJ na Receita Federal, opção pelo
              Simples Nacional no portal da Receita e código municipal no IBGE.
              O comprovante de CNPJ ou o fato de não ser MEI não determina sozinho o CRT.</p>
            <div className="flex flex-wrap gap-x-4 gap-y-2">
              <OfficialLink href={officialLinks.cnpj}>Consultar CNPJ</OfficialLink>
              <OfficialLink href={officialLinks.simples}>Consultar Simples Nacional</OfficialLink>
              <OfficialLink href={officialLinks.municipios}>Código do município (IBGE)</OfficialLink>
            </div>
            <JumpButton onClick={onOpenIssuer}>Ir para dados da loja</JumpButton>
          </GuideStep>

          <GuideStep number={2} title="Conferir credenciamento da NFC-e na SEF/MG"
            status="Confirmação externa obrigatória">
            <p><strong>Acesso necessário:</strong> SIARE da SEF/MG, normalmente por CPF
              e senha do responsável autorizado ou pela opção de certificado digital.
              O contador ou representante com autorização pode orientar o procedimento.
              Esse login é feito no portal do governo, não no SistemaEmGo.</p>
            <p><strong>O que fazer:</strong> verificar se o CNPJ/IE está credenciado
              para emitir NFC-e modelo 65 e, se necessário, seguir as instruções
              oficiais de credenciamento. Confirme a situação antes de avançar.</p>
            <div className="flex flex-wrap gap-x-4 gap-y-2">
              <OfficialLink href={officialLinks.credenciamento}>Instruções da SEF/MG</OfficialLink>
              <OfficialLink href={officialLinks.siare}>Abrir SIARE (login)</OfficialLink>
            </div>
            <p className="text-xs text-slate-600">
              O sistema não consegue verificar sozinho se esse credenciamento foi realizado.
            </p>
          </GuideStep>

          <GuideStep number={3} title="Providenciar certificado digital e conferir CSC"
            status={referenceFilled ? 'Referência A1 cadastrada; não validada' : 'Certificado ainda não vinculado'}>
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
            status={productsFilled ? 'Cadastro básico presente; revisar tributos' : 'Conferir produtos e tributação'}>
            <p>Peça ao contador a classificação das operações e produtos:
              NCM, CEST quando aplicável, CFOP, ICMS (CST ou CSOSN conforme CRT),
              PIS/Cofins e campos IBS/CBS vigentes. Verifique também cartão,
              dinheiro, Pix, descontos, devoluções e dados do consumidor quando exigidos.</p>
            <p><strong>Onde obter:</strong> escrituração, cadastro fiscal dos produtos,
              notas de fornecedores e orientação contábil. Não invente códigos ou
              aplique um mesmo imposto a todos os produtos por conveniência.</p>
            <JumpButton onClick={onOpenReadiness}>Ver pendências detectadas no PDV</JumpButton>
          </GuideStep>

          <GuideStep number={5} title="Preparar homologação e numeração fiscal"
            status={configFilled ? 'Ambiente de testes configurado; validar' : 'Preparar ambiente de testes'}>
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
            <JumpButton onClick={onOpenCertificate}>Abrir configuração de homologação</JumpButton>
          </GuideStep>

          <GuideStep number={6} title="Validar autorização e só então iniciar operação"
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
