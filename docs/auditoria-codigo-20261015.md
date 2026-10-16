# Auditoria de manutenção: arquivos obsoletos e código morto — 15/10/2026

**Escopo:** branch de limpeza baseada no HEAD `715a5b01900a2cfec2a1540b5eef54ccb72a6a95` do PR [#25](https://github.com/GustavoMFDS/SistemaERP/pull/25), que contém os incrementos recentes de catálogo e fiscal. **Não foi feita sobre a `main` antiga.** O inventário inicial dessa branch continha **515 arquivos versionados** (blobs Git; diretórios não incluídos).

## Método e grau de confiança

1. Inspeção da árvore Git, pontos de entrada `web/src/main.tsx` e `web/src/App.tsx`, rotas, componentes, biblioteca frontend, `web/index.html`, manifesto da PWA e service worker.
2. Confronto de referências/importações dos estilos, imagens e helpers candidatos. Os arquivos de demonstração Vite não são importados ou referenciados pelo aplicativo inspecionado. O sprite de ícones de template também não é chamado pela UI inspecionada; os ícones da PWA são **outros** arquivos e foram preservados.
3. Revisão do README, `.env.example`, Docker Compose e `backend/internal/config/config.go` para verificar inicialização real e coerência da conexão PostgreSQL.
4. Triagem conservadora de módulos Go, fiscal, segredos, migrações, testes, PWA e histórico: somente remover itens com evidência forte de desuso. Exportação de função, cobertura de teste incompleta, feature flag desligada ou arquivo histórico de migração **não bastam** para classificar algo como morto.

**Limite da primeira rodada:** a inspeção inicial foi estática. **Na segunda rodada foi criado um clone isolado no dispositivo conectado** e executados os comandos de testes e análise indicados adiante. Esses resultados são anteriores ao commit desta segunda rodada e devem ser reexecutados no SHA final. E2E de navegador com backend/banco, migrações reais e SEFAZ permanecem sem validação nesta limpeza.

## Removidos: arquivos sem uso operacional identificado

| Caminho | Motivo |
| --- | --- |
| `web/src/App.css` | CSS restante da página inicial demonstrativa do Vite, não importado |
| `web/src/assets/react.svg` | Logo demonstrativo do React, sem referência na UI |
| `web/src/assets/vite.svg` | Logo demonstrativo do Vite, sem referência na UI |
| `web/src/assets/hero.png` | Ilustração demonstrativa, sem referência na UI |
| `web/public/icons.svg` | Sprite social/documentação do template, não usado pela UI, HTML ou manifesto |
| `prompt.md` | Prompt original de geração do projeto, não documentação operacional; o histórico permanece no Git |
| `Roadmap.md` | Planejamento inicial em formato de prompt, duplicado/desatualizado perante `docs/roadmap.md` e o catálogo funcional |

**Código TypeScript efetivamente removido:** `clearOfflineQueue()` (`web/src/lib/offlineQueue.ts`) e `clearSuspendedCarts()` (`web/src/lib/suspendedCart.ts`): helpers exportados mas não importados/chamados pelos consumidores do frontend analisados. A limpeza irrestrita de filas sem conciliação seria perigosa; os controles existentes por item no PDV foram mantidos. A documentação de modo offline foi atualizada para refletir isso.

## Corrigidos

- `README.md`: identificação do repositório, estado dos PRs, comandos, ambientes, funcionalidades e limites fiscais reais.
- `web/README.md`: versão Node, comando reprodutível `npm ci`, frontend/PWA e testes E2E.
- `.env.example`: variáveis `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` e `POSTGRES_PORT` alinhadas à `DATABASE_URL` e ao Docker Compose.
- Instrução de arranque Go: `config.LoadFromEnv` exige variáveis no **ambiente do processo**; somente copiar `.env` não era suficiente.

## Explicitamente preservados

- **Migrações SQL `up/down` e seeds:** apaga-las por antiguidade pode quebrar instalações existentes, testes, restores e rastreabilidade. A seed de demo permanece separada da seed de produção.
- **MVP fiscal, provedor SEFAZ, XML, assinatura, esquema, geração de DANFE e filas de acompanhamento:** caminhos desligados por configuração/homologação **não** são código morto; remover pode quebrar testes ou integrações controladas.
- **Chaves de cache/offline com prefixo `sistemaemgo`, identificadores JWT e módulo Go:** a identidade legada tem implicações de migração e não deve ser trocada pela simples alteração do README. Há PR próprio de rebranding.
- **`web/public/favicon.svg`, `app-icon.svg`, `icon-192.png`, `icon-512.png`, `manifest.webmanifest`, `sw.js`:** referenciados pelo HTML/PWA ou pelo registro do service worker.
- **Testes E2E, `.github/workflows/ci.yml`, relatórios e `docs/roadmap.md`:** evidência histórica, regras de validação e plano de produto. Falta de runner não equivale a falta de utilidade.

## Riscos e verificações pendentes

1. Executar no **HEAD final do PR**: `(cd backend && go test ./... && go vet ./...)`, `(cd web && npm ci && npm run lint && npm run build)`. Verificar `gofmt -l`, `go mod tidy` **sem diff** e lockfile sem alteração espúria.
2. Executar cenários selecionados de UI/E2E e testes reais de banco descartável: venda, caixa, fila offline, produtos/fotos, estoque, compras, autorização/consulta fiscal **somente** onde existir ambiente de homologação autorizado.
3. Usar `staticcheck`/`unused` em Go com versão compatível e revisar cada achado contra referências por reflection, testes, flags e integrações futuras. Não excluir símbolos Go ou dependências apenas por contagem de texto; `go mod tidy` deve ser revisado antes de aceitar mudanças.
4. Verificar alterações concorrentes no PR #25 (e PR de rebranding #27) antes do merge; esta limpeza é **dependente da branch de fotos** e não deve entrar diretamente na `main` antes de sua base.
5. Não declarar emissão fiscal em produção, testagem end-to-end completa ou CI verde com base neste relatório.

**Resultado:** remoções de baixo risco e correções documentais aplicadas em branch isolada. A auditoria de código morto mais profunda do backend permanece dependente de compilação/análise estática e regressão real para evitar perda de comportamento.

## Segunda rodada — verificações executadas em clone isolado (15/10/2026)

Clone temporário Windows, HEAD inicial do PR #34: `415caddb249c1ce31d2f918953aeaab876d2dd87`, Go **1.26.2**, Node **24.11.0** e npm **11.6.1** (ambiente de execução diferente do alvo Go 1.25/Node 22 do CI).

| Comando | Resultado |
| --- | --- |
| `go test ./...` | **Aprovado** (exit 0); testes unitários, sem banco real |
| `go vet ./...` | **Aprovado** (exit 0) |
| `npm ci --no-audit --no-fund` | **Aprovado** |
| `npm run lint` | **Aprovado com 12 avisos**, dos quais 2 eram comentários `eslint-disable` sem efeito |
| `npm run build` | **Aprovado** |
| `go run honnef.co/go/tools/cmd/staticcheck@latest ./...` | 2 tipos não usados (`refreshRequest`, `eventEnvelopeXML`) + 1 melhoria de conversão no mapeamento de pagamento; corrigidos neste commit |
| `go run golang.org/x/tools/cmd/deadcode@latest -test ./...` | 28 funções/métodos reportados como inalcançáveis, classificados antes de remover |
| `GOFLAGS=-tags=integration go test -run '^$' ./tests/integration/...` | **Compilação aprovada**, execução dos testes de banco não realizada |
| `GOFLAGS=-tags=integration go run golang.org/x/tools/cmd/deadcode@latest -test ./...` | Repetiu a lista de candidatos; `NewFiscalService` é **mantido** por ter referências explícitas em testes marcados `integration` |
| `npx --yes knip@latest --no-progress` | 3 tipos exportados apenas localmente (removidos os `export`); falso positivo `public/sw.js` (registrado na PWA), imports E2E dinâmicos não resolvidos; **não** eliminar esses arquivos |

Os 239 caminhos listados por `gofmt -l .` no clone Windows resultaram do `core.autocrlf=true` global e conversão CRLF no checkout: o diff de `gofmt -d` mostrou apenas alteração de fim de linha para o exemplo inspecionado. Não fazer reformatação em massa por esse diagnóstico. O CI/Linux deve verificar `gofmt` após normalização.

**Remoções adicionais verificadas:** pacote independente `internal/platform/cache/product_cache.go` (Redis legado, não importado; cache de produtos em `internal/modules/inventory/infrastructure/products_repo_cached.go` permanece); pacote independente `internal/platform/tracing/tracing.go` (não importado; telemetria ativa em `internal/platform/observability` permanece); construtos/funções mortos localizados por análise estática, incluindo wrapper `AuthService.Refresh`, invalidator de permissões vazio, helper de finance/not-found, configurador de deadline sem consumidores e wrapper manual não utilizado de submissão NFC-e. **A implementação efetiva de submissão/reconciliação fiscal permanece intacta.**

**Invariantes preservadas:** autenticação por cookie HttpOnly e refresh real `RefreshWithSubject`; isolamento por empresa, armazenamento/caches reais, `FiscalRepo.MarkNFCeSubmitted` (usado pelo processamento fiscal), rotas, adapters SEFAZ, migrações, PWA e cenários offline.

**Pendente mesmo após a segunda rodada:** repetir unitários, vet, lint, build e análises no **novo** SHA; executar testes E2E integrados e banco descartável; homologação SEFAZ com dados reais continua fora de escopo e não deve ser inferida.

## Validação final do commit de limpeza `36ebe2309b1f67452720a02941b355f992b04ec5`

Reexecutado no **HEAD após a remoção**, em clone temporário no computador conectado:

- `go test ./...`: **aprovado**, código 0.
- `go vet ./...`: **aprovado**, código 0.
- `go run honnef.co/go/tools/cmd/staticcheck@latest ./...`: **aprovado, sem diagnósticos**, código 0.
- `npm run lint`: **aprovado**, 0 erros e **10 avisos** React restantes (antes 12).
- `npm run build`: **aprovado**, código 0, 58 módulos Vite transformados.
- `go test -tags=integration -run '^$' ./tests/integration/...`: compilação de teste integrada **aprovada** sem executar os testes (não requer nem valida Postgres real).
- `go run golang.org/x/tools/cmd/deadcode@latest -test ./...`: resta **um** símbolo reportado como inalcançável: `NewFiscalService`, **preservado por aparecer em cinco testes de integração marcados com a build tag `integration`**. Essa divergência mostra por que não se pode apagar indiscriminadamente todo aviso da ferramenta.
- Playwright Chromium, Vite servido em `127.0.0.1:5183`, testes `product-photos.spec.ts` e `pdv-fiscal-print-choice.spec.ts`: **2 passaram** (23,3s); ambos dependem de API simulada, não provam backend/banco reais.
- `go mod tidy`: **aprovado**, código 0; `git diff -w -- go.mod go.sum` vazio. O modo `go mod tidy -diff` detectou apenas o churn de finais de linha do checkout Windows `core.autocrlf=true` (sem diferença semântica); não foi commitada formatação em massa nem mudança de módulos. Após o experimento, `go.mod` e `go.sum` foram restaurados do commit.
- O servidor Vite de validação foi encerrado após os testes.

**Gates ainda não comprovados:** suíte Playwright inteira, integração real com PostgreSQL/Redis, migrações up/down e homologação fiscal oficial; execução do workflow CI no próprio GitHub e versão alvo (Go 1.25/Node 22). O PR continua **draft e sem merge**.

**Classificação final de código morto no escopo examinado:** duas implementações inteiras sem importadores + tipos/helpers/métodos de compatibilidade não usados eliminados. Não se reivindica 100% de ausência de código morto em caminhos com reflexão, dados de produção ou possíveis combinações de build tags.

## Rodada estendida — integração real, avisos React e E2E (15/10/2026)

### Remoção de avisos e validação

- `dd812106`: 7 efeitos React de consultas assíncronas passaram a invalidar respostas por `active=false` no cleanup; permanece verificação de ID da requisição + empresa/sessão. Não deixar uma resposta antiga sobrescrever o estado após troca de escopo.
- `d86950be`: estabilização de atualização de contas com `useCallback`; `useEffectEvent` para carregamentos de montagem que não devem disparar toda vez que filtros de formulário mudam.
- Resultado em clone do PR: **`npm run lint`: 0 erros e 0 avisos** (eram 12 no começo da auditoria); **`npm run build`: aprovado**, Vite/TypeScript, 58 módulos transformados.
- `033760fa` e `b3ba56a`: manutenção dos cenários Playwright (seletores alinhados à UI real, formulário de dinheiro dentro de `details` e interceptação de API sem porta fixa). Sem mudança de produção nesses commits.

### Banco, migrations e integração Go reais

- Projeto Compose **isolado** `erp-audit-pr34-20261015`, PostgreSQL 16 e Redis 7.4, portas temporárias `55433` e `56379`; volume novo, **nenhuma conexão com banco real de loja**.
- `migrate up` de **v1 até v48**: **aprovado**, `schema_migrations = 48 / dirty=false`. Seed demonstrativo específico do ambiente de teste: **aprovado**.
- `go test -tags=integration -count=1 -p 1 -parallel 1 -timeout 12m ./tests/integration/...` com `TEST_DATABASE_URL`, `TEST_REDIS_ADDR` e `TEST_FISCAL_DISPOSABLE_DB=1`: **aprovado**, exit 0. Isso **executa** cenários PostgreSQL/Redis, diferente da mera compilação na rodada anterior.
- Serviço de API em `127.0.0.1:18080` com `APP_ENV=test`, `FISCAL_PROVIDER=mvp` e flags SEFAZ **desligadas**; Vite em `127.0.0.1:5183`. **Nenhuma transmissão fiscal verdadeira**.
- `go test -cover ./...` aprovado (cobertura **unitária por pacote**, não soma automaticamente os casos de integração): `fiscal/domain` **78,8%**, `fiscal/providers/sefaz` **64,9%**, `fiscal/application` **2,6%**, `finance/application` **2,9%**, `team` **2,9%**, `httpapi/handlers` **7,1%**, `inventory/application` **9,5%** e `sales/application` **30,6%**. Esses números indicam **lacunas de teste, não prova de código morto**.

### E2E — resultados e falhas remanescentes

- Primeira execução da suíte Chromium (121 casos) foi comprometida por limites reais de login: observados `HTTP 429`. Recriado o banco de teste e usada taxa alta **apenas no processo local descartável** (sem editar configuração padrão do produto).
- Segunda execução: 120 selecionados (excluído caso antigo que espera criar XML fiscal sem a preparação exigida), com `--workers=1 --max-failures=8`: **18 passaram, 8 falharam, 2 ignorados, 92 não chegaram a executar**. Não reportar a suíte completa como aprovada.
- Falhas de testes identificadas: título antigo `Produtos` versus UI `Produtos da loja`; ausência da região antiga `Sessão de caixa`; `Prontidão para homologação` versus `Diagnóstico local da configuração fiscal`; tentativa de preencher input escondido em `details`; rota mock com host/porta `127.0.0.1:8080` fixos; estado de caixa aberto deixado por falha anterior, contaminando cenários posteriores.
- **Retestes bem-sucedidos após as correções:** diagnóstico fiscal Chromium **1 passed**; retorno ao login após refresh recusado **1 passed**; smoke de navegação/PDV Chromium **1 passed**; idempotência de movimentação de caixa após reload em banco limpo **1 passed**.
- Firefox e WebKit: teste **não executável neste dispositivo**; executáveis Playwright ausentes. Não declarar compatibilidade cross-browser validada.
- Permanecem tarefas de **isolamento por teste** (abertura/fechamento de caixa, snapshots entre testes e seed), teste integral no navegador e atualização de cenário fiscal que pressupõe geração de XML fora das novas pré-condições. É necessário preservar as regras fiscais ao ajustar a expectativa, não afrouxá-las.

### Rollback fiscal fail-closed: evidência

O ensaio de rollback no **banco descartável com seed e histórico criado por testes** executou `down 13` em v48, mas a migração de identidade/auditoria fiscal protegeu o histórico, emitindo `cannot discard technical fiscal principal or preflight audit history`. O banco descartável ficou **v42, dirty=true**; `up` recusou continuar sobre versão dirty, como esperado. **Não foi aplicado `force`, não foi removido histórico para mascarar o resultado e nenhum banco real foi tocado.** Um ciclo `48 → 35 → 48` em esquema vazio permanece **pendente** de comprovação separada. Esse comportamento mostra proteção de retenção deliberada, mas também a necessidade de procedimento documentado de atualização/recuperação em ambiente operacional.

### Próximos gates

1. Suíte Chromium completa em ambiente que isole o caixa e dados demo por cenário, com todos os 121 resultados registrados, mais Firefox/WebKit se forem instalados.
2. Cobertura de orquestração fiscal, financeiro e handlers; teste de `go test -race` onde toolchain suportar.
3. Reversibilidade de migrações **apenas em banco vazio/descartável**, sem burlar controles fail-closed. Backup/restore real com dados fictícios continua necessário.
4. CI do GitHub (runners executando passos), Go 1.25, Node 22, e homologação fiscal com credenciais oficiais reais — **não comprovados aqui**.

**Integração proibida por enquanto:** PR #34 permanece draft, dependente do PR #25, sem merge automático e sem autorização de NFC-e em produção.

## Reconciliando a base que avançou (16/10/2026)

O PR #25 avançou **5 commits** do SHA `715a5b01900a2cfec2a1540b5eef54ccb72a6a95` até `aacb199f74fcf2f06c089fcc85db477a83330826`, modificando `README.md`, `web/index.html`, `web/public/manifest.webmanifest`, `web/src/components/Layout.tsx` e `web/src/pages/LoginPage.tsx`. O PR #34 inicialmente indicava conflito (`mergeable_state=dirty`).

Foi preparada uma reconciliação **sem force-push e sem merge para main**, preservando integralmente os quatro arquivos visuais da nova base e mesclando manualmente o README detalhado com a marca **Esterium ERP**. A marca não está declarada registrada pelo INPI. Commit de dois pais: HEAD da auditoria e HEAD da base. Os testes precisam ser repetidos para esse novo HEAD, especialmente o frontend e a navegação/branding.
