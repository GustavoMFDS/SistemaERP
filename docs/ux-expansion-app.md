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

## Próximas expansões

- Importação de inventário inicial por contagem física com dupla conferência
  e idempotência, separada do cadastro de produtos.
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
