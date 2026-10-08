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

## Próximas expansões

- Importação de inventário inicial por contagem física com dupla conferência
  e idempotência, separada do cadastro de produtos.
- Central global de estoque mínimo e relatórios gerenciais, com consultas
  paginadas/agrupadas no backend, não somente filtro da lista visível.
- Exportação financeira/gerencial com critérios de período, permissão e
  confidencialidade.
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
