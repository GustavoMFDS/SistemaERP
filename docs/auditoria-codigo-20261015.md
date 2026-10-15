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
