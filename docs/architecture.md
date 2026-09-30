# Etapa 1 — Visão geral e arquitetura

## Visão geral
O sistema é composto por:
- **Backend Go** (API REST): regras de negócio, autenticação/autorização, consistência transacional (venda + estoque + financeiro), auditoria.
- **PostgreSQL**: banco relacional, histórico de movimentações e trilha auditável.
- **Frontend Web**:
  - **ERP administrativo** (cadastros, estoque, financeiro, fiscal)
  - **PDV** (fluxo rápido de venda, otimizado para operador)

## Arquitetura (camadas)
- **HTTP**: handlers/controllers + middlewares
- **Service**: regras de negócio e orquestração transacional
- **Repository**: acesso ao banco (SQL)
- **Domain**: modelos e invariantes do domínio

## Padrões e decisões
- **Transações** obrigatórias para operações críticas (finalizar/cancelar venda, ajuste de estoque, abertura/fechamento de caixa).
- **Controle de concorrência de estoque** via `SELECT ... FOR UPDATE` em saldo por produto.
- **Auditoria** para ações críticas (cancelamento, ajuste, geração XML).
- **RBAC** por `roles` e `permissions` persistidos em banco.

## Componentes
- **Migrations**: SQL versionado em `backend/migrations`.
- **Seed**: dados de teste em `backend/seed/seed.sql`.
- **Logs estruturados**: `slog` (JSON) + `request_id`.

## Fluxos operacionais
- Venda: abrir caixa → carrinho → finalizar venda → baixa estoque → financeiro.
- Estoque: cadastro produto → entrada/ajuste → alerta mínimo.
- Fiscal atual: preparar emitente NFC-e por tenant → classificar produtos com NCM/CEST → preparar referências de CSC/certificado → validar readiness.
- Preview fiscal de desenvolvimento: venda finalizada → preview NFC-e modelo 65 marcado como não fiscal → armazenar/download.
- Fiscal futuro: provider SEFAZ separado para assinatura, schemas oficiais, chave/QR Code, autorização, protocolo, contingência, cancelamento/inutilização e DANFE-NFC-e. Produção mantém `FISCAL_PROVIDER=disabled` até homologação.
# Architecture notes

Current code keeps the existing package layout to avoid a risky full rewrite. The safe refactor direction is to flatten modules incrementally once behavior is covered by tests.

Recommended target structure:

```text
backend/
  cmd/api/
  internal/app/
  internal/httpapi/
  internal/modules/{auth,inventory,sales,finance,fiscal}/
  internal/platform/
  migrations/
web/
  src/{components,lib,pages}
```

Refactor rule for the next pass:

- Move one module at a time from `domain/application/infrastructure` into fewer feature files only after `go test ./...` is green.
- Keep interfaces only where they support tests or real adapters.
- Keep database migrations and Docker Compose paths stable.
- Prefer repository-bound decimal conversion and domain integer money/quantity types over database-wide rewrites.
