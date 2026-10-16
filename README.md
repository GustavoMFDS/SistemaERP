# Esterium ERP — PDV e gestão de empresas independentes

> **Marca adotada:** Esterium ERP (15/10/2026). **Repositório no GitHub:** SistemaERP. **Identificadores internos ainda existentes:** SistemaEmGo. A alteração de marca não renomeia chaves de armazenamento, emissor JWT, módulo Go nem identificadores do banco, por razões de compatibilidade. **A disponibilidade ou exclusividade do nome no INPI ainda não foi confirmada.** A adoção visual do nome não equivale ao registro da marca.

Sistema web de gestão comercial com PDV, produtos, estoque, compras, devoluções, clientes, financeiro e preparação fiscal para empresas brasileiras. Cada loja/CNPJ é uma **empresa independente**, com dados, permissões e parametrização fiscal próprios — não uma filial automática das demais lojas da família.

**Situação:** desenvolvimento/piloto, **não liberado para operação fiscal real**. Há implementações em branches e PRs ainda não integrados à `main`. A presença de um módulo no código **não comprova** build, testes E2E, homologação SEFAZ nem aptidão para produção. Consulte o [catálogo geral de 16/10/2026](docs/catalogo-geral-esterium-2026-10-16.md) e os PRs abertos antes de decidir um deploy.

**Inventário mais recente:** o [catálogo geral do Esterium ERP](docs/catalogo-geral-esterium-2026-10-16.md) contabiliza 15 telas React, 121 registros de rotas HTTP, 13 módulos Go e 48 migrações SQL, distinguindo implementação de validação efetiva. O [catálogo funcional histórico](docs/catalogo-funcional-sistema.md) permanece disponível, mas algumas contagens e pendências estão desatualizadas. A emissão NFC-e em produção não está homologada.

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

Requisitos: Go **1.26.9**, Node.js **22.x**, npm compatível e Docker com Compose, ou PostgreSQL e Redis configurados separadamente.

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

Configure `E2E_BASE_URL` quando a UI não estiver em `http://127.0.0.1:5173`. Os cenários misturam API real e respostas simuladas. Os testes que abrem caixa, finalizam venda ou usam contas demo exigem isolamento/limpeza de estado entre execuções. Muitos logins automatizados podem acionar o rate limiter: ajuste limites **somente no processo descartável de teste**, jamais no ambiente real. Chromium, Firefox e WebKit precisam dos respectivos navegadores instalados. A suíte integral ainda **não** é gate verde; consulte a [auditoria com resultados e falhas](docs/auditoria-codigo-20261015.md).

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
