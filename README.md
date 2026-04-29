# SistemaEmGo (ERP + PDV)

Projeto MVP de um ERP/PDV para o mercado brasileiro (estoque, vendas/PDV, financeiro básico e NF-e XML MVP), com backend em Go e banco PostgreSQL.

## Requisitos
- Node.js 20+ (recomendado) para o frontend
- Go 1.24+ para o backend
- Docker + Docker Compose (opcional, recomendado) para subir Postgres + Redis + migrations/seed

## Subindo o banco (Docker)
1) Copie um arquivo de env (NÃO commitar `.env`):
- `copy .env.dev.example .env`

2) Suba Postgres + migrations + seed:
- `docker compose --env-file .env up -d db redis migrate seed`

> Observação (Windows): por padrão o Postgres do compose publica em `5433` para evitar conflito com Postgres local na `5432`.

> Observação: o Redis do compose está fixado em `redis:7.4-alpine` para evitar incompatibilidade de volume (formato de dump RDB) ao trocar tags.

> Produção: prefira injetar env vars via Docker/K8s/CI e usar `*_FILE` para secrets.

## Rodando o backend
- `cd backend`
- `go mod download`

O backend **não carrega arquivos `.env` automaticamente** — ele lê apenas variáveis de ambiente do processo.

### Windows (PowerShell)
Carregue o `.env` (na raiz do repo) no ambiente do processo e inicie a API:

```powershell
cd C:\Projetos\SistemaEmGo
Get-Content .env | ForEach-Object {
	$l=$_.Trim(); if ($l -match '^#' -or $l -eq '') { return }
	$name,$value = $l -split '=',2
	if ($name -and $value) { Set-Item -Path "Env:$name" -Value $value }
}
cd backend
go run ./cmd/api
```

### Linux/macOS (bash)
```bash
set -a
source ../.env
set +a
go run ./cmd/api
```

Servidor: `http://localhost:8080`

CORS (dev/test): quando `APP_ENV` **não** é prod-like, a API responde preflight `OPTIONS` e libera o `Origin` do browser (necessário para o Vite em `http://localhost:5173`).

Gerar um JWT secret forte:
- `go run ./cmd/gensecret -format base64url -bytes 32`

Observabilidade:
- Métricas Prometheus: `http://localhost:8080/metrics`
- Tracing (OpenTelemetry): habilite com `OTEL_ENABLED=true` e escolha `OTEL_EXPORTER=stdout` (dev) ou `OTEL_EXPORTER=otlp` + `OTEL_EXPORTER_OTLP_ENDPOINT=...`

Performance (Etapa 8):
- Cache Redis para produtos: `GET /api/v1/products` (lista padrão) e `GET /api/v1/products/{id}` usam cache best-effort quando Redis está disponível.
- Baixa de estoque em venda reduz round-trips ao banco (locks/balances em batch quando possível).

Fiscal (Etapa 9):
- Geração de XML NF-e fica atrás de uma interface (`NFeProvider`), com implementação MVP em `internal/modules/fiscal/providers/mvp`.
- Objetivo: manter o fluxo/armazenamento funcionando hoje e permitir evolução futura (assinatura, transmissão SEFAZ, protocolo, DANFE) sem refatorar o serviço/API.

Multi-tenant (Etapa 10):
- `tenant_id` é derivado do JWT via middleware (não é aceito via request).
- Todas as operações relevantes no banco são filtradas por `tenant_id` para evitar vazamento cross-tenant.
- Cache Redis de produtos usa chaves/versionamento separados por tenant.

Event-driven (Etapa 11):
- Bus de eventos in-process em `internal/platform/events`.
- Publicação acontece após `COMMIT` (mantém consistência) — pronto para evoluir para outbox + Kafka/RabbitMQ.
- Eventos principais: `sale.created` e `inventory.debited` (com `tenant_id`).

## Rodando o frontend
- `cd web`
- `npm install`
- `npm run dev`

Por padrão o frontend usa `VITE_API_BASE_URL=http://localhost:8080` (ver `.env.dev.example`).

## Offline PDV (Etapa 13)
O PDV suporta modo offline **best-effort** para quedas de rede durante o atendimento:
- Se estiver offline (ou ocorrer erro de rede), a venda é **enfileirada localmente** e o PDV limpa os itens para seguir operando.
- Ao voltar online, as pendências são **sincronizadas automaticamente**.
- Cada venda enviada usa `Idempotency-Key`, e o backend persiste o resultado para evitar duplicação em retries.

Detalhes: `docs/offline-pdv.md`

## Credenciais (seed)
Ao subir `docker compose ... seed`, são criados usuários para testes (senha: `admin123`):
- `admin@sistema.local`
- `gerente@sistema.local`
- `caixa@sistema.local`

## Documentação
- Arquitetura: `docs/architecture.md`
- Modelo de dados e índices: `docs/data-model.md`
- Regras de negócio: `docs/business-rules.md`
- API (endpoints + exemplos): `docs/api.md`
- Roadmap: `docs/roadmap.md`

> Observação: NF-e aqui é MVP de geração/armazenamento de XML, sem SEFAZ.
