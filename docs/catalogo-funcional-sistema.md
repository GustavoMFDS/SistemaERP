# Catálogo funcional e checklist — SistemaEmGo

> **Referência:** 09/10/2026 · `ops/pilot-readiness-20260923`, PR [#15](https://github.com/GustavoMFDS/SistemaERP/pull/15), HEAD `4333739799874ed98cf533079e9342fc2711734b`, anterior à criação deste catálogo.
>
> **Maturidade:** candidato a **pré-produção**, **não liberado** para uso fiscal real ou implantação definitiva. O PR segue aberto, sem merge. No último CI verificado (`37879908966`), os jobs `backend`, `frontend`, `integration`, `security`, `e2e` e `e2e-prodlike` falharam **antes de iniciar**, todos com `runner_id=0` e `steps=0`. Nenhum teste dessa execução prova aprovação.
>
> **Regra estrutural:** cada loja/CNPJ pertence a uma empresa **independente**, ainda que familiares sejam proprietários. Não modelar as lojas como filiais de uma única pessoa jurídica; contas compartilhadas entre CNPJs precisam de vínculos e autorizações separadas.

## Como ler e atualizar esta checklist

- `[x]` = **implementação localizada no código** da branch, com rota, serviço e/ou tela. **Não** significa teste executado, homologação ou prontidão de produção.
- `[ ]` = **pendência aberta**: funcionalidade não implementada, não identificada como completa na inspeção estática, ou etapa ainda dependente de implantação/validação.
- **Parcial** = existe fundamento/fluxo, mas faltam etapas essenciais descritas na mesma linha.
- **API apenas** = backend presente, sem interface final integrada no aplicativo verificada.
- **P0** = bloqueio de uso real/merge; **P1** = expansão operacional prioritária; **P2** = melhoria de produto; **P3** = expansão futura.
- Evidência primária: [roteador/API](../backend/internal/httpapi/router.go), [telas do app](../web/src/App.tsx), [API documentada](api.md), [expansões de UX](ux-expansion-app.md), [visão do projeto](../README.md), [validação histórica](production-validation-report.md), [CI](../.github/workflows/ci.yml).
- Em cada alteração futura: marcar o item **após inspeção do commit correspondente**; acrescentar link de PR, teste e evidência de execução ao liberar como validado. Não converter `[x]` em "pronto para produção" automaticamente.

## Visão geral por módulo

| Módulo | Itens com código identificado | Pendências e melhorias | Leitura |
|---|---:|---:|---|
| Empresas, contas e permissões | 12 | 6 | Administração multi-CNPJ parcial |
| Assistente inicial e proprietário | 7 | 5 | Guias prontos no código; implantação ainda assistida |
| Produtos | 10 | 5 | Catálogo e CSV; falta edição/exportação avançada |
| Estoque | 11 | 7 | Movimentos e CSV; falta inventário físico formal |
| PDV e caixa | 14 | 8 | Vendas e caixa; faltam integrações de hardware/pagamento |
| Compras e fornecedores | 8 | 5 | Fluxo operacional; faltam automatizações |
| Devoluções | 7 | 3 | Reembolso; falta troca inteiramente guiada |
| Financeiro | 7 | 7 | Ledger/conciliação; faltam crediário e integrações reais |
| Clientes | 5 | 6 | Agenda básica; falta relação completa com vendas |
| Fiscal/SEFAZ | 8 | 9 | Fundação técnica; produção **bloqueada** |
| Privacidade e segurança | 8 | 4 | APIs técnicas; validação externa e UX pendentes |
| PWA/periféricos | 5 | 5 | PWA e impressão do navegador; hardware não integrado |
| Infraestrutura e qualidade | 6 | 9 | Ferramentas prontas; evidências do SHA atual faltando |
| **Total de linhas das seções 1–13** | **108** | **79** | **Não equivale a percentual de produção** |

As seções P0–P3 contêm **22 linhas de ação adicionais**, que em
parte **repetem e priorizam** as 79 pendências acima. Não somar essas
linhas como novas funcionalidades independentes. As contagens são
itens editoriais desta checklist, não métricas de cobertura, de
qualidade de código ou de progresso validado.

## 1. Empresas, contas, funcionários e permissões

**Localização:** autenticação/API, `/staff` e `/accept-invite`. **Estado:** núcleo implementado; onboarding autônomo e alguns casos multiloja ainda parciais.

- [x] Cadastro e isolamento lógico de empresas por `tenant_id` / CNPJ.
- [x] Autenticação com usuário/senha e JWT de acesso.
- [x] Refresh token em cookie HttpOnly, rotação e logout confirmado pelo servidor.
- [x] RBAC contextual por empresa, permissões verificadas em rotas protegidas.
- [x] Revalidação de usuário/vínculo ativo ao usar tokens existentes.
- [x] Administração da equipe na própria loja: listar função e status.
- [x] Convite de 48h, token aleatório de uso único, armazenamento somente do hash.
- [x] Ativação com senha própria, não compartilhada pelo administrador.
- [x] Funções gerente/caixa, alteração de papel com confirmação.
- [x] Suspensão/reativação de vínculo **somente do CNPJ atual**, sem desligar usuário global.
- [x] Revogação de convite não utilizado e auditoria das mudanças.
- [x] Bloqueio de autoelevação e de mudanças de papel administrativo nessa UI.
- [ ] **Parcial:** convidar conta já existente em outro CNPJ mediante verificação segura da identidade; o convite atual restringe-se a novo e-mail.
- [ ] Bootstrap autônomo e seguro de novo dono/nova empresa; atualmente requer procedimento controlado.
- [ ] Recuperação de senha por e-mail ou outro mecanismo comprovado de posse.
- [ ] Envio automático de convites por canal verificado e reenvio seguro, evitando depender de copiar um link manualmente.
- [ ] Gestão de equipes maiores que 200 registros com paginação/filtros e desativação em lote.
- [ ] Teste de alteração/revogação concorrente de autorização durante operações em andamento.

**Melhorias:** convite acessível por e-mail verificado; permissões apresentadas em linguagem de lojista ("Pode dar descontos?", "Pode alterar estoque?"); trilha de mudanças por funcionário; vinculação entre contas existentes sem compartilhar dados dos CNPJs.

## 2. Configuração inicial e experiência do proprietário

**Localização:** `/home`, `/setup` e `/fiscal`.

- [x] Página inicial com atalhos adaptados às permissões.
- [x] Assistente em cinco etapas: empresa, produtos, estoque, equipe, NFC-e.
- [x] Verificação de evidências por APIs e indicação de etapas pendentes/não verificadas.
- [x] Botões de navegação anterior/próxima e revisão de etapas.
- [x] Revisões manuais persistentes por empresa para estoque/equipe, com auditoria.
- [x] Formulário guiado de dados fiscais do emitente e verificação da configuração.
- [x] Atalho para gestão de funcionários a partir do assistente, quando autorizado.
- [ ] **Parcial:** automação integral da ativação de uma loja sem ajuda técnica; provisionamento, segredos e ambiente seguem externos.
- [ ] **Parcial:** verificação de estoque físico; um registro de movimentação não prova inventário conferido.
- [ ] Ajuda contextual, tutoriais curtos e linguagem consistente para primeiro uso.
- [ ] Fluxo de checklist operacional final de abertura da loja com confirmação de caixa, impressora, backup e testes.
- [ ] Teste de usabilidade com donos/caixas sem experiência técnica.

**Melhorias:** sequência de preparação com ações claras e "corrigir agora"; recomendações sem mostrar jargões como tenant, secret-store e idempotência ao usuário comum.

## 3. Produtos e catálogo

**Localização:** `/products` e `/api/v1/products`.

- [x] Cadastro individual e atualização de produto, SKU e nome.
- [x] Preço de venda, preço promocional, custo e estoque mínimo.
- [x] Proteção RBAC para visualizar custo/margem; atualização não deve apagar custo oculto.
- [x] Busca no catálogo e consulta por código de barras.
- [x] Scanner de código de barras no PDV e restrição de duplicidade por empresa.
- [x] Dados fiscais por produto (NCM, CEST e perfil de preparação).
- [x] Ativação/inativação de produto.
- [x] Importação assistida de CSV com modelo, prévia e validação de separadores/decimais brasileiros.
- [x] Importação atômica com até 500 itens, idempotência, recuperação de resposta ambígua e rollback completo em conflito.
- [x] Histórico de lotes confirmados por empresa, paginação, datas e CSV de metadados.
- [ ] Edição em massa de produtos já cadastrados (preço, nome, categoria etc.) com revisão e rollback seguro.
- [ ] Catálogo de categorias/marcas e filtros completos com interface de gestão verificada ponta a ponta.
- [ ] Exportação estruturada de todo o cadastro de produtos **com níveis explícitos de permissão para preço e custo**.
- [ ] Etiquetas com código de barras/preço para prateleiras e balança, quando aplicável.
- [ ] Fotos de produtos, variações/unidades de embalagem e kits/composições.

**Melhorias:** validação de preço/custo em telas simples, diagnóstico de CSV linha a linha, comparação antes/depois da importação, importação de atualização sem recriar produtos.

## 4. Estoque, recebimentos e inventário físico

**Localização:** `/inventory` e `/stock-movements`.

- [x] Saldos por produto e empresa.
- [x] Aviso de estoque mínimo/baixo e contagem global.
- [x] Ajuste manual auditado com tipos restritos; compra/devolução não podem ser forjadas como ajuste.
- [x] Movimentações de entradas, saídas e ajustes no backend.
- [x] Página histórica somente leitura com SKU, produto, variação e saldo antes/depois.
- [x] Filtro de movimentações por produto e paginação limitada.
- [x] Carga inicial por CSV de até 100 SKUs por lote, prévia, revisão e idempotência.
- [x] Proteção contra segunda carga inicial sobre item já movimentado.
- [x] Histórico de cargas iniciais e exportação de metadados.
- [x] Histórico administrativo unificado de produtos/estoque inicial, com RBAC de cada fluxo.
- [x] Exportação CSV de todos os produtos e saldos da empresa até 5.000 itens, sem dados de custo.
- [ ] **Parcial:** conferência com o estoque físico. Não existe fluxo completo de sessão de inventário: contagem, diferença, aprovação e lançamento auditado reconciliado.
- [ ] Consulta/exportação segmentada para catálogo acima de 5.000 itens sem truncamento.
- [ ] Alertas configuráveis por família/produto com previsão de reposição e notificação operacional.
- [ ] Controle de lote, validade, perecíveis e PEPS/FEFO onde necessário.
- [ ] Reservas de estoque, transferência rastreada entre depósitos da **mesma empresa** (não entre CNPJs).
- [ ] Relatórios de perdas, rupturas, giro e cobertura de estoque.
- [ ] Reconciliar tentativas de importação sem recibo local, sem presumir sucesso/falha por erro de rede.

**Melhorias:** inventário guiado para celular, com leitor de código de barras, comparação física vs. sistema e confirmação dupla de ajustes.

## 5. PDV, vendas e operação de caixa

**Localização:** `/pdv`, `/sales`, `/cash`.

- [x] Abrir e fechar sessão de caixa.
- [x] Suprimento/sangria ou movimentação manual de caixa, com controles de permissão.
- [x] Conferência de valor esperado vs. valor declarado ao fechar, incluindo diferenças e métodos.
- [x] Carrinho de venda e busca/leitura de produtos.
- [x] Atalhos de teclado, modo scanner e busca rápida.
- [x] Cálculo de preços no servidor e revalidação do preço promocional.
- [x] Descontos controlados por permissões.
- [x] Suspender/retomar carrinhos, com reconsulta de preços e tratamento de alterações.
- [x] Finalizar venda e registrar métodos de pagamento.
- [x] Registrar `pix` como **forma informada** de pagamento; isso **não** representa integração real com PSP, QR dinâmico ou conciliação bancária automática.
- [x] Cancelar venda observando as proteções de devolução já vinculada.
- [x] Fila offline com idempotência, reconciliação manual, limites e proteção para não perder intencionalmente referências pendentes.
- [x] Cache de catálogo offline por usuário/CNPJ, com validade controlada.
- [x] Comprovante **não fiscal** usando impressão do navegador.
- [ ] Integração certificada com impressora térmica ESC/POS e perfis de modelos/papel.
- [ ] Acionamento de gaveta de dinheiro e teste físico do periférico.
- [ ] Interface completa de teclado de caixa, atalhos configuráveis e acessibilidade testada em terminais reais.
- [ ] Integração com balança comercial, quando aplicável.
- [ ] Conciliação automática de pagamentos por Pix e TEF com adquirente/PSP, webhooks e estorno seguro.
- [ ] Integração comprovada cliente–venda a partir da nova agenda.
- [ ] Teste de falha de energia, múltiplas abas, sincronização entre terminais e restauração do navegador com vendas offline.
- [ ] Emissão NFC-e vinculada à operação real (ver seção fiscal).

**Melhorias:** fluxo do caixa focado em poucos cliques, erros recuperáveis, destaque de pendência offline e modo tela cheia configurável.

## 6. Fornecedores, compras e recebimento

**Localização:** `/purchases`, `/suppliers`.

- [x] Cadastro, consulta e edição de fornecedores por empresa.
- [x] Registro de pedido/compra com fornecedor e itens.
- [x] Recebimento parcial e atualização de saldo/custo **na confirmação do recebimento**, não ao criar pedido.
- [x] Cancelamento de compra permitido sob regras de status.
- [x] Ligação opcional com contas a pagar no modelo existente.
- [x] Permissões dedicadas para leitura, escrita e recebimento.
- [x] Rejeição de produtos e fornecedores inativos onde aplicável.
- [x] Auditoria transacional de criar, receber e cancelar.
- [ ] **Parcial:** rotina financeira completa de contas a pagar, vencimento, baixa, conciliação e contas vencidas em uma experiência simples.
- [ ] Importação assistida de XML de fornecedor (NF-e) para pré-preencher compra, com conferência fiscal/estoque e sem lançamento cego.
- [ ] Sugestão de reposição baseada em estoque mínimo, giro e prazo do fornecedor.
- [ ] Histórico analítico de preços de compra e comparação de fornecedores.
- [ ] Fluxo de devolução a fornecedor, com aprovação e movimentações específicas.

**Melhorias:** "receber mercadorias" guiado por SKU e quantidade, sem obrigar reentrada manual de cada item.

## 7. Devoluções, trocas e reembolsos

**Localização:** `/returns`, `/sales/{id}/returns` e financeiro.

- [x] Devolução parcial/total com limite das quantidades originalmente vendidas.
- [x] Distinção entre retorno ao estoque vendável e item sem reposição (avaria etc.).
- [x] Troca tratada como devolução mais nova venda de substituição.
- [x] Cálculo no servidor do valor potencial de reembolso.
- [x] Status financeiro de reembolso pendente e liquidação controlada.
- [x] Liquidação de reembolsos em múltiplos meios e prevenção de repetição idempotente.
- [x] Proteção contra cancelar venda que já possui devolução.
- [ ] Fluxo de troca inteiramente guiado em uma tela, com diferença a pagar/devolver.
- [ ] Impressão de comprovante de troca/devolução adaptado ao contexto comercial.
- [ ] Relatório de causas de devoluções, avarias e impacto financeiro/estoque.

**Melhorias:** orientações claras sobre quando reestocar, cancelar ou reembolsar; evitar compensações financeiras automáticas sem confirmação.

## 8. Financeiro e conciliação

**Localização:** `/finance`, `/home` e endpoints `/finance/*`.

- [x] Livro financeiro (`ledger`) e dashboard por período.
- [x] Resumo da loja e exportação CSV de indicadores exibidos.
- [x] Registro de pagamentos associados a vendas.
- [x] Conciliação inicial de pagamento digital, imutável e idempotente.
- [x] Ajuste posterior de conciliação com justificativa, ator e histórico auditado.
- [x] Reembolsos e conciliação do fechamento de caixa por forma de pagamento.
- [x] Ranking por subtotal de produtos de vendas finalizadas, por período e CNPJ.
- [ ] **Parcial:** relatório de receita **líquida** e lucro contábil; ranking atual não abate devoluções, taxas, tributos e despesas.
- [ ] Relatórios por operador, turno, meio de pagamento, margem autorizada e períodos comparáveis.
- [ ] Fluxo completo de contas a pagar e a receber com liquidações, juros e multas.
- [ ] Crediário com cliente, limite, vencimentos, parcelas, inadimplência, pagamento e estorno **auditáveis**.
- [ ] Pix dinâmico com confirmação de recebimento por PSP e conciliação real.
- [ ] TEF/maquininhas com comprovante, falhas, cancelamento e conciliação real.
- [ ] Conexão com extratos/importação OFX/CSV para conciliação bancária, sem lançamentos presumidos.

**Melhorias:** apresentar "valor vendido", "recebido", "pendente", "reembolsado" e "lucro estimado" separadamente, sem induzir erro contábil.

## 9. Clientes e relacionamento

**Localização:** `/customers`.

- [x] Diretório de clientes separado por CNPJ.
- [x] Listagem paginada e busca literal por nome.
- [x] Cadastro e edição de nome, telefone e e-mail opcionais.
- [x] Permissões `customer:read` e `customer:write` para gerente/admin; sem acesso automático do caixa.
- [x] Auditoria transacional e verificação de vínculo ativo para alterar contatos.
- [ ] **Parcial:** histórico de compras por cliente: a agenda ainda não está conectada plenamente às vendas.
- [ ] Cadastro opcional de documento/dados fiscais com regras estritas de privacidade quando houver necessidade legal.
- [ ] Exclusão/anomização/autorização de uso comercial integrada à jornada do contato, conforme política LGPD.
- [ ] Crediário e contas a receber (dependem do módulo financeiro seguro).
- [ ] Regras de comunicação com consentimento/opt-out e trilha de alterações.
- [ ] Importação e deduplicação assistida de clientes sob a empresa atual.

**Melhorias:** formulário mínimo por padrão, dados fiscais opcionais e justificativa de finalidade antes de armazenar informações adicionais.

## 10. NFC-e, documentos e SEFAZ

**Localização:** `/fiscal` e módulo fiscal. **Estado:** fundação extensa implementada; **produção não homologada**.

- [x] Preparação de dados da empresa emissora, endereço, IE/CRT, município IBGE.
- [x] Campos de NCM/CEST e preparação fiscal de produtos.
- [x] Configuração NFC-e modelo 65 por empresa, série, ambiente e referências a segredos/certificado.
- [x] Consulta de prontidão, com lista de pendências.
- [x] Prévia XML de desenvolvimento e histórico/download de XML.
- [x] Código de construção/validação de XML e assinatura A1 sob configuração externa.
- [x] Rotas e infraestrutura técnicas para reserva, contingência, inutilização, autorização/cancelamento em homologação, estados de retorno e DANFE.
- [x] Restrições explícitas de acesso, ambiente e transmissão; a UI de configuração **não** promete emissão ao digitar os dados.
- [ ] Homologação efetiva em ambiente SEFAZ para cada UF/CNPJ e certificados/CSC/QR válidos e atuais.
- [ ] Comprovação de compatibilidade com schemas, NTs e regras tributárias **vigentes** no ambiente de destino, incluindo mudanças de 2026.
- [ ] Emissão de NFC-e de produção comprovada ponta a ponta: venda → XML → assinatura → envio → protocolo → DANFE.
- [ ] Rejeições, retransmissão/consulta por chave, indisponibilidade e estados ambíguos comprovados em condições reais.
- [ ] Cancelamento, contingência, inutilização e reconciliação de documentos **homologados** ponta a ponta.
- [ ] Interface operacional de documentos fiscais com pesquisa, status, alertas e orientações de correção, sem autorizar reenvio indevido.
- [ ] Governança de certificado/segredos: rotação, expiração, responsável e recuperação operacional.
- [ ] Revisão fiscal/contábil independente e liberação controlada por empresa.
- [ ] Considerar NF-e modelo 55, se o negócio vier a precisar: **não confundir** o preview existente com emissor modelo 55 pronto.

**Melhorias:** assistente simples com validações passo a passo, pré-preenchimento seguro, estados claros e botões de suporte — mantendo a autorização da SEFAZ como requisito incontornável.

## 11. Privacidade, auditoria e segurança

**Localização:** módulos `privacy`, `audit`, middleware e `/privacy/*`.

- [x] Registros de auditoria com requisição, ator e metadados sanitizados.
- [x] API de consulta de auditoria por empresa e permissão.
- [x] Fluxos técnicos de solicitações de titulares: criar, consultar, atualizar status.
- [x] Exportação, bloqueio e anonimização suportados para tipos de titulares, com retenções legais respeitadas.
- [x] Registro e revogação de consentimentos.
- [x] Rate limiting em rotas sensíveis e proteção de métricas.
- [x] Cabeçalhos de segurança, origens confiáveis para operações sensíveis, CORS configurável.
- [x] Logs estruturados, IDs de requisição e erros HTTP padronizados sem revelar segredos.
- [ ] **Parcial:** painel visual para gestão de privacidade e auditoria; endpoints existem, mas a navegação principal não exibe uma tela dedicada verificada.
- [ ] Política de retenção/anonimização revisada por jurídico/DPO e comprovada no ambiente final.
- [ ] Teste de intrusão independente, rotação de credenciais, varredura de dependências e revisão atualizada de acessos.
- [ ] Revisão de acessibilidade e exposição acidental de dados pessoais em todas as novas telas.

**Melhorias:** trilha administrativa visual e exportação segura sob permissão, filtros por incidente, guardas explícitas para operações destrutivas.

## 12. PWA, experiência em dispositivos e periféricos

- [x] Interface web responsiva React + TypeScript.
- [x] Manifesto, ícones, service worker e instruções de instalação como PWA.
- [x] Arquivos estáticos versionados tratados pelo service worker, sem cache de respostas fiscais/API.
- [x] Fila de vendas offline do navegador com proteção de referências até reconciliação.
- [x] Impressão **via navegador** de comprovante não fiscal.
- [ ] Aplicativo nativo Android/iOS publicado e assinado nas lojas oficiais.
- [ ] Uso validado em dispositivos reais (Android, iOS, desktop e diferentes navegadores).
- [ ] Integração direta ESC/POS com impressoras, gavetas e balanças.
- [ ] Sincronização multi-terminal tolerante a conflitos e quedas de rede.
- [ ] Testes de acessibilidade, contraste, teclado, leitores de tela e eficiência sob carga real.

**Melhorias:** manter PWA como caminho principal inicialmente; criar app nativo apenas se requisitos de hardware, distribuição, notificações ou trabalho offline justificarem.

## 13. Infraestrutura, qualidade e prontidão operacional

- [x] Backend Go, PostgreSQL, Redis e Docker Compose para desenvolvimento.
- [x] Migrations versionadas; últimas expansões incluem `0033` (equipe) e `0034` (clientes).
- [x] Testes Go, integração PostgreSQL, Playwright e checks de segurança **escritos no repositório**.
- [x] Pipeline com seis grupos de jobs: backend, frontend, integração, segurança, E2E e E2E em configuração similar à produção.
- [x] Scripts/runbooks de implantação, migração, backup/restore e observabilidade.
- [x] Health/readiness, métricas Prometheus protegidas e rastreabilidade técnica.
- [ ] **P0:** executar todos os jobs do CI com runners efetivos no SHA final do PR, incluindo lint/build e teste de regressão das migrations `0033`/`0034`.
- [ ] **P0:** corrigir quaisquer falhas reais de compilação, migração, segurança ou E2E encontradas nessas execuções.
- [ ] **P0:** evidenciar backup/restauração e rollback seguro no ambiente real do piloto; uma documentação de comandos não substitui ensaio realizado.
- [ ] **P0:** validar recuperação de PostgreSQL/Redis, TTL e filas offline sob falhas reais e carga delimitada.
- [ ] **P0:** validar hardware, permissões e jornada completa com atendentes e responsáveis de cada CNPJ.
- [ ] **P0:** homologar fiscalmente antes de ativar transmissão de produção.
- [ ] Licença, política de distribuição, termos, suporte e processo de atualização para usuários externos.
- [ ] Relatório de prontidão atualizado para o SHA atual: documentos antigos ainda mencionam schema 23/32 e precisam ser reconciliados com schema 34.
- [ ] Plano de migração progressiva e monitoramento por cliente/loja sem compartilhamento indevido de dados.

**Nota histórica:** houve CI verde em commits antigos de `main`; esses resultados **não** validam os novos commits do PR #15.

## 14. Priorização: o que fazer a seguir

### P0 — fechamento da integração e segurança (antes de expandir)

- [ ] Resolver disponibilidade de executor CI e rodar pipeline completo **na cabeça atual**.
- [ ] Rodar Go `gofmt`, `go vet` e `go test` com integração PostgreSQL.
- [ ] Rodar `npm ci`, `npm run lint`, `npm run build` e Playwright (com HTTPS/ambiente similar ao real).
- [ ] Migrar do zero até schema 34 e simular upgrade/rollback/reapply com dados de teste e vínculos desativados.
- [ ] Fazer auditoria E2E de login, CNPJ A/B, convites, clientes, PDV, estoque, compras, devoluções e financeiro.
- [ ] Definir/aprovar critérios de merge. **Não fazer merge automaticamente.**

### P1 — maior impacto na operação diária

- [ ] Inventário físico com sessão de contagem, divergências, aprovação e ajuste auditado.
- [ ] Integração entre cadastro de clientes e vendas (sem ativar crediário inadvertidamente).
- [ ] Contas a receber/crediário com regras de negócio, testes de liquidação e recuperação.
- [ ] Contas a pagar com vencimentos e baixas assistidas.
- [ ] Impressão térmica não fiscal, configurações de periféricos e teste do caixa real.
- [ ] Monitoramento de pendências de estoque, vendas offline e divergências financeiras.

### P2 — facilidade e inteligência do lojista

- [ ] Relatórios avançados de vendas, perdas, estoque, rentabilidade com definições contábeis precisas.
- [ ] Importação de XML de compra e sugestões de reposição.
- [ ] Onboarding/autosserviço assistido com suporte para usuário não técnico.
- [ ] Exportação/atualização em massa de catálogo e estoque sem truncamento.
- [ ] Agenda de clientes com deduplicação, vínculo de compras e tratamento LGPD.
- [ ] Painéis administrativos de auditoria, privacidade e operações fiscais.

### P3 — integrações externas e expansão de mercado

- [ ] Pix dinâmico e TEF reais com prestadores habilitados e homologação.
- [ ] Homologação SEFAZ de produção por UF/CNPJ; não antecipar por pressão de UX.
- [ ] Aplicativo nativo somente se houver necessidade comprovada além da PWA.
- [ ] E-commerce/omnichannel e conectores contábeis, após estabilidade do PDV e estoques.

## Critérios para mover um item de "implementado no código" para "validado"

Um item só pode ser apresentado como **validado** quando existirem, no mínimo:

1. Link para o commit/PR com backend, interface e migration necessários.
2. Teste automatizado executado em SHA correspondente, com resultado aprovado e logs.
3. Teste negativo de RBAC e isolamento entre **dois CNPJs** quando a função manipula dados de empresa.
4. Testes de entradas inválidas, idempotência/concorrência e recuperação quando houver escrita.
5. Teste manual no fluxo real, com documentação de evidências, para operação física/fiscal.
6. Aprovação externa para tributos, certificado, SEFAZ, privacidade ou pagamento quando aplicável.

### Quadro de atualização contínua

| Data | Entrega/área | Referência | Código | Testes executados | Piloto | Bloqueios |
|---|---|---|---|---|---|---|
| 2026-10-09 | Catálogo de estado do sistema | PR #15 / HEAD acima | Inventariado | **Não**: runner 0 / steps 0 | Não | CI, migrations 33–34, homologação fiscal e operação real |

**Observação final:** nenhum item marcado `[x]` neste documento, isoladamente, autoriza emissão de documento fiscal, movimentação bancária real, merge ou implantação em produção.
