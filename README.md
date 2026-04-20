# SistemaEmGo (ERP + PDV)

Projeto MVP de um ERP/PDV para o mercado brasileiro (estoque, vendas/PDV, financeiro básico e NF-e XML MVP), com backend em Go e banco PostgreSQL.

## Requisitos
- Node.js 20+ (recomendado) para o frontend
- Go 1.22+ para o backend
- Docker + Docker Compose (opcional, recomendado) para subir Postgres/migrations/seed

## Subindo o banco (Docker)
1) Copie um arquivo de env (NÃO commitar `.env`):
- `copy .env.dev.example .env`

2) Suba Postgres + migrations + seed:
- `docker compose --env-file .env up -d db migrate seed`

> Produção: prefira injetar env vars via Docker/K8s/CI e usar `*_FILE` para secrets.

## Rodando o backend
- `cd backend`
- `go mod download`
- `go run ./cmd/api`

Gerar um JWT secret forte:
- `go run ./cmd/gensecret -format base64url -bytes 32`

Servidor: `http://localhost:8080`

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
