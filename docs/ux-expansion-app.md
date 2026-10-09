# Expansão de usabilidade e app instalável

## Princípio de produto

O SistemaEmGo prioriza o operador da loja, não o jargão técnico.
Cada CNPJ continua sendo uma empresa/tenant **independente**; instalar o app
no mesmo dispositivo não autoriza misturar dados, permissões ou configuração fiscal.

## Expansão implementada nesta etapa

### 1. Importação assistida de produtos

Em **Produtos → Importar produtos de uma planilha**:

1. Baixar o modelo CSV (Excel/LibreOffice).
2. Preencher SKU, nome e preço; opcionais: unidade, código de barras, NCM, CEST,
   estoque mínimo.
3. Escolher CSV de até 1 MB com até 500 produtos.
4. Conferir linhas inválidas e uma amostra de registros.
5. Confirmar antes de cadastrar.

O importador interpreta separador `;` ou `,`, campos com aspas e decimal
brasileiro (`24,90`). Rejeita duplicados dentro do arquivo, NCM/CEST mal
formatados, preços não positivos e linhas inválidas. Rejeita o **lote inteiro**
no cliente se qualquer linha estiver incorreta; não grava automaticamente.

A importação agora envia a planilha validada por **um único POST**
`/api/v1/products/import-batches`, com chave de idempotência. O servidor
valida novamente todas as linhas (até 500), SKU e código de barras únicos
no lote, e grava produtos, saldos iniciais **zerados**, recibo e auditoria
em uma única transação PostgreSQL. Se algum produto conflitar com outro
já cadastrado ou a escrita falhar, **nenhuma linha é confirmada**.
Não adivinha custo, tributos nem estoque inicial; o backend não depende da
prévia do navegador como fonte de verdade.

A chave e o hash local dos dados normalizados são preservados por
`tenant_id` e usuário até 30 dias; **não** se guarda CSV, nome dos
produtos, preço ou dados fiscais no armazenamento de recuperação.
`GET /api/v1/products/import-batches/{key}` permite confirmar se o
servidor já aplicou o lote. Respostas ambíguas exigem consulta e,
se necessário, reenvio do **mesmo arquivo** com a mesma referência.
Reenvio idêntico responde `replayed=true`, e reenvio com payload
alterado sob a mesma chave retorna conflito. Um GET 404 não prova que
nenhuma requisição esteja em andamento. Limpar a referência local não
desfaz operações do servidor. Mudança de navegador/dispositivo ou
limpeza do storage perde a referência, mas o catálogo continua protegido
pela unicidade por empresa.

A API exige `product:write`, limite de requisições, origem confiável no
POST, e consulta apenas o CNPJ autenticado. Testes de integração cobrem
replay, rollback por conflito, auditoria e isolamento de duas empresas;
testes E2E cobrem resposta ambígua, recarga, CSV diferente, reconciliação
e negativa de acesso ao caixa. Ainda precisam ser executados.

O destaque *Estoque baixo* da tela Produtos considera os resultados
**atualmente carregados**, não é um relatório global de todo o catálogo.

### 2. Aplicativo PWA

O frontend possui `manifest.webmanifest`, ícones PNG/SVG, registro de
service worker e instruções de instalação no cabeçalho.

- **Windows / macOS / Linux**: navegador compatível, opção de instalar
  aplicativo quando oferecida.
- **Android**: Chrome compatível, menu Instalar aplicativo / Adicionar à tela inicial.
- **iPhone/iPad**: Safari → Compartilhar → Adicionar à Tela de Início.

A disponibilidade exata varia conforme o navegador e seu modo de navegação.
Serviço web em **HTTPS** (exceto `localhost`) e servidor/API configurados são
pré-requisitos. Um ícone instalado não é uma versão nativa da loja de aplicativos.

O service worker usa estratégia **network-first somente para a interface
e arquivos estáticos versionados**. Não armazena respostas de `/api/`,
XML, PDF, dados fiscais, autenticação, requisições de escrita ou chamadas
de backend. A fila de intenções de venda offline continua com a implementação
existente no navegador: **não limpar dados/desinstalar antes de conciliar as
vendas pendentes**. Não se presume que seja possível emitir NFC-e sem internet.

### 3. Configuração fiscal amigável (preparação)

A tela Fiscal passou a apresentar três etapas:

1. Preencher os dados oficiais do emitente.
2. Vincular certificado A1 **por referência segura** do servidor.
3. Consultar as pendências de prontidão.

A API mantém a referência já vinculada se o campo for deixado vazio em uma
edição, mas exige uma referência válida na primeira configuração. A referência
continua write-only para o cliente.

**Limite de segurança:** nenhuma tela pode prometer emissão fiscal real
baseada apenas em dados digitados. Exigem-se certificado válido,
classificação tributária adequada, ambiente homologado, transmissão segura
e autorização da SEFAZ; produção permanece bloqueada até esses gates.

### 4. Início simplificado e visão do proprietário

A tela **Início** (`/home`) pode ser aberta no menu por qualquer usuário,
inclusive no aplicativo instalado. Os atalhos exibidos são filtrados pelas
permissões recebidas de `/auth/me`; as APIs continuam a exigir suas permissões
no servidor. O fluxo anterior de login ainda abre Produtos até migrarmos
os testes E2E existentes que verificam essa navegação.

Com `finance:read`, são oferecidos um resumo do livro financeiro por período
e exportação CSV. Os valores são da loja autenticada, com lançamento por data
e distinção entre vendas após cancelamento, reembolsos e margem bruta estimada.
Não se trata de lucro líquido contábil nem de saldo bancário. O CSV sempre
registra as datas do último período **efetivamente consultado**, mesmo que
o usuário altere os campos sem buscar novamente.

Com `inventory:read`, são exibidos o **total global** de itens ativos com
estoque no mínimo ou abaixo e uma amostra dos mais críticos. A tela Estoque
informa o total global mesmo quando a lista está limitada e oferece exibição
ampliada para até 500 itens. Não há acesso financeiro ao caixa sem
`finance:read`, incluindo na API.

Testes adicionados:

- integração PostgreSQL para agregação, contagem global e isolamento A/B;
- E2E de proprietário, exportação e negativa de acesso financeiro pelo caixa.

Os testes ainda exigem execução em ambiente com runner disponível.

### 5. Estoque inicial por planilha, sem lançamento duplicado

Em **Estoque → Cadastrar estoque inicial por planilha**, o responsável com
`inventory:adjust` baixa o CSV com apenas `sku;quantidade`, preenche a
contagem física de sua empresa (até 100 produtos por lote), confere a prévia,
marca a confirmação e confirma novamente antes de enviar.

O servidor resolve os SKUs sob o tenant autenticado e exige produtos
**ativos, sem qualquer movimentação histórica e com saldo atual zero**.
Qualquer SKU incompatível bloqueia a transação inteira, sem importação
parcial. A referência de idempotência é criada ao escolher o arquivo e
reutilizada enquanto a prévia permanecer aberta. Respostas ambíguas podem
ser repetidas com a mesma referência: lote confirmado retorna `replayed=true`
sem duplicar saldos. Uma nova chave para produto já movimentado é rejeitada.

A tela não fornece estoque negativo, não sobrescreve saldos existentes e não
altera custos nem preços. Produtos com contagem zero são omitidos da carga
inicial; os que já tiveram operação precisam ser ajustados pelos fluxos
auditados normais, jamais por nova carga inicial.

Nova tabela `opening_stock_batches` na migration **0030**, com chaves
únicas por tenant e referência da movimentação. O estado piloto e
backup/restore exigem agora schema 32 e conferem as tabelas de lotes e revisões do assistente. O down da
migration é **fail-closed** se houver lotes confirmados, para preservar
a auditoria histórica.

**Recuperação após interrupção:** antes do primeiro POST, a interface
preserva uma referência UUID e o SHA-256 das linhas normalizadas do CSV em
`localStorage`, isolados por `tenant_id` e `user_id` do token autenticado.
Não armazena o CSV, seus SKUs, quantidades ou senha. Ao recarregar, exibe
a tentativa pendente e permite consultar se o lote foi confirmado via
`GET /api/v1/inventory/opening-stock/batches/{key}` (exige
`inventory:adjust`; o backend lê apenas o tenant autenticado e responde
200 para lote confirmado, 404 caso não exista registro confirmado).
O 404 **não prova** que outra requisição com a mesma chave não esteja
em andamento. O usuário precisa reabrir o mesmo arquivo para reutilizar
a mesma referência; um conteúdo divergente é recusado até a tentativa
anterior ser reconciliada ou descartada explicitamente.

O navegador não reproduz automaticamente o POST, não deduz o resultado
de erros de rede e aborta a transmissão se não conseguir persistir
a referência local. Ao receber confirmação positiva, a chave é removida.
Descartar a referência no navegador **não desfaz** o estoque no servidor.
O dado local pode desaparecer com limpeza do navegador, fim da validade
de 30 dias ou troca de dispositivo; a regra imutável do backend continua
impedindo uma nova carga inicial em produtos já movimentados. Este
recurso ainda não substitui um histórico administrativo central de lotes.

Antes de uso real, validar concorrência, rollback e recuperação de queda
de conexão com banco e dispositivos reais. Os novos testes ainda precisam
ser executados em runner funcional.

### 6. Assistente de configuração inicial da loja

A página **Configurar loja** (`/setup`) reúne cinco etapas com orientação
para usuários não técnicos:

1. **Dados da empresa:** consulta a razão social/CNPJ já provisionados nesta
   empresa e permite preencher inscrição estadual, CRT, endereço, código
   IBGE e CEP por meio do endpoint fiscal existente. Requer
   `invoice:generate` para salvar; nunca altera o CNPJ de outra empresa.
2. **Produtos:** consulta a contagem real do catálogo e leva ao cadastro
   individual ou à importação CSV existente.
3. **Estoque:** consulta o número de movimentações registradas e encaminha à
   contagem física e à carga inicial idempotente. Movimentações **não provam**
   que o saldo físico da loja tenha sido reconciliado.
4. **Funcionários:** mostra os dados e perfil do próprio usuário. Não promete
   criação/convite de contas porque ainda não há uma API administrativa segura
   para isso. A etapa fica marcada para revisão.
5. **NFC-e:** mostra a prontidão de dados e os bloqueios do servidor, com
   link à preparação fiscal. `ready_for_homologation_data` **não é sinônimo**
   de autorização da SEFAZ ou de transmissão liberada.

A navegação agora inclui botões **Etapa anterior / Próxima etapa**, um resumo
com as quantidades de etapas verificadas, a revisar e sem acesso, e um atalho
para a próxima etapa acessível que precise de atenção. Etapas prontas são
contadas apenas a partir de evidências verificadas pelo servidor. Estoque
com movimentações, perfis de equipe e dados de homologação NFC-e continuam
**para revisão**, e não são marcados como concluídos por clique no navegador.
O resumo não equivale a uma porcentagem de prontidão de produção.

O estado é calculado por leitura das APIs autenticadas e verificado novamente
a pedido do operador; a tela não grava uma falsa conclusão local em
`localStorage`. Consultas condicionadas às permissões evitam tentar obter
dados fiscais ou de inventário para usuários não autorizados; falhas nas APIs
ficam como **não verificadas** (fail-closed).

A plataforma ainda exige provisionamento confiável de cada novo CNPJ,
usuário proprietário e credenciais de ambiente pelo administrador da
implantação. O assistente não cadastra uma empresa legal arbitrária e
não pede certificado PFX, senha ou chave privada.

Testes adicionados:
- `web/e2e/setup-progress.spec.ts` para estados desconhecidos, restritos e
  diferenciação entre preparação fiscal e emissão real;
- `web/e2e/setup-wizard.spec.ts` para fluxo de navegação e ausência de
  requisições fiscais para o perfil de caixa.
- regressões de contagem conservadora, próxima etapa acessível e botões
  anterior/próxima no assistente.

**Pendente de execução:** build, lint, testes de navegação, teste E2E
do cadastro fiscal assistido, piloto real e validações da SEFAZ.

### 7. Revisões persistentes do assistente (por empresa)

A migration **0031** adiciona `setup_step_reviews`, com chave
`(tenant_id, step)` e vínculo do revisor à própria empresa por FK composta
`(reviewed_by_user_id, tenant_id)`. A tabela armazena apenas a etapa
(`stock` ou `team`), o usuário que revisou e a data, **sem** CPF,
observações livres, certificado, senha, CNPJ de outra loja ou dados fiscais.

Endpoints autenticados:
- `GET /api/v1/setup/reviews`: consulta as revisões da empresa do token.
- `PUT /api/v1/setup/reviews/{step}`: recebe `{"reviewed":true|false}`
  para registrar ou reabrir a revisão; ambas as rotas exigem
  `invoice:generate`, reservado aos responsáveis fiscais/gestores.
  O PUT também verifica a origem confiável. Não existe seleção de
  `tenant_id` no payload ou na URL.

As operações de escrita são auditadas **na mesma transação**.
Registros persistem entre recargas/navegadores, mas uma revisão manual
**não muda** as métricas de produtos, a conferência física do estoque,
a condição de permissão de colaboradores ou a prontidão da NFC-e.
As cinco etapas permanecem avaliadas a partir das APIs autenticadas,
sem "porcentagem de implantação pronta" artificial. A equipe ainda não
pode ser provisionada pelo assistente.

Testes novos:
- integração PostgreSQL de isolamento entre duas empresas, rollback,
  proibição de marcar fiscal manualmente e auditoria;
- E2E de persistência entre recargas simulando a API, reversão de revisão
  e ausência de requests administrativos no perfil de caixa;
- migração 0031 com upgrade, rollback sem dados e reapply;
- ferramentas de piloto e backup passam a exigir schema >=32 e 44 tabelas.

**Status:** casos de teste adicionados ao repositório, mas CI atual
não executa os steps nos runners hospedados; não inferir testes aprovados.

### 8. Histórico administrativo das importações

As telas **Produtos** e **Estoque** apresentam o histórico apenas quando o
operador tem, respectivamente, `product:write` e `inventory:adjust`.
Os dados vêm do banco da **empresa autenticada**, não de chaves locais:
`GET /api/v1/products/import-batches/history` e
`GET /api/v1/inventory/opening-stock/batches/history`.

A página mostra a data/hora, o nome do usuário responsável, a quantidade de
produtos do lote e o identificador do recibo. A paginação padrão é de 10 na
interface (API: limite de 1–50, offset de 0–5000), com botões de atualizar,
anterior e próximo. Um lote confirmado aparece mesmo depois de trocar de
navegador ou limpar o armazenamento local, desde que o usuário tenha acesso
à **mesma empresa**.

As consultas retornam **somente lotes transacionalmente confirmados**;
não mostram tentativas pendentes, falhas nem importações antigas individuais,
anteriores à migração de lotes. O recibo não possui CSV, SKU, quantidade
unitária, preço, certificado, chave de idempotência ou hash: não pode ser
usado para reconstruir a planilha nem adivinhar o conteúdo de um lote.
A API restringe tenant no SQL, exige permissão no servidor, não tem
parâmetro de tenant, evita cache HTTP e rejeita paginação inválida com 422.
O frontend descarta resultados de consultas antigas em mudança de sessão,
desmontagem ou navegação.

Testes de integração verificam isolamento de duas empresas, ordem,
paginação, nome do revisor e que lotes abortados não aparecem.
Playwright cobre navegação anterior/próxima, histórico de ambos os módulos,
negativa de acesso ao caixa e limites de paginação.
**Esses testes não equivalem a validação aprovada enquanto os jobs não rodarem.**

### 9. Filtros de período e exportação de histórico

Os dois históricos agora permitem escolher data inicial/final e filtrar
lotes confirmados. As datas seguem o **calendário de Brasília**
(`America/Sao_Paulo`), com início/fim inclusivos e intervalo máximo
de 365 dias entre as datas quando ambos os campos são preenchidos.
Ao aplicar/limpar o período, a paginação volta ao início. A interface
também mostra os horários das linhas nessa zona.

O responsável pode baixar um **CSV só de metadados** (data/hora UTC,
nome do operador, quantidade total de itens e ID do lote), com os
mesmos filtros aplicados à consulta. O CSV é gerado pela API, não a
partir das linhas atualmente visíveis na tabela; mantém portanto a
separação por `tenant_id` no servidor. Não exporta planilha original,
SKU, preço, custo, quantidade individual, documento fiscal, chave de
idempotência nem hash da tentativa.

Os arquivos são UTF-8 com BOM e separador `;`, compatíveis com Excel/
LibreOffice. O backend trata valores que poderiam ser interpretados como
fórmulas de planilha, usa nomes de arquivo fixos, evita cache HTTP,
restringe a exportação a **1.000 lotes** e rejeita com erro 422 (sem
enviar CSV parcial) pedidos acima desse limite ou intervalos inválidos.
Ao exceder o limite, o usuário deve restringir o período. Não há
exportação anônima nem possibilidade de solicitar o CNPJ de terceiros.

Testes unitários cobrem validação de data, parâmetros e CSV contra
injeção de fórmulas; integração cobre filtragem e isolamento por empresa,
e E2E cobre seleção de datas, download e o limite. **Ainda dependem de
execução real em runner funcional antes do uso em produção.**

### 10. Histórico consolidado por empresa e permissão

Uma página de **Histórico de importações** acessível pelo menu
(`/imports`) e pelo Início reúne, em uma só linha do tempo, os
recibos de importação de produtos e de contagem inicial de estoque.
O usuário vê apenas os tipos autorizados por `product:write` e
`inventory:adjust`, respectivamente. A autorização real é aplicada
pela API no servidor — não basta esconder o link na interface.

`GET /api/v1/imports/history` usa o CNPJ/tenant **da sessão**,
consulta somente lotes confirmados e pagina a união completa em uma
única consulta SQL, por `created_at DESC, kind ASC, batch_id DESC`.
A resposta inclui tipo, data/hora, responsável, quantidade de itens
e ID do lote; não contém CSV, SKU, preço, dados fiscais, hash nem
chave de idempotência. Não revela nada de outro CNPJ nem de uma área
sem permissão. O endpoint exige ao menos uma das duas permissões,
retorna 403 para o operador sem acesso e não permite parâmetro de
CNPJ fornecido pela interface.

Os filtros por data seguem o calendário de Brasília, e a
navegação usa páginas de 10 lotes (limites de API: 1–50 por página,
offset 0–5000). Os botões de cada registro direcionam à tela
do respectivo módulo; exportações CSV permanecem somente nos
históricos específicos para não misturar permissões.
A tela não autoriza edição, ajuste de estoque, importação nem
emissão fiscal. Não confundir lotes confirmados com tentativas
interrompidas ou não concluídas.

Testes de integração abrangem cronologia e paginação dos dois
tipos, permissões independentes e isolamento entre dois CNPJs.
E2E cobre navegação, período, paginação, negativa ao caixa e
consulta inválida. A aceitação operacional depende de build e
testes efetivamente executados, não só de commits no repositório.

### 11. Gestão de funcionários por empresa

Em **Funcionários** (`/staff`), exclusivamente com a permissão
`team:manage` do administrador da loja:

- Acompanhar a equipe desta empresa, função e vínculo ativo.
- Criar convite para **gerente** ou **caixa** sem inventar senha provisória.
- Entregar o link de uso único por um canal confiável; válido por 48h.
- Permitir que o funcionário defina senha própria antes do primeiro login.
- Promover/reduzir os papéis entre gerente e caixa com confirmação.
- Suspender/reativar o acesso **somente deste CNPJ**, preservando o
  histórico de vendas, estoque e auditoria e eventuais vínculos com
  outras empresas.
- Cancelar convites não utilizados. Não exibir novamente o segredo
  após a criação do convite.

A migration `0033_staff_invitations` cria o status
`user_tenants.active` e a tabela de convites com SHA-256 do token;
a API mantém `users.active` global inalterado. Autenticação,
refresh e leitura de permissões verificam o vínculo ativo na empresa
do JWT; funcionários desativados não seguem usando esse CNPJ só porque
o token ainda não expirou.

Nenhum gerente/caixa recebe `team:manage`. A interface impede
mudar a conta do próprio administrador, e o backend protege todos
os papéis `admin` independentemente de artifícios da UI. A aceitação
cria conta, vínculo, permissão e auditoria em uma única transação
PostgreSQL. O token só trafega no corpo do POST de ativação e não
é guardado em texto puro no banco; a interface recebe o token num
fragmento da URL e remove o fragmento do histórico imediatamente.

**Limitações conscientes:** não há envio automático de e-mail,
suporte a reutilizar automaticamente um e-mail que já possui conta
global em outro CNPJ, nem interface para criar/remover
administradores. A vinculação de identidade existente e o bootstrap
de um novo dono de loja continuam procedimentos controlados.
A listagem inicial é limitada a 200 membros e 200 convites.
A etapa do assistente continua marcada como revisão humana,
não como concluída só porque existem contas.

Testes adicionados: restrições dos papéis e e-mails, uso único
do token, isolamento de 2 CNPJs, revogação imediata em RBAC,
autorização do caixa negada, navegação/aceitação E2E,
upgrade/rollback/reapply da migration v33.
**Tudo isso depende de validação dinâmica real no commit final.**

### 12. CSV de estoque completo para conferência

Na página **Estoque**, `Exportar estoque CSV` gera um relatório
global da loja atual (não apenas os 50/500 produtos exibidos em
baixo estoque). A API consulta por `tenant_id` autenticado e
usa `inventory:read`, sem permitir que a interface escolha outro
CNPJ. O relatório contém produto, SKU, unidade, saldo, estoque
mínimo e sinalizador de estoque baixo, incluindo cadastro ativo/
inativo. Não inclui preços, custo ou dados fiscais.

O servidor limita a 5.000 produtos; acima desse teto recusa
a exportação, sem truncar. CSV em UTF-8 com BOM e separador
`;`, com proteção contra fórmula de planilha. A interface
exibe mensagens claras para o erro de limite ou indisponibilidade.

**Importante:** relatório de saldo não equivale à contagem física
auditada. A implantação de um fluxo formal de inventário e ajuste
com confirmação continua pendente. A validação funcional e
o teste de limites dependem de ambiente executando CI.

## Próximas expansões

- Acrescentar visão de movimentações e histórico fiscal em módulos próprios
  (somente leitura e RBAC), sem transformar recibo em prova de emissão.
- Planejar relatórios por período para gestores, sem misturar dados de CNPJs
  distintos ou elevar permissões do usuário.
- Oferecer conciliação assistida de lotes sem referência local e um histórico
  separado de **tentativas não confirmadas**, sem inferir commit a partir de
  erros de rede.
- Paginação completa e exportação do relatório global de estoque; o total já
  é computado globalmente, mas a listagem está limitada a 500 itens.
- Relatórios gerenciais mais profundos (venda por item, período e custo),
  preservando a autorização de custo e margem.
- Ampliar o onboarding com evidências automáticas versionadas e adicionar
  vinculação verificada de uma mesma identidade a outros CNPJs, sem
  mistura de permissões nem elevação automática a administrador.

## Próxima fase: operação

- Ciclo visual completo venda → NFC-e → SEFAZ → DANFE, com correções
  de rejeição e estados ambíguos, sem reenvio fiscal cego.
- Teste de instalação em aparelhos reais e em navegadores variados; medir
  segurança da fila offline em múltiplas abas, atualização, queda de energia
  e perda de armazenamento.
- Homologação fiscal por UF, backups/restore e piloto com operador real.

## Validação

Nenhuma alteração nesta etapa permite inferir CI verde: o GitHub Actions
segue sem executar steps nas rodadas recentes. Reexecutar testes do frontend,
Playwright, integração fiscal e piloto seguro no SHA final assim que houver
um executor disponível. Não fazer merge nem liberar produção por inferência.
