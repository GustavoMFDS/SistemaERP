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
