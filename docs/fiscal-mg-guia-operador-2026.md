# Guia de configuração fiscal na interface (MG) — 12/10/2026

## Experiência implementada

A página **Fiscal → Nota fiscal da loja** apresenta um `FiscalSetupGuide` com seis passos recolhíveis, conteúdo acessível por teclado e atalhos para abrir as seções existentes de cadastro do emitente, certificado e pendências. URLs externas abrem em nova aba, usando `noopener noreferrer`. A opção de imprimir permanece separada da autorização.

Não há persistência de checkboxes nem campo para usuário/senha do governo. Os indicadores são **somente de preenchimento local** e usam o `readiness` já existente para mostrar "dados básicos registrados; conferir", "referência A1 cadastrada; não validada" e similares. Credenciamento da SEF/MG, enquadramento tributário, numeração/série inicial e homologação oficial continuam exigindo confirmação externa. Nenhuma indicação representa autorização de nota ou ativação da transmissão.

## Como o operador segue o guia

1. Reúne CNPJ e dados cadastrais por consulta pública da Receita; verifica IE via cadastro de contribuintes MG; consulta opção pelo Simples Nacional e código de município no IBGE; pede CRT e regras aplicáveis ao contador. O sistema não infere CRT do fato de a empresa não ser MEI.
2. O responsável autorizado acessa o **SIARE**, pela opção de login do tipo de usuário aplicável (por exemplo, Inscrição Estadual, CPF do responsável e senha pessoal) ou por certificado digital. O responsável ou contador autorizado confere/solicita credenciamento de NFC-e modelo 65. **Esse login não ocorre no PDV.**
3. O emissor providencia certificado ICP-Brasil vinculado ao CNPJ e solicita provisionamento seguro de A1 no servidor; a interface armazena somente referência fornecida pelo técnico. CSC e ID Token podem ser consultados no SIARE: `HOME > DOCUMENTOS ELETRÔNICOS > MANUTENÇÃO CADASTRO EMISSOR`, pesquisar pela IE e selecionar Atualizar. O QR Code versão 3 dispensa CSC segundo NT 2025.001, mas a SEF/MG ainda descreve fornecimento do CSC em seu credenciamento e as versões anteriores podem usá-lo. Não pedir ao operador que exponha o segredo em campo de referência.
4. Contador revisa NCM, CEST quando aplicável, CFOP, ICMS CST/CSOSN, PIS/COFINS, IBS/CBS, meios de pagamento e fluxo de devolução. Os contadores locais de produto com perfil e NCM são **checagem de cadastro**, não classificação tributária certificada.
5. Técnico configura apenas o **ambiente de homologação** e confirma série/numeração e se houve emissão anterior de NFC-e noutro software; modelo fiscal anterior não é assumido. O formulário existente continua sem ligar transmissão.
6. Depois de credenciamento, dados reais e disponibilidade do executor fiscal: validar comunicação/consulta com SEF/MG, assinatura, rejeições, timeout, nfeProc e impressão térmica opcional. NF-e modelo 55 mantém credenciamento e processo separados.

## Fontes oficiais

- [Receita Federal — CNPJ](https://www.gov.br/pt-br/servicos/consultar-cadastro-nacional-de-pessoas-juridicas)
- [Receita Federal — consulta optantes Simples](https://www8.receita.fazenda.gov.br/SIMPLESNACIONAL/Servicos/Grupo.aspx?grp=9)
- [Cadastro de contribuintes / IE MG](https://portalsped.fazenda.mg.gov.br/spedmg/nfe/Consulta-Cadastro-de-Contribuintes/)
- [Códigos de municípios IBGE](https://www.ibge.gov.br/explica/codigos-dos-municipios.php)
- [SEF/MG — credenciamento NFC-e](https://portalsped.fazenda.mg.gov.br/spedmg/nfce/credenciamento/index.html)
- [SEF/MG — SIARE](https://siare.fazenda.mg.gov.br/sicaf/)
- [SEF/MG — dúvidas sobre certificado, CSC e ID Token](https://portalsped.fazenda.mg.gov.br/spedmg/nfce/Perguntas-Frequentes/respostas_i/index.html)
- [SEF/MG — endpoints oficiais](https://portalsped.fazenda.mg.gov.br/spedmg/nfce/web-services/)
- [Portal Nacional NF-e — NT 2025.001 v1.03 (QR Code v3)](https://www.nfe.fazenda.gov.br/Portal/exibirArquivo.aspx?conteudo=NvuzQGYd6E8%3D)
- [SEF/MG — credenciamento NF-e 55](https://portalsped.fazenda.mg.gov.br/spedmg/nfe/credenciamento/)

## Regras de segurança e limites

- **Nunca** solicitar/gravar login e senha SIARE, senha PFX/A1, conteúdo PFX, chave privada ou CSC dentro do guia ou por suporte informal.
- Um certificado/CSC referenciado não confirma identidade criptográfica nem credenciamento.
- A UI é somente educativa, não submete solicitações nos portais oficiais e não liga SEFAZ.
- As informações e normas podem mudar; consultar as fontes oficiais e o contador antes do uso real.
- Teste de navegador com API simulada assegura renderização, URL de consulta oficial e abertura dos campos internos; não substitui E2E com backend real ou homologação fiscal.


## Assistente de configuração na própria aba Fiscal — 12/10/2026

- As seis orientações agora são etapas de um assistente de navegação, com indicador **Etapa N de 6**, botões **Voltar etapa**, **Próxima etapa** e **Revisar desde o início**, e abertura direta de uma etapa pelo seu título.
- O indicador mostra somente onde o operador está lendo, **não percentual de credenciamento ou homologação concluída**. É permitido avançar com campos faltantes para consultar instruções; nunca libera transmissão.
- Na primeira etapa, o assistente exibe, somente para a loja autenticada, os dados já presentes no cadastro: razão social, CNPJ, IE, CRT explicitamente selecionado, endereço, CEP e código IBGE. O formulário do emitente também já é preenchido pelo cadastro salvo, sem gravar qualquer alteração automaticamente. Valores desconhecidos são exibidos como **Não registrado no PDV**, jamais inventados.
- Na etapa de certificado, informa apenas se existe referência cadastrada (sem abrir PFX, senha ou chave). Na etapa de produtos, mostra o resumo de cadastros/NCM/perfis. Na etapa de homologação, mostra ambiente e série **salvos**, sem assumir que os defaults da interface foram aplicados.
- O assistente não chama Receita/SEF/IBGE em nome da loja, não registra login externo, não solicita segredo e não presume regime tributário. Sempre usa os dados já retornados pelos endpoints autenticados da aba Fiscal; não cria nova chamada, gravação ou marcação de aprovação.
- A posição de navegação fica somente em memória durante a visualização da página; o sistema não registra automaticamente etapas legais cumpridas. Em caso de troca de empresa, são usados os dados do novo contexto autenticado carregados pela página e nada é copiado entre CNPJs.
- Teste Chromium com API simulada comprova avanço/retorno e prévia do cadastro; não confirma qualquer conexão fiscal real.


## Progresso manual por empresa — 12/10/2026

As tarefas de organização fiscal agora são salvas no servidor por **tenant/CNPJ**, usando a infraestrutura pré-existente de `setup_step_reviews`, com permissão `invoice:generate`, proteção de origem confiável para `PUT`, gravação/remoção auditada na mesma transação e FK para usuário ligado ao tenant.

A migração 0045 amplia apenas a enumeração de tarefas manuais admissíveis:

- `fiscal_docs_collected`: operador declara ter reunido documentação cadastral;
- `fiscal_siare_access_arranged`: responsável providenciou o acesso ao portal SIARE;
- `fiscal_accountant_consulted`: operador solicitou conferência tributária ao contador;
- `fiscal_previous_series_checked`: operador conferiu o histórico de série e numeração.

Cada botão pode **marcar** e **reabrir** a tarefa. A confirmação é exibida explicitamente como declaração do operador. A listagem da aba Fiscal apresenta `N de 4 tarefas marcadas manualmente`, de forma separada dos diagnósticos automáticos dos campos do PDV e da situação real da SEF/MG.

O backend mantém lista fechada de chaves válidas, **recusando** estados como `fiscal_homologated`, `fiscal_authorized` e `fiscal_transmission_enabled`. A migração reversa recusa rollback se restarem registros fiscais, evitando perda silenciosa da história organizacional. Nenhuma senha, token, PFX/CSC, dados contábeis livres ou documentos são armazenados por essa funcionalidade.

**Atenção:** as marcações não comprovam nem substituem credenciamento, validade do certificado, enquadramento fiscal, homologação ou autorização de NFC-e/NF-e. A transmissão continua desligada. A verificação real em MG depende de credenciamento, dados oficiais, executor fiscal, homologação e respostas da SEFAZ.

Validações locais executadas em 12/10/2026: `go test ./...` e `go vet ./...` aprovados; frontend `npm run build` e `npm run lint` aprovados (0 erros, 12 avisos históricos); Chromium Playwright fiscal com API simulada 1/1 aprovado, incluindo gravação de checklist. PostgreSQL 16 temporário em porta separada: 45 migrações aplicadas, seed de demonstração somente no banco descartável, integração `TestFiscalSetupReviewsTenantBoundaryAndAudit` aprovada, `0045.down`/`0045.up` aprovadas sem registros fiscais, e rollback corretamente recusado diante de um registro fiscal. Nenhum teste SEFAZ real ou interação com dados de lojas ocorreu. Não usar Actions e manter PR draft/sem merge.


## Verificação estrutural unificada — 12/10/2026

O assistente mostra **pendências locais detalhadas** extraídas do endpoint `GET /api/v1/fiscal/nfce/readiness`, sem novas credenciais ou chamadas a sites governamentais. O mesmo validador de estrutura do emitente, `NFCeMGIssuerLocalBlockers`, é aplicado em três caminhos:

- **Readiness** da loja: diferencia razão social, dígitos verificadores do CNPJ, IE ausente, CRT desconhecido, rua, número, bairro, cidade, CEP com exatamente oito dígitos, UF MG e prefixo IBGE `31` com sete dígitos; mantém os códigos gerais anteriores para compatibilidade.
- **Preflight local** de venda finalizada: não classifica como `inputs_present_unverified` um emitente com uma dessas pendências.
- **Ativação em produção e reserva de número**: o pedido de habilitação reutiliza `FiscalService.NFCeReadiness`, incluindo a inspeção detalhada; a reserva valida o mesmo emitente antes de criar qualquer número. Estas proteções não ligam o executor fiscal.

A tela indica expressamente que `Campos locais presentes` **não** equivale a homologação. Um CRT preenchido não está necessariamente confirmado pela contabilidade; a IE é verificada quanto à presença, não à existência no cadastro estadual; códigos IBGE são testados apenas quanto a formato e UF, não contra a tabela oficial. NCMs são protegidos por formato na base, mas a escolha do código e dos perfis fiscais é responsabilidade da validação contábil. CEST pode ser aplicável conforme o caso. A transmissão e a impressão de DANFE seguem condicionadas à autorização fiscal genuína.

**Validações locais:** `go test ./...`, `go vet ./...`, `npm run build`, `npm run lint` (0 erros, 12 avisos preexistentes), Playwright Chromium com API simulada 1/1. Em PostgreSQL 16 **isolado e descartável**: 45 migrations aplicadas, testes integrados `TestFiscalDispatchClassifiesWithoutIssuing`, `TestNFCeReadinessReportsInvalidIssuerAndMGCode` e `TestNFCeReservation_IsAtomicAndIdempotentPerSale` aprovados. O último verifica que município incompatível não gera reserva nem consome sequência. Sem homologação/SEFAZ real, sem Actions e sem merge.
