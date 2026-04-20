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

## Fluxos MVP
- Venda: abrir caixa → carrinho → finalizar venda → baixa estoque → financeiro → pronto para NF-e.
- Estoque: cadastro produto → entrada/ajuste → alerta mínimo.
- Fiscal: selecionar venda finalizada → gerar XML → armazenar/download.
