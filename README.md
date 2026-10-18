# Esterium ERP — PDV e gestão de empresas independentes

> **Marca adotada:** Esterium ERP (15/10/2026). **Repositório no GitHub:** SistemaERP. **Identificadores internos ainda existentes:** SistemaEmGo. A alteração de marca não renomeia chaves de armazenamento, emissor JWT, módulo Go nem identificadores do banco, por razões de compatibilidade. **A disponibilidade ou exclusividade do nome no INPI ainda não foi confirmada.** A adoção visual do nome não equivale ao registro da marca.

Sistema web de gestão comercial com PDV, produtos, estoque, compras, devoluções, clientes, financeiro e preparação fiscal para empresas brasileiras. Cada loja/CNPJ é uma **empresa independente**, com dados, permissões e parametrização fiscal próprios — não uma filial automática das demais lojas da família.

**Situação:** desenvolvimento/piloto, **não liberado para operação fiscal real**. Há implementações em branches e PRs ainda não integrados à `main`. A presença de um módulo no código **não comprova** build, testes E2E, homologação SEFAZ nem aptidão para produção. Consulte o [catálogo geral de 16/10/2026](docs/catalogo-geral-esterium-2026-10-16.md) e os PRs abertos antes de decidir um deploy.

**Inventário histórico (16/10/2026):** o [catálogo geral do Esterium ERP](docs/catalogo-geral-esterium-2026-10-16.md) contabilizava 15 telas React, 121 registros de rotas HTTP, 13 módulos Go e 48 migrações SQL. As contagens desse documento pertencem à data da auditoria; a cadeia local atual inclui **51 pares de migrações SQL** no código, ainda sem integração automática à `main`. O [catálogo funcional histórico](docs/catalogo-funcional-sistema.md) permanece disponível, mas algumas contagens e pendências estão desatualizadas. A emissão NFC-e em produção não está homologada.

**Refinamento operacional de 16/10/2026:** o [plano de refinamento](docs/refinamento-final-esterium-2026-10-16.md) documenta backup PostgreSQL verificado em Linux/Windows (agendamento ainda a configurar), Central de Atenção, conferência fiscal diária e relatórios operacionais por período. Recuperação ensaiada apenas com banco fictício; isso não comprova backup externo, homologação SEF/MG nem prontidão operacional.

## Funcionalidades existentes no código de integração

- Vendas/PDV, controle de caixa, leitura de código de barras, carrinhos suspensos e prevenção de duplicidade por chave de idempotência.
- Produtos, fotos e miniaturas, cadastro/importação CSV, movimentações de estoque e alertas de estoque mínimo.
- Compras e fornecedores, devoluções/reembolsos, clientes, contas financeiras e conciliação.
- Contas de acesso, funcionários, convites e permissões isoladas por empresa.
- PWA instalável e fila local de intenções de venda com sincronização assistida. **Venda preservada offline não é nota autorizada.**
- Fiscal: pendências por venda, preparação de emitente, perfis tributários, documentos e mecanismos técnicos de NFC-e, incluindo componentes de comunicação SEFAZ em **homologação controlada**. **Não é emissão automática de produção**, nem substitui validação tributária real.
- Auditoria, controles técnicos de privacidade/LGPD, métricas, logs e testes automatizados.

O que funciona e o que ainda está pendente consta em [docs/catalogo-funcional-sistema.md](docs/catalogo-funcional-sistema.md), [docs/fiscal-mg-automacao-homologacao-2026.md](docs/fiscal-mg-automacao-homologacao-2026.md) e [docs/pdv-impressao-fiscal-semi-automatica.md](docs/pdv-impressao-fiscal-semi-automatica.md).

## Tecnologias e estrutura

| Área | Tecnologia / local |
| --- | --- |
| Backend | Go 1.26.9, API HTTP em `backend/cmd/api`, módulos em `backend/internal` |
| Frontend | React 19, TypeScript, Vite, Tailwind, em `web/` |
| Persistência | PostgreSQL 16, migrações incrementais em `backend/migrations/` |
| Sessões e cache | Redis |
| Infra local | `docker-compose.yml`; observabilidade complementar em `docker-compose.observability.yml` |
| Testes | Go (unitários e integração), ESLint, TypeScript, Vite e Playwright em `web/e2e` |
| Documentação | `docs/` (API, implantação, segurança, fiscal, operação e relatórios de validação) |

**Para desenvolvimento e compilação:** Go **1.26.9**, Node.js **22.x**, npm compatível e Docker com Compose, ou PostgreSQL e Redis configurados separadamente. **O pacote técnico Windows** já compila Go e React e não exige Go/Node/Docker para abrir a interface na loja, mas PostgreSQL/Redis ainda exigem provisionamento seguro separado.

## Esterium Local para Windows (estágio técnico, sem instalação comercial)

A arquitetura principal aprovada prevê **um computador servidor na loja** e terminais pela rede interna. Os PRs [#84–#94](https://github.com/GustavoMFDS/SistemaERP/pulls) preparam o pacote Windows x64, com backend Go compilado, interface React estática, migrações SQL versionadas e ferramentas de manutenção protegidas. Essa cadeia **permanece em PRs draft**, sem liberação automática na `main`. Aprovado no CI **não significa** homologação presencial ou fiscal.

O pacote é produzido na máquina de build, não no computador da loja, por `scripts/build-esterium-local-windows.ps1`. O ZIP técnico tem manifesto SHA-256 que pode ser conferido antes e depois da instalação, mas **ainda não tem assinatura digital de distribuição**.

Há também uma verificação de instalação nova em um único comando técnico, `tools/plan-esterium-local-setup.ps1`, **somente leitura**: ela não instala PostgreSQL/Redis ou os serviços do Esterium. O procedimento técnico, em ordem, está descrito em [registro do serviço Windows](docs/esterium-local-service-registration.md), [backup externo](docs/esterium-local-backup-externo.md) e [checklist de aceite físico](docs/esterium-local-pilot-checklist.md). As ferramentas locais incluem checagem não destrutiva do PC e do pacote, inventário somente leitura de serviços PostgreSQL/Redis compatíveis já instalados, provisão limitada de segredos próprios do ERP, registro/reversão conservadores de serviço `LocalService`, backup externo protegido e tarefa diária **desabilitada até aceite**. Há também um **plano de atualização sem escrita** que verifica cópia externa recente e impede alterações retroativas em migrações SQL, mas **não instala, migra ou atualiza** o sistema. Não há migração automática nem apagamento de dados da loja.

**Ainda pendente antes de um piloto real:** provisionamento integrado de PostgreSQL/Redis com TLS e usuários restritos; instalação/atualização e assinatura de release; HTTPS confiável para dois terminais na LAN; testes com leitores, impressoras e caixas físicos; backup/restauração em computador independente; gestão de falhas/atualizações; emissão e XML fiscal genuinamente autorizados pela SEFAZ. O modo de testes deve usar somente banco descartável e nunca ativar transmissão fiscal de produção.

## Distribuição Windows — verificação obrigatória de release

O bundle do CI continua **técnico e não assinado**, destinado apenas a testes descartáveis. Uma versão oficial exige assinatura Authenticode válida de **todo executável e de cada script PowerShell privilegiado**, além de um SHA-256 do manifesto recebido por canal independente e do certificado do editor previamente aprovado. O gate `tools/check-esterium-signed-release.ps1` exige esses elementos e **reprova os arquivos técnicos atuais**. O teste automatizado confirma que um pacote sem assinatura ou com digest divergente nunca passa como release.

Esse controle não cria nem substitui assinaturas, certificados, instalador completo ou os componentes PostgreSQL/Garnet; tampouco permite uso comercial, alteração fiscal ou integração automática dos PRs. Somente depois da homologação física e da conclusão dos itens não fiscais a cadeia de PRs deverá ser revisada e incorporada em ordem à `main`.

## Execução local para desenvolvimento

Na **raiz do repositório**, crie seu arquivo privado de configuração:

```bash
cp .env.example .env
docker compose up -d db redis migrate
docker compose run --rm seed
```

No Windows, copie `.env.example` para `.env` pelo Explorer ou PowerShell com `Copy-Item .env.example .env`.

**Atenção:** `.env` é usado automaticamente pelo **Docker Compose** para interpolação. O binário **Go não lê arquivos `.env`**: ele recebe apenas variáveis do ambiente do processo (ver `backend/internal/config/config.go`). Antes de iniciar a API, carregue essas variáveis no terminal. No Linux/macOS, a partir da raiz:

```bash
set -a
source .env
set +a
cd backend
go run ./cmd/api
```

No PowerShell, a partir da raiz (para o arquivo de exemplo sem valores multiline):

```powershell
Get-Content .env | Where-Object { $_ -match '^[A-Za-z_][A-Za-z0-9_]*=' } | ForEach-Object {
  $nome, $valor = $_ -split '=', 2
  [Environment]::SetEnvironmentVariable($nome, $valor, 'Process')
}
Set-Location backend
go run ./cmd/api
```

Em outro terminal, na raiz do repositório:

```bash
cd web
npm ci
npm run dev
```

Frontend: `http://localhost:5173` | API: `http://localhost:8080`.

**Banco e senhas locais:** `.env.example` fornece `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` e `DATABASE_URL` compatíveis entre si. A senha é **apenas de desenvolvimento**: substitua por segredo próprio fora do ambiente de teste. Se já existir um volume PostgreSQL inicializado com outra senha, alterar `.env` **não altera** automaticamente o usuário do banco; ajuste as credenciais ou recrie **somente um volume de teste descartável**. Nunca use `docker compose down -v` em dados reais.

**Seed:** `backend/seed/seed.sql` é **somente para demo/desenvolvimento**, não execute em homologação ou produção. O serviço `seed` no Compose é explicitamente destinado a esse uso. `backend/seed/seed.prod.sql` não cria administrador com senha padrão.

## Configuração importante

- `APP_ENV`: `dev`, `test`, `staging` ou `prod`.
- `DATABASE_URL`: conexão PostgreSQL; produção/staging requer TLS validado conforme `docs/security.md`.
- `REDIS_ADDR` para desenvolvimento; `REDIS_URL` com `rediss://` em produção/staging.
- `JWT_SECRET`: segredo aleatório forte; gere em `backend/` com `go run ./cmd/gensecret -format base64url -bytes 32`.
- `CORS_ALLOWED_ORIGINS`, `TRUSTED_PROXY_CIDRS`, `METRICS_BEARER_TOKEN` e `PRIVACY_CONTACT_EMAIL`: configurar conforme ambiente.
- `VITE_API_BASE_URL`: URL da API usada pelo frontend; padrão local `http://localhost:8080`.
- `FISCAL_PROVIDER` e `NFCE_*`: integração fiscal e caminhos para certificados/esquemas devem ser explicitamente configurados. **Flags desligadas por padrão não autorizam transmissão.**

**Nunca** comite `.env`, certificados PFX/PEM, senhas, token CSC ou dados reais de clientes. Para habilitar qualquer tentativa real de homologação NFC-e em MG, consulte [o guia fiscal](docs/fiscal-mg-guia-operador-2026.md), [a preparação de MG](docs/fiscal-mg-configuracao-sem-legado-2026.md) e [a automação de homologação](docs/fiscal-mg-automacao-homologacao-2026.md). NF-e modelo 55 não deve ser tratada como emissor de produção pronto.

## Validação antes de integrar ou implantar

Backend:

```bash
cd backend
gofmt -l .
go test ./...
go vet ./...
```

Frontend:

```bash
cd web
npm ci
npm run lint
npm run build
```

### Integração com banco de teste

Prepare **PostgreSQL e Redis descartáveis, com banco exclusivo**, aplique as migrações e carregue o seed demonstrativo apenas nesse banco. Configure `TEST_DATABASE_URL` (DSN do banco de teste) e `TEST_REDIS_ADDR` (Redis do teste). Use `TEST_FISCAL_DISPOSABLE_DB=1` **exclusivamente nesse ambiente isolado**, pois libera cenários que alteram dados fiscais de teste.

```bash
cd backend
go test -tags=integration -count=1 -p 1 -parallel 1 -timeout 12m ./tests/integration/...
```

**Nunca** use uma base da loja ou de homologação com dados reais nesses testes. Algumas migrações têm rollback **fail-closed** quando há registros fiscais/auditoria e podem deixar a versão em `dirty=true` ao recusar a exclusão; não force a versão nem apague histórico operacional para contornar o bloqueio.

### E2E do navegador

Inicie a API com o banco **de teste**, a UI Vite apontando para a API de teste (`VITE_API_BASE_URL`) e execute em outro terminal:

```bash
cd web
npm run test:e2e -- --project=chromium --workers=1
```

Configure `E2E_BASE_URL` quando a UI não estiver em `http://127.0.0.1:5173`. Os cenários misturam API real e respostas simuladas. Os testes que abrem caixa, finalizam venda ou usam contas demo exigem isolamento/limpeza de estado entre execuções. Muitos logins automatizados podem acionar o rate limiter: ajuste limites **somente no processo descartável de teste**, jamais no ambiente real. Chromium, Firefox e WebKit precisam dos respectivos navegadores instalados. O resultado completo deve ser confirmado nos jobs do GitHub Actions **para o SHA da branch escolhida**; aprovações de outros PRs não validam a branch atual. A [auditoria de 15/10/2026](docs/auditoria-codigo-20261015.md) é histórica, não o placar de CI de todas as branches atuais.

Outras referências: `.github/workflows/ci.yml` e `docs/smoke-test.md`. **Não confundir comandos recomendados com resultados realmente executados.**

## Segurança e operação fiscal

O servidor isola operações por empresa/tenant e aplica RBAC; continua necessária revisão de segurança, integrações externas, regras tributárias, backup/restore, implantação e procedimentos operacionais. O sistema mantém fila de obrigações fiscais por venda, mas **obrigação registrada ≠ documento autorizado pela SEFAZ**. A opção do cliente de não imprimir não cancela a obrigação fiscal. DANFE em papel depende de documento realmente autorizado e não substitui a emissão.

A **produção** depende de credenciamento, dados reais por CNPJ, certificado adequado, esquema oficial, tributação validada com contador, execução de homologação e evidências de ponta a ponta. Consulte também:

- [Arquitetura](docs/architecture.md), [API](docs/api.md), [regras de negócio](docs/business-rules.md).
- [Implantação](docs/deployment.md), [segurança](docs/security.md), [smoke tests](docs/smoke-test.md).
- [PDV offline](docs/offline-pdv.md), [privacidade/LGPD](docs/privacy/data-inventory.md).
- [Auditoria de manutenção e código morto](docs/auditoria-codigo-20261015.md).

**Segurança do runtime (outubro/2026):** o CI usa Go **1.26.9** e dependências `golang.org/x/*` atualizadas devido a correções de segurança. Não execute a API com Go 1.25.x nem com versões 1.26 anteriores à 1.26.9. O scanner `govulncheck` continua obrigatório.

## Licença e contribuição

Ainda não há licença definida no repositório. Isso **não** significa permissão automática para redistribuição. Mudanças significativas devem seguir por branch e PR com validação no HEAD final, sem merge com testes pendentes.
