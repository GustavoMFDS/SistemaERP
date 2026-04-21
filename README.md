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
- `go run ./cmd/api`

Gerar um JWT secret forte:
- `go run ./cmd/gensecret -format base64url -bytes 32`

Servidor: `http://localhost:8080`

Observabilidade:
- Métricas Prometheus: `http://localhost:8080/metrics`
- Tracing (OpenTelemetry): habilite com `OTEL_ENABLED=true` e escolha `OTEL_EXPORTER=stdout` (dev) ou `OTEL_EXPORTER=otlp` + `OTEL_EXPORTER_OTLP_ENDPOINT=...`

Performance (Etapa 8):
- Cache Redis para produtos: `GET /api/v1/products` (lista padrão) e `GET /api/v1/products/{id}` usam cache best-effort quando Redis está disponível.
- Baixa de estoque em venda reduz round-trips ao banco (locks/balances em batch quando possível).

## Rodando o frontend
- `cd web`
- `npm install`
- `npm run dev`

## Documentação
- Arquitetura: `docs/architecture.md`
- Modelo de dados e índices: `docs/data-model.md`
- Regras de negócio: `docs/business-rules.md`
- API (endpoints + exemplos): `docs/api.md`
- Roadmap: `docs/roadmap.md`

> Observação: NF-e aqui é MVP de geração/armazenamento de XML, sem SEFAZ.
