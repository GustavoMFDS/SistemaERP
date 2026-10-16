# Inventário completo de funcionalidades — Esterium ERP

**Referência:** 16/10/2026. Inspeção estática da branch **feature/catalog-product-photos-20261009**, no PR [#25](https://github.com/GustavoMFDS/SistemaERP/pull/25), sobre **ops/pilot-readiness-20260923** (PR #15). Snapshot inicial inspecionado: **aacb199f74fcf2f06c089fcc85db477a83330826**. Este documento registra a inspeção; sua inclusão documental gera commits posteriores.

**Nome comercial de trabalho:** Esterium ERP (antigo SistemaEmGo/SistemaERP). A pesquisa pública de disponibilidade do nome não concluiu consulta nominal oficial no INPI; não presumir direito exclusivo de marca.

**ESTADO GLOBAL: EM DESENVOLVIMENTO / PILOTO, NÃO LIBERADO PARA PRODUÇÃO FISCAL.** Recursos presentes no código não são automaticamente testados ponta a ponta, homologados ou seguros para uso real. Este catálogo prevalece sobre as classificações históricas de 09/10/2026 em [catalogo-funcional-sistema.md](catalogo-funcional-sistema.md), que fica como histórico detalhado.

## 0. Convenções e cobertura

- **I — Implementado no código:** há API/serviço, tela ou armazenamento conferido nesta branch. Não significa operacionalmente validado.
- **P — Parcial/limitado:** existe um fluxo concreto, mas ainda falta parte importante da experiência ou da integração.
- **A — Apenas API/fundação:** falta interface administrativa completa ou implantação da capacidade.
- **N — Não implementado/comprovação ausente:** não apresentar como funcionalidade entregue.
- **E — Externo:** exige serviço terceiro, certificados, testes físicos, revisão contábil/jurídica ou homologação.
- **V — Validação localizada:** testes locais efetivamente registrados; não implica que toda a funcionalidade foi testada.

**Dimensões conferidas:** 15 arquivos de página React (incluindo login/aceite de convite), 13 diretórios de módulos Go, **121 registros de rotas HTTP** no roteador (GET 62; POST 45; PUT 11; PATCH 1; DELETE 2), **48 migrações SQL UP** (0001–0048), 49 arquivos Playwright *.spec.ts. São contagens de artefatos, **não** 121 funcionalidades de ponta a ponta nem 49 testes aprovados. Os componentes auxiliares, jobs internos e validações do banco não são todos acessíveis pela UI.

**Arquitetura multiloja:** cada empresa/CNPJ possui sua própria identidade e dados (tenant), mesmo quando o dono é da mesma família. **Não** modelar CNPJs de familiares como filiais da mesma pessoa jurídica, nem compartilhar clientes, produtos, fiscal ou caixa automaticamente.

## 1. Telas de uso do ERP

| Área | Rota React | Estado |
|---|---|---|
| Login | /login | I |
| Aceitação de convite | /accept-invite | I |
| Início/visão da loja | /home | I |
| Configuração inicial | /setup | I/P |
| Equipe e permissões | /staff | I |
| Cadastro e galeria de produtos | /products | I/P |
| Estoque, ajustes, carga inicial | /inventory | I/P |
| Histórico de movimentações | /stock-movements | I |
| Histórico de importações | /imports | I |
| Fornecedores e compras | /purchases | I |
| Clientes | /customers | I/P |
| Devoluções/trocas | /returns | I/P |
| Ponto de venda/caixa | /pdv | I/P |
| Financeiro e conciliação | /finance | I/P |
| Dados fiscais, pendências, documentos | /fiscal | I/P/E |

**Páginas que não existem na navegação principal:** painel dedicado de auditoria, solicitações LGPD, crediário, emissão completa de NF-e 55 e gestão multidepósito (A/N conforme os serviços abaixo). Não confundir API com tela disponível ao lojista.

## 2. Empresas, autenticação e usuários — I/P

**O que já temos**
- Empresa isolada por tenant_id/CNPJ; consultas e transações com escopo por empresa e vínculos de usuário.
- Login por e-mail e senha; token JWT; refresh HttpOnly com rotação; logout e validação de vínculos atuais.
- Permissões por função (admin, gerente, caixa), RBAC das rotas e mascaramento de custo/lucro sem permissão adequada.
- Equipe na loja: listar membros, convidar novo usuário por link temporário de ativação, ativar senha própria, mudar papel, suspender/reativar acesso por empresa e revogar convites.
- Prevenção de autoelevação e alguns controles de mudanças administrativas; trilha auditada.

**Limitações:** criar uma nova empresa/dono 100% por autosserviço, recuperação segura de senha, entrega automática de convites por e-mail e ingresso de conta preexistente em outro CNPJ não estão completos. Revisão de concorrência e gestão volumosa da equipe continuam como testes/expansões.

**Evidências:** backend/internal/modules/auth, backend/internal/modules/team, backend/internal/httpapi/handlers/team_handler.go; telas StaffPage e AcceptStaffInvitePage; migrations 0003, 0012, 0033.

## 3. Implantação inicial e experiência do proprietário — I/P

- Home com atalhos filtrados por permissão e visão resumida do negócio.
- Assistente passo a passo: dados da empresa, catálogo, estoque, equipe e preparação fiscal; indicadores de pendências e revisões de estoque/equipe persistidas com auditoria.
- Cadastro guiado de emitente, endereço e município; prontidão fiscal separa dado preenchido de confirmação/homologação real.
- A instalação demonstrativa local pode rodar com PostgreSQL próprio e dados sintéticos; **não é um instalador distribuível de produção**.
- **Pendente:** provisionamento automático de certificados/segredos, checagem física do estoque, checklist final de impressora e backup, tutoriais e validação por lojistas reais.

**Evidências:** HomePage, SetupPage, FiscalPage, setup_handler.go, migration 0031.

## 4. Produtos, fotos, categorias e variações — I/P

- Cadastro e edição por empresa: SKU, descrição/nome, unidade, preços, custo, promoções, estoque mínimo, código de barras e dados básicos fiscais NCM/CEST.
- Busca paginada, scanner/consulta de código de barras, bloqueio de SKU/GTIN repetidos dentro do CNPJ, produto ativo/inativo.
- Controle de leitura de custo/margem por permissão; proteção para atualização não apagar custo oculto.
- Importação de catálogo via CSV com modelo, prévia, limites, atomicidade, idempotência e recuperação de importação de resultado incerto; histórico e exportação de metadados.
- **Fotos opcionais:** até cinco imagens por produto; normalização JPEG e miniatura no servidor; visualização em catálogo/estoque/PDV; legenda opcional (ex.: Azul, Rosa) e exclusão, respeitando escopo da empresa. **Não** criam estoque automaticamente nem entram no XML fiscal.
- **Cores/tamanhos com saldo separado:** famílias de produtos por vínculo explícito; cada variante possui SKU/product_id e estoque independentes, sem dividir automaticamente o saldo do produto-base. Se a loja não optar por saldo por cor, as fotos permanecem ilustrativas.
- Catálogo mostra variações e PDV permite escolher a opção; compra, venda, devolução, ajuste e cache de saldo são amarrados ao SKU correspondente. Testes de integração local registrados para essas transações.

**Parcial/pendente:** seleção e reordenação manual da foto principal; edição em massa e exportação total controlada de preços/custos; gestão completa de categorias/marcas, etiquetas, kits, embalagens, balança e regras complexas de alteração de variantes depois de vendas. Não declarar todas as jornadas offline + variantes prontas para produção.

**Evidências:** ProductsPage, product_images_handler.go, product_variations_handler.go, products_import_handler.go; migrations 0032, 0036–0038; testes product-photos, pdv-product-variations, product_variations_sale_return_test.go.

## 5. Estoque, saldos, ajustes e movimentações — I/P

- Saldo por produto/SKU e CNPJ; limite de estoque mínimo e avisos de reposição.
- Ajustes auditados com motivo e controle de permissões; movimentos reservados de compra, venda e devolução têm origem própria.
- Histórico paginado das movimentações com quantidade, antes/depois e produto/variante.
- Importação de **estoque inicial CSV** com prévia, lote, histórico, chaves idempotentes e reconciliação de respostas.
- Relatório CSV de produtos e saldos por empresa (com limite atual de itens); histórico combinado das cargas de catálogo e saldo.
- Transações de SKU variante atualizam apenas o saldo daquele SKU; validação no servidor protege contra estoque indevidamente negativo conforme a configuração.

**Não completo:** inventário físico formal (sessão de contagem, divergência, aprovação, consolidação), lotes/validade/FEFO, múltiplos depósitos, transferência interna auditada, reposição preditiva, inventário cíclico e grandes exportações sem limite.

**Evidências:** InventoryPage, StockMovementsPage, UnifiedImportHistoryPage; inventory_handler.go, stock_report_handler.go; migrations 0030, 0038.

## 6. PDV, vendas, recuperação offline — I/P

- Tela de venda com busca por produto, scanner de barras, seleção de variação, quantidades e carrinho.
- Preço final recalculado no servidor, promoções, descontos sob permissão, validações de estoque e finalização transacional.
- Registro de formas de pagamento no fechamento, venda em dinheiro/PIX/cartão conforme modos existentes; registrar um método **não comprova recebimento bancário**.
- Suspender e retomar carrinhos, com revalidação de dados/preços; teclas de atalho.
- Cancelamento de venda sob regras, inclusive bloqueio quando já há devolução.
- Fila offline do navegador com idempotência e reconciliação; cache de produtos escopado por usuário + CNPJ, validade aproximada de 24h, revalidação no servidor ao sincronizar.
- Após venda, registro da **obrigação fiscal separado da escolha do cliente por imprimir**. A tela consulta estado fiscal e não deve mostrar arquivo não autorizado como DANFE.
- Impressão de comprovante não fiscal via navegador; impressão opcional após a venda sem desfazer a venda ao cancelar a impressão.

**Não completo:** múltiplos terminais concorrentes comprovados em hardware real, garantia de venda integralmente offline em qualquer falha, testes de corte de energia, Pix dinâmico confirmado, TEF e captura comprovada de pagamento. Crediário e vínculo opcional cliente–venda completos não estão entregues.

**Evidências:** PDVPage, sales_handler.go, sale_fiscal_handler.go, módulos sales e fiscal; testes pdv-offline, pdv-fiscal-print-choice e offline-integrity (presença de código ≠ execução completa neste SHA).

## 7. Caixa, fechamento, comprovantes e impressora — I/P/E

- Abertura de turno/sessão; conferência de sessão atual; suprimento e sangria; fechamento por forma de pagamento e diferença entre informado/esperado.
- Permissões separadas para abrir, movimentar e fechar.
- Papel fiscal de 58 mm ou 80 mm configurado **por caixa**, com persistência e controle de CNPJ; página de calibração **não fiscal** com régua/QR e impressão pelo diálogo do navegador.
- DANFE NFC-e renderizado para 58/80 mm **somente com autorização verdadeira e XML processado arquivado íntegro**; também há download do XML autorizado a partir da venda sob permissão.

**Limite do desenho:** impressão **semiautomática no navegador**. Não requer app nativo apenas para mostrar o diálogo de impressão; **não há** validação física de impressora/corte automático, conexão ESC/POS direta, gaveta ou balança. O botão de imprimir não autoriza uma nota.

**Evidências:** PDVPage, cash_handler.go, cash_printer_handler.go, migrations 0013–0015 e 0048; testes danfe-thermal-layout e printer-calibration-layout.

## 8. Compras, fornecedores e recebimentos — I/P

- Cadastro/edição/listagem de fornecedores por empresa.
- Pedidos de compra com produtos, preço e quantidades, status e cancelamento conforme regras.
- Recebimento parcial: estoque e custo mudam no recebimento confirmado, não só ao criar pedido.
- Permissões específicas, bloqueios para entidades inativas e auditoria de operações; possível vínculo de compra a obrigação a pagar.
- SKU variante comprável/recebível sem alterar saldos de outras opções.

**Pendente:** importar XML de NF-e do fornecedor com conferência assistida; devolução a fornecedor; comparação histórica de custo, sugestão de compra e aprovações de compras complexas.

**Evidências:** PurchasesPage, procurement_handler.go, migrations 0018 e 0035.

## 9. Devoluções, trocas e reembolsos — I/P

- Devolução parcial/total com verificação do limite de itens vendidos e proteção de duplicidade.
- Decisão de retornar estoque vendável ou registrar não reposição/avaria.
- Reembolso devido calculado no servidor e liquidação auditada, inclusive com mais de uma forma de pagamento.
- Troca por devolução e nova venda, preservando rastreabilidade da operação.
- Cancelamento de venda com devolução anterior é bloqueado; operação de variante retorna ao SKU correto.

**Pendente:** troca completa em uma experiência única com diferença a pagar/devolver, comprovante próprio e relatórios analíticos de motivos/avarias.

**Evidências:** ReturnsPage, returns_handler.go, finance_handler.go, migrations 0019–0022.

## 10. Financeiro, contas e relatórios — I/P

- Ledger financeiro, dashboard e resumo do proprietário por período.
- Contas **manuais** a pagar/receber com valor, vencimento, situação, filtros/paginação e baixa explícita. Gráfico de entradas/saídas registradas, não extrato bancário.
- Registro de pagamentos da venda, reconciliação digital inicial imutável e correções auditadas posteriores com justificativa e histórico.
- Reembolsos vinculados a devoluções, fechamento por método, relatórios e ranking de produtos vendidos.
- Exportação CSV de indicadores e relatórios de estoque/importação em suas telas.

**Não afirmar:** contabilidade completa, DRE fiscal, lucro líquido contábil, confirmações bancárias, OFX integrado, cálculo automático de juros/multa ou crediário com parcelas/limite. Fluxos de pagamento são majoritariamente **registro operacional**, não gateway de pagamentos.

**Evidências:** FinancePage, FinancialOverview, TopProductsReport, finance_handler.go, finance_accounts_handler.go, product_ranking_handler.go, migration 0035.

## 11. Clientes e relacionamento — I/P

- Agenda de clientes por empresa, paginação/busca, inclusão e edição de nome, telefone/e-mail opcionais.
- RBAC de leitura/escrita e trilha de alteração.
- **Parcial:** dados de identificação fiscal opcionais, ligação direta cliente–venda, histórico de consumo, deduplicação, comunicação por consentimento e crediário não formam ainda uma jornada completa.
- Atenção à LGPD e minimização de dados em qualquer nova identificação CPF/CNPJ.

**Evidências:** CustomersPage, customers_handler.go, migration 0034.

## 12. Fiscal: NFC-e 65 MG, obrigações e NF-e 55 — I/P/E

**Existe código para**
- Registrar a obrigação fiscal de cada venda e classificá-la (NFC-e ou NF-e) de forma separada de imprimir ou não o comprovante.
- Configurar emitente/CNPJ, IE/CRT, endereço, município, série, ambiente, certificado referenciado, dados fiscais de produtos e cálculos tributários preparatórios.
- Validar prontidão e gerar/previsualizar XML candidato, reservar números, assinar A1 quando configurado, tratar inutilização, contingência offline e eventos de cancelamento em ambiente previsto.
- Integração técnica SEFAZ **em homologação, sob configuração explícita**, com envio, consulta por chave e estados de retorno, usando credenciais/artefatos oficiais quando disponíveis.
- Guardar protocolo e **nfeProc** autorizado com verificação de SHA-256 para impressão, download e status; impedir DANFE autorizado com arquivo ausente/corrompido.
- Filas locais de preparação, pré-validação, emissão opcional, consulta de resultado incerto, leases, repetição idempotente, limites de tentativas e revisão manual.
- Reconciliação de fila após crash: documento submetido nunca deve ser retransmitido cegamente; erro de integridade **pós-autorização** reabre estado das filas para revisão sem alterar a nota ou enviá-la de novo.
- Tela Fiscal com pendências por venda, preparação, diagnósticos, estados da fila, tentativas de consulta, configuração e acesso a XML/DANFE somente quando elegíveis.

**Bloqueios reais**
- **NFC-e oficial em MG não homologada/validada com CNPJ e certificado A1 reais nesta branch.** Os testes recentes de sucesso usam SEFAZ **simulada** e banco fictício. A existência de classe SOAP/assinatura não equivale à autorização governamental.
- Transmissão automática de homologação opt-in e produção desabilitada por padrão; segredos oficiais, CSC/QR, regras de MG e revisão contábil/fiscal externa continuam pendentes.
- Verificar esquemas e NTs atuais, regras IBS/CBS, QRCode, pagamentos, dados de destinatário, contingência, rejeições e cancelamentos de verdade antes de cada liberação.
- **NF-e modelo 55 não é um emissor completo.** Há referências de XML/obrigação e prévia legada, mas o fluxo certificado de emissão modelo 55 não está implementado fim a fim. Não declarar NF-e 55 pronta.
- Nenhuma operação do clone de demonstração com seed deve ser interpretada como documento fiscal real.

**Evidências:** FiscalPage, sale_fiscal_handler.go, sale_fiscal_obligations_handler.go, fiscal_handler.go, módulos fiscal/application, fiscal/providers/sefaz; migrations 0023–0029, 0039–0047; [runbook MG](fiscal-mg-recuperacao-fail-closed-2026.md).

## 13. Segurança, auditoria, privacidade e LGPD — I/A/P

- Auditoria de operações financeiras, dados comerciais, empresa e fiscal; registros com escopo, ator e saneamento de metadados; API de logs.
- Autenticação, permissões contextualizadas, limitação de requisições, verificação de origem e CORS, cabeçalhos e erros com request ID.
- APIs de pedidos LGPD: abertura, listagem, andamento, exportação e anonimização/bloqueio onde legalmente permitidos; consentimentos e revogação.
- **API/fundação, não painel completo:** auditoria e gestão de solicitações LGPD não aparecem como telas principais no App.tsx.
- Revisão jurídica, inventário/retention real, ensaio de restore, pentest, política de credenciais, monitoramento de incidentes e aceite de segurança em produção permanecem externos/pendentes.

**Evidências:** audit_handler.go, privacy_handler.go, auth_handler.go, middleware; migrations 0006, 0009–0012.

## 14. Web/PWA, periféricos e infraestrutura — I/P/E

- App responsivo React/TypeScript/Vite; nome visual Esterium ERP, manifesto PWA, service worker e instalação via navegador quando o dispositivo permitir.
- Backend Go, PostgreSQL, Redis opcional no desenvolvimento e Docker Compose. Redis oferece cache/apoio a sessões conforme modo de implantação.
- 48 migrações de esquema, dados de demonstração explicitamente protegidos, health live/ready, métricas, logging e documentação de implantação.
- Testes Go, integração com PostgreSQL, Playwright e jobs de CI escritos no repositório.
- **Não existente/comprovado:** aplicativo nativo publicado, distribuição gerenciada em lojas, periféricos homologados, sincronização robusta multi-caixa em implantação real, backup/restauração evidenciados no piloto.
- Migrations/código novos **ainda no PR #25 (draft)**, que depende da base PR #15. Não chamar isto de versão consolidada em main.
- Histórico de Actions: jobs anteriores sem runner executado; não usar esses registros para declarar o PR validado.
- Docker, PWA e documentação são meios técnicos disponíveis, não substituem operação assistida em empresa real.

**Evidências:** web/src/App.tsx, web/public/manifest.webmanifest, backend/internal/httpapi/router.go, backend/migrations, docker-compose.yml, docs/deployment.md, .github/workflows/ci.yml.

## 15. Evidência de testes e aceitação (sem inflar resultados)

| Data / ambiente | Escopo registrado | O que PROVA | O que NÃO prova |
|---|---|---|---|
| 09/10/2026, PostgreSQL + Redis isolados | Integração de variantes: venda, estoque, devolução, compras e cache | Regras selecionadas com dados fictícios, isolamento de SKU e idempotência | Todo o navegador e hardware reais |
| 14–15/10/2026, PostgreSQL 16 descartável | 48/48 migrações; testes Go de emissão/consulta com SEFAZ falsa, lease/crash e checksum | Consistência de banco e estado fiscal em cenários testados | Homologação verdadeira SEF/MG |
| 15/10/2026, clone Windows | Frontend compilado, API/PG rodando, login/consulta de produtos/estoque/financeiro na demo | Ambiente local demonstrável com seed sintético | Segurança e disponibilidade de produção |
| 15/10/2026, PR #25 | go test ./..., go vet ./..., npm run build e lint aprovados nas rodadas registradas; alguns Playwright focalizados aprovados | Compilação e cenários selecionados no commit testado | Suíte Playwright integral, Actions final ou produção |
| 15/10/2026, último rebranding | npm run build aprovado no commit **aacb199** | Interface com nome Esterium compilou | Validação jurídica da marca ou regressão fiscal end-to-end |

**Importante:** a suíte E2E completa tem 49 **arquivos** de especificações, não necessariamente 49 cenários; ela não foi toda executada/aprovada no HEAD atual. Testes de interface com API interceptada são distintos de E2E com API+PostgreSQL reais.

## 16. Pendências ordenadas por risco e valor

### P0 — não operar oficialmente antes

1. Validar a cadeia **PR #15 → PR #25** no commit final com CI executado de verdade; Go, frontend, segurança, integração, 49 specs/ambientes relevantes, migrations UP/DOWN/UP de forma segura e isolamento por CNPJ. Não fazer merge implicitamente.
2. Execução E2E real dos fluxos críticos: criação/edição de produto+foto+variação → estoque/compra → venda e pagamentos → devolução → conciliação; testar permissão de duas empresas, falhas de rede, concorrência e idempotência.
3. Backup/restauração completos, controle de acesso a arquivos fiscais, armazenamento, logs, rotação de segredos, monitoramento e testes de recuperação em ambiente semelhante ao real.
4. **Homologação de NFC-e 65 MG** com certificados, identidade fiscal da empresa, CSC/QR, XSD/NT atuais, impostos, contingência e autorização/cancelamento reais, sob validação contábil/fiscal; manter fiscal de produção bloqueado.
5. Validação física da impressora térmica 58/80 mm, papel/corte, impressão opcional e jornada de caixa em cada estabelecimento.
6. Licença, suporte, uso real dos dados, política LGPD, autorização da marca e eventual registro no INPI antes de distribuição comercial ampla.

### P1 — funcionamento diário e facilidade de uso

1. Inventário físico guiado (contagem/ajuste aprovado) e operação segura de estoque com milhares de produtos.
2. Concluir vínculo cliente–venda e histórico de compras, sem exigir CPF desnecessariamente.
3. Refinar contas a receber/pagar, vencimentos, conciliações e experiência do operador; crediário somente com modelo auditável completo.
4. Refinar variações/fotos, seleção de imagem principal, atualização em massa, etiquetas, exportação de catálogo e carga de migração entre lojas independentes.
5. Consolidar central de pendências fiscais e alertas compreensíveis, sem expor jargões técnicos ao caixa.

### P2 — expansões

1. Pix dinâmico/TEF com integração oficial, conciliação real e reembolso transacional.
2. Importação assistida de XML de entrada; NF-e **modelo 55** realmente completa e homologada se necessária.
3. Lotes e validade, múltiplos depósitos dentro de um CNPJ, relatórios gerenciais e fiscais comparáveis.
4. Painéis administrativos de auditoria/LGPD e exportações controladas, UX mobile e acessibilidade.
5. Aplicativo nativo **apenas se** PWA/navegador não atender necessidades de hardware ou operação.

## 17. Fontes de verdade por módulo

| Domínio | Código principal |
|---|---|
| Navegação e páginas | web/src/App.tsx; web/src/pages; web/src/components |
| API e RBAC | backend/internal/httpapi/router.go; backend/internal/httpapi/middleware |
| Autenticação/equipe | backend/internal/modules/auth; backend/internal/modules/team |
| Catálogo, fotos/variações | backend/internal/httpapi/handlers/products_handler.go; product_images_handler.go; product_variations_handler.go |
| Estoque | backend/internal/modules/inventory; inventory_handler.go |
| PDV e vendas | backend/internal/modules/sales; sales_handler.go |
| Caixa/impressão | cash_handler.go; cash_printer_handler.go; sale_fiscal_handler.go |
| Compras/devoluções | backend/internal/modules/procurement; backend/internal/modules/returns |
| Contas/financeiro | backend/internal/modules/finance; finance_accounts_handler.go |
| Clientes | backend/internal/modules/customers; customers_handler.go |
| Fiscal | backend/internal/modules/fiscal; backend/internal/httpapi/handlers/fiscal_handler.go |
| Privacidade/auditoria | backend/internal/modules/privacy; backend/internal/modules/audit |
| Banco/testes | backend/migrations/0001–0048; backend/tests/integration; web/e2e |

**Regra de manutenção:** toda nova entrega altera o módulo correspondente neste catálogo indicando commit, validação efetivamente executada e pendências. Não substituir pendência por “concluído” só porque existe uma função, um botão ou um teste escrito.
