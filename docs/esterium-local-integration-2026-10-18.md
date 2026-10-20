# Esterium Local — ordem de integração e aceite (18/10/2026)

## Situação técnica

Esta folha descreve a sequência **proposta**, não uma aprovação de distribuição. A branch `main` permanece separada da cadeia draft. CI verde valida fixtures automatizadas, não instala uma loja real nem libera emissão fiscal. Sem impressoras, leitores ou dados reais neste ciclo de desenvolvimento.

| Ordem | PR | Entrega isolada | Gate de integração |
| --- | --- | --- | --- |
| 1 | [#95](https://github.com/GustavoMFDS/SistemaERP/pull/95) | Instalador **apenas do programa**, plano padrão e aplicação administrativa restrita a pacotes assinados | CI completo e pacote aprovado, sem sobrescrever instalação |
| 2 | [#96](https://github.com/GustavoMFDS/SistemaERP/pull/96) | Refinamento parcial de linguagem de Início, Produtos, Estoque, Movimentações, Financeiro e Relatórios | Navegação, permissões, mensagens e E2E sem regressões |
| 3 | [#97](https://github.com/GustavoMFDS/SistemaERP/pull/97) | Credenciais não legíveis por outras contas nem graváveis por LocalService | Smoke Windows SCM e ACL sintética aprovados |
| 4 | [#98](https://github.com/GustavoMFDS/SistemaERP/pull/98) | Atualização **somente de arquivos** com retorno preservado; migração SQL adicional proibida | Retorno em ambiente sintético, serviço parado e backup externo válido |
| 5 | [#99](https://github.com/GustavoMFDS/SistemaERP/pull/99) | Verificação opcional de duas gerações locais e externas de backup | Fixtures de arquivo fresco, antigo, corrompido e ausente |
| 6 | [#100](https://github.com/GustavoMFDS/SistemaERP/pull/100) | Gate que recusa release sem assinatura e hash independentemente aprovado | Assinaturas válidas de EXE/scripts, editor confiável e manifesto aprovado |


### Continuação da cadeia de instalação local (19/10/2026)

| PR | Dependência de base | Entrega | Gate ainda necessário |
| --- | --- | --- | --- |
| [#101](https://github.com/GustavoMFDS/SistemaERP/pull/101) | #100 | Inventário SHA-256 e origem dos instaladores offline PostgreSQL/Garnet | Aprovação independente dos fornecedores e assinaturas |
| [#102](https://github.com/GustavoMFDS/SistemaERP/pull/102) | #101 | Processos Windows verdadeiros nas portas PostgreSQL/Garnet | Testes com serviços reais descartáveis |
| [#103](https://github.com/GustavoMFDS/SistemaERP/pull/103) | #102 | Linguagem simples de compras, devoluções e importações | Revisão completa das 19 telas |
| [#104](https://github.com/GustavoMFDS/SistemaERP/pull/104) | #103 | Rejeição de ZIPs terceiros perigosos | Fonte aprovada, sem extração automática |
| [#105](https://github.com/GustavoMFDS/SistemaERP/pull/105) | #104 | Assinatura antes de delegar scripts privilegiados | Launcher confiável antes de iniciar execução |
| [#106](https://github.com/GustavoMFDS/SistemaERP/pull/106) | #105 | Configuração protegida Garnet TLS/ACL/Lua, sem serviço | Compatibilidade real e identidade TLS/ACL |
| [#107](https://github.com/GustavoMFDS/SistemaERP/pull/107) | #106 | Opções offline EDB PostgreSQL 16 em pasta protegida | Execução EDB/conta/TLS em Windows descartável |
| [#108](https://github.com/GustavoMFDS/SistemaERP/pull/108) | #107 | Testes independentes de assinatura Authenticode | CI e revisão de diferença **em ramo irmão** |
| [#109](https://github.com/GustavoMFDS/SistemaERP/pull/109) | #107 | Preparação conjunta dos arquivos PostgreSQL e Garnet | CI verde e install real ainda não realizado |
| [#110](https://github.com/GustavoMFDS/SistemaERP/pull/110) | #109 | Verificação estática de PostgreSQL TLS/loopback/SCRAM | Inspecionar política ativa com banco de teste |

**Atenção à bifurcação:** #108 e #109 partiram de #107. **Não** presumir que um está contido no outro nem juntá-los automaticamente. Reconciliar e revisar ambas as diferenças, depois refazer CI na base escolhida antes de integrar à `main`. `#110` depende de `#109`, não de `#108`.


### Como integrar futuramente

1. Resolver a cadeia anterior **#25–#94** e suas dependências, conferindo HEAD, base, versões SQL, mergeability, CI e conflitos reais, sem pular para a branch mais recente.
2. Integrar cada PR em sua base revisada **na ordem**, somente depois de todas as verificações do SHA final e revisão de segurança. Revalidar a próxima branch contra a base resultante, sem merges de agent para agent que escondam mudanças.
3. Não executar migrações em bancos existentes sem verificação de recuperação; o atualizador de arquivos **falha fechado** diante de qualquer migração SQL alterada/adicionada.
4. Não gerar release comercial a partir do ZIP técnico do CI: faltam assinatura real, distribuição aprovada e provisionamento automatizado/offline e seguro de PostgreSQL 16 e Redis/Garnet.
5. Manter transmissão SEFAZ desabilitada e mensagens da interface precisas; funcionalidades indisponíveis não aparecem como concluídas para operador.

### Pendências intencionalmente fora deste ciclo

- Integração física de impressoras, leitores e periféricos; testes em loja/com dados reais.
- Emissão oficial NFC-e/NF-e, certificado e protocolo SEFAZ autênticos (issues [#32](https://github.com/GustavoMFDS/SistemaERP/issues/32), [#33](https://github.com/GustavoMFDS/SistemaERP/issues/33)).

### Requisitos ainda não entregues

- Instalador unificado de terceiros: **PostgreSQL e Redis/Garnet** com origem verificável, versão aprovada, TLS, privilégios mínimos, serviço e backup sob operação autônoma no Windows. O instalador #95 só registra o **Esterium** se esses serviços já existirem.
- Assinatura Authenticode real dos arquivos distribuídos; o gate #100 foi desenhado para **recusar o pacote atual**.
- Atualização de bancos existentes com migrações e rollback ensaiados; #98 atualiza **só código/UI**.
- Exercício de restauração em outro computador de uma cópia independente, usando banco **inteiramente sintético**, e estratégia de alerta/recuperação de falhas do agendamento.
- Aceite conclusivo das **19 telas**; #96 é um refinamento parcial, a [issue #43](https://github.com/GustavoMFDS/SistemaERP/issues/43) continua aberta.

**Proibido declarar pronto para produção com base apenas neste checklist ou em CI sintético.**

---

## Continuação e gates de integração — 19/10/2026

> Este registro descreve validações **somente de CI/revisão**, não o aceite para instalação em loja. As PRs continuam *draft* e a `main` não foi alterada.

### Cadeia não fiscal revisada

| PR | Escopo | Evidência/limite |
| --- | --- | --- |
| [#111](https://github.com/GustavoMFDS/SistemaERP/pull/111) | Teste de identidade de signatário e criptografia CMS em Windows descartável | 16/16 CI; **não** comprova assinatura comercial Authenticode confiável |
| [#112–#116](https://github.com/GustavoMFDS/SistemaERP/pull/116) | PostgreSQL `auto.conf`, revalidação do atualizador e linguagem das telas | Integração #116: 16/16 CI |
| [#117–#119](https://github.com/GustavoMFDS/SistemaERP/pull/119) | Código de lotes recolhido e proteção contra certificados, segredos e backups no pacote | Integração #119: 16/16 CI |
| [#120–#121](https://github.com/GustavoMFDS/SistemaERP/pull/121) | Lista restrita de arquivos públicos `web/` e teste conjunto da assinatura | #121: 16/16 CI |
| [#124](https://github.com/GustavoMFDS/SistemaERP/pull/124) | Autenticidade obrigatória de todos os helpers PowerShell executados com privilégios | 16/16 CI; assinatura comercial continua pendente |
| [#125](https://github.com/GustavoMFDS/SistemaERP/pull/125) | PDV aceita centavos com vírgula ou ponto e rejeita valores ambíguos | 16/16 CI, inclusive Playwright; sem pagamento bancário real |
| [#126](https://github.com/GustavoMFDS/SistemaERP/pull/126) | ACL estrita para configuração e senha do backup agendado | Testes Windows sintéticos, aguardando aceite integral do SHA final |

As #108 e #109 nasceram de bases irmãs. A cobertura válida da #108 foi recriada **com limitações explícitas** na #111 e agregada à #121; não assumir que um teste de CMS substitui o teste de um editor Authenticode genuinamente confiável.

### Integração da base histórica — bloqueio real

A ponta #121 até `main` depende de **60 PRs empilhadas**, incluindo #15 e #25. Não confundir CI verde no topo com aprovação de cada etapa antiga.

- **#15:** rerun em 19/10 evidenciou 10 arquivos Go com formatação divergente, alertas `govulncheck` e dezenas de testes E2E antigos falhos; os jobs não são todos aprovados.
- **#122:** formatação `gofmt` exata aplicada em branch separada da HEAD de #15; backend aprovado, sem alegação de release.
- **#123:** Go 1.26.9 e dependências da ponta aplicados em branch sobre #122; backend e segurança aprovados, mas o E2E legado terminou com **42 falhas, 71 aprovados e um ignorado**. A base continua bloqueada.
- **#25:** erros próprios de backend, vulnerabilidades e E2E permanecem históricos; não resolvidos por CI da branch descendente.
- Comparação do último ramo de implementação com `main`: **2.330 commits à frente** e pelo menos 300 arquivos na listagem limitada da API; não efetuar merge sem plano de revisão para migrations, permissões, tenants e auditoria.

Referências de controle: issues [#52](https://github.com/GustavoMFDS/SistemaERP/issues/52), [#43](https://github.com/GustavoMFDS/SistemaERP/issues/43) e [#44](https://github.com/GustavoMFDS/SistemaERP/issues/44).

### Ordem de fechamento antes de distribuir

1. Resolver estratégia de integração e E2E das bases **#15 / #25**, sem apagar os avisos históricos nem substituir a revisão por um squash sem aceite.
2. Instalar **PostgreSQL e Garnet** via pacotes oficiais independemente aprovados, com serviço/hospedeiro adequado, contas restritas, TLS, credenciais, reinício e desinstalação conservadora testados em Windows descartável.
3. Disponibilizar upgrade com **migrações SQL + backup/restauração e rollback compatíveis**, separadamente do atualizador atual que **proíbe alteração de SQL**.
4. Assinar o programa/instalador com certificado comercial genuinamente aprovado; validar publisher e distribuição no PC limpo.
5. Concluir aceite visual e funcional das 19 telas (funções de administrador, gerente e caixa) e testes sintéticos de recuperação em computador distinto.

Continuam deliberadamente fora desta etapa: emissão fiscal/credenciamento SEFAZ real, leitores, impressoras, dados reais de empresas e homologação presencial com dois caixas.

**CI verde = evidência de testes automatizados, não homologação de produção, fiscal ou operação física.**
