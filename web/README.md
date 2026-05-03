# SistemaEmGo — Frontend (Vite + React)

## Requisitos
- Node.js 20+

## Rodar em dev

```bash
cd web
npm install
npm run dev
```

Frontend: `http://localhost:5173/`

## API

Por padrão, a base da API é `http://localhost:8080`.
- Configure com `VITE_API_BASE_URL` (ver `.env.dev.example` na raiz do repositório).

## Offline PDV (MVP)

A página de PDV tem fallback offline **best-effort**: se estiver sem internet, ela enfileira a venda localmente e sincroniza quando voltar online.

Detalhes: `../docs/offline-pdv.md`
