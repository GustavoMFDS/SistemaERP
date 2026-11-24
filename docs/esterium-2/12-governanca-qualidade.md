# Governança, qualidade e documentação de engenharia

## Processo
1. Definir problema real, comportamento existente e critérios de aceite; obter decisão de produto quando necessário.
2. Issue/ID -> branch dedicada baseada em SHA conhecido -> PR pequena, sem misturar refatoração gratuita.
3. Implementar testes antes ou junto da mudança; explicar arquitetura, fluxo de execução, arquivos/funções, invariantes, schema, API, UI e dependências.
4. Executar validações adequadas no HEAD final, registrar comandos, ambiente, SHA, run, resultados, falhas, skips e limitações. Nunca ocultar falha, usar sucesso de outro commit ou apagar dados para fazer teste passar.
5. Revisar diff, segurança, regressão, migração, recuperação e documentação; só integrar após autorização e gates aplicáveis. Fechar PR substituída apenas com equivalência demonstrada.
6. Atualizar catálogo, roadmap e changelog com resultado efetivo. Registrar decisão técnica em ADR: contexto, alternativas, decisão, tradeoffs, riscos, revisão.

## Matriz mínima de testes
| Camada | Cobertura |
|---|---|
| Domínio | Invariantes, valores, estados, limites, arredondamento |
| Integração | PostgreSQL/Redis descartáveis, transações, locking, migrações, reruns |
| API | Contrato, tenant/RBAC, erro, idempotência, paginação |
| UI | Fluxos reais e mock claramente distintos, acessibilidade e responsividade |
| Concorrência/resiliência | Última unidade, replay, timeout, offline, reinício, jobs |
| Segurança | Dependências, segredos, permissões, TLS, exportação, logs, dados sensíveis |
| Instalação | Windows limpo, assinatura, backup, reparo, atualização e recuperação |
| Operação externa | Periféricos, LAN física, fiscal real autorizado, aceite humano |

Comandos base: gofmt -l ., go test ./..., go vet ./..., govulncheck, integração conforme README, npm ci, npm run lint, npm run build, npm audit, Playwright conforme configuração. Não assumir disponibilidade de runner ou executar em banco real. Repetir suítes críticas em bases descartáveis para detectar resíduos. Fiscal em teste com transmissão desligada salvo homologação explicitamente autorizada. Gates de segurança conforme workflows e versão.

## Definition of Done
Código e documentação versionados; critérios verificáveis satisfeitos; testes positivos/negativos e isolamento; logs saneados; evidência rastreável; nenhum bloqueio crítico omitido. Para UI, evidência visual e feedback real quando aplicável. Para operação, prova física/externa separada; para analytics, reconciliação; para IA, baseline/backtest e explicação. Sem aprovação de produção implícita.

## Modelo de relatório
ID | SHA/base | objetivo | arquivos e explicação do código | comandos/resultados | evidências | falhas/skips | riscos/limites | decisão | próximo passo. Em repositório público, não incluir dados da família, chaves, certificados, dumps, extratos ou informações pessoais.
