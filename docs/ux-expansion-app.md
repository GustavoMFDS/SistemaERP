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

Cada registro é criado pela API existente, que valida autenticação,
`product:write`, tenant e unicidade. Não importa estoque inicial nem
adivinha custo, tributos ou preços. **Não é uma transação única**: se algum
registro falhar no servidor, os anteriores podem ter sido criados e a tela
mostra o resultado parcial. Conferir as falhas antes de repetir o lote.

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
backup/restore exigem agora schema 30 e conferem essa tabela. O down da
migration é **fail-closed** se houver lotes confirmados, para preservar
a auditoria histórica.

**Limites atuais:** a chave ainda existe na página do navegador, e o
usuário pode perder a prévia após atualizar ou limpar os dados. Mesmo assim,
a regra do backend de histórico impede novo saldo inicial em produtos usados.
Antes do uso real, validar concorrência, rollback e recuperação de queda
de conexão com um banco e dispositivos reais. O GitHub Actions segue sem
executar os jobs.

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

## Próximas expansões

- Melhorar recuperação de importação após fechar a aba, com referência de lote
  recuperável em local persistente seguro e consulta administrativa.
- Paginação completa e exportação do relatório global de estoque; o total já
  é computado globalmente, mas a listagem está limitada a 500 itens.
- Relatórios gerenciais mais profundos (venda por item, período e custo),
  preservando a autorização de custo e margem.
- Onboarding por empresa com estado persistente de etapas e verificações
  sem inserir segredos no banco ou no frontend.

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
